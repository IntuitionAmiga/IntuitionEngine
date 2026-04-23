//go:build headless && musashi && m68k_test

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/intuitionamiga/IntuitionEngine/internal/musashi"
)

type m68kSnapshot struct {
	mem []byte
	pc  uint32
	sr  uint16
	d   [8]uint32
	a   [8]uint32
	usp uint32
	ssp uint32
	vbr uint32
	ret uint32
}

const musashiMemSize = 16 * 1024 * 1024

func snapshotAtPC(t *testing.T, target uint32) m68kSnapshot {
	t.Helper()

	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	driveRoot := resolveAROSDrivePath("", filepath.Join(wd, "IntuitionEngine"))
	if !isAROSDrivePath(driveRoot) {
		t.Skip("AROS drive tree not available")
	}

	env, err := NewAROSInterpreterBootEnvironment(rom, driveRoot)
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}
	defer env.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	if result.TimedOut || !result.Ready.Ready || len(result.Faults) != 0 {
		t.Fatalf("bounded boot failed: ready=%+v faults=%+v timeout=%v", result.Ready, result.Faults, result.TimedOut)
	}

	cpu := env.CPU
	cpu.DebugWatchFn = nil
	done := make(chan struct{}, 1)
	var (
		snap m68kSnapshot
		once sync.Once
	)
	cpu.InstructionHook = func(cpu *M68KCPU) {
		if cpu.lastExecPC != target {
			return
		}
		once.Do(func() {
			mem := cpu.bus.GetMemory()
			snap = m68kSnapshot{
				mem: make([]byte, M68K_MEMORY_SIZE),
				pc:  cpu.lastExecPC,
				sr:  cpu.SR,
				d:   cpu.DataRegs,
				a:   cpu.AddrRegs,
				usp: cpu.USP,
				ssp: cpu.SSP,
				vbr: cpu.VBR,
				ret: cpu.Read32(cpu.AddrRegs[7]),
			}
			copy(snap.mem, mem[:len(snap.mem)])
		})
		cpu.SetRunning(false)
		select {
		case done <- struct{}{}:
		default:
		}
	}
	defer func() { cpu.InstructionHook = nil }()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("timed out waiting for PC=%08X", target)
	}
	return snap
}

func snapshotAfterFaultRegionWrite(t *testing.T) m68kSnapshot {
	t.Helper()

	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	driveRoot := resolveAROSDrivePath("", filepath.Join(wd, "IntuitionEngine"))
	if !isAROSDrivePath(driveRoot) {
		t.Skip("AROS drive tree not available")
	}

	env, err := NewAROSInterpreterBootEnvironment(rom, driveRoot)
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}
	defer env.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	if result.TimedOut || !result.Ready.Ready || len(result.Faults) != 0 {
		t.Fatalf("bounded boot failed: ready=%+v faults=%+v timeout=%v", result.Ready, result.Faults, result.TimedOut)
	}

	cpu := env.CPU
	done := make(chan struct{}, 1)
	var (
		snap     m68kSnapshot
		once     sync.Once
		sawWrite bool
	)
	cpu.DebugWatchFn = func(addr, value, pc uint32, size int) {
		if addr < 0x009E3D80 || addr >= 0x009E3E00 {
			return
		}
		sawWrite = true
	}
	cpu.InstructionHook = func(cpu *M68KCPU) {
		if !sawWrite {
			return
		}
		once.Do(func() {
			mem := cpu.bus.GetMemory()
			snap = m68kSnapshot{
				mem: make([]byte, M68K_MEMORY_SIZE),
				pc:  cpu.lastExecPC,
				sr:  cpu.SR,
				d:   cpu.DataRegs,
				a:   cpu.AddrRegs,
				usp: cpu.USP,
				ssp: cpu.SSP,
				vbr: cpu.VBR,
				ret: cpu.Read32(cpu.AddrRegs[7]),
			}
			copy(snap.mem, mem[:len(snap.mem)])
		})
		cpu.SetRunning(false)
		select {
		case done <- struct{}{}:
		default:
		}
	}
	defer func() {
		cpu.DebugWatchFn = nil
		cpu.InstructionHook = nil
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for instruction boundary after fault-region write")
	}
	return snap
}

func newCPUFromSnapshot(s m68kSnapshot) *M68KCPU {
	bus := NewMachineBus()
	mem := bus.GetMemory()
	copy(mem[:len(s.mem)], s.mem)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = false
	cpu.PC = s.pc
	cpu.SR = s.sr
	cpu.DataRegs = s.d
	cpu.AddrRegs = s.a
	cpu.USP = s.usp
	cpu.SSP = s.ssp
	cpu.VBR = s.vbr
	cpu.stackLowerBound = 0
	cpu.stackUpperBound = M68K_MEMORY_SIZE
	cpu.SetRunning(true)
	return cpu
}

func writeMusashiBE16(cpu *musashi.CPU, addr uint32, val uint16) {
	cpu.WriteByte(addr, byte(val>>8))
	cpu.WriteByte(addr+1, byte(val))
}

func newMusashiFromSnapshot(s m68kSnapshot) *musashi.CPU {
	m := musashi.New()
	m.Init()
	m.ClearMem()
	limit := len(s.mem)
	if limit > musashiMemSize {
		limit = musashiMemSize
	}
	for i, b := range s.mem[:limit] {
		m.WriteByte(uint32(i), b)
	}
	for i := range 7 {
		m.SetReg(musashi.RegD0+i, s.d[i])
		m.SetReg(musashi.RegA0+i, s.a[i])
	}
	m.SetReg(musashi.RegD7, s.d[7])
	m.SetReg(musashi.RegPC, s.pc)
	m.SetReg(musashi.RegSR, uint32(s.sr))
	m.SetReg(musashi.RegUSP, s.usp)
	m.SetReg(musashi.RegISP, s.ssp)
	m.SetReg(musashi.RegA7, s.a[7])
	return m
}

func remapWindowForMusashi(s m68kSnapshot, fromStart, toStart, size uint32) m68kSnapshot {
	out := s
	out.mem = make([]byte, len(s.mem))
	copy(out.mem, s.mem)

	copy(out.mem[toStart:toStart+size], s.mem[fromStart:fromStart+size])

	adjust := func(v uint32) uint32 {
		if v >= fromStart && v < fromStart+size {
			return toStart + (v - fromStart)
		}
		return v
	}

	out.pc = adjust(out.pc)
	out.ret = adjust(out.ret)
	out.usp = adjust(out.usp)
	out.ssp = adjust(out.ssp)
	for i := range out.a {
		out.a[i] = adjust(out.a[i])
	}
	return out
}

func makeSnapshotMusashiCompatible(s m68kSnapshot) m68kSnapshot {
	if s.a[7] < musashiMemSize {
		return s
	}
	fromStart := s.a[7] &^ 0xFFF
	if fromStart < 0x1000000 {
		return s
	}
	toStart := fromStart - 0x1000000
	return remapWindowForMusashi(s, fromStart, toStart, 0x4000)
}

func TestAROSMusashi_CallbackSetupRoutineParity(t *testing.T) {
	snap := makeSnapshotMusashiCompatible(snapshotAtPC(t, 0x009E31EC))

	ours := newCPUFromSnapshot(snap)
	musa := newMusashiFromSnapshot(snap)

	// Replace the return site with BRA.S * so both CPUs stop there cleanly.
	ours.Write16(snap.ret, 0x60FE)
	writeMusashiBE16(musa, snap.ret, 0x60FE)

	for i := 0; i < 200000; i++ {
		if ours.PC == snap.ret {
			break
		}
		if ours.StepOne() == 0 {
			t.Fatalf("our CPU halted before return at pc=%08X", ours.PC)
		}
	}
	if ours.PC != snap.ret {
		t.Fatalf("our CPU did not reach return trap, pc=%08X ret=%08X", ours.PC, snap.ret)
	}

	for i := 0; i < 200000; i++ {
		if musa.GetReg(musashi.RegPC) == snap.ret {
			break
		}
		musa.Execute(4)
	}
	if musa.GetReg(musashi.RegPC) != snap.ret {
		t.Fatalf("Musashi did not reach return trap, pc=%08X ret=%08X", musa.GetReg(musashi.RegPC), snap.ret)
	}

	for i := range 8 {
		if ours.DataRegs[i] != musa.GetReg(musashi.RegD0+i) {
			t.Fatalf("D%d mismatch: ours=%08X musashi=%08X", i, ours.DataRegs[i], musa.GetReg(musashi.RegD0+i))
		}
		if ours.AddrRegs[i] != musa.GetReg(musashi.RegA0+i) {
			t.Fatalf("A%d mismatch: ours=%08X musashi=%08X", i, ours.AddrRegs[i], musa.GetReg(musashi.RegA0+i))
		}
	}
	if ours.PC != musa.GetReg(musashi.RegPC) {
		t.Fatalf("PC mismatch: ours=%08X musashi=%08X", ours.PC, musa.GetReg(musashi.RegPC))
	}
	if uint32(ours.SR) != musa.GetReg(musashi.RegSR) {
		t.Fatalf("SR mismatch: ours=%04X musashi=%04X", ours.SR, musa.GetReg(musashi.RegSR))
	}

	check := []uint32{0x009E2FB6, 0x009E3194, 0x009E3208, 0x009E321C}
	for _, addr := range check {
		if ours.Read32(addr) != musa.Read32(addr) {
			t.Fatalf("mem[%08X] mismatch: ours=%08X musashi=%08X", addr, ours.Read32(addr), musa.Read32(addr))
		}
	}
}

func TestAROSMusashi_CallbackSetupStepParity(t *testing.T) {
	snap := makeSnapshotMusashiCompatible(snapshotAtPC(t, 0x009E31EC))
	ours := newCPUFromSnapshot(snap)
	musa := newMusashiFromSnapshot(snap)

	compareCPUs(t, ours, musa, 0)
	for step := 1; step <= 256; step++ {
		if ours.StepOne() == 0 {
			t.Fatalf("step %d our CPU halted at pc=%08X", step, ours.PC)
		}
		oldPC := musa.GetReg(musashi.RegPC)
		if !stepMusashiUntilPCChange(musa, oldPC) {
			t.Fatalf("step %d Musashi did not advance from pc=%08X", step, oldPC)
		}
		compareCPUs(t, ours, musa, step)
	}
}

func stepMusashiUntilPCChange(cpu *musashi.CPU, oldPC uint32) bool {
	for range 64 {
		cpu.Execute(1)
		if cpu.GetReg(musashi.RegPC) != oldPC {
			return true
		}
	}
	return false
}

func compareCPUs(t *testing.T, ours *M68KCPU, musa *musashi.CPU, step int) {
	t.Helper()

	if ours.PC != musa.GetReg(musashi.RegPC) {
		t.Fatalf("step %d PC mismatch: ours=%08X musashi=%08X", step, ours.PC, musa.GetReg(musashi.RegPC))
	}
	if uint32(ours.SR) != musa.GetReg(musashi.RegSR) {
		t.Fatalf("step %d SR mismatch: ours=%04X musashi=%04X", step, ours.SR, musa.GetReg(musashi.RegSR))
	}
	for i := range 7 {
		if ours.DataRegs[i] != musa.GetReg(musashi.RegD0+i) {
			t.Fatalf("step %d D%d mismatch: ours=%08X musashi=%08X", step, i, ours.DataRegs[i], musa.GetReg(musashi.RegD0+i))
		}
		if ours.AddrRegs[i] != musa.GetReg(musashi.RegA0+i) {
			t.Fatalf("step %d A%d mismatch: ours=%08X musashi=%08X", step, i, ours.AddrRegs[i], musa.GetReg(musashi.RegA0+i))
		}
	}
	if ours.DataRegs[7] != musa.GetReg(musashi.RegD7) {
		t.Fatalf("step %d D7 mismatch: ours=%08X musashi=%08X", step, ours.DataRegs[7], musa.GetReg(musashi.RegD7))
	}
	if ours.AddrRegs[7] != musa.GetReg(musashi.RegSP) {
		t.Fatalf("step %d SP mismatch: ours=%08X musashi=%08X", step, ours.AddrRegs[7], musa.GetReg(musashi.RegSP))
	}
}

func TestAROSMusashi_DestructiveRoutineParity(t *testing.T) {
	targets := []uint32{0x00BFCC20, 0x00BFF7D4}
	for _, target := range targets {
		t.Run(fmt.Sprintf("pc_%08x", target), func(t *testing.T) {
			snap := snapshotAtPC(t, target)
			snap = makeSnapshotMusashiCompatible(snap)
			ours := newCPUFromSnapshot(snap)
			musa := newMusashiFromSnapshot(snap)

			compareCPUs(t, ours, musa, 0)
			for step := 1; step <= 256; step++ {
				if ours.StepOne() == 0 {
					t.Fatalf("step %d our CPU halted at pc=%08X", step, ours.PC)
				}
				oldPC := musa.GetReg(musashi.RegPC)
				if !stepMusashiUntilPCChange(musa, oldPC) {
					t.Fatalf("step %d Musashi did not advance from pc=%08X", step, oldPC)
				}
				compareCPUs(t, ours, musa, step)
			}
		})
	}
}

func TestAROSMusashi_SupervisorGlueStepParity(t *testing.T) {
	targets := []uint32{0x0064E460, 0x0064E4CC, 0x00607744, 0x00607776}
	for _, target := range targets {
		t.Run(fmt.Sprintf("pc_%08x", target), func(t *testing.T) {
			snap := snapshotAtPC(t, target)
			snap = makeSnapshotMusashiCompatible(snap)
			ours := newCPUFromSnapshot(snap)
			musa := newMusashiFromSnapshot(snap)

			compareCPUs(t, ours, musa, 0)
			for step := 1; step <= 128; step++ {
				if ours.StepOne() == 0 {
					t.Fatalf("step %d our CPU halted at pc=%08X", step, ours.PC)
				}
				oldPC := musa.GetReg(musashi.RegPC)
				if !stepMusashiUntilPCChange(musa, oldPC) {
					t.Fatalf("step %d Musashi did not advance from pc=%08X", step, oldPC)
				}
				compareCPUs(t, ours, musa, step)
			}
		})
	}
}

func TestAROSMusashi_SupervisorGlueTrace0064E460(t *testing.T) {
	snap := makeSnapshotMusashiCompatible(snapshotAtPC(t, 0x0064E460))
	ours := newCPUFromSnapshot(snap)
	musa := newMusashiFromSnapshot(snap)

	for step := 0; step <= 32; step++ {
		ourPC := ours.PC
		musaPC := musa.GetReg(musashi.RegPC)
		ourSP := ours.AddrRegs[7]
		musaSP := musa.GetReg(musashi.RegSP)
		t.Logf("step=%d ours_pc=%08X musa_pc=%08X ours_op=%04X musa_op=%04X ours_d0=%08X ours_d1=%08X ours_d2=%08X ours_a0=%08X ours_sp=%08X musa_d0=%08X musa_d1=%08X musa_d2=%08X musa_a0=%08X musa_sp=%08X",
			step,
			ourPC,
			musaPC,
			ours.Read16(ourPC),
			uint16(musa.Read32(musaPC)>>16),
			ours.DataRegs[0],
			ours.DataRegs[1],
			ours.DataRegs[2],
			ours.AddrRegs[0],
			ourSP,
			musa.GetReg(musashi.RegD0),
			musa.GetReg(musashi.RegD1),
			musa.GetReg(musashi.RegD2),
			musa.GetReg(musashi.RegA0),
			musaSP,
		)
		if step >= 23 && step <= 25 {
			t.Logf("step=%d stack ours=[%08X %08X %08X %08X] musa=[%08X %08X %08X %08X]",
				step,
				ours.Read32(ourSP),
				ours.Read32(ourSP+4),
				ours.Read32(ourSP+8),
				ours.Read32(ourSP+12),
				musa.Read32(musaSP),
				musa.Read32(musaSP+4),
				musa.Read32(musaSP+8),
				musa.Read32(musaSP+12),
			)
		}
		if ours.PC != musaPC ||
			uint32(ours.SR) != musa.GetReg(musashi.RegSR) ||
			ours.DataRegs[0] != musa.GetReg(musashi.RegD0) ||
			ours.DataRegs[1] != musa.GetReg(musashi.RegD1) ||
			ours.DataRegs[2] != musa.GetReg(musashi.RegD2) ||
			ours.AddrRegs[0] != musa.GetReg(musashi.RegA0) ||
			ours.AddrRegs[7] != musa.GetReg(musashi.RegSP) {
			t.Fatalf("mismatch at step %d", step)
		}
		if step == 32 {
			break
		}
		if ours.StepOne() == 0 {
			t.Fatalf("our CPU halted at step %d pc=%08X", step+1, ours.PC)
		}
		oldPC := musa.GetReg(musashi.RegPC)
		if !stepMusashiUntilPCChange(musa, oldPC) {
			t.Fatalf("Musashi did not advance at step %d from pc=%08X", step+1, oldPC)
		}
	}
}

func TestAROSMusashi_Trace00BFCFC8(t *testing.T) {
	snap := makeSnapshotMusashiCompatible(snapshotAtPC(t, 0x00BFCFC8))
	ours := newCPUFromSnapshot(snap)
	musa := newMusashiFromSnapshot(snap)

	for step := 0; step <= 24; step++ {
		ourPC := ours.PC
		musaPC := musa.GetReg(musashi.RegPC)
		t.Logf("step=%d ours_pc=%08X musa_pc=%08X ours_op=%04X musa_op=%04X d0=%08X d1=%08X d2=%08X d3=%08X a0=%08X a1=%08X a2=%08X a3=%08X sp=%08X",
			step,
			ourPC,
			musaPC,
			ours.Read16(ourPC),
			uint16(musa.Read32(musaPC)>>16),
			ours.DataRegs[0],
			ours.DataRegs[1],
			ours.DataRegs[2],
			ours.DataRegs[3],
			ours.AddrRegs[0],
			ours.AddrRegs[1],
			ours.AddrRegs[2],
			ours.AddrRegs[3],
			ours.AddrRegs[7],
		)
		if ours.PC != musaPC ||
			uint32(ours.SR) != musa.GetReg(musashi.RegSR) ||
			ours.DataRegs[0] != musa.GetReg(musashi.RegD0) ||
			ours.DataRegs[1] != musa.GetReg(musashi.RegD1) ||
			ours.DataRegs[2] != musa.GetReg(musashi.RegD2) ||
			ours.DataRegs[3] != musa.GetReg(musashi.RegD3) ||
			ours.AddrRegs[0] != musa.GetReg(musashi.RegA0) ||
			ours.AddrRegs[1] != musa.GetReg(musashi.RegA1) ||
			ours.AddrRegs[2] != musa.GetReg(musashi.RegA2) ||
			ours.AddrRegs[3] != musa.GetReg(musashi.RegA3) ||
			ours.AddrRegs[7] != musa.GetReg(musashi.RegSP) {
			t.Fatalf("mismatch at step %d", step)
		}
		if step == 24 {
			break
		}
		if ours.StepOne() == 0 {
			t.Fatalf("our CPU halted at step %d pc=%08X", step+1, ours.PC)
		}
		oldPC := musa.GetReg(musashi.RegPC)
		if !stepMusashiUntilPCChange(musa, oldPC) {
			t.Fatalf("Musashi did not advance at step %d from pc=%08X", step+1, oldPC)
		}
	}
}

func TestAROSMusashi_Trace00BFC508(t *testing.T) {
	snap := makeSnapshotMusashiCompatible(snapshotAtPC(t, 0x00BFC508))
	ours := newCPUFromSnapshot(snap)
	musa := newMusashiFromSnapshot(snap)

	for step := 0; step <= 24; step++ {
		ourPC := ours.PC
		musaPC := musa.GetReg(musashi.RegPC)
		t.Logf("step=%d ours_pc=%08X musa_pc=%08X ours_op=%04X musa_op=%04X d0=%08X d1=%08X d2=%08X d3=%08X a0=%08X a1=%08X a2=%08X a3=%08X sp=%08X",
			step,
			ourPC,
			musaPC,
			ours.Read16(ourPC),
			uint16(musa.Read32(musaPC)>>16),
			ours.DataRegs[0],
			ours.DataRegs[1],
			ours.DataRegs[2],
			ours.DataRegs[3],
			ours.AddrRegs[0],
			ours.AddrRegs[1],
			ours.AddrRegs[2],
			ours.AddrRegs[3],
			ours.AddrRegs[7],
		)
		if ours.PC != musaPC ||
			uint32(ours.SR) != musa.GetReg(musashi.RegSR) ||
			ours.DataRegs[0] != musa.GetReg(musashi.RegD0) ||
			ours.DataRegs[1] != musa.GetReg(musashi.RegD1) ||
			ours.DataRegs[2] != musa.GetReg(musashi.RegD2) ||
			ours.DataRegs[3] != musa.GetReg(musashi.RegD3) ||
			ours.AddrRegs[0] != musa.GetReg(musashi.RegA0) ||
			ours.AddrRegs[1] != musa.GetReg(musashi.RegA1) ||
			ours.AddrRegs[2] != musa.GetReg(musashi.RegA2) ||
			ours.AddrRegs[3] != musa.GetReg(musashi.RegA3) ||
			ours.AddrRegs[7] != musa.GetReg(musashi.RegSP) {
			t.Fatalf("mismatch at step %d", step)
		}
		if step == 24 {
			break
		}
		if ours.StepOne() == 0 {
			t.Fatalf("our CPU halted at step %d pc=%08X", step+1, ours.PC)
		}
		oldPC := musa.GetReg(musashi.RegPC)
		if !stepMusashiUntilPCChange(musa, oldPC) {
			t.Fatalf("Musashi did not advance at step %d from pc=%08X", step+1, oldPC)
		}
	}
}

func TestAROSMusashi_Trace00BFCA80(t *testing.T) {
	snap := makeSnapshotMusashiCompatible(snapshotAtPC(t, 0x00BFCA80))
	ours := newCPUFromSnapshot(snap)
	musa := newMusashiFromSnapshot(snap)

	for step := 0; step <= 32; step++ {
		ourPC := ours.PC
		musaPC := musa.GetReg(musashi.RegPC)
		t.Logf("step=%d ours_pc=%08X musa_pc=%08X ours_op=%04X musa_op=%04X d0=%08X d1=%08X d2=%08X d3=%08X a0=%08X a1=%08X a2=%08X a3=%08X sp=%08X mem3d80=%08X",
			step,
			ourPC,
			musaPC,
			ours.Read16(ourPC),
			uint16(musa.Read32(musaPC)>>16),
			ours.DataRegs[0],
			ours.DataRegs[1],
			ours.DataRegs[2],
			ours.DataRegs[3],
			ours.AddrRegs[0],
			ours.AddrRegs[1],
			ours.AddrRegs[2],
			ours.AddrRegs[3],
			ours.AddrRegs[7],
			ours.Read32(0x009E3D80),
		)
		if ours.PC != musaPC ||
			uint32(ours.SR) != musa.GetReg(musashi.RegSR) ||
			ours.DataRegs[0] != musa.GetReg(musashi.RegD0) ||
			ours.DataRegs[1] != musa.GetReg(musashi.RegD1) ||
			ours.DataRegs[2] != musa.GetReg(musashi.RegD2) ||
			ours.DataRegs[3] != musa.GetReg(musashi.RegD3) ||
			ours.AddrRegs[0] != musa.GetReg(musashi.RegA0) ||
			ours.AddrRegs[1] != musa.GetReg(musashi.RegA1) ||
			ours.AddrRegs[2] != musa.GetReg(musashi.RegA2) ||
			ours.AddrRegs[3] != musa.GetReg(musashi.RegA3) ||
			ours.AddrRegs[7] != musa.GetReg(musashi.RegSP) ||
			ours.Read32(0x009E3D80) != musa.Read32(0x009E3D80) {
			t.Fatalf("mismatch at step %d", step)
		}
		if step == 32 {
			break
		}
		if ours.StepOne() == 0 {
			t.Fatalf("our CPU halted at step %d pc=%08X", step+1, ours.PC)
		}
		oldPC := musa.GetReg(musashi.RegPC)
		if !stepMusashiUntilPCChange(musa, oldPC) {
			t.Fatalf("Musashi did not advance at step %d from pc=%08X", step+1, oldPC)
		}
	}
}

func TestAROSMusashi_PostFaultRegionWriteParity(t *testing.T) {
	snap := makeSnapshotMusashiCompatible(snapshotAfterFaultRegionWrite(t))
	ours := newCPUFromSnapshot(snap)
	musa := newMusashiFromSnapshot(snap)

	for step := 0; step <= 64; step++ {
		t.Logf("step=%d pc=%08X op=%04X ext=%04X d0=%08X d1=%08X d2=%08X d3=%08X a0=%08X a1=%08X a2=%08X a3=%08X sp=%08X mem3d80=%08X",
			step, ours.PC, ours.Read16(ours.PC), ours.Read16(ours.PC+2), ours.DataRegs[0], ours.DataRegs[1], ours.DataRegs[2], ours.DataRegs[3],
			ours.AddrRegs[0], ours.AddrRegs[1], ours.AddrRegs[2], ours.AddrRegs[3], ours.AddrRegs[7], ours.Read32(0x009E3D80))
		compareCPUs(t, ours, musa, step)
		if ours.Read32(0x009E3D80) != musa.Read32(0x009E3D80) {
			t.Fatalf("step %d mem3d80 mismatch: ours=%08X musashi=%08X",
				step, ours.Read32(0x009E3D80), musa.Read32(0x009E3D80))
		}
		if step == 64 {
			break
		}
		if ours.StepOne() == 0 {
			t.Fatalf("our CPU halted at step %d pc=%08X", step+1, ours.PC)
		}
		oldPC := musa.GetReg(musashi.RegPC)
		if !stepMusashiUntilPCChange(musa, oldPC) {
			t.Fatalf("Musashi did not advance at step %d from pc=%08X", step+1, oldPC)
		}
	}
}

func TestMOVEMPostincrement_A3FromMemory_Parity(t *testing.T) {
	bus := NewMachineBus()
	ours := NewM68KCPU(bus)
	ours.m68kJitEnabled = false
	ours.PC = M68K_ENTRY_POINT
	ours.SR = M68K_SR_S | 0x0700
	ours.AddrRegs[7] = 0x00004000
	ours.AddrRegs[2] = 0x00002000
	ours.Write16(0x00002000, 0x1234)
	ours.Write16(0x00002002, 0x5678)
	ours.Write16(ours.PC, 0x4C1A)
	ours.Write16(ours.PC+2, 0x0800)

	musa := musashi.New()
	musa.Init()
	musa.ClearMem()
	writeMusashiBE32(musa, 0, 0x00004000)
	writeMusashiBE32(musa, 4, M68K_ENTRY_POINT)
	writeMusashiBE16(musa, M68K_ENTRY_POINT, 0x4C1A)
	writeMusashiBE16(musa, M68K_ENTRY_POINT+2, 0x0800)
	writeMusashiBE16(musa, 0x00002000, 0x1234)
	writeMusashiBE16(musa, 0x00002002, 0x5678)
	musa.Reset()
	musa.SetReg(musashi.RegSR, uint32(M68K_SR_S|0x0700))
	musa.SetReg(musashi.RegA2, 0x00002000)
	musa.SetReg(musashi.RegA7, 0x00004000)
	musa.SetReg(musashi.RegUSP, 0x00004000)
	musa.SetReg(musashi.RegISP, 0x00004000)

	if ours.StepOne() == 0 {
		t.Fatalf("our CPU halted at pc=%08X", ours.PC)
	}
	oldPC := musa.GetReg(musashi.RegPC)
	if !stepMusashiUntilPCChange(musa, oldPC) {
		t.Fatalf("Musashi did not advance from pc=%08X", oldPC)
	}
	compareCPUs(t, ours, musa, 1)
}
