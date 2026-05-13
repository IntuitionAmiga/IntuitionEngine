package main

import (
	"encoding/binary"
	"testing"
)

type fakeBacktraceCPU struct {
	name      string
	width     int
	registers map[string]uint64
	memory    map[uint64]byte
}

func newFakeBacktraceCPU(name string, width int) *fakeBacktraceCPU {
	return &fakeBacktraceCPU{name: name, width: width, registers: make(map[string]uint64), memory: make(map[uint64]byte)}
}

func (f *fakeBacktraceCPU) CPUName() string              { return f.name }
func (f *fakeBacktraceCPU) AddressWidth() int            { return f.width }
func (f *fakeBacktraceCPU) GetRegisters() []RegisterInfo { return nil }
func (f *fakeBacktraceCPU) GetRegister(name string) (uint64, bool) {
	v, ok := f.registers[name]
	return v, ok
}
func (f *fakeBacktraceCPU) SetRegister(name string, value uint64) bool {
	f.registers[name] = value
	return true
}
func (f *fakeBacktraceCPU) GetPC() uint64                                         { return f.registers["PC"] }
func (f *fakeBacktraceCPU) SetPC(addr uint64)                                     { f.registers["PC"] = addr }
func (f *fakeBacktraceCPU) IsRunning() bool                                       { return false }
func (f *fakeBacktraceCPU) Freeze()                                               {}
func (f *fakeBacktraceCPU) Resume()                                               {}
func (f *fakeBacktraceCPU) RequestBreakIn()                                       {}
func (f *fakeBacktraceCPU) BreakInRequested() bool                                { return false }
func (f *fakeBacktraceCPU) ConsumeBreakIn() bool                                  { return false }
func (f *fakeBacktraceCPU) Step() int                                             { return 0 }
func (f *fakeBacktraceCPU) Disassemble(addr uint64, count int) []DisassembledLine { return nil }
func (f *fakeBacktraceCPU) SetBreakpoint(addr uint64) bool                        { return false }
func (f *fakeBacktraceCPU) SetConditionalBreakpoint(addr uint64, cond *BreakpointCondition) bool {
	return false
}
func (f *fakeBacktraceCPU) ClearBreakpoint(addr uint64) bool { return false }
func (f *fakeBacktraceCPU) ClearAllBreakpoints()             {}
func (f *fakeBacktraceCPU) ListBreakpoints() []uint64        { return nil }
func (f *fakeBacktraceCPU) ListConditionalBreakpoints() []*ConditionalBreakpoint {
	return nil
}
func (f *fakeBacktraceCPU) HasBreakpoint(addr uint64) bool { return false }
func (f *fakeBacktraceCPU) GetConditionalBreakpoint(addr uint64) *ConditionalBreakpoint {
	return nil
}
func (f *fakeBacktraceCPU) SnapshotBreakpoint(addr uint64) (BreakpointSnapshot, bool) {
	return BreakpointSnapshot{}, false
}
func (f *fakeBacktraceCPU) IncrementBreakpointHit(addr uint64) (uint64, bool) { return 0, false }
func (f *fakeBacktraceCPU) SetBreakpointCondition(addr uint64, cond *BreakpointCondition) bool {
	return false
}
func (f *fakeBacktraceCPU) ListBreakpointSnapshots() []BreakpointSnapshot { return nil }
func (f *fakeBacktraceCPU) SetWatchpoint(addr uint64) bool                { return false }
func (f *fakeBacktraceCPU) ClearWatchpoint(addr uint64) bool              { return false }
func (f *fakeBacktraceCPU) ClearAllWatchpoints()                          {}
func (f *fakeBacktraceCPU) ListWatchpoints() []uint64                     { return nil }
func (f *fakeBacktraceCPU) SnapshotWatchpoint(addr uint64) (WatchpointSnapshot, bool) {
	return WatchpointSnapshot{}, false
}
func (f *fakeBacktraceCPU) UpdateWatchpointLastValue(addr uint64, val byte) bool {
	return false
}
func (f *fakeBacktraceCPU) ListWatchpointSnapshots() []WatchpointSnapshot { return nil }
func (f *fakeBacktraceCPU) ValidateAddress(addr uint64) error             { return nil }
func (f *fakeBacktraceCPU) ReadMemory(addr uint64, size int) []byte {
	out := make([]byte, size)
	for i := range size {
		out[i] = f.memory[addr+uint64(i)]
	}
	return out
}
func (f *fakeBacktraceCPU) WriteMemory(addr uint64, data []byte) {
	for i, b := range data {
		f.memory[addr+uint64(i)] = b
	}
}
func (f *fakeBacktraceCPU) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {}

func writeStackLE(f *fakeBacktraceCPU, addr uint64, value uint64, size int) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, value)
	f.WriteMemory(addr, buf[:size])
}

func writeStackBE(f *fakeBacktraceCPU, addr uint64, value uint64, size int) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, value)
	f.WriteMemory(addr, buf[8-size:])
}

func TestBacktrace_SymbolAware_AllCPUs(t *testing.T) {
	cases := []struct {
		cpu   string
		width int
		ret   uint64
		setup func(*fakeBacktraceCPU, uint64)
	}{
		{"IE64", 64, 0x2000, func(f *fakeBacktraceCPU, ret uint64) {
			f.registers["SP"] = 0x800
			writeStackLE(f, 0x800, ret, 8)
		}},
		{"IE32", 32, 0x2000, func(f *fakeBacktraceCPU, ret uint64) {
			f.registers["SP"] = 0x800
			writeStackLE(f, 0x800, ret, 4)
		}},
		{"M68K", 32, 0x2000, func(f *fakeBacktraceCPU, ret uint64) {
			f.registers["A6"] = 0x900
			writeStackBE(f, 0x900, 0, 4)
			writeStackBE(f, 0x904, ret, 4)
		}},
		{"Z80", 16, 0x2000, func(f *fakeBacktraceCPU, ret uint64) {
			f.registers["SP"] = 0x800
			writeStackLE(f, 0x800, ret, 2)
		}},
		{"6502", 16, 0x2000, func(f *fakeBacktraceCPU, ret uint64) {
			f.registers["SP"] = 0x7F
			writeStackLE(f, 0x180, ret-1, 2)
		}},
		{"X86", 32, 0x2000, func(f *fakeBacktraceCPU, ret uint64) {
			f.registers["ESP"] = 0x800
			writeStackLE(f, 0x800, ret, 4)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.cpu, func(t *testing.T) {
			cpu := newFakeBacktraceCPU(tc.cpu, tc.width)
			tc.setup(cpu, tc.ret)
			symbols := NewSymbolTable()
			symbols.Add(tc.cpu, tc.ret, "callee", 0x20, SymbolFunc)
			frames := symbolAwareBacktrace(cpu, 4, symbols, NewRegionRegistry())
			if len(frames) != 1 {
				t.Fatalf("frames len = %d, want 1: %+v", len(frames), frames)
			}
			if frames[0].Address != tc.ret || !frames[0].HasSymbol || frames[0].Symbol.Symbol.Name != "callee" {
				t.Fatalf("frame = %+v, want callee at %#x", frames[0], tc.ret)
			}
		})
	}
}

func TestBacktrace_RejectsNonCodeReturnAddrs_AllCPUs(t *testing.T) {
	cpu := newFakeBacktraceCPU("IE64", 64)
	cpu.registers["SP"] = 0x800
	writeStackLE(cpu, 0x800, 0x2000, 8)
	writeStackLE(cpu, 0x808, 0xF0010, 8)
	symbols := NewSymbolTable()
	symbols.Add("IE64", 0x2000, "real_frame", 0x20, SymbolFunc)
	symbols.Add("IE64", 0xF0010, "mmio_noise", 0x20, SymbolFunc)

	frames := symbolAwareBacktrace(cpu, 2, symbols, NewRegionRegistry())
	if len(frames) != 1 {
		t.Fatalf("frames len = %d, want 1: %+v", len(frames), frames)
	}
	if frames[0].Symbol.Symbol.Name != "real_frame" {
		t.Fatalf("frame = %+v, want real_frame only", frames[0])
	}
}

func TestBacktrace_M68KFallsBackToSPScanWhenA6Invalid(t *testing.T) {
	cpu := newFakeBacktraceCPU("M68K", 32)
	cpu.registers["A6"] = 0
	cpu.registers["A7"] = 0x900
	writeStackBE(cpu, 0x900, 0x2000, 4)
	symbols := NewSymbolTable()
	symbols.Add("M68K", 0x2000, "fallback_frame", 0x20, SymbolFunc)

	frames := symbolAwareBacktrace(cpu, 1, symbols, NewRegionRegistry())
	if len(frames) != 1 || frames[0].Symbol.Symbol.Name != "fallback_frame" {
		t.Fatalf("frames = %+v, want fallback_frame", frames)
	}
}

func TestBacktrace_X86PrefersEBPChainWhenAvailable(t *testing.T) {
	cpu := newFakeBacktraceCPU("X86", 32)
	cpu.registers["ESP"] = 0x700
	cpu.registers["EBP"] = 0x900
	writeStackLE(cpu, 0x700, 0x3000, 4)
	writeStackLE(cpu, 0x900, 0, 4)
	writeStackLE(cpu, 0x904, 0x2000, 4)

	frames := backtrace(cpu, 2)
	if len(frames) != 1 || frames[0] != 0x2000 {
		t.Fatalf("backtrace = %#v, want EBP-framed return 0x2000", frames)
	}
}
