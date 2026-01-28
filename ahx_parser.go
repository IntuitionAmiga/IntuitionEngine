// ahx_parser.go - AHX (Abyss' Highest eXperience) module parser
// Reference: ahxformat.txt and AHX-Sources/AHX.cpp

package main

import (
	"errors"
	"fmt"
)

// AHX data structures

// AHXFile represents a parsed AHX module
type AHXFile struct {
	Name            string
	Revision        int // 0 = AHX0, 1 = AHX1
	SpeedMultiplier int // 1-4 (50/100/150/200 Hz)
	Restart         int
	PositionNr      int
	TrackLength     int
	TrackNr         int
	InstrumentNr    int
	SubsongNr       int
	Positions       []AHXPosition
	Tracks          [][]AHXStep // [trackNum][rowNum]
	Instruments     []AHXInstrument
	Subsongs        []int
}

// AHXPosition represents a position entry in the song
type AHXPosition struct {
	Track     [4]int
	Transpose [4]int8
}

// AHXStep represents a single track row (note/instrument/effect)
type AHXStep struct {
	Note       int // 0-60 (0 = no note)
	Instrument int // 0-63
	FX         int // 0-15
	FXParam    int // 0-255
}

// AHXEnvelope represents ADSR envelope settings
type AHXEnvelope struct {
	AFrames int // Attack frames
	AVolume int // Attack volume (0-64)
	DFrames int // Decay frames
	DVolume int // Decay volume (0-64)
	SFrames int // Sustain frames
	RFrames int // Release frames
	RVolume int // Release volume (0-64)
}

// AHXPListEntry represents a playlist entry for instrument modulation
type AHXPListEntry struct {
	Note     int
	Fixed    int
	Waveform int
	FX       [2]int
	FXParam  [2]int
}

// AHXPList represents the instrument playlist
type AHXPList struct {
	Speed   int
	Length  int
	Entries []AHXPListEntry
}

// AHXInstrument represents an instrument definition
type AHXInstrument struct {
	Name                 string
	Volume               int // 0-64
	WaveLength           int // 0-5 (shifts: 0=$04, 1=$08, 2=$10, 3=$20, 4=$40, 5=$80)
	Envelope             AHXEnvelope
	FilterLowerLimit     int
	FilterUpperLimit     int
	FilterSpeed          int
	SquareLowerLimit     int
	SquareUpperLimit     int
	SquareSpeed          int
	VibratoDelay         int
	VibratoDepth         int
	VibratoSpeed         int
	HardCutRelease       int
	HardCutReleaseFrames int
	PList                AHXPList
}

// ParseAHX parses an AHX module from binary data
func ParseAHX(data []byte) (*AHXFile, error) {
	if len(data) < 14 {
		return nil, errors.New("AHX: data too short for header")
	}

	// Check magic bytes: "THX" followed by revision (0 or 1)
	if data[0] != 'T' || data[1] != 'H' || data[2] != 'X' {
		return nil, errors.New("AHX: invalid magic (expected THX)")
	}

	revision := int(data[3])
	if revision > 1 {
		return nil, fmt.Errorf("AHX: unsupported revision %d (max 1)", revision)
	}

	song := &AHXFile{
		Revision: revision,
	}

	// Parse header
	// Bytes 4-5: Name offset (big-endian) - we'll use this later
	nameOffset := int(data[4])<<8 | int(data[5])

	// Byte 6: bit 7 = track 0 NOT saved (if set, skip track 0), bits 6-5 = speed multiplier (AHX1 only), bits 3-0 = high nibble of PositionNr
	// Byte 7: low byte of PositionNr
	// NOTE: The spec says bit 7 = 1 means "track 0 is saved", but the C++ reference code
	// treats bit 7 = 1 as "skip track 0 (it's empty and not in file)". We follow the C++ behavior.
	byte6 := data[6]
	track0NotSaved := (byte6 & 0x80) != 0

	// Speed multiplier is ((byte6 >> 5) & 3) + 1
	// For AHX0, it's always 1 (50Hz)
	if revision == 0 {
		song.SpeedMultiplier = 1
	} else {
		song.SpeedMultiplier = int((byte6>>5)&3) + 1
	}

	// PositionNr: 12 bits from bytes 6-7, masked with 0xFFF
	song.PositionNr = int(byte6&0x0F)<<8 | int(data[7])

	// Bytes 8-9: Restart position (big-endian)
	song.Restart = int(data[8])<<8 | int(data[9])

	// Byte 10: TrackLength
	song.TrackLength = int(data[10])

	// Byte 11: TrackNr (number of tracks saved, not including track 0 unless track0Saved)
	song.TrackNr = int(data[11])

	// Byte 12: InstrumentNr
	song.InstrumentNr = int(data[12])

	// Byte 13: SubsongNr
	song.SubsongNr = int(data[13])

	// Current read position
	pos := 14

	// Parse subsong list
	song.Subsongs = make([]int, song.SubsongNr)
	for i := 0; i < song.SubsongNr; i++ {
		if pos+2 > len(data) {
			return nil, errors.New("AHX: unexpected end of data in subsong list")
		}
		song.Subsongs[i] = int(data[pos])<<8 | int(data[pos+1])
		pos += 2
	}

	// Parse position list
	song.Positions = make([]AHXPosition, song.PositionNr)
	for i := 0; i < song.PositionNr; i++ {
		if pos+8 > len(data) {
			return nil, errors.New("AHX: unexpected end of data in position list")
		}
		for j := 0; j < 4; j++ {
			song.Positions[i].Track[j] = int(data[pos])
			song.Positions[i].Transpose[j] = int8(data[pos+1])
			pos += 2
		}
	}

	// Parse tracks
	// TrackNr is the number of tracks saved in the file
	// Positions can reference tracks 0 to TrackNr, so we need TrackNr+1 slots
	song.Tracks = make([][]AHXStep, song.TrackNr+1)

	// Initialize all track slots
	for i := 0; i <= song.TrackNr; i++ {
		song.Tracks[i] = make([]AHXStep, song.TrackLength)
	}

	// Read tracks from file - matching C++ reference behavior:
	// - Loop from 0 to TrackNr (TrackNr+1 total slots)
	// - If track0NotSaved is true AND we're at track 0, skip reading (keep zeros)
	// - Otherwise read track data from file
	for i := 0; i <= song.TrackNr; i++ {
		if track0NotSaved && i == 0 {
			// Track 0 is empty and not saved in file, skip reading
			continue
		}
		for j := 0; j < song.TrackLength; j++ {
			if pos+3 > len(data) {
				return nil, errors.New("AHX: unexpected end of data in track data")
			}
			song.Tracks[i][j] = unpackAHXStep(data[pos : pos+3])
			pos += 3
		}
	}

	// Parse instruments
	// Instruments are 1-indexed (instrument 0 is unused)
	song.Instruments = make([]AHXInstrument, song.InstrumentNr+1)

	// We need to read names from the name offset area
	// First, read all instrument data, then read names
	namePtr := nameOffset

	// Read song name
	if namePtr < len(data) {
		song.Name = readNullString(data, namePtr)
		namePtr += len(song.Name) + 1
	}

	// Parse each instrument
	for i := 1; i <= song.InstrumentNr; i++ {
		if pos+22 > len(data) {
			return nil, errors.New("AHX: unexpected end of data in instrument data")
		}

		inst := &song.Instruments[i]

		// Read instrument name from names section
		if namePtr < len(data) {
			inst.Name = readNullString(data, namePtr)
			namePtr += len(inst.Name) + 1
		}

		// Parse instrument data (22 bytes minimum)
		inst.Volume = int(data[pos])

		// Byte 1: bits 7-3 = filter speed bits 4-0, bits 2-0 = wavelength
		inst.FilterSpeed = int((data[pos+1] >> 3) & 0x1F)
		inst.WaveLength = int(data[pos+1] & 0x07)

		// Envelope
		inst.Envelope.AFrames = int(data[pos+2])
		inst.Envelope.AVolume = int(data[pos+3])
		inst.Envelope.DFrames = int(data[pos+4])
		inst.Envelope.DVolume = int(data[pos+5])
		inst.Envelope.SFrames = int(data[pos+6])
		inst.Envelope.RFrames = int(data[pos+7])
		inst.Envelope.RVolume = int(data[pos+8])

		// Bytes 9-11 are unused

		// Byte 12: bit 7 = filter speed bit 5, bits 6-0 = filter lower limit
		inst.FilterSpeed |= int((data[pos+12] >> 2) & 0x20)
		inst.FilterLowerLimit = int(data[pos+12] & 0x7F)

		// Byte 13: vibrato delay
		inst.VibratoDelay = int(data[pos+13])

		// Byte 14: bits 7 = hard cut release, bits 6-4 = hard cut frames, bits 3-0 = vibrato depth
		inst.HardCutRelease = int((data[pos+14] >> 7) & 1)
		inst.HardCutReleaseFrames = int((data[pos+14] >> 4) & 7)
		inst.VibratoDepth = int(data[pos+14] & 0x0F)

		// Byte 15: vibrato speed
		inst.VibratoSpeed = int(data[pos+15])

		// Byte 16: square lower limit
		inst.SquareLowerLimit = int(data[pos+16])

		// Byte 17: square upper limit
		inst.SquareUpperLimit = int(data[pos+17])

		// Byte 18: square speed
		inst.SquareSpeed = int(data[pos+18])

		// Byte 19: bit 7 = filter speed bit 6, bits 5-0 = filter upper limit
		inst.FilterSpeed |= int((data[pos+19] >> 1) & 0x40)
		inst.FilterUpperLimit = int(data[pos+19] & 0x3F)

		// Byte 20: playlist speed
		inst.PList.Speed = int(data[pos+20])

		// Byte 21: playlist length
		plistLen := int(data[pos+21])
		inst.PList.Length = plistLen

		pos += 22

		// Parse playlist entries
		inst.PList.Entries = make([]AHXPListEntry, plistLen)
		for j := 0; j < plistLen; j++ {
			if pos+4 > len(data) {
				return nil, errors.New("AHX: unexpected end of data in playlist")
			}
			inst.PList.Entries[j] = unpackAHXPListEntry(data[pos : pos+4])
			pos += 4
		}
	}

	return song, nil
}

// unpackAHXStep unpacks a 3-byte track row into an AHXStep
// Format:
//
//	byte 0: NNNNNNII (note bits 5-0 in bits 7-2, instrument bits 5-4 in bits 1-0)
//	byte 1: IIIIFFFF (instrument bits 3-0 in bits 7-4, fx bits 3-0 in bits 3-0)
//	byte 2: PPPPPPPP (fx param)
func unpackAHXStep(data []byte) AHXStep {
	return AHXStep{
		Note:       (int(data[0]) >> 2) & 0x3F,
		Instrument: ((int(data[0]) & 0x03) << 4) | (int(data[1]) >> 4),
		FX:         int(data[1]) & 0x0F,
		FXParam:    int(data[2]),
	}
}

// unpackAHXPListEntry unpacks a 4-byte playlist entry
// Format (32 bits, big-endian):
//
//	bits 31-29: FX2 command (0-7)
//	bits 28-26: FX1 command (0-7)
//	bits 25-23: Waveform (0-7)
//	bit 22: Fixed note flag
//	bits 21-16: Note (0-60)
//	bits 15-8: FX1 parameter
//	bits 7-0: FX2 parameter
func unpackAHXPListEntry(data []byte) AHXPListEntry {
	// Combine bytes into 32-bit value
	v := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])

	return AHXPListEntry{
		FX:       [2]int{int((v >> 26) & 7), int((v >> 29) & 7)},
		Waveform: int((v >> 23) & 7),
		Fixed:    int((v >> 22) & 1),
		Note:     int((v >> 16) & 0x3F),
		FXParam:  [2]int{int((v >> 8) & 0xFF), int(v & 0xFF)},
	}
}

// readNullString reads a null-terminated string from data starting at offset
func readNullString(data []byte, offset int) string {
	end := offset
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[offset:end])
}
