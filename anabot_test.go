package main

import "testing"

func TestCaesar(t *testing.T) {
	tests := []struct {
		r        rune
		shift    int
		expected rune
	}{
		{'a', 1, 'b'},
		{'z', 1, 'a'},
		{'a', 26, 'a'},
		{'a', 27, 'b'},
		{'a', 52, 'a'},
		{'A', 1, 'B'},
		{'Z', 1, 'A'},
		{'A', 27, 'B'},
		{'a', -1, 'z'},
		{'a', -26, 'a'},
		{'a', -27, 'z'},
		{'a', 1000000, 'o'},
	}

	for _, tt := range tests {
		got := caesar(tt.r, tt.shift)
		if got != tt.expected {
			t.Errorf("caesar(%q, %d): expected %q, got %q", tt.r, tt.shift, tt.expected, got)
		}
	}
}
