//go:build headless && (((amd64 || arm64) && linux) || (js && wasm))

package main

import (
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"
)

//go:embed sdk/examples/prebuilt/rotozoomer_z80.ie80 sdk/examples/prebuilt/robocop_intro_z80.ie80 sdk/examples/assets/rotozoomtexture_z80.raw
var z80FullFixtureFiles embed.FS

const (
	z80FixtureFrameInstructions = uint64(50_000)
	z80FixtureWatchdogFrames    = 4
	z80FullRotozoomerSHA256     = "498d35495a0b6e3aedf3bb9a8a4a19cff63522ee8f408b7801d68a41b0ea5c2c"
	z80FullRobocopSHA256        = "c44ae3cab2ccd5daa6d823aacc8429694c27768a363edda4f9318b72aefb502a"
)

type z80FullFixtureCheckpoint struct {
	retired uint64
	cpu     [32]byte
	memory  [32]byte
	frame   [32]byte
	device  [32]byte
	iemon   [32]byte
}

func z80FullFixtureCPUHash(cpu *CPU_Z80) [32]byte {
	var image [48]byte
	copy(image[0:12], []byte{cpu.A, cpu.F, cpu.B, cpu.C, cpu.D, cpu.E, cpu.H, cpu.L, cpu.A2, cpu.F2, cpu.B2, cpu.C2})
	copy(image[12:16], []byte{cpu.D2, cpu.E2, cpu.H2, cpu.L2})
	binary.LittleEndian.PutUint16(image[16:], cpu.IX)
	binary.LittleEndian.PutUint16(image[18:], cpu.IY)
	binary.LittleEndian.PutUint16(image[20:], cpu.SP)
	binary.LittleEndian.PutUint16(image[22:], cpu.PC)
	copy(image[24:27], []byte{cpu.I, cpu.R, cpu.IM})
	binary.LittleEndian.PutUint16(image[28:], cpu.WZ)
	if cpu.IFF1 {
		image[30] = 1
	}
	if cpu.IFF2 {
		image[31] = 1
	}
	if cpu.Halted {
		image[32] = 1
	}
	if cpu.irqLine.Load() {
		image[33] = 1
	}
	if cpu.nmiLine.Load() {
		image[34] = 1
	}
	if cpu.nmiPending.Load() {
		image[35] = 1
	}
	image[36] = byte(cpu.iffDelay)
	binary.LittleEndian.PutUint32(image[37:], cpu.irqVector.Load())
	binary.LittleEndian.PutUint64(image[40:], cpu.Cycles)
	return sha256.Sum256(image[:])
}

func z80IEMonSnapshotHash(cpu *CPU_Z80, runner *CPUZ80Runner) [32]byte {
	snapshot := TakeSnapshot(NewDebugZ80(cpu, runner))
	h := sha256.New()
	h.Write([]byte(snapshot.CPUType))
	var value [8]byte
	for _, register := range snapshot.Registers {
		h.Write([]byte(register.Name))
		binary.LittleEndian.PutUint64(value[:], register.Value)
		h.Write(value[:])
	}
	h.Write(snapshot.Memory)
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

func runZ80FullFixture(t *testing.T, path string, program []byte, jit bool) []z80FullFixtureCheckpoint {
	t.Helper()
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatal(err)
	}
	if err := video.Stop(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = video.Stop() })
	video.AttachBus(bus)
	video.SetBigEndianMode(false)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	fileIO := NewFileIODevice(bus, ".")
	if texture, err := z80FullFixtureFiles.ReadFile("sdk/examples/assets/rotozoomtexture_z80.raw"); err == nil {
		fileIO.memFS = true
		fileIO.SetMemFile("sdk/examples/assets/rotozoomtexture_z80.raw", texture)
	}
	bus.MapIO(FILE_IO_BASE, FILE_IO_END, fileIO.HandleRead, fileIO.HandleWrite)
	bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fileIO.HandleWrite8)

	runner := NewCPUZ80Runner(bus, CPUZ80Config{DisableJIT: !jit})
	runner.LoadProgramBytes(program)
	cpu := runner.CPU()
	// A deterministic 50,000-instruction frame replaces the wall-clock video
	// ticker. The final tenth of each frame is VBlank, so polling loops observe
	// both edges identically on the interpreter and JIT machines.
	bus.MapIO(VIDEO_STATUS, VIDEO_STATUS+3, func(addr uint32) uint32 {
		if cpu.InstructionCount%z80FixtureFrameInstructions >= z80FixtureFrameInstructions*9/10 {
			return videoStatusVBlank
		}
		return 0
	}, nil)

	checkpoints := make([]z80FullFixtureCheckpoint, 0, z80FixtureWatchdogFrames)
	var renderedContent bool
	var previousFrame [32]byte
	var changedFrame bool
	for frame := uint64(1); frame <= z80FixtureWatchdogFrames; frame++ {
		cpu.InstructionCount = 0
		if jit {
			cpu.PerfEnabled = true
			cpu.jitSingleStep = true
			cpu.executionBoundary = func() {
				if cpu.InstructionCount >= z80FixtureFrameInstructions {
					cpu.SetRunning(false)
				}
			}
			cpu.SetRunning(true)
			cpu.z80JitExecute()
			cpu.executionBoundary = nil
		} else {
			for cpu.InstructionCount < z80FixtureFrameInstructions && !cpu.Halted {
				cpu.Step()
				cpu.InstructionCount++
			}
		}
		if cpu.Halted || cpu.InstructionCount != z80FixtureFrameInstructions {
			t.Fatalf("fixture watchdog at frame %d: halted=%v retired=%d want=%d pc=%04x", frame, cpu.Halted, cpu.InstructionCount, z80FixtureFrameInstructions, cpu.PC)
		}
		version, state, err := video.DebugSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		deviceImage := make([]byte, 4+len(state))
		binary.LittleEndian.PutUint32(deviceImage, version)
		copy(deviceImage[4:], state)
		frameBytes := video.FinishFrame()
		for _, value := range frameBytes {
			if value != 0 {
				renderedContent = true
				break
			}
		}
		frameHash := sha256.Sum256(frameBytes)
		if frame > 1 && frameHash != previousFrame {
			changedFrame = true
		}
		previousFrame = frameHash
		checkpoints = append(checkpoints, z80FullFixtureCheckpoint{
			retired: frame * z80FixtureFrameInstructions,
			cpu:     z80FullFixtureCPUHash(cpu),
			memory:  sha256.Sum256(bus.GetMemory()),
			frame:   frameHash,
			device:  sha256.Sum256(deviceImage),
			iemon:   z80IEMonSnapshotHash(cpu, runner),
		})
	}
	if jit && cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("full fixture did not execute a native Z80 block")
	}
	if !renderedContent || !changedFrame {
		t.Fatalf("full fixture did not render changing frame content: content=%v changed=%v", renderedContent, changedFrame)
	}
	return checkpoints
}

func requireZ80FullFixtureParity(t *testing.T, path string, program []byte) {
	t.Helper()
	interpreter := runZ80FullFixture(t, path, program, false)
	native := runZ80FullFixture(t, path, program, true)
	if len(interpreter) != len(native) {
		t.Fatalf("checkpoint count: interpreter=%d JIT=%d", len(interpreter), len(native))
	}
	for i := range interpreter {
		if interpreter[i] != native[i] {
			t.Fatalf("%s checkpoint %d mismatch\ninterpreter=%+v\nJIT=%+v", filepath.Base(path), i+1, interpreter[i], native[i])
		}
	}
}

func TestZ80JIT_FullRotozoomerShadowParity(t *testing.T) {
	path := filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_z80.ie80")
	data, err := z80FullFixtureFiles.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != z80FullRotozoomerSHA256 {
		t.Fatalf("fixture hash = %s, want %s", got, z80FullRotozoomerSHA256)
	}
	requireZ80FullFixtureParity(t, path, data)
}

func TestZ80JIT_FullRobocopShadowParity(t *testing.T) {
	path := filepath.Join("sdk", "examples", "prebuilt", "robocop_intro_z80.ie80")
	data, err := z80FullFixtureFiles.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != z80FullRobocopSHA256 {
		t.Fatalf("fixture hash = %s, want %s", got, z80FullRobocopSHA256)
	}
	requireZ80FullFixtureParity(t, path, data)
}
