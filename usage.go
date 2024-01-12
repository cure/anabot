// Copyright (C) The Arvados Authors. All rights reserved.
//
// SPDX-License-Identifier: AGPL-3.0

package main

import (
	"flag"
	"fmt"
	"os"
)

var exampleConfigFile = []byte(`
---
OAuthAccessToken: "xoxb-..."
VerificationToken: "..."
Debug: false
DictionaryPath: "/usr/share/dict/american-english-insane"
NotificationChannels: 
- "general"
- "status"
`)

func usage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, `
anabot is a Slack bot that returns anagrams when given a word. It also
announces newly created channels.

Config file locations:

  /etc/anabot/config.yml
  ~/.anabot/config.yml
  ./config.yml

`)
	fs.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Example config file:
%s

`, exampleConfigFile)
}
