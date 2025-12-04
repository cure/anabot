package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/spf13/viper"
)

var (
	// Debug output goes nowhere by default
	debug = func(string, ...interface{}) {}
	// Set up a *log.Logger for debug output
	debugLog = log.New(os.Stderr, "DEBUG: ", log.LstdFlags)
	mu       sync.Mutex
)

func loadConfigDefaults() {
	viper.SetDefault("OAuthAccessToken", "")
	viper.SetDefault("VerificationToken", "")
	viper.SetDefault("Debug", false)
	viper.SetDefault("DictionaryPath", "/usr/share/dict/american-english-insane")
	viper.SetDefault("NotificationChannels", []string{"general"})
}

func loadConfig(flags *flag.FlagSet) {
	loadConfigDefaults()

	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/anabot/")
	viper.AddConfigPath("$HOME/.anabot")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		usage(flags)
		log.Fatal("Fatal error reading config file:", err.Error())
	}

	if viper.GetBool("Debug") {
		debug = debugLog.Printf
	}
}

func parseFlags() (flags *flag.FlagSet) {
	flags = flag.NewFlagSet("anabot", flag.ExitOnError)
	flags.Usage = func() { usage(flags) }

	// Parse args; omit the first arg which is the command name
	err := flags.Parse(os.Args[1:])
	if err != nil {
		log.Fatal("Unable to parse command line arguments:", err.Error())
	}
	return
}

// adapted from "peterso" in https://stackoverflow.com/questions/54873920/how-to-optimize-finding-anagrams-from-a-text-file-in-go
func findAnagrams(find string, text io.ReadSeeker) map[string]bool {
	// Make sure we only have one process accessing the io.ReadSeeker at a time
	mu.Lock()
	defer mu.Unlock()
	text.Seek(0, 0)
	find = strings.Trim(strings.ToLower(find), " ")
	findSum := 0
	findRunes := []rune(find)
	j := 0
	for i, r := range findRunes {
		if r != ' ' {
			findSum += int(r)
			if i != j {
				findRunes[j] = r
			}
			j++
		}
	}
	findRunes = findRunes[:j]
	sort.Slice(findRunes, func(i, j int) bool { return findRunes[i] < findRunes[j] })
	findStr := string(findRunes)

	anagrams := make(map[string]bool)
	s := bufio.NewScanner(text)
	for s.Scan() {
		word := strings.Trim(strings.ToLower(s.Text()), " ")
		wordSum := 0
		wordRunes := []rune(word)
		j := 0
		for i, r := range wordRunes {
			if r != ' ' {
				wordSum += int(r)
				if i != j {
					wordRunes[j] = r
				}
				j++
			}
		}
		wordRunes = wordRunes[:j]
		if len(wordRunes) != len(findRunes) {
			continue
		}
		if wordSum != findSum {
			continue
		}
		sort.Slice(wordRunes, func(i, j int) bool { return wordRunes[i] < wordRunes[j] })
		if string(wordRunes) == findStr {
			fmt.Printf("***%+v****%+v***\n", word, find)
			if word != find {
				anagrams[word] = true
			}
		}
	}
	if err := s.Err(); err != nil {
		panic(err)
	}
	return anagrams
}

func keysString(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

func handleAnagram(text string, reader io.ReadSeeker) string {
	re := regexp.MustCompile("<.*?>")
	cleanedUp := strings.Trim(re.ReplaceAllString(text, ""), " ")
	anagrams := findAnagrams(cleanedUp, reader)
	if len(anagrams) > 1 {
		return "Anagrams for _" + cleanedUp + "_ are: " + keysString(anagrams)
	} else if len(anagrams) > 0 {
		return "Anagram for _" + cleanedUp + "_ is: " + keysString(anagrams)
	}
	return "No anagrams found for _" + cleanedUp + "_"
}

func caesar(r rune, shift int) rune {
	// Shift character by specified number of places.
	// ... If beyond range, shift backward or forward.
	shift %= 26
	if shift < 0 {
		shift += 26
	}

	var s int
	if r >= 'a' && r <= 'z' {
		s = int(r) + shift
		if s > 'z' {
			s -= 26
		}
	} else if r >= 'A' && r <= 'Z' {
		s = int(r) + shift
		if s > 'Z' {
			s -= 26
		}
	} else {
		return r
	}
	//fmt.Printf("Caesar: %+v distance %d => %+v\n", r, shift, rune(s))
	return rune(s)
}

func handleRot(text string, reader io.ReadSeeker) string {
	re := regexp.MustCompile("<.*?>")
	cleanedUp := strings.Trim(re.ReplaceAllString(text, ""), " ")
	words := strings.Fields(cleanedUp)
	distance, err := strconv.Atoi(words[0])
	if err != nil {
		return "Rotation failed: first argument is not a number (e.g. 13)"
	}
	cleanedUp = strings.Join(words[1:], " ")
	rotated := strings.Map(func(r rune) rune {
		return caesar(r, distance)
	}, cleanedUp)
	fmt.Printf("ROT: ***%d**** ****%+v**** => ****%+v****\n", distance, cleanedUp, rotated)
	return fmt.Sprintf("ROT %d for _%s_ is: %s", distance, cleanedUp, rotated)
}

func joinNotificationChannel(api *slack.Client, channel string) {
	// Yes, iterating through all channels is the 'best practice' to get a
	// channel's ID, which is apparently required to join a channel. Posting
	// messages can be done with the channel name...
	// Technically, this only needs to be done once.
	// FIXME: check if we are already in the channel, and if so, skip this.
	fmt.Println("Joining the notification channel " + channel)
	channels, _, err := api.GetConversations(&slack.GetConversationsParameters{})
	if err != nil {
		panic(err)
	}
	for _, c := range channels {
		if c.Name == channel {
			_, warning, warnings, err := api.JoinConversation(c.ID)
			fmt.Println(warning, warnings, err)
			if err != nil {
				panic(err)
			}
			break
		}
	}

}

func main() {
	flags := parseFlags()
	loadConfig(flags)

	reader, err := os.Open(viper.GetString("DictionaryPath"))
	if err != nil {
		panic(err)
	}
	defer reader.Close()

	var api = slack.New(viper.GetString("OAuthAccessToken"))
	var verificationToken = viper.GetString("VerificationToken")

	for _, n := range viper.GetStringSlice("NotificationChannels") {
		joinNotificationChannel(api, n)
	}

	http.HandleFunc("/command-endpoint", func(w http.ResponseWriter, r *http.Request) {
		s, e := slack.SlashCommandParse(r)
		fmt.Printf("%+v\n", s)
		if e != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Printf("Slash parse failed: %+v\n", e)
			return
		}
		if !s.ValidateToken(verificationToken) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Printf("Slash command unauthorized request (token validation failed)")
			return
		}

		switch s.Command {
		case "/ana", "/devana":
			params := &slack.Msg{
				ResponseType: "in_channel",
				Text:         handleAnagram(s.Text, reader),
			}
			b, err := json.Marshal(params)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		case "/rot":
			params := &slack.Msg{
				ResponseType: "in_channel",
				Text:         handleRot(s.Text, reader),
			}
			b, err := json.Marshal(params)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/events-endpoint", func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		body := buf.String()
		eventsAPIEvent, e := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionVerifyToken(&slackevents.TokenComparator{VerificationToken: verificationToken}))
		if e != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Printf("Event parse failed: %+v\n", e)
			return
		}

		if eventsAPIEvent.Type == slackevents.URLVerification {
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal([]byte(body), &r)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Printf("Unmarshal failed: %+v\n", e)
				return
			}
			w.Header().Set("Content-Type", "text")
			w.Write([]byte(r.Challenge))
		}
		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				fmt.Println(ev)
				api.PostMessage(ev.Channel, slack.MsgOptionText(handleAnagram(ev.Text, reader), false))
			case *slackevents.ChannelCreatedEvent:
				fmt.Println(ev)
				for _, n := range viper.GetStringSlice("NotificationChannels") {
					respChannel, respTimestamp, err := api.PostMessage(n, slack.MsgOptionText("A new channel with name #"+ev.Channel.Name+" was created", false), slack.MsgOptionParse(true))
					fmt.Println(respChannel, respTimestamp, err)
				}
			}
		}
	})
	fmt.Println("[INFO] Server listening")
	http.ListenAndServe(":3000", nil)
}
