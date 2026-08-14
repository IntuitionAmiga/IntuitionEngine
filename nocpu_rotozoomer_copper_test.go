package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	nocpuCopperBase = uint32(0x1000000)
	nocpuStateBase  = uint32(0x1400000)
	nocpuRenderA    = uint32(0xA00000)
	nocpuAngleStep  = uint16(313)
	nocpuScaleStep  = uint16(104)
	nocpuCopperSize = uint32(62264)
	nocpuStateSize  = uint32(0x695000)
	nocpuAngleBits  = uint32(0x1404000)
	nocpuScaleBits  = uint32(0x1408000)
	nocpuBitStride  = uint32(0x8000)
	nocpuFrameTable = uint32(0x1494000)
	nocpuAddrTable  = uint32(0x1A94000)
	nocpuCompact    = uint32(0x1B00000)
)

var (
	nocpuProgramOnce sync.Once
	nocpuProgram     []byte
	nocpuProgramErr  error
)

func assembleNoCPUProgram(t testing.TB) []byte {
	t.Helper()
	assembler := buildAssembler(t)
	nocpuProgramOnce.Do(func() {
		root := repoRootDir(t)
		directory, err := os.MkdirTemp("", "rotozoomer-nocpu-assembly")
		if err != nil {
			nocpuProgramErr = fmt.Errorf("create no-CPU assembly directory: %w", err)
			return
		}
		defer os.RemoveAll(directory)
		output := filepath.Join(directory, "rotozoomer_nocpu.ie64")
		command := exec.Command(assembler,
			"-I", filepath.Join(root, "sdk", "include"),
			"-o", output,
			filepath.Join(root, "sdk", "examples", "asm", "rotozoomer_nocpu.asm"),
		)
		if combined, err := command.CombinedOutput(); err != nil {
			nocpuProgramErr = fmt.Errorf("assemble no-CPU source: %v\n%s", err, combined)
			return
		}
		nocpuProgram, nocpuProgramErr = os.ReadFile(output)
	})
	if nocpuProgramErr != nil {
		t.Fatal(nocpuProgramErr)
	}
	return nocpuProgram
}

func bootNoCPUProgram(t testing.TB, withMIDI bool) (*VideoChip, *MachineBus, *CPU64, *MIDIPlayer) {
	t.Helper()

	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = video.Stop() })
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	video.SetDirectVRAM(bus.memory[VRAM_START : VRAM_START+VRAM_SIZE])
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleWrite8)

	var midi *MIDIPlayer
	if withMIDI {
		midi = NewMIDIPlayer(nil, SAMPLE_RATE)
		midi.AttachBus(bus)
		bus.MapIO(MIDI_PLAY_PTR, MIDI_END, midi.HandlePlayRead, midi.HandlePlayWrite)
	}

	cpu := NewCPU64(bus)
	cpu.jitEnabled = false
	cpu.PerfEnabled = true
	video.mu.Lock()
	video.copperManagedByCompositor = true
	video.mu.Unlock()
	if err := cpu.LoadFlatProgramBytes(assembleNoCPUProgram(t)); err != nil {
		t.Fatal(err)
	}
	cpu.Execute()
	if cpu.running.Load() {
		t.Fatal("IE64 remained running after the no-CPU bootstrap")
	}
	return video, bus, cpu, midi
}

func loadNoCPUCopperFixture(t testing.TB) (*VideoChip, *MachineBus) {
	t.Helper()
	video, bus, _, _ := bootNoCPUProgram(t, false)
	return video, bus
}

func readNoCPUPackedWord(memory []byte, address uint32) uint32 {
	var value uint32
	for byteIndex := uint32(0); byteIndex < 4; byteIndex++ {
		value |= uint32(memory[address+byteIndex*4]) << (byteIndex * 8)
	}
	return value
}

func writeNoCPUPhase(memory []byte, base uint32, value uint16) {
	for bit := uint32(0); bit < 16; bit++ {
		word := uint32(0)
		if value&(1<<bit) != 0 {
			word = ^uint32(0)
		}
		block := base + bit*nocpuBitStride
		for offset := uint32(0); offset < 0x4000; offset += 4 {
			binary.LittleEndian.PutUint32(memory[block+offset:], word)
		}
	}
}

func readNoCPUPhase(memory []byte, base uint32) uint16 {
	var value uint16
	for bit := uint32(0); bit < 16; bit++ {
		if binary.LittleEndian.Uint32(memory[base+bit*nocpuBitStride:]) != 0 {
			value |= 1 << bit
		}
	}
	return value
}

func BenchmarkNoCPUCopperFrame(b *testing.B) {
	video, _ := loadNoCPUCopperFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	started := time.Now()
	for range b.N {
		video.RunCopperFrameForTest()
	}
	elapsed := time.Since(started)
	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "frames/s")
}

func BenchmarkNoCPUCompositorFrame(b *testing.B) {
	video, _ := loadNoCPUCopperFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	started := time.Now()
	for range b.N {
		runNoCPUCompositorFrame(video)
	}
	elapsed := time.Since(started)
	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "frames/s")
}

func runNoCPUCompositorFrame(video *VideoChip) []byte {
	video.StartFrame()
	video.ProcessScanlineRange(0, 480)
	return video.FinishFrame()
}

func TestNoCPUCompositorOwnsEveryCopperFrame(t *testing.T) {
	video, _ := loadNoCPUCopperFixture(t)
	comp := NewVideoCompositor(nil)
	comp.scheduler = NewManualVideoScheduler()
	comp.RegisterSource(video)
	if err := comp.Start(); err != nil {
		t.Fatalf("compositor Start: %v", err)
	}
	t.Cleanup(func() { _ = comp.Close() })
	video.mu.Lock()
	video.copperManagedByCompositor = false
	video.mu.Unlock()

	before := copperFrameStartCountForTest(video)
	for frame := uint64(1); frame <= 3; frame++ {
		comp.scheduler.TickManual()
		if got := copperFrameStartCountForTest(video); got != before+frame {
			t.Fatalf("compositor tick %d advanced Copper %d times, want %d", frame, got-before, frame)
		}
		video.runPrivateRefreshTick()
		if got := copperFrameStartCountForTest(video); got != before+frame {
			t.Fatalf("private tick after compositor frame %d increased Copper frames to %d", frame, got-before)
		}
	}
}

func TestNoCPUBootstrapBuildsEveryPackedAffineRecord(t *testing.T) {
	_, bus := loadNoCPUCopperFixture(t)
	for angle := 0; angle < 256; angle++ {
		tableAddress := nocpuFrameTable + uint32(angle)*0x6000
		if got := readNoCPUPackedWord(bus.memory, nocpuAddrTable+uint32(angle)*16); got != tableAddress {
			t.Fatalf("angle %d table address = %#x, want %#x", angle, got, tableAddress)
		}
		for scale := 0; scale < 256; scale++ {
			want := nocpuEstablishedFrame(uint16(angle)<<8, uint16(scale)<<8)
			address := tableAddress + uint32(scale)*96
			got := nocpuFrame{
				u0:    readNoCPUPackedWord(bus.memory, address),
				v0:    readNoCPUPackedWord(bus.memory, address+16),
				duCol: readNoCPUPackedWord(bus.memory, address+32),
				dvCol: readNoCPUPackedWord(bus.memory, address+48),
				duRow: readNoCPUPackedWord(bus.memory, address+64),
				dvRow: readNoCPUPackedWord(bus.memory, address+80),
			}
			if got != want {
				t.Fatalf("angle %d scale %d record = %#v, want %#v", angle, scale, got, want)
			}
		}
	}
}

func TestNoCPUBootstrapBuildsCompactRecordsThenScattersThem(t *testing.T) {
	video, bus := loadNoCPUCopperFixture(t)
	for _, point := range []struct {
		angle int
		scale int
	}{
		{angle: 0, scale: 0},
		{angle: 73, scale: 149},
		{angle: 255, scale: 255},
	} {
		want := nocpuEstablishedFrame(uint16(point.angle)<<8, uint16(point.scale)<<8)
		address := nocpuCompact + uint32(point.angle*256+point.scale)*24
		got := nocpuFrame{
			u0:    binary.LittleEndian.Uint32(bus.memory[address:]),
			v0:    binary.LittleEndian.Uint32(bus.memory[address+4:]),
			duCol: binary.LittleEndian.Uint32(bus.memory[address+8:]),
			dvCol: binary.LittleEndian.Uint32(bus.memory[address+12:]),
			duRow: binary.LittleEndian.Uint32(bus.memory[address+16:]),
			dvRow: binary.LittleEndian.Uint32(bus.memory[address+20:]),
		}
		if got != want {
			t.Fatalf("angle %d scale %d compact record = %#v, want %#v", point.angle, point.scale, got, want)
		}
	}
	if got := video.bltStartCount; got != 2 {
		t.Fatalf("bootstrap blitter starts = %d, want 2", got)
	}
}

func TestNoCPUCopperPhaseAndSelectionMatchOracle(t *testing.T) {
	video, bus := loadNoCPUCopperFixture(t)
	tests := []struct {
		angle uint16
		scale uint16
	}{
		{0, 0},
		{1, 1},
		{0x00FF, 0xFF00},
		{0x7FFF, 0x8000},
		{0xFEFF, 0x0100},
		{0xFFFF, 0xFFFF},
	}
	for _, test := range tests {
		writeNoCPUPhase(bus.memory, nocpuAngleBits, test.angle)
		writeNoCPUPhase(bus.memory, nocpuScaleBits, test.scale)
		video.RunCopperFrameForTest()

		want := nocpuEstablishedFrame(test.angle, test.scale)
		got := nocpuFrame{
			u0: bus.Read32(BLT_MODE7_U0), v0: bus.Read32(BLT_MODE7_V0),
			duCol: bus.Read32(BLT_MODE7_DU_COL), dvCol: bus.Read32(BLT_MODE7_DV_COL),
			duRow: bus.Read32(BLT_MODE7_DU_ROW), dvRow: bus.Read32(BLT_MODE7_DV_ROW),
		}
		if got != want {
			t.Fatalf("angle %#x scale %#x selected %#v, want %#v", test.angle, test.scale, got, want)
		}
		if gotAngle := readNoCPUPhase(bus.memory, nocpuAngleBits); gotAngle != test.angle+nocpuAngleStep {
			t.Fatalf("angle %#x advanced to %#x, want %#x", test.angle, gotAngle, test.angle+nocpuAngleStep)
		}
		if gotScale := readNoCPUPhase(bus.memory, nocpuScaleBits); gotScale != test.scale+nocpuScaleStep {
			t.Fatalf("scale %#x advanced to %#x, want %#x", test.scale, gotScale, test.scale+nocpuScaleStep)
		}
	}
}

func TestNoCPUCopperCommandPrefixesStayInBounds(t *testing.T) {
	for _, starts := range []int{1, 8, 16, 32, 64, 96, 128, 160, 167} {
		t.Run(fmt.Sprintf("starts_%d", starts), func(t *testing.T) {
			video, bus := loadNoCPUCopperFixture(t)
			pc := nocpuCopperBase
			seen := 0
			for pc < nocpuCopperBase+nocpuCopperSize {
				word := binary.LittleEndian.Uint32(bus.memory[pc:])
				switch word >> copperOpcodeShift {
				case copperOpcodeWait, copperOpcodeSetBase:
					pc += 4
				case copperOpcodeMove:
					address := uint32(VIDEO_REG_BASE) + (((word >> copperRegShift) & copperRegMask) * 4)
					if address == BLT_CTRL && binary.LittleEndian.Uint32(bus.memory[pc+4:]) == 1 {
						seen++
						if seen == starts {
							binary.LittleEndian.PutUint32(bus.memory[pc+8:], copperEndWord())
							pc = nocpuCopperBase + nocpuCopperSize
							continue
						}
					}
					pc += 8
				case copperOpcodeEnd:
					pc = nocpuCopperBase + nocpuCopperSize
				}
			}
			if seen < starts {
				t.Fatalf("Copper list has %d blit starts, want at least %d", seen, starts)
			}
			video.RunCopperFrameForTest()
			if status := bus.Read32(BLT_STATUS); status&bltStatusErr != 0 {
				t.Fatalf("prefix ending after blit %d set status %#x", starts, status)
			}
		})
	}
}

func TestNoCPUIE64BootstrapHaltsWhileVideoAndMIDIContinue(t *testing.T) {
	video, bus, cpu, midi := bootNoCPUProgram(t, true)
	retired := cpu.InstructionCount
	if retired == 0 {
		t.Fatal("bootstrap retired no instructions")
	}
	if retired != 1_644_371 {
		t.Fatalf("bootstrap retired %d instructions, want 1644371", retired)
	}
	video.mu.Lock()
	copperPointer := video.copperPtr
	video.mu.Unlock()
	if got := copperPointer; got != nocpuCopperBase {
		t.Fatalf("bootstrap Copper pointer = %#x, want %#x; retired %d, PC %#x, blitter starts %d", got, nocpuCopperBase, retired, cpu.PC, video.bltStartCount)
	}
	copperHash := sha256.Sum256(bus.memory[nocpuCopperBase : nocpuCopperBase+nocpuCopperSize])
	if got := fmt.Sprintf("%x", copperHash); got != "e9ee212e99196e4ac4167a8a60967d31a74e5d74fdbfd923f37a6cff53f8b40f" {
		t.Fatalf("bootstrap Copper hash = %s", got)
	}
	stateHash := sha256.Sum256(bus.memory[nocpuStateBase : nocpuStateBase+nocpuStateSize])
	if got := fmt.Sprintf("%x", stateHash); got != "5e8608b14808c14c6bd0e21a45dc3f06a9749d301e6e83988c404a9707032134" {
		t.Fatalf("bootstrap state hash = %s", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !midi.IsPlaying() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !midi.IsPlaying() {
		t.Fatal("MIDI playback did not start before IE64 halted")
	}
	positionBefore := midi.engine.PositionSamples()
	midi.engine.TickBlock(2048)
	if got := midi.engine.PositionSamples(); got <= positionBefore {
		t.Fatalf("MIDI position = %d after ticking, want greater than %d", got, positionBefore)
	}

	video.RunCopperFrameForTest()
	firstFB := bus.Read32(VIDEO_FB_BASE)
	if firstFB != nocpuRenderA {
		t.Fatalf("first framebuffer = %#x, want %#x; Copper PC %#x, starts %d", firstFB, nocpuRenderA, bus.Read32(COPPER_PC), video.bltStartCount)
	}
	firstHash := hashNoCPUFrame(bus.memory[firstFB : firstFB+640*480*4])
	video.RunCopperFrameForTest()
	secondFB := bus.Read32(VIDEO_FB_BASE)
	if secondFB != nocpuRenderA {
		t.Fatalf("second framebuffer = %#x, want %#x; Copper PC %#x, starts %d", secondFB, nocpuRenderA, bus.Read32(COPPER_PC), video.bltStartCount)
	}
	secondHash := hashNoCPUFrame(bus.memory[secondFB : secondFB+640*480*4])
	if firstHash == secondHash {
		pc := bus.Read32(COPPER_PC)
		t.Fatalf("successive no-CPU frame hashes both equal %#x; framebuffers %#x %#x, Copper PC %#x status %#x previous word %#x, starts %d, u0 %#x, duCol %#x", firstHash, firstFB, secondFB, pc, bus.Read32(COPPER_STATUS), bus.Read32(pc-4), video.bltStartCount, bus.Read32(BLT_MODE7_U0), bus.Read32(BLT_MODE7_DU_COL))
	}
	if cpu.InstructionCount != retired || cpu.running.Load() {
		t.Fatalf("IE64 changed after halt: retired %d to %d, running %v", retired, cpu.InstructionCount, cpu.running.Load())
	}
}

func TestNoCPUCopperCalculatesAndRendersSuccessiveFrames(t *testing.T) {
	video, bus := loadNoCPUCopperFixture(t)
	originalList := append([]byte(nil), bus.memory[nocpuCopperBase:nocpuCopperBase+nocpuCopperSize]...)
	angle, scale := uint16(0), uint16(0)
	var previousHash uint32
	previousStarts := uint64(0)

	for frame := 0; frame < 3; frame++ {
		video.RunCopperFrameForTest()
		if video.bltStartCount <= previousStarts {
			t.Fatalf("frame %d did not start any blitter commands", frame)
		}
		previousStarts = video.bltStartCount
		if status := bus.Read32(BLT_STATUS); status&bltStatusErr != 0 {
			t.Fatalf("frame %d ended with blitter status %#x", frame, status)
		}
		if status := bus.Read32(COPPER_STATUS); status&copperStatusHalted == 0 {
			t.Fatalf("frame %d ended with Copper status %#x", frame, status)
		}
		want := nocpuEstablishedFrame(angle, scale)
		rewrittenOperand := false
		for _, field := range []struct {
			name string
			addr uint32
			want uint32
		}{
			{name: "u0", addr: BLT_MODE7_U0, want: want.u0},
			{name: "v0", addr: BLT_MODE7_V0, want: want.v0},
			{name: "duCol", addr: BLT_MODE7_DU_COL, want: want.duCol},
			{name: "dvCol", addr: BLT_MODE7_DV_COL, want: want.dvCol},
			{name: "duRow", addr: BLT_MODE7_DU_ROW, want: want.duRow},
			{name: "dvRow", addr: BLT_MODE7_DV_ROW, want: want.dvRow},
		} {
			if got := bus.Read32(field.addr); got != field.want {
				t.Errorf("frame %d %s = %#x, want %#x; copper PC %#x status %#x blits %d", frame, field.name, got, field.want, bus.Read32(COPPER_PC), bus.Read32(COPPER_STATUS), video.bltStartCount)
			}
		}
		for pc := 0; pc < len(originalList); {
			word := binary.LittleEndian.Uint32(originalList[pc:])
			switch word >> copperOpcodeShift {
			case copperOpcodeWait, copperOpcodeSetBase:
				pc += 4
			case copperOpcodeMove:
				if got := binary.LittleEndian.Uint32(bus.memory[nocpuCopperBase+uint32(pc):]); got != word {
					t.Fatalf("frame %d rewrote Copper opcode at byte %d to %#x", frame, pc, got)
				}
				if binary.LittleEndian.Uint32(bus.memory[nocpuCopperBase+uint32(pc+4):]) != binary.LittleEndian.Uint32(originalList[pc+4:]) {
					rewrittenOperand = true
				}
				pc += 8
			case copperOpcodeEnd:
				pc = len(originalList)
			default:
				t.Fatalf("frame %d found unknown original Copper opcode at byte %d", frame, pc)
			}
		}
		if !rewrittenOperand {
			t.Fatalf("frame %d did not rewrite any later Copper operand", frame)
		}
		fb := bus.Read32(VIDEO_FB_BASE)
		if fb != nocpuRenderA {
			t.Fatalf("frame %d framebuffer = %#x, blitter status %#x", frame, fb, bus.Read32(BLT_STATUS))
		}
		hash := hashNoCPUFrame(bus.memory[fb : fb+640*480*4])
		if hash == 0 || frame > 0 && hash == previousHash {
			t.Fatalf("frame %d hash = %#x, previous %#x", frame, hash, previousHash)
		}
		previousHash = hash
		angle += nocpuAngleStep
		scale += nocpuScaleStep
	}
}

func TestNoCPUCopperHoldsPresentationUntilFrameIsComplete(t *testing.T) {
	video, _ := loadNoCPUCopperFixture(t)
	startCopperFrame(video)
	video.StepCopperRasterForTest(0, 0)
	video.mu.Lock()
	heldDuringCalculation := video.presentationHeld || video.presentationPriming
	video.mu.Unlock()
	if !heldDuringCalculation {
		t.Fatal("presentation was neither held nor primed during no-CPU calculation")
	}

	for y := 1; y < 480; y++ {
		video.StepCopperRasterForTest(y, 0)
	}
	video.mu.Lock()
	heldAfterFrame := video.presentationHeld || video.presentationPriming
	video.mu.Unlock()
	if heldAfterFrame {
		t.Fatal("presentation remained held after the completed Mode7 frame")
	}

	startCopperFrame(video)
	video.StepCopperRasterForTest(0, 0)
	video.mu.Lock()
	heldOnSecondFrame := video.presentationHeld
	video.mu.Unlock()
	if !heldOnSecondFrame {
		t.Fatal("second no-CPU calculation did not retain the completed first frame")
	}
}

type nocpuFrame struct {
	u0, v0, duCol, dvCol, duRow, dvRow uint32
}

func nocpuEstablishedFrame(angle, scale uint16) nocpuFrame {
	ai, si := uint8(angle>>8), uint8(scale>>8)
	ca := int32(nocpuSineTable[uint8(ai+64)]) * int32(nocpuReciprocalTable[si])
	sa := int32(nocpuSineTable[ai]) * int32(nocpuReciprocalTable[si])
	return nocpuFrame{
		u0:    uint32(int64(128<<16) - int64(ca)*320 + int64(sa)*240),
		v0:    uint32(int64(128<<16) - int64(sa)*320 - int64(ca)*240),
		duCol: uint32(ca), dvCol: uint32(sa), duRow: uint32(-sa), dvRow: uint32(ca),
	}
}

func hashNoCPUFrame(data []byte) uint32 {
	h := uint32(2166136261)
	for len(data) >= 4 {
		h ^= binary.LittleEndian.Uint32(data)
		h *= 16777619
		data = data[4:]
	}
	return h
}

var nocpuSineTable = func() [256]int16 {
	var table [256]int16
	for i := range table {
		table[i] = int16(math.Round(math.Sin(float64(i)*2*math.Pi/256) * 256))
	}
	return table
}()

var nocpuReciprocalTable = func() [256]uint16 {
	var table [256]uint16
	for i := range table {
		phase := math.Sin(float64(i) * 2 * math.Pi / 256)
		table[i] = uint16(math.Round(256 / (0.5 + phase*0.3)))
	}
	return table
}()
