package main

import (
	"os"
	"reflect"
	"testing"
)

func TestAYZ80RobocopIEMonWriteCheckpointParity(t *testing.T) {
	data, err := os.ReadFile("sdk/examples/assets/music/Robocop1.ay")
	if err != nil {
		t.Fatal(err)
	}
	file, err := ParseAYZ80Data(data)
	if err != nil {
		t.Fatal(err)
	}
	makePlayer := func(jit bool) *ayZ80Player {
		p, err := newAYZ80Player(file, int(file.Header.FirstSongIndex), 44100, Z80_CLOCK_ZX_SPECTRUM, 50, nil)
		if err != nil {
			t.Fatal(err)
		}
		p.cpu.jitEnabled = jit
		return p
	}
	type hit struct {
		pc, af, bc, de, hl, ix, iy, sp uint16
		value                          byte
	}
	run := func(jit bool) []hit {
		player := makePlayer(jit)
		adapter := player.cpu.bus.(*Z80BusAdapter)
		monitor := NewMachineMonitor(adapter.bus)
		debugCPU := NewDebugZ80(player.cpu, nil)
		monitor.RegisterCPU("Z80", debugCPU)
		var hits []hit
		for len(hits) < 64 && !player.cpu.Halted {
			monitor.ExecuteCommand("bpmbw $c066")
			player.cpu.SetRunning(true)
			if jit {
				player.cpu.z80JitExecute()
			} else {
				for player.cpu.Running() && !player.cpu.Halted {
					player.cpu.Step()
				}
			}
			_ = TakeSnapshot(debugCPU)
			hits = append(hits, hit{
				pc: player.cpu.PC, af: player.cpu.AF(), bc: player.cpu.BC(),
				de: player.cpu.DE(), hl: player.cpu.HL(), ix: player.cpu.IX,
				iy: player.cpu.IY, sp: player.cpu.SP, value: player.bus.ram[0xC066],
			})
			monitor.ExecuteCommand("wc $c066")
		}
		return hits
	}
	interp, jit := run(false), run(true)
	for i := 0; i < min(len(interp), len(jit)); i++ {
		if interp[i] != jit[i] {
			t.Fatalf("IEMon write checkpoint %d mismatch: interpreter=%+v JIT=%+v", i, interp[i], jit[i])
		}
	}
	if len(interp) != len(jit) {
		t.Fatalf("IEMon write checkpoint count: interpreter=%d JIT=%d", len(interp), len(jit))
	}
}

func TestAYZ80PlaybackRealProgramsJITParity(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 JIT unavailable")
	}
	for _, path := range []string{
		"sdk/examples/assets/music/WaksonsZak018.ay",
		"sdk/examples/assets/music/Robocop1.ay",
	} {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			file, err := ParseAYZ80Data(data)
			if err != nil {
				t.Fatal(err)
			}
			newPlayer := func(jit bool) *ayZ80Player {
				player, err := newAYZ80Player(file, int(file.Header.FirstSongIndex), 44100, Z80_CLOCK_ZX_SPECTRUM, 50, nil)
				if err != nil {
					t.Fatal(err)
				}
				player.cpu.jitEnabled = jit
				return player
			}
			jit, interpreter := newPlayer(true), newPlayer(false)
			for frame := 0; frame < 16; frame++ {
				jitEvents, _ := jit.RenderFrames(1)
				interpreterEvents, _ := interpreter.RenderFrames(1)
				if *jit.bus.ram != *interpreter.bus.ram {
					for addr := range 0x10000 {
						if jit.bus.ram[addr] != interpreter.bus.ram[addr] {
							t.Fatalf("frame %d RAM[%04X]: JIT=%02X interpreter=%02X", frame, addr, jit.bus.ram[addr], interpreter.bus.ram[addr])
						}
					}
				}
				if len(jitEvents) != len(interpreterEvents) {
					t.Fatalf("frame %d event count: JIT=%d interpreter=%d", frame, len(jitEvents), len(interpreterEvents))
				}
				for i := range jitEvents {
					if jitEvents[i] != interpreterEvents[i] {
						t.Fatalf("frame %d event %d: JIT=%+v interpreter=%+v", frame, i, jitEvents[i], interpreterEvents[i])
					}
				}
			}
		})
	}
}

func TestAYZ80PlaybackInstructionCountJITParity(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 JIT unavailable")
	}
	file, err := ParseAYZ80Data(buildAYZ80EmulData("CountSong", 2))
	if err != nil {
		t.Fatal(err)
	}
	run := func(jit bool) (uint64, []PSGEvent) {
		player, err := newAYZ80Player(file, int(file.Header.FirstSongIndex), 44100, Z80_CLOCK_ZX_SPECTRUM, 50, nil)
		if err != nil {
			t.Fatal(err)
		}
		player.cpu.jitEnabled = jit
		events, _ := player.RenderFrames(2)
		return player.instructionCount, append([]PSGEvent(nil), events...)
	}
	jitCount, jitEvents := run(true)
	interpreterCount, interpreterEvents := run(false)
	if jitCount == 0 || interpreterCount == 0 {
		t.Fatalf("retired instructions: JIT=%d interpreter=%d", jitCount, interpreterCount)
	}
	if !reflect.DeepEqual(jitEvents, interpreterEvents) {
		t.Fatalf("event mismatch: JIT=%v interpreter=%v", jitEvents, interpreterEvents)
	}
}

func TestAYZ80PlayerInterruptRoutineWrites(t *testing.T) {
	file := &AYZ80File{
		Header: AYZ80Header{
			PlayerVersion: 3,
		},
		Songs: []AYZ80Song{
			{
				Name: "IRQSong",
				Data: AYZ80SongData{
					HiReg: 0x00,
					LoReg: 0x00,
					Points: &AYZ80Points{
						Stack:     0xF000,
						Init:      0x0000,
						Interrupt: 0x4000,
					},
					Blocks: []AYZ80Block{
						{
							Addr: 0x4000,
							Data: []byte{
								0x01, 0xFD, 0xFF, // LD BC,0xFFFD
								0x3E, 0x07, // LD A,0x07
								0xED, 0x79, // OUT (C),A
								0x01, 0xFD, 0xBF, // LD BC,0xBFFD
								0x3E, 0x55, // LD A,0x55
								0xED, 0x79, // OUT (C),A
								0xC9, // RET
							},
						},
					},
				},
			},
		},
	}

	player, err := newAYZ80Player(file, 0, 44100, Z80_CLOCK_ZX_SPECTRUM, 50, nil)
	if err != nil {
		t.Fatalf("player create: %v", err)
	}
	events, _ := player.RenderFrames(1)
	if z80JitAvailable && player.cpu.jitStats.nativeEntries.Load() == 0 {
		t.Fatal("AY playback bypassed the default Z80 JIT")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: pc=%04x halted=%v iff1=%v irq=%v services=%d cycles=%d", len(events), player.cpu.PC, player.cpu.Halted, player.cpu.IFF1, player.cpu.irqLine.Load(), player.cpu.irqServices, player.bus.cycles)
	}
	if events[1].Reg != 0x07 || events[1].Value != 0x55 {
		t.Fatalf("unexpected event: %+v", events[1])
	}
}

func TestAYZ80PlayerTimingConversion(t *testing.T) {
	file := &AYZ80File{
		Header: AYZ80Header{
			PlayerVersion: 3,
		},
		Songs: []AYZ80Song{
			{
				Name: "TimeSong",
				Data: AYZ80SongData{
					HiReg: 0x00,
					LoReg: 0x00,
					Points: &AYZ80Points{
						Stack:     0xF000,
						Init:      0x0000,
						Interrupt: 0x4000,
					},
					Blocks: []AYZ80Block{
						{
							Addr: 0x4000,
							Data: []byte{
								0x01, 0xFD, 0xFF, // LD BC,0xFFFD
								0x3E, 0x01, // LD A,0x01
								0xED, 0x79, // OUT (C),A
								0x01, 0xFD, 0xBF, // LD BC,0xBFFD
								0x3E, 0x22, // LD A,0x22
								0xED, 0x79, // OUT (C),A
								0xC9, // RET
							},
						},
					},
				},
			},
		},
	}

	player, err := newAYZ80Player(file, 0, 44100, Z80_CLOCK_ZX_SPECTRUM, 50, nil)
	if err != nil {
		t.Fatalf("player create: %v", err)
	}
	events, _ := player.RenderFrames(1)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	write := player.bus.writes[0]
	expectedSample := (write.Cycle * 44100) / uint64(Z80_CLOCK_ZX_SPECTRUM)
	if events[0].Sample != expectedSample {
		t.Fatalf("sample=%d want %d", events[0].Sample, expectedSample)
	}
}
