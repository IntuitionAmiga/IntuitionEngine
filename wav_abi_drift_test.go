package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestWAV_ABI_Drift(t *testing.T) {
	expected := map[string]uint32{
		"WAV_PLAY_PTR":     WAV_PLAY_PTR,
		"WAV_PLAY_LEN":     WAV_PLAY_LEN,
		"WAV_PLAY_CTRL":    WAV_PLAY_CTRL,
		"WAV_PLAY_STATUS":  WAV_PLAY_STATUS,
		"WAV_POSITION":     WAV_POSITION,
		"WAV_PLAY_PTR_HI":  WAV_PLAY_PTR_HI,
		"WAV_CHANNEL_BASE": WAV_CHANNEL_BASE,
		"WAV_VOLUME_L":     WAV_VOLUME_L,
		"WAV_VOLUME_R":     WAV_VOLUME_R,
		"WAV_FLAGS":        WAV_FLAGS,
	}
	files := []string{
		"sdk/include/ie32.inc",
		"sdk/include/ie64.inc",
		"sdk/include/ie65.inc",
		"sdk/include/ie68.inc",
		"sdk/include/ie80.inc",
		"sdk/include/ie86.inc",
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			constants := readSDKConstants(t, file)
			if _, ok := constants["WAV_END"]; ok {
				t.Fatal("SDK includes must not expose WAV_END")
			}
			for name, want := range expected {
				got, ok := constants[name]
				if !ok {
					got, ok = constants[name+"_0"]
				}
				if !ok {
					t.Fatalf("missing %s", name)
				}
				if file == "sdk/include/ie65.inc" {
					want -= 0xE1000
				}
				if got != want {
					t.Fatalf("%s = %#x, want %#x", name, got, want)
				}
			}
		})
	}
}

func readSDKConstants(t *testing.T, file string) map[string]uint32 {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\s*(?:\.equ|\.set)?\s*([A-Za-z0-9_]+)\s*(?:equ|=|,)?\s*(\$[0-9A-Fa-f]+|0x[0-9A-Fa-f]+)`)
	out := make(map[string]uint32)
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		name := m[1]
		if !strings.HasPrefix(name, "WAV_") {
			continue
		}
		num := strings.TrimPrefix(strings.TrimPrefix(m[2], "$"), "0x")
		v, err := strconv.ParseUint(num, 16, 32)
		if err != nil {
			t.Fatalf("%s %s parse: %v", file, name, err)
		}
		out[name] = uint32(v)
	}
	return out
}
