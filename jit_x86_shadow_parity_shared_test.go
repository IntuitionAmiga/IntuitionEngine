// jit_x86_shadow_parity_shared_test.go - deterministic x86 workload parity harness.
//
// Drives identical workload bytes through the interpreter and force-native JIT
// paths in fixed guest-instruction windows. Each checkpoint compares canonical
// CPU and x87 state, guest backing memory, and a deterministic video-device
// state hash derived from a bus-mapped VideoChip fixture with a guest-cycle-
// keyed VBlank schedule.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"time"
)

const (
	x86ShadowCheckpointInstrs = 50_000
	x86ShadowCheckpoints      = 4
	x86ShadowVBlankHalfPeriod = 256
)

type x86ShadowFixture struct {
	cpu   *CPU_X86
	video *VideoChip
}

type x86ShadowTB interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Skip(args ...any)
	Skipf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
}

var x86ShadowWaitVSyncPattern = []byte{
	0xA1, 0x08, 0x00, 0x0F, 0x00,
	0xA9, 0x02, 0x00, 0x00, 0x00,
	0x75, 0xF4,
	0xA1, 0x08, 0x00, 0x0F, 0x00,
	0xA9, 0x02, 0x00, 0x00, 0x00,
	0x74, 0xF4,
	0xC3,
}

// x86CanonicalStateHash returns a SHA-256 over the canonical guest register
// byte image: GP regs in encoding order, segment regs in encoding order, EIP,
// Flags, cycles and complete x87 state. Order is fixed so any future field
// must be appended, never inserted, to keep snapshots stable.
func x86CanonicalStateHash(cpu *CPU_X86) (regBytes [160]byte, sum [32]byte) {
	binary.LittleEndian.PutUint32(regBytes[0:], cpu.EAX)
	binary.LittleEndian.PutUint32(regBytes[4:], cpu.ECX)
	binary.LittleEndian.PutUint32(regBytes[8:], cpu.EDX)
	binary.LittleEndian.PutUint32(regBytes[12:], cpu.EBX)
	binary.LittleEndian.PutUint32(regBytes[16:], cpu.ESP)
	binary.LittleEndian.PutUint32(regBytes[20:], cpu.EBP)
	binary.LittleEndian.PutUint32(regBytes[24:], cpu.ESI)
	binary.LittleEndian.PutUint32(regBytes[28:], cpu.EDI)
	binary.LittleEndian.PutUint16(regBytes[32:], cpu.ES)
	binary.LittleEndian.PutUint16(regBytes[34:], cpu.CS)
	binary.LittleEndian.PutUint16(regBytes[36:], cpu.SS)
	binary.LittleEndian.PutUint16(regBytes[38:], cpu.DS)
	binary.LittleEndian.PutUint16(regBytes[40:], cpu.FS)
	binary.LittleEndian.PutUint16(regBytes[42:], cpu.GS)
	binary.LittleEndian.PutUint32(regBytes[44:], cpu.EIP)
	binary.LittleEndian.PutUint32(regBytes[48:], cpu.Flags)
	binary.LittleEndian.PutUint64(regBytes[52:], cpu.Cycles)
	if cpu.FPU != nil {
		const fpuOff = 60
		for i, value := range cpu.FPU.regs {
			binary.LittleEndian.PutUint64(regBytes[fpuOff+i*8:], math.Float64bits(value))
		}
		binary.LittleEndian.PutUint16(regBytes[124:], cpu.FPU.FCW)
		binary.LittleEndian.PutUint16(regBytes[126:], cpu.FPU.FSW)
		binary.LittleEndian.PutUint16(regBytes[128:], cpu.FPU.FTW)
		binary.LittleEndian.PutUint32(regBytes[130:], cpu.FPU.FIP)
		binary.LittleEndian.PutUint16(regBytes[134:], cpu.FPU.FCS)
		binary.LittleEndian.PutUint32(regBytes[136:], cpu.FPU.FDP)
		binary.LittleEndian.PutUint16(regBytes[140:], cpu.FPU.FDS)
		binary.LittleEndian.PutUint16(regBytes[142:], cpu.FPU.FOP)
	}
	sum = sha256.Sum256(regBytes[:])
	return
}

func x86GuestMemoryHash(cpu *CPU_X86) [32]byte {
	return sha256.Sum256(cpu.memory)
}

func x86CanonicalByteName(off int) string {
	switch {
	case off >= 0 && off < 4:
		return "EAX"
	case off >= 4 && off < 8:
		return "ECX"
	case off >= 8 && off < 12:
		return "EDX"
	case off >= 12 && off < 16:
		return "EBX"
	case off >= 16 && off < 20:
		return "ESP"
	case off >= 20 && off < 24:
		return "EBP"
	case off >= 24 && off < 28:
		return "ESI"
	case off >= 28 && off < 32:
		return "EDI"
	case off >= 32 && off < 34:
		return "ES"
	case off >= 34 && off < 36:
		return "CS"
	case off >= 36 && off < 38:
		return "SS"
	case off >= 38 && off < 40:
		return "DS"
	case off >= 40 && off < 42:
		return "FS"
	case off >= 42 && off < 44:
		return "GS"
	case off >= 44 && off < 48:
		return "EIP"
	case off >= 48 && off < 52:
		return "Flags"
	case off >= 52 && off < 60:
		return "Cycles"
	case off >= 60 && off < 124:
		return "FPU.regs"
	case off >= 124 && off < 126:
		return "FPU.FCW"
	case off >= 126 && off < 128:
		return "FPU.FSW"
	case off >= 128 && off < 130:
		return "FPU.FTW"
	case off >= 130 && off < 134:
		return "FPU.FIP"
	case off >= 134 && off < 136:
		return "FPU.FCS"
	case off >= 136 && off < 140:
		return "FPU.FDP"
	case off >= 140 && off < 142:
		return "FPU.FDS"
	case off >= 142 && off < 144:
		return "FPU.FOP"
	default:
		return "unknown"
	}
}

func x86ShadowVideoHashes(t x86ShadowTB, video *VideoChip) ([32]byte, [32]byte) {
	t.Helper()
	frame := video.FinishFrame()
	frameSum := sha256.Sum256(frame)
	version, snapshot, err := video.DebugSnapshot()
	if err != nil {
		t.Fatalf("video snapshot: %v", err)
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], version)
	devicePayload := append(header[:], snapshot...)
	deviceSum := sha256.Sum256(devicePayload)
	return frameSum, deviceSum
}

func x86ShadowVBlankForCycles(cycles uint64) bool {
	if x86ShadowVBlankHalfPeriod == 0 {
		return false
	}
	return (cycles/uint64(x86ShadowVBlankHalfPeriod))%2 != 0
}

func x86ShadowPatternVBlank(cpu *CPU_X86) (bool, bool) {
	pc := cpu.EIP
	mem := cpu.memory
	if pc > uint32(len(mem)) || len(mem)-int(pc) < 12 {
		return false, false
	}
	window := mem[pc : pc+12]
	if binary.LittleEndian.Uint32(window[1:5]) != VIDEO_STATUS ||
		window[5] != 0xA9 || binary.LittleEndian.Uint32(window[6:10]) != 0x00000002 ||
		window[11] != 0xF4 {
		return false, false
	}
	switch window[10] {
	case 0x75:
		return false, true
	case 0x74:
		return true, true
	default:
		return false, false
	}
}

func x86ShadowNormaliseDisplayWaits(rom []byte) []byte {
	patched := append([]byte(nil), rom...)
	searchFrom := 0
	for {
		off := bytes.Index(patched[searchFrom:], x86ShadowWaitVSyncPattern)
		if off < 0 {
			return patched
		}
		off += searchFrom
		patched[off] = 0xC3
		for i := 1; i < len(x86ShadowWaitVSyncPattern); i++ {
			patched[off+i] = 0x90
		}
		searchFrom = off + len(x86ShadowWaitVSyncPattern)
	}
}

func newX86ShadowFixture(t x86ShadowTB, rom []byte, forceNative, largeBus bool) *x86ShadowFixture {
	t.Helper()
	var (
		bus *MachineBus
		err error
	)
	if largeBus {
		bus, err = NewMachineBusSized(256 * 1024 * 1024)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		bus = NewMachineBus()
	}
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	if err := video.Stop(); err != nil {
		t.Fatalf("VideoChip.Stop: %v", err)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(false)

	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.EIP = 0
	cpu.ESP = 0xFFF0
	cpu.x86JitEnabled = forceNative

	readVideo := func(addr uint32) uint32 {
		if addr == VIDEO_STATUS {
			if vblank, ok := x86ShadowPatternVBlank(cpu); ok {
				video.setVBlank(vblank)
			} else {
				video.setVBlank(x86ShadowVBlankForCycles(cpu.Cycles))
			}
		}
		return video.HandleRead(addr)
	}
	writeVideo := func(addr uint32, value uint32) {
		video.HandleWrite(addr, value)
		if addr == BLT_CTRL && value&bltCtrlStart != 0 {
			video.mu.Lock()
			mode := VideoModes[video.currentMode]
			video.runBlitterLocked(mode)
			video.mu.Unlock()
		}
	}
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, readVideo, writeVideo)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)

	if len(rom) > len(cpu.memory) {
		t.Fatalf("workload too large for test bus: %d > %d", len(rom), len(cpu.memory))
	}
	copy(cpu.memory, rom)
	return &x86ShadowFixture{cpu: cpu, video: video}
}

// x86ShadowStepBudget runs the given CPU forward by exactly budget guest
// instructions, or until the CPU halts. The caller captures snapshots after
// return.
func x86ShadowStepBudget(t x86ShadowTB, cpu *CPU_X86, forceNative bool, budget int64, deadline time.Duration) {
	t.Helper()
	cpu.x86InstrBudget = budget
	cpu.x86BudgetActive = true
	cpu.running.Store(true)
	cpu.Halted = false
	done := make(chan struct{})
	go func() {
		if forceNative {
			cpu.X86ExecuteJIT()
		} else {
			cpu.x86RunInterpreter()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(deadline):
		cpu.running.Store(false)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit after deadline + stop")
		}
		t.Fatalf("budget run did not complete within deadline %v (forceNative=%v)", deadline, forceNative)
	}
}

func x86ShadowParityCheckpointsBytes(t x86ShadowTB, workload string, rom []byte, largeBus bool) {
	t.Helper()
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	rom = x86ShadowNormaliseDisplayWaits(rom)
	interp := newX86ShadowFixture(t, rom, false, largeBus)
	jit := newX86ShadowFixture(t, rom, true, largeBus)
	deadline := 30 * time.Second

	for cp := 1; cp <= x86ShadowCheckpoints; cp++ {
		x86ShadowStepBudget(t, interp.cpu, false, x86ShadowCheckpointInstrs, deadline)
		x86ShadowStepBudget(t, jit.cpu, true, x86ShadowCheckpointInstrs, deadline)

		interpBytes, interpRegs := x86CanonicalStateHash(interp.cpu)
		jitBytes, jitRegs := x86CanonicalStateHash(jit.cpu)
		interpMem := x86GuestMemoryHash(interp.cpu)
		jitMem := x86GuestMemoryHash(jit.cpu)
		interpFrame, interpDevice := x86ShadowVideoHashes(t, interp.video)
		jitFrame, jitDevice := x86ShadowVideoHashes(t, jit.video)

		if interpRegs != jitRegs {
			t.Errorf("%s checkpoint %d: register-state SHA mismatch", workload, cp)
			for i := range interpBytes {
				if interpBytes[i] != jitBytes[i] {
					t.Errorf("  first diff at canonical-image byte %d (%s): interp=%02X jit=%02X", i, x86CanonicalByteName(i), interpBytes[i], jitBytes[i])
					break
				}
			}
			t.Errorf("  interp regs: EIP=%08X EAX=%08X ECX=%08X EDX=%08X EBX=%08X ESP=%08X EBP=%08X ESI=%08X EDI=%08X Flags=%08X Cycles=%d",
				interp.cpu.EIP, interp.cpu.EAX, interp.cpu.ECX, interp.cpu.EDX, interp.cpu.EBX, interp.cpu.ESP, interp.cpu.EBP, interp.cpu.ESI, interp.cpu.EDI, interp.cpu.Flags, interp.cpu.Cycles)
			t.Errorf("  jit    regs: EIP=%08X EAX=%08X ECX=%08X EDX=%08X EBX=%08X ESP=%08X EBP=%08X ESI=%08X EDI=%08X Flags=%08X Cycles=%d",
				jit.cpu.EIP, jit.cpu.EAX, jit.cpu.ECX, jit.cpu.EDX, jit.cpu.EBX, jit.cpu.ESP, jit.cpu.EBP, jit.cpu.ESI, jit.cpu.EDI, jit.cpu.Flags, jit.cpu.Cycles)
			t.Errorf("  interp: %s", hex.EncodeToString(interpRegs[:]))
			t.Errorf("  jit:    %s", hex.EncodeToString(jitRegs[:]))
			if detail, ok := x86ShadowLocateFirstDivergence(rom, largeBus, int64(cp)*x86ShadowCheckpointInstrs); ok {
				t.Errorf("  first divergent instruction: %s", detail)
			}
			return
		}
		if interpMem != jitMem {
			t.Errorf("%s checkpoint %d: guest-memory SHA mismatch", workload, cp)
			t.Errorf("  interp: %s", hex.EncodeToString(interpMem[:]))
			t.Errorf("  jit:    %s", hex.EncodeToString(jitMem[:]))
			return
		}
		if interpFrame != jitFrame {
			t.Errorf("%s checkpoint %d: framebuffer SHA mismatch", workload, cp)
			t.Errorf("  interp: %s", hex.EncodeToString(interpFrame[:]))
			t.Errorf("  jit:    %s", hex.EncodeToString(jitFrame[:]))
			return
		}
		if interpDevice != jitDevice {
			t.Errorf("%s checkpoint %d: video-device SHA mismatch", workload, cp)
			t.Errorf("  interp: %s", hex.EncodeToString(interpDevice[:]))
			t.Errorf("  jit:    %s", hex.EncodeToString(jitDevice[:]))
			return
		}
		t.Logf("%s checkpoint %d ok: reg=%s mem=%s frame=%s EIP=%08X Cycles=%d",
			workload, cp,
			hex.EncodeToString(interpRegs[:8]),
			hex.EncodeToString(interpMem[:8]),
			hex.EncodeToString(interpFrame[:8]),
			jit.cpu.EIP, jit.cpu.Cycles)
	}
}

func x86ShadowLocateFirstDivergence(rom []byte, largeBus bool, limit int64) (string, bool) {
	interp := newX86ShadowFixture(noopTB{}, rom, false, largeBus)
	jit := newX86ShadowFixture(noopTB{}, rom, true, largeBus)
	for step := int64(1); step <= limit; step++ {
		x86ShadowStepBudget(noopTB{}, interp.cpu, false, 1, 5*time.Second)
		x86ShadowStepBudget(noopTB{}, jit.cpu, true, 1, 5*time.Second)
		if ib, is := x86CanonicalStateHash(interp.cpu); ib != [160]byte{} {
			_, js := x86CanonicalStateHash(jit.cpu)
			if is != js {
				first := -1
				jb, _ := x86CanonicalStateHash(jit.cpu)
				for i := range ib {
					if ib[i] != jb[i] {
						first = i
						break
					}
				}
				return fmt.Sprintf("step=%d eip=%08X field=%s interp=%02X jit=%02X", step, interp.cpu.EIP, x86CanonicalByteName(first), ib[first], jb[first]), true
			}
		}
	}
	return "", false
}

func x86ShadowNextBytes(cpu *CPU_X86) []byte {
	const n = 8
	if cpu.EIP >= uint32(len(cpu.memory)) {
		return nil
	}
	end := int(cpu.EIP) + n
	if end > len(cpu.memory) {
		end = len(cpu.memory)
	}
	return append([]byte(nil), cpu.memory[cpu.EIP:end]...)
}

type noopTB struct{}

func (noopTB) Error(args ...any)                 { panic(fmt.Sprint(args...)) }
func (noopTB) Errorf(format string, args ...any) { panic(fmt.Sprintf(format, args...)) }
func (noopTB) Fatal(args ...any)                 { panic(fmt.Sprint(args...)) }
func (noopTB) Fatalf(format string, args ...any) { panic(fmt.Sprintf(format, args...)) }
func (noopTB) Helper()                           {}
func (noopTB) Logf(format string, args ...any)   {}
func (noopTB) Skip(args ...any)                  { panic(fmt.Sprint(args...)) }
func (noopTB) Skipf(format string, args ...any)  { panic(fmt.Sprintf(format, args...)) }
