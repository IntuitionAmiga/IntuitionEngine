package main

import "testing"

func TestTerminalMMIO_ABI_Drift(t *testing.T) {
	expected := map[string]uint32{
		"TERM_OUT":         TERM_OUT,
		"TERM_STATUS":      TERM_STATUS,
		"TERM_IN":          TERM_IN,
		"TERM_LINE_STATUS": TERM_LINE_STATUS,
		"TERM_ECHO":        TERM_ECHO,
		"TERM_CTRL":        TERM_CTRL,
		"TERM_KEY_IN":      TERM_KEY_IN,
		"TERM_KEY_STATUS":  TERM_KEY_STATUS,
		"SCAN_CODE":        SCAN_CODE,
		"SCAN_STATUS":      SCAN_STATUS,
		"SCAN_MODIFIERS":   SCAN_MODIFIERS,
		"MOUSE_CTRL":       MOUSE_CTRL,
		"MOUSE_DX":         MOUSE_DX,
		"MOUSE_DY":         MOUSE_DY,
		"RTC_EPOCH":        RTC_EPOCH,
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
					want = 0x2000 + (want % 0x2000)
				}
				if got != want {
					t.Fatalf("%s = %#x, want %#x", name, got, want)
				}
			}
			if file == "sdk/include/ie65.inc" || file == "sdk/include/ie80.inc" {
				if got := constants["TERM_IO_BANK"]; got != 0x78 {
					t.Fatalf("TERM_IO_BANK = %#x, want 0x78", got)
				}
				for _, pair := range [][2]string{
					{"TERM_OUT", "BANK1_REG_LO"},
					{"TERM_STATUS", "BANK3_REG_LO"},
				} {
					if constants[pair[0]] == constants[pair[1]] {
						t.Fatalf("%s aliases %s at %#x", pair[0], pair[1], constants[pair[0]])
					}
				}
			}
		})
	}
}

func TestTerminalMMIO_8BitBankWindowABI(t *testing.T) {
	const (
		termIOBank       = 0x78
		termOutAlias     = 0x2700
		termStatusAlias  = 0x2704
		terminalReadyBit = 0x02
	)

	t.Run("ie65", func(t *testing.T) {
		bus := NewMachineBus()
		terminal := NewTerminalMMIO()
		bus.MapIO(TERM_OUT, TERMINAL_REGION_END, terminal.HandleRead, terminal.HandleWrite)

		adapter := NewBus6502Adapter(bus)
		adapter.Write(BANK1_REG_LO, termIOBank)
		adapter.Write(BANK1_REG_HI, 0)

		if got := adapter.Read(termStatusAlias); got&terminalReadyBit == 0 {
			t.Fatalf("TERM_STATUS alias returned %#x, want output-ready bit set", got)
		}
		adapter.Write(termOutAlias, '6')
		if got := terminal.DrainOutput(); got != "6" {
			t.Fatalf("TERM_OUT alias output = %q, want %q", got, "6")
		}

		adapter.Write(BANK1_REG_LO, 0x12)
		if got := adapter.Read(BANK1_REG_LO); got != 0x12 {
			t.Fatalf("BANK1_REG_LO readback = %#x, want 0x12", got)
		}
		if got := terminal.DrainOutput(); got != "" {
			t.Fatalf("BANK1_REG_LO unexpectedly reached terminal output: %q", got)
		}
	})

	t.Run("ie80", func(t *testing.T) {
		bus := NewMachineBus()
		terminal := NewTerminalMMIO()
		bus.MapIO(TERM_OUT, TERMINAL_REGION_END, terminal.HandleRead, terminal.HandleWrite)

		adapter := NewZ80BusAdapter(bus)
		adapter.Write(Z80_BANK1_REG_LO, termIOBank)
		adapter.Write(Z80_BANK1_REG_HI, 0)

		if got := adapter.Read(termStatusAlias); got&terminalReadyBit == 0 {
			t.Fatalf("TERM_STATUS alias returned %#x, want output-ready bit set", got)
		}
		adapter.Write(termOutAlias, '8')
		if got := terminal.DrainOutput(); got != "8" {
			t.Fatalf("TERM_OUT alias output = %q, want %q", got, "8")
		}

		adapter.Write(Z80_BANK1_REG_LO, 0x34)
		if got := adapter.Read(Z80_BANK1_REG_LO); got != 0x34 {
			t.Fatalf("Z80_BANK1_REG_LO readback = %#x, want 0x34", got)
		}
		if got := terminal.DrainOutput(); got != "" {
			t.Fatalf("Z80_BANK1_REG_LO unexpectedly reached terminal output: %q", got)
		}
	})
}
