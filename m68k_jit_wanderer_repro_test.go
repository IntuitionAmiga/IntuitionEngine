//go:build headless && m68k_test && amd64

package main

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

// TestM68KJIT_AROSWandererBootRepro boots AROS with the M68K JIT (no native
// ceiling) and a live video compositor — the configuration `go run . -aros`
// uses — and runs until the Wanderer desktop paints a non-black frame or the
// CPU faults. It reproduces, headlessly, the JIT-only desktop bring-up crash
// (interpreter boots to Wanderer; JIT faults during Wanderer launch).
// Gate: IE_DIAG_WANDERER=1.
func TestM68KJIT_AROSWandererBootRepro(t *testing.T) {
	if os.Getenv("IE_DIAG_WANDERER") != "1" {
		t.Skip("set IE_DIAG_WANDERER=1 to run the JIT Wanderer-boot reproduction")
	}
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	env, err := NewAROSBootEnvironmentWithOptions(rom, isolatedAROSDriveRoot(t, "wanderer"),
		AROSBootEnvironmentOptions{NoNativeCeiling: true,
			DeterministicIRQs: os.Getenv("IE_DIAG_WANDERER_DETIRQ") == "1"})
	if err != nil {
		t.Fatalf("new AROS boot environment failed: %v", err)
	}
	defer env.Close()

	env.CPU.m68kJitPersist = true
	env.CPU.m68kJitRecordNativePCs.Store(true)
	defer env.CPU.m68kJitRecordNativePCs.Store(false)

	compositor := NewVideoCompositor(nil)
	compositor.RegisterSource(env.Video)
	compositor.SetFrameCallback(func() {})
	if err := compositor.Start(); err != nil {
		t.Fatalf("compositor.Start() failed: %v", err)
	}
	defer compositor.Stop()

	var (
		faultMu sync.Mutex
		faults  []M68KFaultRecord
	)
	prev := env.CPU.FaultHook
	env.CPU.FaultHook = func(rec M68KFaultRecord) {
		rec = NormalizeM68KFaultRecord(env.CPU, rec)
		faultMu.Lock()
		first := len(faults) == 0
		if len(faults) < 8 {
			faults = append(faults, rec)
		}
		faultMu.Unlock()
		if first {
			t.Logf("FIRST FAULT PC=%08X addr=%08X op=%04X", rec.PC, rec.AccessAddr, rec.Opcode)
			t.Logf("  D0-7=%08X %08X %08X %08X %08X %08X %08X %08X",
				env.CPU.DataRegs[0], env.CPU.DataRegs[1], env.CPU.DataRegs[2], env.CPU.DataRegs[3],
				env.CPU.DataRegs[4], env.CPU.DataRegs[5], env.CPU.DataRegs[6], env.CPU.DataRegs[7])
			t.Logf("  A0-7=%08X %08X %08X %08X %08X %08X %08X %08X",
				env.CPU.AddrRegs[0], env.CPU.AddrRegs[1], env.CPU.AddrRegs[2], env.CPU.AddrRegs[3],
				env.CPU.AddrRegs[4], env.CPU.AddrRegs[5], env.CPU.AddrRegs[6], env.CPU.AddrRegs[7])
			env.CPU.m68kDumpNativePCRing()
			readMem := func(addr uint64, size int) []byte {
				if addr+uint64(size) > uint64(len(env.CPU.memory)) {
					return nil
				}
				return env.CPU.memory[addr : addr+uint64(size)]
			}
			dumpLongs := func(label string, addr uint32, count int) {
				t.Logf("--- %s @%08X ---", label, addr)
				for i := 0; i < count; i++ {
					a := addr + uint32(i*4)
					if uint64(a)+4 > uint64(len(env.CPU.memory)) {
						t.Logf("  %08X: <out of range>", a)
						continue
					}
					t.Logf("  %08X: %08X", a, env.CPU.Read32(a))
				}
			}
			lastPC := env.CPU.lastExecPC
			disasmBases := []uint64{}
			if lastPC >= 128 {
				disasmBases = append(disasmBases, uint64(lastPC-128))
			}
			if lastPC >= 64 {
				disasmBases = append(disasmBases, uint64(lastPC-64))
			}
			disasmBases = append(disasmBases, uint64(lastPC), uint64(env.CPU.AddrRegs[2]), uint64(env.CPU.AddrRegs[0]))
			for _, base := range disasmBases {
				t.Logf("--- disasm block @%08X ---", base)
				for _, ln := range disassembleM68K(readMem, base, 40) {
					t.Logf("  %08X: %-18s %s", ln.Address, ln.HexBytes, ln.Mnemonic)
				}
			}
			dumpLongs("A2 source window", env.CPU.AddrRegs[2], 12)
			if env.CPU.AddrRegs[2] >= 16 {
				dumpLongs("A2 source before", env.CPU.AddrRegs[2]-16, 8)
			}
			dumpLongs("A7 stack", env.CPU.AddrRegs[7], 32)
			if env.CPU.AddrRegs[5] >= 32 {
				dumpLongs("A5 frame", env.CPU.AddrRegs[5]-32, 24)
			}
		}
		if prev != nil {
			prev(rec)
		}
	}
	defer func() { env.CPU.FaultHook = prev }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := env.BootAndWait(ctx); err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}

	waitSeconds := m68kJITLockstepEnvInt("IE_DIAG_WANDERER_WAIT_SECONDS", 60)
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	var nonBlack bool
	var rawStats, compStats frameStats
	for time.Now().Before(deadline) {
		faultMu.Lock()
		nf := len(faults)
		faultMu.Unlock()
		if nf > 0 {
			break
		}
		rawStats = collectFrameStats(env.Video.GetFrame())
		compStats = collectFrameStats(compositor.GetCurrentFrame())
		if rawStats.NonBlackRGB > 0 || compStats.NonBlackRGB > 0 {
			nonBlack = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	faultMu.Lock()
	defer faultMu.Unlock()
	for i, f := range faults {
		t.Logf("FAULT[%d] class=%v vec=%d PC=%08X faultPC=%08X op=%04X addr=%08X instr=%v %s",
			i, f.Class, f.Vector, f.PC, f.FaultPC, f.Opcode, f.AccessAddr, f.Instruction, f.Message)
	}
	ready := ProbeAROSReadyState(env.CPU, env.Loader)
	t.Logf("nonBlackFrame=%v faults=%d waitSeconds=%d instr=%d pc=%08X sr=%04X stopped=%v ready=%+v dos_events=%d native_blocks=%d fallback=%d raw=%+v compositor=%+v video_ctrl=%08X mode=%08X status=%08X fb=%08X color=%08X",
		nonBlack, len(faults), waitSeconds, env.CPU.InstructionCount, env.CPU.PC, env.CPU.SR,
		env.CPU.stopped.Load(), ready, len(env.DOS.FullCommandsSnapshot()),
		env.CPU.m68kJitNativeBlocksExecuted.Load(), env.CPU.m68kJitFallbackInstructions.Load(),
		rawStats, compStats,
		env.Bus.Read32(VIDEO_CTRL), env.Bus.Read32(VIDEO_MODE), env.Bus.Read32(VIDEO_STATUS),
		env.Bus.Read32(VIDEO_FB_BASE), env.Bus.Read32(VIDEO_COLOR_MODE))
	logAROSTaskLists(t, "wanderer", env.CPU)
	t.Logf("D0-D7=%08X %08X %08X %08X %08X %08X %08X %08X",
		env.CPU.DataRegs[0], env.CPU.DataRegs[1], env.CPU.DataRegs[2], env.CPU.DataRegs[3],
		env.CPU.DataRegs[4], env.CPU.DataRegs[5], env.CPU.DataRegs[6], env.CPU.DataRegs[7])
	t.Logf("A0-A7=%08X %08X %08X %08X %08X %08X %08X %08X",
		env.CPU.AddrRegs[0], env.CPU.AddrRegs[1], env.CPU.AddrRegs[2], env.CPU.AddrRegs[3],
		env.CPU.AddrRegs[4], env.CPU.AddrRegs[5], env.CPU.AddrRegs[6], env.CPU.AddrRegs[7])
	t.Logf("native pc ring:")
	env.CPU.m68kDumpNativePCRing()
	if len(faults) != 0 {
		t.Fatalf("JIT Wanderer boot faulted: %+v", faults[0])
	}
	if !nonBlack {
		t.Fatalf("JIT Wanderer boot produced no desktop frame (timed out)")
	}
}
