// cpu_reload_scope_test.go - PLAN_MAX_RAM reload-hardening slice.
//
// Pins that buildReloadClosure() reload paths for Z80, 6502, x86, and
// M68K do not iterate over guest RAM beyond the CPU-visible program
// window. Mirrors the IE32/IE64 reset_scope tests: F10/reload of an
// oversize cached program must not spill past the CPU's natural load
// ceiling and clobber memory or MMIO that lives above it.

package main

import (
	"os"
	"strings"
	"testing"
)

// reloadOversizeSentinelTest is the shared shape for all four cores.
// The reload closure is invoked with `oversize` bytes; `sentinelAddr`
// must survive untouched after the closure runs.
type reloadOversizeSentinelTest struct {
	mode         string
	makeRunner   func(bus *MachineBus) EmulatorCPU
	configureBus func(bus *MachineBus)
	loadAddr     uint32
	sentinelAddr uint32
	overflow     uint32 // bytes beyond intended-end the program is sized to
}

func runReloadOversizeSentinelTest(t *testing.T, tc reloadOversizeSentinelTest) {
	t.Helper()

	bus := NewMachineBus()
	if tc.configureBus != nil {
		tc.configureBus(bus)
	}
	runner := tc.makeRunner(bus)

	const sentinel byte = 0xA5
	bus.Write8(tc.sentinelAddr, sentinel)
	if got := bus.Read8(tc.sentinelAddr); got != sentinel {
		t.Fatalf("sentinel write probe failed at %#x: got %#x want %#x",
			tc.sentinelAddr, got, sentinel)
	}

	// Oversize program: bytes from loadAddr through sentinelAddr+overflow.
	progLen := int(tc.sentinelAddr-tc.loadAddr) + int(tc.overflow)
	if progLen <= 0 {
		t.Fatalf("invalid test sizing: progLen=%d", progLen)
	}
	prog := make([]byte, progLen)
	for i := range prog {
		prog[i] = 0xCC
	}

	reload := buildReloadClosure(tc.mode, runner, prog, bus)
	reload()

	if got := bus.Read8(tc.sentinelAddr); got != sentinel {
		t.Fatalf("%s reload clobbered sentinel at %#x: got %#x want %#x (reload must not write past CPU-visible program ceiling)",
			tc.mode, tc.sentinelAddr, got, sentinel)
	}
}

// TestReloadScope_Z80_OversizeDoesNotSpillPastBankedCeiling pins the Z80
// reload closure clamps to BankedVisibleCeiling. With a small published
// ceiling, oversize bytes must not write past it.
func TestReloadScope_Z80_OversizeDoesNotSpillPastBankedCeiling(t *testing.T) {
	runReloadOversizeSentinelTest(t, reloadOversizeSentinelTest{
		mode: "z80",
		configureBus: func(bus *MachineBus) {
			bus.SetSizing(MemorySizing{ActiveVisibleRAM: 8 * 1024})
		},
		makeRunner: func(bus *MachineBus) EmulatorCPU {
			return NewCPUZ80Runner(bus, CPUZ80Config{LoadAddr: 0, Entry: 0})
		},
		loadAddr:     0,
		sentinelAddr: 16 * 1024,
		overflow:     4 * 1024,
	})
}

// TestReloadScope_6502_OversizeDoesNotSpillPastBankedCeiling: same shape
// but with the 6502 default load addr of 0x0600.
func TestReloadScope_6502_OversizeDoesNotSpillPastBankedCeiling(t *testing.T) {
	runReloadOversizeSentinelTest(t, reloadOversizeSentinelTest{
		mode: "6502",
		configureBus: func(bus *MachineBus) {
			bus.SetSizing(MemorySizing{ActiveVisibleRAM: 8 * 1024})
		},
		makeRunner: func(bus *MachineBus) EmulatorCPU {
			return NewCPU6502Runner(bus, CPU6502Config{LoadAddr: 0x0600, Entry: 0x0600})
		},
		loadAddr:     0x0600,
		sentinelAddr: 16 * 1024,
		overflow:     4 * 1024,
	})
}

func TestReloadScope_IntuitionOSUsesKernelBootLoader(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	program := []byte{0x01, 0x02, 0x03, 0x04}
	staleAddr := uint32(PROG_START + len(program) + 8)
	bus.Write8(staleAddr, 0xA5)

	reload := buildReloadClosure("intuitionos", cpu, program, bus)
	reload()

	if got := cpu.PC; got != PROG_START {
		t.Fatalf("IntuitionOS reload PC = %#x, want %#x", got, uint64(PROG_START))
	}
	for i, want := range program {
		if got := bus.Read8(uint32(PROG_START + i)); got != want {
			t.Fatalf("IntuitionOS reload byte[%d] = %#x, want %#x", i, got, want)
		}
	}
	if got := bus.Read8(staleAddr); got != 0xA5 {
		t.Fatalf("IntuitionOS flat reload clobbered byte outside image at %#x: got %#x, want 0xA5", staleAddr, got)
	}
}

func TestReloadScope_IntuitionOSFlatReloadLoadsPastLegacyStackWindow(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	programLen := int(STACK_START-PROG_START) + 4096
	program := make([]byte, programLen)
	for i := range program {
		program[i] = byte(i)
	}

	reload := buildReloadClosure("intuitionos", cpu, program, bus)
	reload()

	pastStack := uint32(STACK_START + 512)
	want := program[int(pastStack-PROG_START)]
	if got := bus.Read8(pastStack); got != want {
		t.Fatalf("IntuitionOS flat reload byte past STACK_START at %#x = %#x, want %#x", pastStack, got, want)
	}
	if got := cpu.PC; got != PROG_START {
		t.Fatalf("IntuitionOS flat reload PC = %#x, want %#x", got, uint64(PROG_START))
	}
}

func TestInitialIntuitionOSKernelLoadUsesFlatImagePath(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	programLen := int(STACK_START-PROG_START) + 4096
	program := make([]byte, programLen)
	for i := range program {
		program[i] = byte(i)
	}

	if err := loadIntuitionOSKernelImage(cpu, program); err != nil {
		t.Fatalf("loadIntuitionOSKernelImage returned error: %v", err)
	}

	pastStack := uint32(STACK_START + 512)
	want := program[int(pastStack-PROG_START)]
	if got := bus.Read8(pastStack); got != want {
		t.Fatalf("initial IntuitionOS load byte past STACK_START at %#x = %#x, want %#x", pastStack, got, want)
	}
	if got := cpu.PC; got != PROG_START {
		t.Fatalf("initial IntuitionOS load PC = %#x, want %#x", got, uint64(PROG_START))
	}
}

func TestInitialIntuitionOSMainBootUsesFlatKernelLoader(t *testing.T) {
	srcBytes, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile(main.go): %v", err)
	}
	src := string(srcBytes)
	start := strings.Index(src, "if modeIOS {")
	if start < 0 {
		t.Fatalf("main.go missing initial modeIOS branch")
	}
	endRel := strings.Index(src[start:], "} else if modeBasic {")
	if endRel < 0 {
		t.Fatalf("main.go modeIOS branch missing following modeBasic branch")
	}
	branch := src[start : start+endRel]
	if !strings.Contains(branch, "loadIntuitionOSKernelImage(ie64CPU, imageBytes)") {
		t.Fatalf("initial modeIOS branch does not call loadIntuitionOSKernelImage")
	}
	if strings.Contains(branch, "LoadProgramBytes(imageBytes)") {
		t.Fatalf("initial modeIOS branch uses LoadProgramBytes(imageBytes), which truncates kernels at STACK_START")
	}
}

func TestInitialIntuitionOSMainBootRecordsDistinctCurrentMode(t *testing.T) {
	srcBytes, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile(main.go): %v", err)
	}
	src := string(srcBytes)
	start := strings.Index(src, "} else if modeIE64 || modeIOS {")
	if start < 0 {
		t.Fatalf("main.go missing shared IE64/IntuitionOS boot branch")
	}
	endRel := strings.Index(src[start:], "} else if modeM68K {")
	if endRel < 0 {
		t.Fatalf("main.go shared IE64/IntuitionOS boot branch missing following modeM68K branch")
	}
	branch := src[start : start+endRel]
	if !strings.Contains(branch, `if modeIOS {
			currentMode = "intuitionos"
		} else {
			currentMode = "ie64"
		}`) {
		t.Fatalf("initial modeIOS branch does not preserve distinct currentMode for later reload/reset decisions")
	}
}

func TestResetLoadPathKeepsIntuitionOSOnDedicatedReloadClosure(t *testing.T) {
	srcBytes, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile(main.go): %v", err)
	}
	src := string(srcBytes)
	start := strings.Index(src, "// 10. Update cached state")
	if start < 0 {
		t.Fatalf("main.go missing reset cached-state section")
	}
	endRel := strings.Index(src[start:], "// 11. Load program")
	if endRel < 0 {
		t.Fatalf("main.go reset cached-state section missing load-program section")
	}
	section := src[start : start+endRel]
	if !strings.Contains(section, "currentMode = mode") {
		t.Fatalf("reset path does not carry resolved mode into currentMode")
	}
	if !strings.Contains(section, "buildReloadClosure(mode, cpuRunner, bytes, sysBus)") {
		t.Fatalf("reset path does not rebuild reload closure from resolved mode")
	}
	if strings.Contains(section, `mode == "ie64"`) || strings.Contains(section, `case "ie64"`) {
		t.Fatalf("reset path must not special-case only ie64 before rebuilding reload closure")
	}
}

// TestReloadScope_x86_OversizeDoesNotSpillPastVisibleCeiling pins the
// x86 reload closure honours the same address-space cap LoadProgramData
// already enforces (len(bus.memory)).
func TestReloadScope_x86_OversizeDoesNotSpillPastVisibleCeiling(t *testing.T) {
	runReloadOversizeSentinelTest(t, reloadOversizeSentinelTest{
		mode: "x86",
		configureBus: func(bus *MachineBus) {
			bus.SetSizing(MemorySizing{ActiveVisibleRAM: 8 * 1024})
		},
		makeRunner: func(bus *MachineBus) EmulatorCPU {
			return NewCPUX86Runner(bus, &CPUX86Config{LoadAddr: 0, Entry: 0})
		},
		loadAddr:     0,
		sentinelAddr: 16 * 1024,
		overflow:     4 * 1024,
	})
}

// TestReloadScope_x86_ReloadHonoursConfiguredLoadAddr pins that x86
// reload writes program bytes at the configured loadAddr, not at
// address 0. The current closure uses `r.bus.Write(uint32(i), b)` which
// silently drops loadAddr; routing reload through LoadProgramData fixes
// both this and the oversize bound.
func TestReloadScope_x86_ReloadHonoursConfiguredLoadAddr(t *testing.T) {
	bus := NewMachineBus()
	const cfgLoadAddr uint32 = 0x10000
	runner := NewCPUX86Runner(bus, &CPUX86Config{
		LoadAddr: cfgLoadAddr,
		Entry:    cfgLoadAddr,
	})

	// Pre-write a sentinel byte at address 0 — if reload erroneously
	// writes from address 0, this gets clobbered with prog[0]=0xCC.
	const zeroSentinel byte = 0x5A
	bus.Write8(0, zeroSentinel)

	prog := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	reload := buildReloadClosure("x86", runner, prog, bus)
	reload()

	if got := bus.Read8(0); got != zeroSentinel {
		t.Fatalf("x86 reload wrote at address 0 instead of loadAddr=%#x: got %#x want %#x",
			cfgLoadAddr, got, zeroSentinel)
	}
	for i, b := range prog {
		if got := bus.Read8(cfgLoadAddr + uint32(i)); got != b {
			t.Fatalf("x86 reload missed loadAddr: byte %d at %#x = %#x want %#x",
				i, cfgLoadAddr+uint32(i), got, b)
		}
	}
}

// TestReloadScope_M68K_OversizeDoesNotSpillPastStackStart pins the
// M68K reload closure (LoadProgramBytes) cannot write past
// M68K_STACK_START. M68K_ENTRY_POINT is 0x1000 and M68K_STACK_START is
// 0x00FF0000; an oversize program must be clamped at STACK_START.
func TestReloadScope_M68K_OversizeDoesNotSpillPastStackStart(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	runner := NewM68KRunner(cpu)

	const sentinel byte = 0x77
	const sentinelAddr uint32 = M68K_STACK_START
	bus.Write8(sentinelAddr, sentinel)

	progLen := int(M68K_STACK_START-uint32(M68K_ENTRY_POINT)) + 4096
	prog := make([]byte, progLen)
	for i := range prog {
		prog[i] = 0xCC
	}

	reload := buildReloadClosure("m68k", runner, prog, bus)
	reload()

	if got := bus.Read8(sentinelAddr); got != sentinel {
		t.Fatalf("m68k reload clobbered sentinel at %#x (=M68K_STACK_START): got %#x want %#x",
			sentinelAddr, got, sentinel)
	}
}
