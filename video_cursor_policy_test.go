package main

import "testing"

func TestShouldHideSystemCursor(t *testing.T) {
	cases := []struct {
		name          string
		fullscreen    bool
		profileHidden bool
		want          bool
	}{
		{name: "windowed default", want: false},
		{name: "fullscreen hides", fullscreen: true, want: true},
		{name: "profile hides", profileHidden: true, want: true},
		{name: "fullscreen and profile hide", fullscreen: true, profileHidden: true, want: true},
	}

	for _, tc := range cases {
		if got := shouldHideSystemCursor(tc.fullscreen, tc.profileHidden); got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
