package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestMIDI_ABI_Drift(t *testing.T) {
	expected := map[string]uint32{
		"MIDI_PLAY_PTR":       MIDI_PLAY_PTR,
		"MIDI_PLAY_LEN":       MIDI_PLAY_LEN,
		"MIDI_PLAY_CTRL":      MIDI_PLAY_CTRL,
		"MIDI_PLAY_STATUS":    MIDI_PLAY_STATUS,
		"MIDI_POSITION":       MIDI_POSITION,
		"MIDI_VOLUME":         MIDI_VOLUME,
		"MIDI_TEMPO_BPM":      MIDI_TEMPO_BPM,
		"MIDI_STATUS_LOADING": MIDI_STATUS_LOADING,
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
			constants := readSDKConstantsWithPrefix(t, file, "MIDI_")
			if _, ok := constants["MIDI_END"]; ok {
				t.Fatal("SDK includes must not expose MIDI_END")
			}
			for name, want := range expected {
				got, ok := constants[name]
				if !ok {
					got, ok = constants[name+"_0"]
				}
				if !ok {
					t.Fatalf("missing %s", name)
				}
				if file == "sdk/include/ie65.inc" && want >= 0xE1000 {
					want -= 0xE1000
				}
				if got != want {
					t.Fatalf("%s = %#x, want %#x", name, got, want)
				}
			}
		})
	}
}

func TestRawlandMiniPublicName(t *testing.T) {
	if RawlandMiniPatchTableName != "RawlandMini" {
		t.Fatalf("patch table name = %q, want RawlandMini", RawlandMiniPatchTableName)
	}
	if strings.Contains(RawlandMiniPatchTableName, "Subsynth") || strings.Contains(RawlandMiniPatchTableName, "Rawland ") {
		t.Fatalf("public patch table name must stay RawlandMini, got %q", RawlandMiniPatchTableName)
	}
}

func readSDKConstantsWithPrefix(t *testing.T, file, prefix string) map[string]uint32 {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\s*(?:\.equ|\.set)?\s*([A-Za-z0-9_]+)\s*(?:equ|=|,)?\s*(\$[0-9A-Fa-f]+|0x[0-9A-Fa-f]+)`)
	out := make(map[string]uint32)
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		name := m[1]
		if !strings.HasPrefix(name, prefix) {
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
