package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	reDotEqu = regexp.MustCompile(`^\s*\.equ\s+([A-Za-z_][A-Za-z0-9_]*)\s+([$\w]+)`)
	reEqu    = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s+equ\s+([$\w]+)`)
	reDotSet = regexp.MustCompile(`^\s*\.set\s+([A-Za-z_][A-Za-z0-9_]*)\s*,\s*([$\w]+)`)
	reEq     = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*([$\w]+)`)
)

func parseIncConstants(t *testing.T, path string) map[string]uint32 {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	out := make(map[string]uint32)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var name, raw string
		if m := reDotEqu.FindStringSubmatch(line); m != nil {
			name, raw = m[1], m[2]
		} else if m := reEqu.FindStringSubmatch(line); m != nil {
			name, raw = m[1], m[2]
		} else if m := reDotSet.FindStringSubmatch(line); m != nil {
			name, raw = m[1], m[2]
		} else if m := reEq.FindStringSubmatch(line); m != nil {
			name, raw = m[1], m[2]
		} else {
			continue
		}

		val, ok := parseNum(raw)
		if !ok {
			continue
		}
		out[name] = val
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return out
}

func parseNum(raw string) (uint32, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if strings.HasPrefix(raw, "$") {
		v, err := strconv.ParseUint(raw[1:], 16, 32)
		return uint32(v), err == nil
	}
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		v, err := strconv.ParseUint(raw[2:], 16, 32)
		return uint32(v), err == nil
	}
	v, err := strconv.ParseUint(raw, 10, 32)
	return uint32(v), err == nil
}

func TestSDKInclude_32BitConstantParity(t *testing.T) {
	expected := map[string]uint32{
		"PSG_PLUS_CTRL":        PSG_PLUS_CTRL,
		"POKEY_PLUS_CTRL":      POKEY_PLUS_CTRL,
		"SID_PLUS_CTRL":        SID_PLUS_CTRL,
		"TED_PLUS_CTRL":        TED_PLUS_CTRL,
		"AHX_PLUS_CTRL":        AHX_PLUS_CTRL,
		"FLEX_CH_BASE":         FLEX_CH_BASE,
		"FLEX_CH_STRIDE":       FLEX_CH_STRIDE,
		"FLEX_CH3_BASE":        FLEX_CH3_BASE,
		"SN_PORT_WRITE":        SN_PORT_WRITE,
		"SN_PORT_READY":        SN_PORT_READY,
		"SN_PORT_MODE":         SN_PORT_MODE,
		"SN76489_MODE_LFSR_15": SN76489_MODE_LFSR_15,
		"SN76489_MODE_LFSR_16": SN76489_MODE_LFSR_16,
	}

	incFiles := []string{
		filepath.Join("sdk", "include", "ie32.inc"),
		filepath.Join("sdk", "include", "ie64.inc"),
		filepath.Join("sdk", "include", "ie68.inc"),
		filepath.Join("sdk", "include", "ie86.inc"),
	}

	for _, path := range incFiles {
		vals := parseIncConstants(t, path)
		for key, want := range expected {
			got, ok := vals[key]
			if !ok {
				t.Fatalf("%s: missing %s", path, key)
			}
			if got != want {
				t.Fatalf("%s: %s mismatch: got 0x%X want 0x%X", path, key, got, want)
			}
		}
	}
}

func TestSDKInclude_IE86POKEYPortBaseMatchesRuntime(t *testing.T) {
	vals := parseIncConstants(t, filepath.Join("sdk", "include", "ie86.inc"))
	got, ok := vals["POKEY_PORT_BASE"]
	if !ok {
		t.Fatal("sdk/include/ie86.inc: missing POKEY_PORT_BASE")
	}
	if got != X86_PORT_POKEY_BASE {
		t.Fatalf("sdk/include/ie86.inc: POKEY_PORT_BASE got 0x%X want 0x%X", got, X86_PORT_POKEY_BASE)
	}

	b, err := os.ReadFile(filepath.Join("sdk", "include", "ie86.inc"))
	if err != nil {
		t.Fatalf("read sdk/include/ie86.inc: %v", err)
	}
	s := string(b)
	if strings.Contains(s, "register offset (0-15)") {
		t.Fatal("sdk/include/ie86.inc: pokey_write documents stale 0-15 register range")
	}
	if !strings.Contains(s, "writable register offset (0-9)") {
		t.Fatal("sdk/include/ie86.inc: pokey_write missing current 0-9 register range")
	}
}

func TestDocs_X86POKEYPortsMatchRuntime(t *testing.T) {
	for _, path := range []string{
		filepath.Join("sdk", "docs", "architecture.md"),
		filepath.Join("sdk", "docs", "include-files.md"),
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		s := string(b)
		if strings.Contains(s, "$D0-$D3") || strings.Contains(s, "$D8-$DF") || strings.Contains(s, "0xD0`-`0xDF") {
			t.Fatalf("%s: stale x86 POKEY port range present", path)
		}
		if !strings.Contains(s, "0x60") && !strings.Contains(s, "$60-$69") {
			t.Fatalf("%s: missing current x86 POKEY port range", path)
		}
	}
}

func TestSDKInclude_SN8BitAliases(t *testing.T) {
	ie65 := parseIncConstants(t, filepath.Join("sdk", "include", "ie65.inc"))
	for key, want := range map[string]uint32{
		"SN_PORT_WRITE":        0xFC30,
		"SN_PORT_READY":        0xFC31,
		"SN_PORT_MODE":         0xFC32,
		"SN76489_MODE_LFSR_15": SN76489_MODE_LFSR_15,
		"SN76489_MODE_LFSR_16": SN76489_MODE_LFSR_16,
	} {
		if got := ie65[key]; got != want {
			t.Fatalf("ie65.inc: %s got 0x%X want 0x%X", key, got, want)
		}
	}

	ie80 := parseIncConstants(t, filepath.Join("sdk", "include", "ie80.inc"))
	for key, want := range map[string]uint32{
		"SN_PORT_WRITE":        0xFC30,
		"SN_PORT_READY":        0xFC31,
		"SN_PORT_MODE":         0xFC32,
		"SN76489_MODE_LFSR_15": SN76489_MODE_LFSR_15,
		"SN76489_MODE_LFSR_16": SN76489_MODE_LFSR_16,
		"Z80_SN_PORT_DATA":     Z80_SN_PORT_DATA,
		"Z80_SN_PORT_STATUS":   Z80_SN_PORT_STATUS,
	} {
		if got := ie80[key]; got != want {
			t.Fatalf("ie80.inc: %s got 0x%X want 0x%X", key, got, want)
		}
	}
}

func TestSDKInclude_TimerSymbolsDeprecated(t *testing.T) {
	files := []string{
		filepath.Join("sdk", "include", "ie32.inc"),
		filepath.Join("sdk", "include", "ie64.inc"),
		filepath.Join("sdk", "include", "ie68.inc"),
		filepath.Join("sdk", "include", "ie86.inc"),
	}

	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		s := string(b)
		if !strings.Contains(s, "TIMER REGISTERS (DEPRECATED)") {
			t.Fatalf("%s: expected deprecated timer section marker", path)
		}
		for _, sym := range []string{"TIMER_CTRL", "TIMER_COUNT", "TIMER_RELOAD"} {
			lineHasSym := false
			lineHasDeprecated := false
			sc := bufio.NewScanner(strings.NewReader(s))
			for sc.Scan() {
				line := sc.Text()
				if strings.Contains(line, sym) {
					lineHasSym = true
					if strings.Contains(strings.ToUpper(line), "DEPRECATED") {
						lineHasDeprecated = true
						break
					}
				}
			}
			if !lineHasSym {
				t.Fatalf("%s: missing %s", path, sym)
			}
			if !lineHasDeprecated {
				t.Fatalf("%s: %s must be marked DEPRECATED", path, sym)
			}
		}
	}
}

func TestSDKInclude_IE80ULACommentMatchesRuntime(t *testing.T) {
	path := filepath.Join("sdk", "include", "ie80.inc")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	s := string(b)
	if strings.Contains(s, "bit 3 = MIC") || strings.Contains(s, "bit 4 = EAR") {
		t.Fatalf("%s: stale MIC/EAR comment present; runtime currently handles border bits 0-2 only", path)
	}
}
