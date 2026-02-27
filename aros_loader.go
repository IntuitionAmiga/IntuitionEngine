package main

import (
	"context"
	"encoding/binary"
	"fmt"
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
	return &AROSLoader{
		bus:       bus,
		cpu:       cpu,
		videoChip: videoChip,
	}
}

// LoadROM loads an AROS ROM image into bus memory with big-endian byte swapping,
// installs reset vectors (SP at address 0, PC at address 4), and resets the CPU.
func (l *AROSLoader) LoadROM(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("AROS ROM too small: %d bytes", len(data))
	}

	base := uint32(arosROMBase)
	end := base + uint32(len(data))
	if end > DEFAULT_MEMORY_SIZE {
		return fmt.Errorf("AROS ROM range 0x%08X-0x%08X outside memory", base, end-1)
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
	l.cpu.stackLowerBound = 0
	l.cpu.stackUpperBound = DEFAULT_MEMORY_SIZE

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
				if l.l5Armed {
					l.cpu.AssertInterrupt(5)
				}
			}
		}
	}()

	// VBL goroutine: 16.667ms tick (60Hz) → Level 4 autovector.
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
		l.l2Armed = isValidAROSVector(vec2)
	}
	if !l.l3Armed {
		vec3 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL3)*4)
		l.l3Armed = isValidAROSVector(vec3)
	}
	if !l.l4Armed {
		vec4 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL4)*4)
		l.l4Armed = isValidAROSVector(vec4)
	}
	if !l.l5Armed {
		vec5 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL5)*4)
		l.l5Armed = isValidAROSVector(vec5)
	}
}

// isValidAROSVector returns true if the given PC value looks like a valid
// interrupt handler address (not zero, not 0xFFFFFFFF, within bus memory).
func isValidAROSVector(pc uint32) bool {
	if pc == 0 || pc == 0xFFFFFFFF {
		return false
	}
	// Accept ROM handler addresses and valid RAM vectors.
	if pc >= arosROMBase && pc < DEFAULT_MEMORY_SIZE {
		return true
	}
	return pc >= 0x00001000 && pc < DEFAULT_MEMORY_SIZE
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
