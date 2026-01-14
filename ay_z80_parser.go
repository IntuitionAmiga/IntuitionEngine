// ay_z80_parser.go - ZXAYEMUL parser for AY files with Z80 player code.

package main

import (
	"encoding/binary"
	"fmt"
)

const (
	ayZXHeaderSize = 20
	ayZXSongSize   = 4

	ayZXSystemSpectrum = 0
	ayZXSystemCPC      = 1
	ayZXSystemMSX      = 2
)

type AYZ80Header struct {
	FileVersion      uint16
	PlayerVersion    byte
	SpecialPlayer    byte
	Author           string
	Misc             string
	SongCount        byte
	FirstSongIndex   byte
	SongTablePointer int
}

type AYZ80Points struct {
	Stack     uint16
	Init      uint16
	Interrupt uint16
}

type AYZ80Block struct {
	Addr uint16
	Data []byte
}

type AYZ80SongData struct {
	ChannelMap   [4]byte
	LengthFrames uint16
	FadeFrames   uint16
	HiReg        byte
	LoReg        byte
	Points       *AYZ80Points
	Blocks       []AYZ80Block
	PlayerSystem byte
}

type AYZ80Song struct {
	Name string
	Data AYZ80SongData
}

type AYZ80File struct {
	Header AYZ80Header
	Songs  []AYZ80Song
}

type ayZ80Parser struct {
	data []byte
}

func ParseAYZ80Data(data []byte) (*AYZ80File, error) {
	if len(data) < ayZXHeaderSize {
		return nil, fmt.Errorf("ay z80 header too short")
	}
	if string(data[0:8]) != "ZXAYEMUL" {
		return nil, fmt.Errorf("ay z80 invalid signature")
	}
	parser := ayZ80Parser{data: data}
	return parser.parse()
}

func (p *ayZ80Parser) parse() (*AYZ80File, error) {
	fileVersion := p.readU16(8)
	playerVersion := p.readU8(10)
	specialPlayer := p.readU8(11)
	author, err := p.readStringPointer(12)
	if err != nil {
		return nil, err
	}
	misc, err := p.readStringPointer(14)
	if err != nil {
		return nil, err
	}
	rawSongCount := p.readU8(16)
	rawFirstSong := p.readU8(17)
	songCount := int(rawSongCount) + 1
	if rawFirstSong >= byte(songCount) {
		return nil, fmt.Errorf("ay z80 first song out of range")
	}
	songsPtr, err := p.readRequiredPointer(18)
	if err != nil {
		return nil, err
	}

	header := AYZ80Header{
		FileVersion:      fileVersion,
		PlayerVersion:    playerVersion,
		SpecialPlayer:    specialPlayer,
		Author:           author,
		Misc:             misc,
		SongCount:        byte(songCount),
		FirstSongIndex:   rawFirstSong,
		SongTablePointer: songsPtr,
	}

	songs, err := p.parseSongs(songCount, songsPtr)
	if err != nil {
		return nil, err
	}

	return &AYZ80File{
		Header: header,
		Songs:  songs,
	}, nil
}

func (p *ayZ80Parser) parseSongs(count int, base int) ([]AYZ80Song, error) {
	songs := make([]AYZ80Song, 0, count)
	for i := 0; i < count; i++ {
		entry := base + i*ayZXSongSize
		if entry+4 > len(p.data) {
			return nil, fmt.Errorf("ay z80 song structure out of range")
		}
		namePtr, err := p.resolveOptionalPointer(entry, p.readI16(entry))
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("Song %d", i+1)
		if namePtr != nil {
			if parsed, err := p.readNTString(*namePtr); err == nil {
				name = parsed
			}
		}
		dataPtrOpt, err := p.resolvePointer(entry+2, p.readI16(entry+2))
		if err != nil {
			return nil, err
		}
		if dataPtrOpt == nil {
			return nil, fmt.Errorf("ay z80 missing song data pointer")
		}
		data, err := p.parseSongData(*dataPtrOpt)
		if err != nil {
			return nil, err
		}
		songs = append(songs, AYZ80Song{
			Name: name,
			Data: data,
		})
	}
	return songs, nil
}

func (p *ayZ80Parser) parseSongData(offset int) (AYZ80SongData, error) {
	if offset+14 > len(p.data) {
		return AYZ80SongData{}, fmt.Errorf("ay z80 song data truncated")
	}
	data := AYZ80SongData{
		ChannelMap: [4]byte{
			p.readU8(offset),
			p.readU8(offset + 1),
			p.readU8(offset + 2),
			p.readU8(offset + 3),
		},
		LengthFrames: p.readU16(offset + 4),
		FadeFrames:   p.readU16(offset + 6),
		HiReg:        p.readU8(offset + 8),
		LoReg:        p.readU8(offset + 9),
		PlayerSystem: ayZXSystemSpectrum,
	}
	pointsPtr, err := p.resolveOptionalPointer(offset+10, p.readI16(offset+10))
	if err != nil {
		return AYZ80SongData{}, err
	}
	addrPtr, err := p.resolveOptionalPointer(offset+12, p.readI16(offset+12))
	if err != nil {
		return AYZ80SongData{}, err
	}
	if pointsPtr != nil {
		points, err := p.parsePoints(*pointsPtr)
		if err != nil {
			return AYZ80SongData{}, err
		}
		data.Points = &points
	}
	if addrPtr != nil {
		blocks, err := p.parseBlocks(*addrPtr)
		if err != nil {
			return AYZ80SongData{}, err
		}
		data.Blocks = blocks
	}
	return data, nil
}

func (p *ayZ80Parser) parsePoints(offset int) (AYZ80Points, error) {
	if offset+6 > len(p.data) {
		return AYZ80Points{}, fmt.Errorf("ay z80 points truncated")
	}
	return AYZ80Points{
		Stack:     p.readU16(offset),
		Init:      p.readU16(offset + 2),
		Interrupt: p.readU16(offset + 4),
	}, nil
}

func (p *ayZ80Parser) parseBlocks(offset int) ([]AYZ80Block, error) {
	blocks := make([]AYZ80Block, 0)
	for {
		if offset+2 > len(p.data) {
			return nil, fmt.Errorf("ay z80 unterminated block table")
		}
		addr := p.readU16(offset)
		if addr == 0 {
			break
		}
		if offset+6 > len(p.data) {
			return nil, fmt.Errorf("ay z80 block entry truncated")
		}
		length := p.readU16(offset + 2)
		dataPtrOpt, err := p.resolvePointer(offset+4, p.readI16(offset+4))
		if err != nil {
			return nil, err
		}
		if dataPtrOpt == nil {
			return nil, fmt.Errorf("ay z80 missing block pointer")
		}
		dataPtr := *dataPtrOpt
		if dataPtr >= len(p.data) {
			return nil, fmt.Errorf("ay z80 block pointer out of range")
		}
		maxLen := uint32(0x10000 - uint32(addr))
		if uint32(length) > maxLen {
			length = uint16(maxLen)
		}
		if dataPtr+int(length) > len(p.data) {
			length = uint16(len(p.data) - dataPtr)
		}
		blockData := make([]byte, length)
		copy(blockData, p.data[dataPtr:dataPtr+int(length)])
		blocks = append(blocks, AYZ80Block{
			Addr: addr,
			Data: blockData,
		})
		offset += 6
	}
	return blocks, nil
}

func (p *ayZ80Parser) readU8(offset int) byte {
	if offset < 0 || offset >= len(p.data) {
		return 0
	}
	return p.data[offset]
}

func (p *ayZ80Parser) readU16(offset int) uint16 {
	if offset < 0 || offset+1 >= len(p.data) {
		return 0
	}
	return binary.BigEndian.Uint16(p.data[offset : offset+2])
}

func (p *ayZ80Parser) readI16(offset int) int16 {
	if offset < 0 || offset+1 >= len(p.data) {
		return 0
	}
	return int16(binary.BigEndian.Uint16(p.data[offset : offset+2]))
}

func (p *ayZ80Parser) readRequiredPointer(origin int) (int, error) {
	ptr, err := p.resolvePointer(origin, p.readI16(origin))
	if err != nil {
		return 0, err
	}
	if ptr == nil {
		return 0, fmt.Errorf("ay z80 missing pointer at %d", origin)
	}
	return *ptr, nil
}

func (p *ayZ80Parser) resolvePointer(origin int, rel int16) (*int, error) {
	if rel == 0 {
		return nil, nil
	}
	target := origin + int(rel)
	if target < 0 || target >= len(p.data) {
		return nil, fmt.Errorf("ay z80 pointer out of range")
	}
	return &target, nil
}

func (p *ayZ80Parser) resolveOptionalPointer(origin int, rel int16) (*int, error) {
	return p.resolvePointer(origin, rel)
}

func (p *ayZ80Parser) readStringPointer(origin int) (string, error) {
	ptr, err := p.resolvePointer(origin, p.readI16(origin))
	if err != nil {
		return "", err
	}
	if ptr == nil {
		return "", nil
	}
	return p.readNTString(*ptr)
}

func (p *ayZ80Parser) readNTString(start int) (string, error) {
	if start < 0 || start >= len(p.data) {
		return "", fmt.Errorf("ay z80 string offset out of range")
	}
	end := start
	for end < len(p.data) && p.data[end] != 0 {
		end++
	}
	if end >= len(p.data) {
		return "", fmt.Errorf("ay z80 unterminated string")
	}
	return string(p.data[start:end]), nil
}
