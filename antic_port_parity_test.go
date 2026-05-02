package main

import "testing"

func newMappedANTICTestBus() (*MachineBus, *ANTICEngine) {
	bus := NewMachineBus()
	antic := NewANTICEngine(bus)
	bus.MapIO(ANTIC_BASE, ANTIC_END, antic.HandleRead, antic.HandleWrite)
	bus.MapIO(GTIA_BASE, GTIA_END, antic.HandleRead, antic.HandleWrite)
	return bus, antic
}

func TestANTICGTIAPortParityFullRegisterWindow(t *testing.T) {
	type portBus interface {
		In(port uint16) byte
		Out(port uint16, value byte)
	}

	tests := []struct {
		name       string
		newAdapter func(*MachineBus) portBus
		anticSel   uint16
		anticData  uint16
		gtiaSel    uint16
		gtiaData   uint16
	}{
		{
			name:       "z80",
			newAdapter: func(bus *MachineBus) portBus { return NewZ80BusAdapter(bus) },
			anticSel:   Z80_ANTIC_PORT_SELECT,
			anticData:  Z80_ANTIC_PORT_DATA,
			gtiaSel:    Z80_GTIA_PORT_SELECT,
			gtiaData:   Z80_GTIA_PORT_DATA,
		},
		{
			name:       "x86",
			newAdapter: func(bus *MachineBus) portBus { return NewX86BusAdapter(bus) },
			anticSel:   X86_PORT_ANTIC_SELECT,
			anticData:  X86_PORT_ANTIC_DATA,
			gtiaSel:    X86_PORT_GTIA_SELECT,
			gtiaData:   X86_PORT_GTIA_DATA,
		},
	}

	gtiaRegs := []struct {
		name  string
		idx   byte
		addr  uint32
		value byte
	}{
		{"COLPF0", 0, GTIA_COLPF0, 0x21},
		{"HPOSP0", 12, GTIA_HPOSP0, 0x52},
		{"HPOSM1", 17, GTIA_HPOSM1, 0x63},
		{"GRAFP0", 25, GTIA_GRAFP0, 0x84},
		{"GRAFM", 29, GTIA_GRAFM, 0x95},
		{"HITCLR", 46, GTIA_HITCLR, 0x00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus, _ := newMappedANTICTestBus()
			adapter := tt.newAdapter(bus)

			adapter.Out(tt.anticSel, 0)
			adapter.Out(tt.anticData, 0x37)
			if got := adapter.In(tt.anticData); got != 0x37 {
				t.Fatalf("ANTIC DMACTL port read got 0x%02X, want 0x37", got)
			}
			if got := bus.Read8(ANTIC_DMACTL); got != 0x37 {
				t.Fatalf("ANTIC DMACTL direct read got 0x%02X, want 0x37", got)
			}

			for _, reg := range gtiaRegs {
				t.Run(reg.name, func(t *testing.T) {
					adapter.Out(tt.gtiaSel, reg.idx)
					adapter.Out(tt.gtiaData, reg.value)
					if got := adapter.In(tt.gtiaData); got != reg.value {
						t.Fatalf("GTIA idx %d port read got 0x%02X, want 0x%02X", reg.idx, got, reg.value)
					}
					if got := bus.Read8(reg.addr); got != reg.value {
						t.Fatalf("GTIA idx %d direct read got 0x%02X, want 0x%02X", reg.idx, got, reg.value)
					}
				})
			}
		})
	}
}

func TestANTICGTIAPortCollisionRegistersAndHITCLR(t *testing.T) {
	tests := []struct {
		name       string
		newAdapter func(*MachineBus) interface {
			In(uint16) byte
			Out(uint16, byte)
		}
		gtiaSel  uint16
		gtiaData uint16
	}{
		{"z80", func(bus *MachineBus) interface {
			In(uint16) byte
			Out(uint16, byte)
		} {
			return NewZ80BusAdapter(bus)
		}, Z80_GTIA_PORT_SELECT, Z80_GTIA_PORT_DATA},
		{"x86", func(bus *MachineBus) interface {
			In(uint16) byte
			Out(uint16, byte)
		} {
			return NewX86BusAdapter(bus)
		}, X86_PORT_GTIA_SELECT, X86_PORT_GTIA_DATA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus, antic := newMappedANTICTestBus()
			adapter := tt.newAdapter(bus)
			antic.playerPF[0] = 0x05

			adapter.Out(tt.gtiaSel, 34) // P0PF
			if got := adapter.In(tt.gtiaData); got != 0x05 {
				t.Fatalf("P0PF port read got 0x%02X, want 0x05", got)
			}
			adapter.Out(tt.gtiaData, 0xAA)
			if got := antic.playerPF[0]; got != 0x05 {
				t.Fatalf("P0PF write changed read-only latch to 0x%02X", got)
			}

			adapter.Out(tt.gtiaSel, 46) // HITCLR
			adapter.Out(tt.gtiaData, 0)
			if got := antic.playerPF[0]; got != 0 {
				t.Fatalf("HITCLR did not clear P0PF, got 0x%02X", got)
			}
		})
	}
}

func TestANTICGTIAPortOutOfRangeRejected(t *testing.T) {
	type portBus interface {
		In(port uint16) byte
		Out(port uint16, value byte)
	}

	tests := []struct {
		name       string
		newAdapter func(*MachineBus) portBus
		anticSel   uint16
		anticData  uint16
		gtiaSel    uint16
		gtiaData   uint16
	}{
		{"z80", func(bus *MachineBus) portBus { return NewZ80BusAdapter(bus) }, Z80_ANTIC_PORT_SELECT, Z80_ANTIC_PORT_DATA, Z80_GTIA_PORT_SELECT, Z80_GTIA_PORT_DATA},
		{"x86", func(bus *MachineBus) portBus { return NewX86BusAdapter(bus) }, X86_PORT_ANTIC_SELECT, X86_PORT_ANTIC_DATA, X86_PORT_GTIA_SELECT, X86_PORT_GTIA_DATA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus, _ := newMappedANTICTestBus()
			adapter := tt.newAdapter(bus)

			adapter.Out(tt.anticSel, ANTIC_REG_COUNT)
			adapter.Out(tt.anticData, 0xAA)
			if got := adapter.In(tt.anticData); got != 0 {
				t.Fatalf("out-of-range ANTIC read got 0x%02X, want 0", got)
			}
			if got := bus.Read8(ANTIC_BASE); got == 0xAA {
				t.Fatalf("out-of-range ANTIC write modified DMACTL")
			}

			adapter.Out(tt.gtiaSel, GTIA_REG_COUNT)
			adapter.Out(tt.gtiaData, 0xBB)
			if got := adapter.In(tt.gtiaData); got != 0 {
				t.Fatalf("out-of-range GTIA read got 0x%02X, want 0", got)
			}
			if got := bus.Read8(GTIA_BASE); got == 0xBB {
				t.Fatalf("out-of-range GTIA write modified COLPF0")
			}
		})
	}
}
