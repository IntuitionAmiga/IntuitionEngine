package main

import (
	"encoding/binary"
	"testing"
)

func buildAYZ80EmulData(songName string, lengthFrames uint16) []byte {
	data := make([]byte, 0x120)
	copy(data[0:8], []byte("ZXAYEMUL"))
	binary.BigEndian.PutUint16(data[0x08:0x0A], 0x0103)
	data[0x0A] = 0x03
	data[0x0B] = 0x00
	data[0x10] = 0x00
	data[0x11] = 0x00
	binary.BigEndian.PutUint16(data[0x12:0x14], 0x0010) // songs at 0x22

	songStruct := 0x22
	songData := 0x30
	points := 0x40
	blocks := 0x50
	blockData := 0x60
	nameOff := 0x80

	binary.BigEndian.PutUint16(data[songStruct:songStruct+2], uint16(nameOff-songStruct))
	binary.BigEndian.PutUint16(data[songStruct+2:songStruct+4], uint16(songData-(songStruct+2)))
	data[songData] = 0
	data[songData+1] = 1
	data[songData+2] = 2
	data[songData+3] = 3
	binary.BigEndian.PutUint16(data[songData+4:songData+6], lengthFrames)
	binary.BigEndian.PutUint16(data[songData+6:songData+8], 0)
	data[songData+8] = 0x00
	data[songData+9] = 0x00
	binary.BigEndian.PutUint16(data[songData+10:songData+12], uint16(points-(songData+10)))
	binary.BigEndian.PutUint16(data[songData+12:songData+14], uint16(blocks-(songData+12)))

	binary.BigEndian.PutUint16(data[points:points+2], 0xF000)
	binary.BigEndian.PutUint16(data[points+2:points+4], 0x4000)
	binary.BigEndian.PutUint16(data[points+4:points+6], 0x4000)

	binary.BigEndian.PutUint16(data[blocks:blocks+2], 0x4000)
	binary.BigEndian.PutUint16(data[blocks+2:blocks+4], 0x000E)
	binary.BigEndian.PutUint16(data[blocks+4:blocks+6], uint16(blockData-(blocks+4)))
	binary.BigEndian.PutUint16(data[blocks+6:blocks+8], 0x0000)

	copy(data[blockData:blockData+0x0E], []byte{
		0x01, 0xFD, 0xFF, // LD BC,0xFFFD
		0x3E, 0x07, // LD A,0x07
		0xED, 0x79, // OUT (C),A
		0x01, 0xFD, 0xBF, // LD BC,0xBFFD
		0x3E, 0x55, // LD A,0x55
		0xED, 0x79, // OUT (C),A
		0xC9, // RET
	})

	copy(data[nameOff:], append([]byte(songName), 0x00))
	return data
}

func TestRenderAYZ80MetadataAndClock(t *testing.T) {
	data := buildAYZ80EmulData("RenderSong", 2)
	meta, events, total, clockHz, frameRate, loop, loopSample, _, _, err := renderAYZ80(data, 44100)
	if err != nil {
		t.Fatalf("render ay z80: %v", err)
	}
	if meta.Title != "RenderSong" || meta.System != "ZX Spectrum" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if clockHz != PSG_CLOCK_ZX_SPECTRUM || frameRate != 50 {
		t.Fatalf("unexpected timing: clock=%d frame=%d", clockHz, frameRate)
	}
	if loop || loopSample != 0 {
		t.Fatalf("unexpected loop flags")
	}
	if len(events) == 0 || total == 0 {
		t.Fatalf("expected events and total samples")
	}
}

func TestRenderAYZ80LoopDefault(t *testing.T) {
	data := buildAYZ80EmulData("LoopSong", 0)
	_, _, total, _, _, loop, loopSample, _, _, err := renderAYZ80WithLimit(data, 44100, 2)
	if err != nil {
		t.Fatalf("render ay z80: %v", err)
	}
	if !loop || loopSample != 0 {
		t.Fatalf("expected loop enabled for length 0")
	}
	if total == 0 {
		t.Fatalf("expected total samples for loop default")
	}
}
