package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"time"
)

const (
	// arosROMBase is the load address for the AROS ROM image.
	// AROS m68k-amiga uses 0xF80000; the IE port uses 0x600000
	// (above the 5MB VRAM region, safely below the I/O hole).
	arosROMBase = 0x600000

	// arosBootSP is the initial supervisor stack pointer.
	// Placed at the top of the first memory bank (below I/O holes).
	arosBootSP = 0x00020000

	// arosSentinel is a non-filesystem path passed to runProgramWithFullReset
	// to trigger AROS boot without a filename (ROM resolved via loadAROSImage).
	arosSentinel = "\x00aros\x00"
)

// AROSLoader manages the AROS ROM boot lifecycle on the IE M68K CPU.
// It follows the same patterns as EmuTOSLoader: ROM loading with big-endian
// conversion, interrupt timer goroutines, and vector arming checks.
type AROSLoader struct {
	bus       *MachineBus
	cpu       *M68KCPU
	videoChip *VideoChip

	profile ProfileBounds // AROS M68K memory-map contract; populated in NewAROSLoader and re-evaluated in LoadROM.

	ctx    context.Context
	cancel context.CancelFunc

	timerDone  chan struct{}
	vblankDone chan struct{}

	l2Armed bool // Level 2: input devices
	l3Armed bool // Level 3: audio DMA
	l4Armed bool // Level 4: VBL (60Hz)
	l5Armed bool // Level 5: system timer (200Hz)

	arosDrive string // host directory for DOS volume
}

func NewAROSLoader(bus *MachineBus, cpu *M68KCPU, videoChip *VideoChip) *AROSLoader {
	l := &AROSLoader{
		bus:       bus,
		cpu:       cpu,
		videoChip: videoChip,
	}
	// Populate the profile so vector validity checks work in tests that
	// construct a loader without calling LoadROM. LoadROM re-evaluates
	// the bounds against the current bus sizing.
	l.profile = AROSProfileBounds(bus)
	return l
}

// LoadROM loads an AROS ROM image into bus memory with big-endian byte swapping,
// installs reset vectors (SP at address 0, PC at address 4), and resets the CPU.
func (l *AROSLoader) LoadROM(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("AROS ROM too small: %d bytes", len(data))
	}

	pb := AROSProfileBounds(l.bus)
	if pb.Err != nil {
		return fmt.Errorf("AROS profile bounds: %w", pb.Err)
	}
	l.profile = pb
	base := uint32(arosROMBase)
	end := base + uint32(len(data))
	if end > pb.TopOfRAM {
		return fmt.Errorf("AROS ROM range 0x%08X-0x%08X outside AROS profile (top=0x%08X)",
			base, end-1, pb.TopOfRAM)
	}

	// Verify ROM pages don't overlap with I/O regions.
	startPage := base >> 8
	endPage := (end - 1) >> 8
	for page := startPage; page <= endPage; page++ {
		if l.bus.ioPageBitmap[page] {
			return fmt.Errorf("AROS ROM page 0x%X overlaps I/O", page)
		}
	}

	// Load ROM with big-endian word swap (M68K is big-endian, bus is little-endian).
	for i := 0; i+1 < len(data); i += 2 {
		beWord := binary.BigEndian.Uint16(data[i : i+2])
		l.cpu.Write16(base+uint32(i), beWord)
	}
	if len(data)%2 != 0 {
		l.bus.Write8(base+uint32(len(data)-1), data[len(data)-1])
	}

	// Install reset vectors: SP at address 0, PC at address 4.
	// AROS ROM starts with SSP (long) then initial PC (long) in standard M68K format.
	l.cpu.Write32(0, arosBootSP)
	l.cpu.Write32(M68K_RESET_VECTOR, binary.BigEndian.Uint32(data[4:8]))
	l.cpu.Reset()

	// AROS moves SP throughout boot (initial→supervisor→user stacks).
	// Disable the stack bounds check which is designed for simple programs.
	// The M68K core also clamps PC-fetch and prefetch to the profile top so
	// AROS never inherits the architectural 4 GiB visible range by default.
	l.cpu.SetProfileTopOfRAM(pb.TopOfRAM)
	l.cpu.stackLowerBound = 0
	l.cpu.stackUpperBound = pb.TopOfRAM

	// Enable Amiga INTENA emulation. AROS kernel_cpu.c uses
	// move.w #$4000/$C000, $DFF09A to disable/enable the interrupt master
	// gate during task switching. Without this, interrupts nest unboundedly
	// when KrnSti drops IPL while the timer goroutine re-asserts level 5.
	intena := &atomic.Bool{}
	intena.Store(true) // Interrupts enabled at boot
	l.cpu.AmigaINTENA = intena

	// Debug watch: catch any writes to SysBase (address 4-7) or bus error vector (address 8-11)
	// after boot setup. These should only be written once during krnPrepareExecBase.
	var sysBaseWriteCount int
	l.cpu.DebugWatchFn = func(addr, value, pc uint32, size int) {
		if addr < 16 {
			sysBaseWriteCount++
			if sysBaseWriteCount > 100 {
				if sysBaseWriteCount == 101 {
					fmt.Printf("LOWMEM WATCH: suppressing further (>100 writes)\r\n")
				}
				return
			}
			fmt.Printf("LOWMEM WRITE: pc=%08X addr=%02X val=%08X size=%d sr=%04X sp=%08X\r\n",
				pc, addr, value, size, l.cpu.SR, l.cpu.AddrRegs[7])
		}
	}

	fmt.Printf("AROS vectors: mem[0]=%08X mem[4]=%08X a7=%08X pc=%08X\r\n",
		l.cpu.Read32(0), l.cpu.Read32(M68K_RESET_VECTOR), l.cpu.AddrRegs[7], l.cpu.PC)
	fmt.Printf("AROS ROM: %d bytes loaded at 0x%06X-0x%06X\r\n", len(data), base, end-1)

	return nil
}

// StartTimer launches the VBL (60Hz) and system timer (200Hz, 5ms) goroutines.
// Interrupts are only asserted once the CPU has installed valid vector handlers.
// This matches the EmuTOS loader's proven interrupt model:
//   - Level 2 = Input devices (keyboard/mouse polling)
//   - Level 4 = VBL (display vertical blank, 60Hz)
//   - Level 5 = System timer (5ms tick, 200Hz scheduling)
func (l *AROSLoader) StartTimer() {
	l.stopTimers()

	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.timerDone = make(chan struct{})
	l.vblankDone = make(chan struct{})
	l.l2Armed = false
	l.l3Armed = false
	l.l4Armed = false
	l.l5Armed = false

	// System timer goroutine: 5ms tick (200Hz) → Level 5 autovector.
	go func() {
		defer close(l.timerDone)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-l.ctx.Done():
				return
			case <-ticker.C:
				if !l.cpu.Running() {
					continue
				}
				l.refreshIRQArming()
				// Always assert if armed — don't gate on INTENA here.
				// ProcessInterrupt checks INTENA at delivery time.
				// Asserting unconditionally matches real Amiga INTREQ
				// behavior: bits stay pending even when INTENA masks
				// delivery, and fire immediately when re-enabled.
				if l.l5Armed {
					l.cpu.AssertInterrupt(5)
				}
			}
		}
	}()

	// VBL goroutine: 16.667ms tick (60Hz) → Level 4 autovector + Level 2 input polling.
	// Level 2 (PORTS) handles keyboard/mouse input on real Amiga hardware.
	// Assert it at VBL rate so AROS's input.device processes mouse/key events.
	go func() {
		defer close(l.vblankDone)
		ticker := time.NewTicker(16_666_667 * time.Nanosecond)
		defer ticker.Stop()
		for {
			select {
			case <-l.ctx.Done():
				return
			case <-ticker.C:
				if !l.cpu.Running() {
					continue
				}
				l.refreshIRQArming()
				if l.l2Armed {
					l.cpu.AssertInterrupt(2)
				}
				if l.l4Armed {
					l.cpu.AssertInterrupt(4)
				}
			}
		}
	}()
}

// refreshIRQArming checks whether the CPU has installed valid interrupt handlers
// in the vector table. Only assert interrupts once handlers are installed to
// avoid spurious exceptions during early boot.
func (l *AROSLoader) refreshIRQArming() {
	if l.l2Armed && l.l3Armed && l.l4Armed && l.l5Armed {
		return
	}

	// AROS uses autovectors: L2→vector 26, L3→vector 27, L4→vector 28, L5→vector 29.
	base := l.cpu.VBR
	if !l.l2Armed {
		vec2 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL2)*4)
		l.l2Armed = l.isValidVector(vec2)
	}
	if !l.l3Armed {
		vec3 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL3)*4)
		l.l3Armed = l.isValidVector(vec3)
	}
	if !l.l4Armed {
		vec4 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL4)*4)
		l.l4Armed = l.isValidVector(vec4)
	}
	if !l.l5Armed {
		vec5 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL5)*4)
		l.l5Armed = l.isValidVector(vec5)
	}
}

// isValidVector applies the AROS profile bound to a candidate handler PC.
// Accepts ROM-resident handlers and RAM vectors above the low vector base
// up to the profile top of RAM. Rejects sentinel zero/all-ones values.
func (l *AROSLoader) isValidVector(pc uint32) bool {
	return isValidAROSVectorBound(pc, l.profile.ROMBase, l.profile.LowVecBase, l.profile.TopOfRAM)
}

// isValidAROSVectorBound applies the explicit AROS profile bounds to a
// candidate handler PC. Pure helper so tests can sweep boundaries without
// constructing a full loader.
func isValidAROSVectorBound(pc, romBase, lowVecBase, topOfRAM uint32) bool {
	if pc == 0 || pc == 0xFFFFFFFF {
		return false
	}
	if pc >= romBase && pc < topOfRAM {
		return true
	}
	return pc >= lowVecBase && pc < topOfRAM
}

// stopTimers stops the timer and vblank goroutines.
func (l *AROSLoader) stopTimers() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	if l.timerDone != nil {
		<-l.timerDone
		l.timerDone = nil
	}
	if l.vblankDone != nil {
		<-l.vblankDone
		l.vblankDone = nil
	}
}

// Stop performs a full shutdown: stops timers.
func (l *AROSLoader) Stop() {
	l.stopTimers()
}

// MapIRQDiagnostics registers read-only MMIO at 0xF23C0-0xF23DF exposing
// M68K CPU interrupt diagnostic counters for freeze investigation scripts.
func (l *AROSLoader) MapIRQDiagnostics() {
	cpu := l.cpu
	l.bus.MapIO(IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END,
		func(addr uint32) uint32 {
			switch addr {
			case IRQ_DIAG_ISR:
				return cpu.interruptInService
			case IRQ_DIAG_FLAGS:
				var flags uint32
				if cpu.stopped.Load() {
					flags |= 1
				}
				if cpu.inException.Load() {
					flags |= 2
				}
				if cpu.AmigaINTENA != nil && cpu.AmigaINTENA.Load() {
					flags |= 4
				}
				if cpu.running.Load() {
					flags |= 8
				}
				return flags
			case IRQ_DIAG_PENDING:
				return cpu.pendingInterrupt.Load()
			case IRQ_DIAG_COUNTERS:
				return (cpu.irqL4Delivered.Load() << 16) | (cpu.irqL5Delivered.Load() & 0xFFFF)
			case IRQ_DIAG_BLOCKED:
				return (cpu.irqL4Blocked.Load() << 16) | (cpu.irqL5Blocked.Load() & 0xFFFF)
			case IRQ_DIAG_RTE:
				return cpu.rteCount.Load()
			case IRQ_DIAG_STOP_SPINS:
				return cpu.stopSpinCount.Load()
			case IRQ_DIAG_WATCHDOG:
				return cpu.stopWatchdogHits.Load()
			}
			return 0
		},
		nil, // read-only
	)
}
