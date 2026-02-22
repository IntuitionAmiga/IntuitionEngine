package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

const (
	emutosROM192K = 192 * 1024
	emutosROM256K = 256 * 1024
	emutosBase192 = 0xFC0000
	emutosBaseStd = 0xE00000
	emutosBootSP  = 0x00020000 // Above BSS end (MEMBOT), startup.S replaces with _stktop
)

type EmuTOSLoader struct {
	bus       *MachineBus
	cpu       *M68KCPU
	videoChip *VideoChip

	ctx    context.Context
	cancel context.CancelFunc

	timerDone  chan struct{}
	vblankDone chan struct{}

	l4Armed bool
	l5Armed bool

	gemdos *GemdosInterceptor

	// IOREC keyboard buffer addresses extracted from ROM
	iorecBufBase  uint32 // RAM address of IOREC ibuf field
	iorecBufSize  uint32 // RAM address of IOREC ibufsiz field
	iorecReadIdx  uint32 // RAM address of IOREC ibufhd field
	iorecWriteIdx uint32 // RAM address of IOREC ibuftl field
	iorecFixed    bool   // true once we've initialized the IOREC
	iorecDelay    int    // delay ticks after L5 armed before attempting fix
}

func NewEmuTOSLoader(bus *MachineBus, cpu *M68KCPU, videoChip *VideoChip) *EmuTOSLoader {
	return &EmuTOSLoader{
		bus:       bus,
		cpu:       cpu,
		videoChip: videoChip,
	}
}

func emutosROMBase(size int) uint32 {
	switch size {
	case emutosROM192K:
		return emutosBase192
	case emutosROM256K:
		return emutosBaseStd
	default:
		if size >= 512*1024 {
			return emutosBaseStd
		}
		return emutosBaseStd
	}
}

func (l *EmuTOSLoader) LoadROM(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("ROM too small: %d bytes", len(data))
	}
	base := emutosROMBase(len(data))
	end := base + uint32(len(data))
	if end > DEFAULT_MEMORY_SIZE {
		return fmt.Errorf("ROM range 0x%08X-0x%08X outside memory", base, end-1)
	}

	startPage := base >> 8
	endPage := (end - 1) >> 8
	for page := startPage; page <= endPage; page++ {
		if l.bus.ioPageBitmap[page] {
			return fmt.Errorf("ROM page 0x%X overlaps I/O", page)
		}
	}

	for i := 0; i+1 < len(data); i += 2 {
		beWord := binary.BigEndian.Uint16(data[i : i+2])
		l.cpu.Write16(base+uint32(i), beWord)
	}
	if len(data)%2 != 0 {
		l.bus.Write8(base+uint32(len(data)-1), data[len(data)-1])
	}

	// EmuTOS padded ROM images start with a branch opcode, not a valid SSP.
	// Install a deterministic reset stack pointer and keep reset PC from ROM.
	l.cpu.Write32(0, emutosBootSP)
	l.cpu.Write32(M68K_RESET_VECTOR, binary.BigEndian.Uint32(data[4:8]))
	l.cpu.Reset()
	// EmuTOS moves SP throughout boot (initial→_stktop→supervisor→user stacks).
	// Disable the stack bounds check which is designed for simple programs.
	l.cpu.stackLowerBound = 0
	l.cpu.stackUpperBound = DEFAULT_MEMORY_SIZE
	fmt.Printf("EmuTOS vectors: mem[0]=%08X mem[4]=%08X a7=%08X pc=%08X\r\n",
		l.cpu.Read32(0), l.cpu.Read32(M68K_RESET_VECTOR), l.cpu.AddrRegs[7], l.cpu.PC)

	// Scan ROM for the IOREC push function to extract keyboard buffer addresses.
	// The IE machine target may not initialize the IOREC during boot.
	l.scanForIORECPush(data)

	return nil
}

func (l *EmuTOSLoader) StartTimer() {
	l.stopTimers()

	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.timerDone = make(chan struct{})
	l.vblankDone = make(chan struct{})
	l.l4Armed = false
	l.l5Armed = false

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
					l.fixIORECIfNeeded()
				}
				if l.gemdos != nil {
					l.gemdos.PollDrvbits()
				}
			}
		}
	}()

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

func (l *EmuTOSLoader) refreshIRQArming() {
	if l.l4Armed && l.l5Armed {
		return
	}

	// EmuTOS uses autovectors: L4->vector 28 (0x70), L5->vector 29 (0x74).
	// Only assert timer/VBL interrupts once handlers are installed.
	base := l.cpu.VBR
	if !l.l4Armed {
		vec4 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL4)*4)
		l.l4Armed = isValidEmuTOSVector(vec4)
	}
	if !l.l5Armed {
		vec5 := l.cpu.Read32(base + uint32(M68K_VEC_LEVEL5)*4)
		l.l5Armed = isValidEmuTOSVector(vec5)
	}
}

// scanForIORECPush scans the ROM data for the IOREC push function pattern
// and extracts the addresses of the IOREC buffer fields. The function has a
// distinctive opcode sequence:
//
//	3039 AAAA AAAA  MOVE.W (writeIdx),D0
//	5840            ADDQ.W #4,D0
//	B079 BBBB BBBB  CMP.W (bufSize),D0
//	6D02            BLT.S +2
//	4240            CLR.W D0
//	3239 CCCC CCCC  MOVE.W (readIdx),D1
//	B041            CMP.W D1,D0
//	67xx            BEQ.S (rts)
//	2079 DDDD DDDD  MOVEA.L (bufBase),A0
func (l *EmuTOSLoader) scanForIORECPush(data []byte) {
	// Search for the distinctive 4-byte sequence: ADDQ.W #4,D0 + CMP.W (abs.L),D0
	// Bytes: 58 40 B0 79
	for i := 6; i+28 < len(data); i++ {
		if data[i] != 0x58 || data[i+1] != 0x40 || data[i+2] != 0xB0 || data[i+3] != 0x79 {
			continue
		}
		// Verify preceding instruction: MOVE.W (abs.L),D0 → opcode 3039
		if data[i-6] != 0x30 || data[i-5] != 0x39 {
			continue
		}
		// Layout after 5840 B079:
		// +2..+7: B079 XXXX XXXX  (CMP.W abs.L,D0 — 6 bytes)
		// +8..+9: 6D02            (BLT.S +2 — 2 bytes)
		// +10..+11: 4240          (CLR.W D0 — 2 bytes)
		// +12..+17: 3239 XXXX XXXX (MOVE.W abs.L,D1 — 6 bytes)
		// +18..+19: B041          (CMP.W D1,D0 — 2 bytes)
		// +20..+21: 67xx          (BEQ.S — 2 bytes)
		// +22..+27: 2079 XXXX XXXX (MOVEA.L abs.L,A0 — 6 bytes)

		// Verify CLR.W D0 at +10: 42 40
		if data[i+10] != 0x42 || data[i+11] != 0x40 {
			continue
		}
		// Verify MOVE.W (abs.L),D1 at +12: 32 39
		if data[i+12] != 0x32 || data[i+13] != 0x39 {
			continue
		}
		// Verify CMP.W D1,D0 at +18: B0 41
		if data[i+18] != 0xB0 || data[i+19] != 0x41 {
			continue
		}
		// Verify MOVEA.L (abs.L),A0 at +22: 20 79
		if data[i+22] != 0x20 || data[i+23] != 0x79 {
			continue
		}

		// Extract addresses from the instruction operands
		writeIdx := binary.BigEndian.Uint32(data[i-4 : i])    // from MOVE.W before 5840
		bufSize := binary.BigEndian.Uint32(data[i+4 : i+8])   // from CMP.W after B079
		readIdx := binary.BigEndian.Uint32(data[i+14 : i+18]) // from MOVE.W after 3239
		bufBase := binary.BigEndian.Uint32(data[i+24 : i+28]) // from MOVEA.L after 2079

		l.iorecBufBase = bufBase
		l.iorecBufSize = bufSize
		l.iorecReadIdx = readIdx
		l.iorecWriteIdx = writeIdx
		fmt.Printf("EmuTOS IOREC: bufBase=$%06X bufSize=$%06X readIdx=$%06X writeIdx=$%06X\n",
			bufBase, bufSize, readIdx, writeIdx)
		return
	}
	fmt.Println("EmuTOS IOREC: pattern not found in ROM (keyboard buffer init must be in ROM)")
}

// fixIORECIfNeeded checks if the keyboard IOREC buffer is initialized and
// sets it up if not. Called from the timer goroutine after boot progresses
// enough that BSS zeroing is complete (L5 armed).
func (l *EmuTOSLoader) fixIORECIfNeeded() {
	if l.iorecFixed || l.iorecBufBase == 0 {
		return
	}
	// Wait ~1 second after L5 armed (200 ticks at 5ms) to let BIOS init finish
	l.iorecDelay++
	if l.iorecDelay < 200 {
		return
	}
	l.iorecFixed = true

	// Check if the IOREC buffer base pointer is still NULL
	bufPtr := l.cpu.Read32(l.iorecBufBase)
	if bufPtr != 0 {
		return // Already initialized by the ROM
	}

	// Allocate a 256-byte keyboard buffer at $9E000 (top of main RAM, safe location)
	const kbdBufAddr = 0x9E000
	const kbdBufSize = 256

	l.cpu.Write32(l.iorecBufBase, kbdBufAddr)
	l.cpu.Write16(l.iorecBufSize, kbdBufSize)
	l.cpu.Write16(l.iorecReadIdx, 0)
	l.cpu.Write16(l.iorecWriteIdx, 0)

	fmt.Printf("EmuTOS IOREC: initialized keyboard buffer at $%06X (size=%d)\n", kbdBufAddr, kbdBufSize)
}

func isValidEmuTOSVector(pc uint32) bool {
	if pc == 0 || pc == 0xFFFFFFFF {
		return false
	}
	// Accept ROM handler addresses and valid RAM vectors once boot code installs them.
	if pc >= emutosBaseStd && pc < DEFAULT_MEMORY_SIZE {
		return true
	}
	return pc >= 0x00001000 && pc < DEFAULT_MEMORY_SIZE
}

// SetupGemdos creates a GEMDOS interceptor mapping hostPath as the given drive number.
func (l *EmuTOSLoader) SetupGemdos(hostPath string, driveNum uint16) error {
	g, err := NewGemdosInterceptor(l.cpu, l.bus, hostPath, driveNum)
	if err != nil {
		return err
	}
	l.gemdos = g
	l.cpu.gemdosHandler = g
	return nil
}

// stopTimers stops the timer and vblank goroutines without touching the
// GEMDOS interceptor. Used by StartTimer to restart timers cleanly.
func (l *EmuTOSLoader) stopTimers() {
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

// Stop performs a full shutdown: stops timers and closes the GEMDOS interceptor.
func (l *EmuTOSLoader) Stop() {
	l.stopTimers()
	if l.gemdos != nil {
		l.gemdos.Close()
		l.gemdos = nil
	}
}
