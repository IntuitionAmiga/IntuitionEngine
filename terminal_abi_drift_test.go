package main

import "testing"

func TestTerminalMMIO_ABI_Drift(t *testing.T) {
	expected := map[string]uint32{
		"TERM_KEY_IN":      TERM_KEY_IN,
		"TERM_KEY_STATUS":  TERM_KEY_STATUS,
		"SCAN_CODE":        SCAN_CODE,
		"SCAN_STATUS":      SCAN_STATUS,
		"SCAN_MODIFIERS":   SCAN_MODIFIERS,
		"MOUSE_CTRL":       MOUSE_CTRL,
		"MOUSE_DX":         MOUSE_DX,
		"MOUSE_DY":         MOUSE_DY,
		"RTC_MONO_USEC_LO": RTC_MONO_USEC_LO,
		"RTC_MONO_USEC_HI": RTC_MONO_USEC_HI,
	}
	for _, file := range []string{
		"sdk/include/ie32.inc",
		"sdk/include/ie64.inc",
		"sdk/include/ie65.inc",
		"sdk/include/ie68.inc",
		"sdk/include/ie80.inc",
		"sdk/include/ie86.inc",
	} {
		t.Run(file, func(t *testing.T) {
			constants := readSDKConstantsWithPrefix(t, file, "")
			for name, want := range expected {
				got, ok := constants[name]
				if !ok {
					t.Fatalf("missing %s", name)
				}
				if (file == "sdk/include/ie65.inc" || file == "sdk/include/ie80.inc") && want >= 0xE1000 {
					want -= 0xE1000
				}
				if got != want {
					t.Fatalf("%s = %#x, want %#x", name, got, want)
				}
			}
		})
	}
}
