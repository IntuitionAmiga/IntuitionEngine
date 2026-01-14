package main

import "testing"

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
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
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
