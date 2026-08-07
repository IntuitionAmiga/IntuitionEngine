package main

import "testing"

type z80FrozenFetchTestBus struct {
	mem [0x10000]byte
}

func (b *z80FrozenFetchTestBus) Read(addr uint16) byte         { return b.mem[addr] }
func (b *z80FrozenFetchTestBus) Write(addr uint16, value byte) { b.mem[addr] = value }
func (b *z80FrozenFetchTestBus) In(uint16) byte                { return 0 }
func (b *z80FrozenFetchTestBus) Out(uint16, byte)              {}
func (b *z80FrozenFetchTestBus) Tick(int)                      {}

func TestZ80CanonicalPayloadFromBytesDecodesABI(t *testing.T) {
	tests := []struct {
		name           string
		bytes          [4]byte
		prefix, opcode byte
		length, rInc   byte
		operand        uint16
		displacement   int8
	}{
		{"base immediate", [4]byte{0x3E, 0xAA}, 0, 0x3E, 2, 1, 0xAA, 0},
		{"ed word", [4]byte{0xED, 0x43, 0x34, 0x12}, 0xED, 0x43, 4, 2, 0x1234, 0},
		{"indexed displacement", [4]byte{0xDD, 0x36, 0xFE, 0xAA}, 0xDD, 0x36, 4, 2, 0xAA, -2},
		{"indexed high immediate", [4]byte{0xDD, 0x26, 0xA5}, 0xDD, 0x26, 3, 2, 0xA5, 0},
		{"indexed low immediate", [4]byte{0xFD, 0x2E, 0x5A}, 0xFD, 0x2E, 3, 2, 0x5A, 0},
		{"ignored index jump", [4]byte{0xDD, 0xC3, 0x34, 0x12}, 0xDD, 0xC3, 4, 2, 0x1234, 0},
		{"ignored index immediate", [4]byte{0xFD, 0x3E, 0xA5}, 0xFD, 0x3E, 3, 2, 0xA5, 0},
		{"ddcb", [4]byte{0xFD, 0xCB, 0x80, 0x06}, 0xFD, 0x06, 4, 3, 0, -128},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := z80CanonicalPayloadFromBytes(0x1200, tt.bytes)
			if got.Prefix != tt.prefix || got.Opcode != tt.opcode || got.Length != tt.length || got.RIncrements != tt.rInc || got.Operand != tt.operand || got.Displacement != tt.displacement || got.ResumePC != 0x1200+uint16(tt.length) {
				t.Fatalf("payload=%+v", got)
			}
		})
	}
}

func TestZ80CanonicalHelperFreezesIgnoredIndexPrefixOperands(t *testing.T) {
	bus := &z80FrozenFetchTestBus{}
	bus.mem[0], bus.mem[1], bus.mem[2], bus.mem[3] = 0xDD, 0xC3, 0x34, 0x12
	cpu := NewCPU_Z80(bus)
	payload := z80CanonicalPayloadFromBytes(0, [4]byte{0xDD, 0xC3, 0x34, 0x12})
	bus.mem[2], bus.mem[3] = 0x78, 0x56
	cpu.executeZ80CanonicalHelper(payload)
	if cpu.PC != 0x1234 {
		t.Fatalf("ignored DD prefix JP used mutable operand: PC=%04X", cpu.PC)
	}
}

func TestZ80CanonicalHelperPayloadFromFetchUsesMappedInstructionBytes(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	mem := bus.GetMemory()
	mem[0xEFFF] = 0xD3 // OUT (n),A
	mem[0xF000] = 0x11 // unrelated backing byte
	bus.MapIO(0xF0000, 0xF0000, func(uint32) uint32 { return 0xA5 }, nil)

	payload := z80CanonicalHelperPayloadFromFetch(adapter.fetchRead, 0xEFFF)
	if payload.Length != 2 || payload.Bytes[1] != 0xA5 || payload.Operand != 0xA5 {
		t.Fatalf("payload did not freeze mapped operand: %+v", payload)
	}
}

func TestZ80CanonicalHelperRejectsUnboundedIndexPrefixes(t *testing.T) {
	for _, bytes := range [][4]byte{
		{0xDD, 0xDD, 0x21, 0x34},
		{0xFD, 0xFD, 0x2E, 0x5A},
		{0xDD, 0xED, 0x43, 0x00},
	} {
		payload := z80CanonicalPayloadFromBytes(0x100, bytes)
		if z80CanonicalHelperPayloadComplete(payload) {
			t.Fatalf("partial frozen payload accepted: bytes=% X payload=%+v", bytes, payload)
		}
	}
	if !z80CanonicalHelperPayloadComplete(z80CanonicalPayloadFromBytes(0x100, [4]byte{0xDD, 0x21, 0x34, 0x12})) {
		t.Fatal("complete index-prefixed payload rejected")
	}
}

func TestZ80CanonicalHelperFreezesRepeatedIndexPrefixes(t *testing.T) {
	bus := &z80FrozenFetchTestBus{}
	// DD FD prefixes are ignored until the final LD A,n. Mutating the final
	// immediate after capture must not change the helper's result.
	bus.mem[0], bus.mem[1], bus.mem[2], bus.mem[3] = 0xDD, 0xFD, 0x3E, 0x42
	cpu := NewCPU_Z80(bus)
	payload := z80CanonicalHelperPayloadFromFetch(func(addr uint16) byte { return bus.mem[addr] }, 0)
	if !z80CanonicalHelperPayloadComplete(payload) {
		t.Fatal("repeated index-prefix payload was not frozen")
	}
	bus.mem[3] = 0x99
	cpu.SetRunning(true)
	cpu.executeZ80CanonicalHelper(payload)
	if cpu.A != 0x42 || cpu.PC != 4 || cpu.R != 3 {
		t.Fatalf("repeated prefix helper state A=%02X PC=%04X R=%02X, want 42/0004/03", cpu.A, cpu.PC, cpu.R)
	}
}

func TestZ80CanonicalHelperDoesNotFreezeArchitecturalReads(t *testing.T) {
	bus := &z80FrozenFetchTestBus{}
	// LD A,(0000) reads the opcode byte as data. The helper must freeze only
	// fetches, not turn that architectural read into its captured image.
	bus.mem[0], bus.mem[1], bus.mem[2] = 0x3A, 0x00, 0x00
	cpu := NewCPU_Z80(bus)
	payload := z80CanonicalHelperPayloadFromFetch(func(addr uint16) byte { return bus.mem[addr] }, 0)
	bus.mem[0] = 0xA5
	cpu.SetRunning(true)
	cpu.executeZ80CanonicalHelper(payload)
	if cpu.A != 0xA5 {
		t.Fatalf("LD A,(0000) used frozen opcode as data: A=%02X, want A5", cpu.A)
	}
}

func TestZ80FrontendRegionPlanFollowsOnlyStaticJPJRWithPlanBounds(t *testing.T) {
	mem := make([]byte, 0x400)
	mem[0x100] = 0xC3 // JP $0200
	mem[0x101], mem[0x102] = 0x00, 0x02
	mem[0x200] = 0x18 // JR $0210
	mem[0x201] = 0x0E
	mem[0x210] = 0xC3 // JP $0300
	mem[0x211], mem[0x212] = 0x00, 0x03
	mem[0x300] = 0xC9 // RET, included as the final non-static boundary
	plan := z80FrontendRegionPlan(func(pc uint16) byte { return mem[pc] }, func(uint16) bool { return true }, func(z80CanonicalHelperPayload) bool { return true }, 0x100)
	want := []uint16{0x100, 0x200, 0x210, 0x300}
	if len(plan) != len(want) {
		t.Fatalf("plan=%#v want=%#v", plan, want)
	}
	for i := range want {
		if plan[i] != want[i] {
			t.Fatalf("plan[%d]=%#x want %#x", i, plan[i], want[i])
		}
	}
}

func TestZ80FrontendScanBlockSharesPrefixAndDirectPagePolicy(t *testing.T) {
	mem := make([]byte, 0x300)
	mem[0x100], mem[0x101], mem[0x102], mem[0x103] = 0xDD, 0x36, 0x7F, 0x5A // LD (IX+127),$5A
	mem[0x104] = 0xC3
	mem[0x105], mem[0x106] = 0x00, 0x02
	got := z80FrontendScanBlock(func(pc uint16) byte { return mem[pc] }, func(pc uint16) bool { return pc>>8 != 2 }, func(z80CanonicalHelperPayload) bool { return true }, 0x100)
	if len(got) != 2 || got[0].Prefix != 0xDD || got[0].Length != 4 || got[1].Opcode != 0xC3 || got[1].Operand != 0x0200 {
		t.Fatalf("shared scan = %+v", got)
	}
}
