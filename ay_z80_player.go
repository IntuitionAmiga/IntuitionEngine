// ay_z80_player.go - ZXAYEMUL Z80 playback runner and event capture.

package main

import "fmt"

type ayZ80Player struct {
	cpu            *CPU_Z80
	bus            *ayZ80Bus
	song           AYZ80Song
	header         AYZ80Header
	clockHz        uint32
	frameRate      uint16
	sampleRate     int
	cyclesPerFrame uint64
	useIRQ         bool
	frameAcc       uint64
	currentSample  uint64
	initDone       bool
}

func newAYZ80Player(file *AYZ80File, songIndex int, sampleRate int, clockHz uint32, frameRate uint16, writer ayZ80PSGWriter) (*ayZ80Player, error) {
	if file == nil {
		return nil, fmt.Errorf("ay z80 file is nil")
	}
	if songIndex < 0 || songIndex >= len(file.Songs) {
		return nil, fmt.Errorf("ay z80 song index out of range")
	}
	if frameRate == 0 {
		return nil, fmt.Errorf("invalid frame rate")
	}
	if clockHz == 0 {
		return nil, fmt.Errorf("invalid z80 clock")
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("invalid sample rate")
	}

	song := file.Songs[songIndex]
	ram, err := buildAYZ80RAM(file.Header, song.Data)
	if err != nil {
		return nil, err
	}
	bus := newAYZ80Bus(&ram, song.Data.PlayerSystem, writer)
	cpu := NewCPU_Z80(bus)
	applyAYZ80Registers(cpu, songIndex, song.Data)

	return &ayZ80Player{
		cpu:            cpu,
		bus:            bus,
		song:           song,
		header:         file.Header,
		clockHz:        clockHz,
		frameRate:      frameRate,
		sampleRate:     sampleRate,
		cyclesPerFrame: uint64(clockHz) / uint64(frameRate),
		useIRQ:         true,
	}, nil
}

func (p *ayZ80Player) RenderFrames(frameCount int) ([]PSGEvent, uint64) {
	if frameCount <= 0 {
		return nil, 0
	}
	events := make([]PSGEvent, 0)

	if !p.initDone {
		p.cpu.PC = 0x0000
		p.initDone = true
	}

	samplesPerFrameNum := uint64(p.sampleRate)
	samplesPerFrameDen := uint64(p.frameRate)
	acc := p.frameAcc
	samplePos := p.currentSample

	for frame := 0; frame < frameCount; frame++ {
		startCycle := p.bus.cycles
		startIndex := len(p.bus.writes)
		p.runIRQFrame(p.cyclesPerFrame)
		events = append(events, p.collectEvents(samplePos, startCycle, startIndex)...)

		acc += samplesPerFrameNum
		step := acc / samplesPerFrameDen
		samplePos += step
		acc -= step * samplesPerFrameDen
	}

	p.frameAcc = acc
	p.currentSample = samplePos
	return events, samplePos
}

func (p *ayZ80Player) runIRQFrame(budget uint64) {
	idlePC := p.cpu.PC
	startCycles := p.bus.cycles
	executed := false
	irqAsserted := false
	irqServiced := false

	for p.bus.cycles-startCycles < budget {
		if p.cpu.Halted && !irqAsserted {
			p.cpu.SetIRQLine(true)
			irqAsserted = true
		}

		prevIFF1 := p.cpu.IFF1
		p.cpu.Step()
		executed = true

		if irqAsserted && prevIFF1 && !p.cpu.IFF1 && !irqServiced {
			irqServiced = true
			p.cpu.SetIRQLine(false)
		}
		if executed && p.cpu.PC == idlePC && irqServiced {
			return
		}
	}

	if irqAsserted {
		p.cpu.SetIRQLine(false)
	}
}

func (p *ayZ80Player) collectEvents(frameBaseSample uint64, startCycle uint64, startIndex int) []PSGEvent {
	if startIndex >= len(p.bus.writes) {
		return nil
	}
	events := make([]PSGEvent, 0, len(p.bus.writes)-startIndex)
	for _, write := range p.bus.writes[startIndex:] {
		cycleDelta := write.Cycle - startCycle
		sampleOffset := (cycleDelta * uint64(p.sampleRate)) / uint64(p.clockHz)
		events = append(events, PSGEvent{
			Sample: frameBaseSample + sampleOffset,
			Reg:    write.Reg,
			Value:  write.Value,
		})
	}
	return events
}

func buildAYZ80RAM(header AYZ80Header, song AYZ80SongData) ([0x10000]byte, error) {
	var ram [0x10000]byte

	playerVersion := header.PlayerVersion
	if playerVersion == 0 {
		playerVersion = 3
	}

	switch {
	case playerVersion >= 3:
		for i := 0; i < 0x0100; i++ {
			ram[i] = 0xC9
		}
		for i := 0x0100; i < 0x4000; i++ {
			ram[i] = 0xFF
		}
		for i := 0x4000; i < len(ram); i++ {
			ram[i] = 0x00
		}
	case playerVersion == 2:
		for i := 0; i < 0x0100; i++ {
			ram[i] = 0xC9
		}
	}

	ram[0x0038] = 0xFB
	ram[0x0300] = 0x00
	ram[0x0301] = 0x00

	for _, block := range song.Blocks {
		if block.Addr == 0 || len(block.Data) == 0 {
			continue
		}
		start := int(block.Addr)
		end := start + len(block.Data)
		if end > len(ram) {
			end = len(ram)
		}
		copy(ram[start:end], block.Data[:end-start])
	}

	points := song.Points
	if points == nil {
		return ram, fmt.Errorf("ay z80 song missing points")
	}
	initAddr := points.Init
	if initAddr == 0 && len(song.Blocks) > 0 {
		initAddr = song.Blocks[0].Addr
	}
	interrupt := points.Interrupt
	stub := buildAYZ80Stub(initAddr, interrupt)
	copy(ram[:], stub)

	return ram, nil
}

func buildAYZ80Stub(initAddr uint16, interrupt uint16) []byte {
	code := make([]byte, 0, 32)
	code = append(code, 0xF3) // DI
	if initAddr != 0 {
		code = appendCall(code, initAddr)
	}
	loopPos := len(code)
	if interrupt == 0 {
		code = append(code, 0xED, 0x5E) // IM 2
	} else {
		code = append(code, 0xED, 0x56) // IM 1
	}
	code = append(code, 0xFB, 0x76) // EI, HALT
	if interrupt != 0 {
		code = appendCall(code, interrupt)
	}
	rel := loopPos - (len(code) + 2)
	code = append(code, 0x18, byte(int8(rel))) // JR loop
	return code
}

func appendCall(code []byte, addr uint16) []byte {
	code = append(code, 0xCD, byte(addr&0xFF), byte((addr>>8)&0xFF))
	return code
}

func applyAYZ80Registers(cpu *CPU_Z80, songIndex int, song AYZ80SongData) {
	hi := song.HiReg
	lo := song.LoReg

	cpu.A = hi
	cpu.F = lo
	cpu.B = hi
	cpu.C = lo
	cpu.D = hi
	cpu.E = lo
	cpu.H = hi
	cpu.L = lo
	cpu.A2 = hi
	cpu.F2 = lo
	cpu.B2 = hi
	cpu.C2 = lo
	cpu.D2 = hi
	cpu.E2 = lo
	cpu.H2 = hi
	cpu.L2 = lo

	if song.Points != nil {
		cpu.SP = song.Points.Stack
	}
	if cpu.SP == 0 {
		cpu.SP = 0xFFFF
	}
	cpu.I = 3
	cpu.IM = 0
	cpu.IFF1 = false
	cpu.IFF2 = false
	cpu.PC = 0x0000
	cpu.SetIRQVector(0x00)
	_ = songIndex
}
