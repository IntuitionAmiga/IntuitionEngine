package main

import (
	"encoding/binary"
	"testing"
)

func TestParseAYZ80HeaderAndSongData(t *testing.T) {
	data := make([]byte, 0x90)
	copy(data[0:8], []byte("ZXAYEMUL"))
	binary.BigEndian.PutUint16(data[0x08:0x0A], 0x0103)
	data[0x0A] = 0x03
	data[0x0B] = 0x00
	data[0x10] = 0x00
	data[0x11] = 0x00
	binary.BigEndian.PutUint16(data[0x12:0x14], 0x000E) // songs at 0x20

	songStruct := 0x20
	songData := 0x30
	points := 0x40
	blocks := 0x50
	blockData := 0x60
	nameOff := 0x70

	// Song structure at 0x20
	binary.BigEndian.PutUint16(data[songStruct:songStruct+2], uint16(nameOff-songStruct))
	binary.BigEndian.PutUint16(data[songStruct+2:songStruct+4], uint16(songData-(songStruct+2)))

	data[songData] = 0
	data[songData+1] = 1
	data[songData+2] = 2
	data[songData+3] = 3
	binary.BigEndian.PutUint16(data[songData+4:songData+6], 100)
	binary.BigEndian.PutUint16(data[songData+6:songData+8], 10)
	data[songData+8] = 0xAA
	data[songData+9] = 0x55
	binary.BigEndian.PutUint16(data[songData+10:songData+12], uint16(points-(songData+10)))
	binary.BigEndian.PutUint16(data[songData+12:songData+14], uint16(blocks-(songData+12)))

	binary.BigEndian.PutUint16(data[points:points+2], 0xF000)
	binary.BigEndian.PutUint16(data[points+2:points+4], 0x4000)
	binary.BigEndian.PutUint16(data[points+4:points+6], 0x5000)

	binary.BigEndian.PutUint16(data[blocks:blocks+2], 0x6000)
	binary.BigEndian.PutUint16(data[blocks+2:blocks+4], 0x0002)
	binary.BigEndian.PutUint16(data[blocks+4:blocks+6], uint16(blockData-(blocks+4)))
	binary.BigEndian.PutUint16(data[blocks+6:blocks+8], 0x0000)

	data[blockData] = 0xDE
	data[blockData+1] = 0xAD
	copy(data[nameOff:], []byte("Song\x00"))

	ay, err := ParseAYZ80Data(data)
	if err != nil {
		t.Fatalf("parse ay z80: %v", err)
	}
	if ay.Header.FileVersion != 0x0103 || ay.Header.PlayerVersion != 0x03 {
		t.Fatalf("unexpected header: %+v", ay.Header)
	}
	if len(ay.Songs) != 1 {
		t.Fatalf("expected 1 song, got %d", len(ay.Songs))
	}
	song := ay.Songs[0]
	if song.Name != "Song" {
		t.Fatalf("unexpected song name: %s", song.Name)
	}
	if song.Data.Points == nil || song.Data.Points.Init != 0x4000 || song.Data.Points.Interrupt != 0x5000 {
		t.Fatalf("unexpected points: %+v", song.Data.Points)
	}
	if len(song.Data.Blocks) != 1 || song.Data.Blocks[0].Addr != 0x6000 {
		t.Fatalf("unexpected blocks: %+v", song.Data.Blocks)
	}
	if song.Data.Blocks[0].Data[0] != 0xDE || song.Data.Blocks[0].Data[1] != 0xAD {
		t.Fatalf("unexpected block data")
	}
}

func TestParseAYZ80InvalidHeader(t *testing.T) {
	if _, err := ParseAYZ80Data([]byte("notay")); err == nil {
		t.Fatalf("expected error for invalid header")
	}
}
