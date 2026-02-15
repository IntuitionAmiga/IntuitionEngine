package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

type SIDHeader struct {
	MagicID     string
	Version     uint16
	DataOffset  uint16
	LoadAddress uint16
	InitAddress uint16
	PlayAddress uint16
	Songs       uint16
	StartSong   uint16
	Speed       uint32
	Name        string
	Author      string
	Released    string
	Flags       uint16
	StartPage   uint8
	PageLength  uint8
	Sid2Addr    uint16
	Sid3Addr    uint16
	IsRSID      bool
}

type SIDFile struct {
	Header SIDHeader
	Data   []byte
}

func ParseSIDFile(path string) (*SIDFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSIDData(data)
}

func ParseSIDData(data []byte) (*SIDFile, error) {
	if len(data) < 0x76 {
		return nil, errors.New("SID data too short")
	}

	magic := string(data[:4])
	header := SIDHeader{
		MagicID: magic,
	}

	switch magic {
	case "PSID":
		header.IsRSID = false
	case "RSID":
		header.IsRSID = true
	default:
		return nil, fmt.Errorf("invalid SID magic: %q", magic)
	}

	header.Version = binary.BigEndian.Uint16(data[0x04:0x06])
	header.DataOffset = binary.BigEndian.Uint16(data[0x06:0x08])
	header.LoadAddress = binary.BigEndian.Uint16(data[0x08:0x0A])
	header.InitAddress = binary.BigEndian.Uint16(data[0x0A:0x0C])
	header.PlayAddress = binary.BigEndian.Uint16(data[0x0C:0x0E])
	header.Songs = binary.BigEndian.Uint16(data[0x0E:0x10])
	header.StartSong = binary.BigEndian.Uint16(data[0x10:0x12])
	header.Speed = binary.BigEndian.Uint32(data[0x12:0x16])
	header.Name = parsePaddedString(data[0x16:0x36])
	header.Author = parsePaddedString(data[0x36:0x56])
	header.Released = parsePaddedString(data[0x56:0x76])

	if header.DataOffset >= 0x78 && len(data) >= 0x78 {
		header.Flags = binary.BigEndian.Uint16(data[0x76:0x78])
	}
	if header.DataOffset >= 0x7C && len(data) >= 0x7C {
		header.StartPage = data[0x78]
		header.PageLength = data[0x79]
	}
	if header.DataOffset >= 0x80 && len(data) >= 0x80 {
		header.Sid2Addr = binary.BigEndian.Uint16(data[0x7C:0x7E])
		header.Sid3Addr = binary.BigEndian.Uint16(data[0x7E:0x80])
	}

	if header.Sid2Addr != 0 || header.Sid3Addr != 0 {
		return nil, errors.New("multi-SID files are not supported")
	}

	if header.DataOffset == 0 || int(header.DataOffset) > len(data) {
		return nil, fmt.Errorf("invalid data offset: 0x%04X", header.DataOffset)
	}

	dataStart := int(header.DataOffset)
	if header.LoadAddress == 0 {
		if dataStart+2 > len(data) {
			return nil, errors.New("SID data missing embedded load address")
		}
		header.LoadAddress = binary.LittleEndian.Uint16(data[dataStart : dataStart+2])
		dataStart += 2
	}

	if dataStart > len(data) {
		return nil, errors.New("SID data offset beyond file length")
	}

	// Allocate exact size instead of using append idiom
	sidData := make([]byte, len(data)-dataStart)
	copy(sidData, data[dataStart:])

	file := &SIDFile{
		Header: header,
		Data:   sidData,
	}

	return file, nil
}
