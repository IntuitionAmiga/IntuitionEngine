// ahx_test_helpers_test.go - Shared AHX test helpers.

package main

import "testing"

type ahxModuleOptions struct {
	Revision        int
	SpeedMultiplier int
	Restart         int
	PositionNr      int
	TrackLength     int
	TrackNr         int
	InstrumentNr    int
	Subsongs        []int
	Positions       []AHXPosition
	Tracks          [][]AHXStep
	Instruments     []AHXInstrument
	Name            string
}

func newTestAHXReplayer(t *testing.T) *AHXReplayer {
	t.Helper()
	r := NewAHXReplayer()
	song := &AHXFile{
		Revision:        1,
		PositionNr:      1,
		TrackLength:     1,
		TrackNr:         0,
		Restart:         0,
		Positions:       []AHXPosition{{Track: [4]int{0, 0, 0, 0}}},
		Tracks:          [][]AHXStep{{{}}},
		Instruments:     []AHXInstrument{{}},
		SpeedMultiplier: 1,
	}
	r.InitSong(song)
	if err := r.InitSubsong(0); err != nil {
		t.Fatalf("InitSubsong failed: %v", err)
	}
	return r
}

func buildAHXModule(opts ahxModuleOptions) []byte {
	if opts.PositionNr == 0 {
		opts.PositionNr = 1
	}
	if opts.TrackLength == 0 {
		opts.TrackLength = 1
	}
	if opts.Name == "" {
		opts.Name = "Test"
	}
	if opts.SpeedMultiplier == 0 {
		opts.SpeedMultiplier = 1
	}
	if opts.TrackNr == 0 && len(opts.Tracks) > 1 {
		opts.TrackNr = len(opts.Tracks) - 1
	}
	if opts.InstrumentNr == 0 && len(opts.Instruments) > 1 {
		opts.InstrumentNr = len(opts.Instruments) - 1
	}

	subsongNr := len(opts.Subsongs)
	body := make([]byte, 0)
	for _, subsong := range opts.Subsongs {
		body = append(body, byte(subsong>>8), byte(subsong))
	}
	for i := 0; i < opts.PositionNr; i++ {
		pos := AHXPosition{}
		if i < len(opts.Positions) {
			pos = opts.Positions[i]
		}
		for ch := range 4 {
			body = append(body, byte(pos.Track[ch]), byte(pos.Transpose[ch]))
		}
	}
	for tr := 0; tr <= opts.TrackNr; tr++ {
		for row := 0; row < opts.TrackLength; row++ {
			step := AHXStep{}
			if tr < len(opts.Tracks) && row < len(opts.Tracks[tr]) {
				step = opts.Tracks[tr][row]
			}
			body = append(body, packAHXStep(step)...)
		}
	}
	for instNr := 1; instNr <= opts.InstrumentNr; instNr++ {
		inst := AHXInstrument{Volume: 64}
		if instNr < len(opts.Instruments) {
			inst = opts.Instruments[instNr]
		}
		body = append(body, packAHXInstrumentHeader(inst)...)
		for _, entry := range inst.PList.Entries {
			body = append(body, packAHXPListEntry(entry)...)
		}
	}

	nameOffset := 14 + len(body)
	byte6 := byte((opts.PositionNr >> 8) & 0x0f)
	if opts.Revision > 0 {
		byte6 |= byte(((opts.SpeedMultiplier - 1) & 0x03) << 5)
	}
	data := []byte{
		'T', 'H', 'X', byte(opts.Revision),
		byte(nameOffset >> 8), byte(nameOffset),
		byte6, byte(opts.PositionNr),
		byte(opts.Restart >> 8), byte(opts.Restart),
		byte(opts.TrackLength),
		byte(opts.TrackNr),
		byte(opts.InstrumentNr),
		byte(subsongNr),
	}
	data = append(data, body...)
	data = append(data, []byte(opts.Name)...)
	data = append(data, 0)
	for instNr := 1; instNr <= opts.InstrumentNr; instNr++ {
		name := "Inst"
		if instNr < len(opts.Instruments) && opts.Instruments[instNr].Name != "" {
			name = opts.Instruments[instNr].Name
		}
		data = append(data, []byte(name)...)
		data = append(data, 0)
	}
	return data
}

func packAHXStep(step AHXStep) []byte {
	return []byte{
		byte((step.Note&0x3f)<<2 | (step.Instrument>>4)&0x03),
		byte((step.Instrument&0x0f)<<4 | step.FX&0x0f),
		byte(step.FXParam),
	}
}

func packAHXPListEntry(entry AHXPListEntry) []byte {
	v := uint32(entry.FX[1]&7)<<29 |
		uint32(entry.FX[0]&7)<<26 |
		uint32(entry.Waveform&7)<<23 |
		uint32(entry.Fixed&1)<<22 |
		uint32(entry.Note&0x3f)<<16 |
		uint32(entry.FXParam[0]&0xff)<<8 |
		uint32(entry.FXParam[1]&0xff)
	return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func packAHXInstrumentHeader(inst AHXInstrument) []byte {
	plistLen := inst.PList.Length
	if plistLen == 0 {
		plistLen = len(inst.PList.Entries)
	}
	return []byte{
		byte(inst.Volume),
		byte((inst.FilterSpeed&0x1f)<<3 | inst.WaveLength&0x07),
		byte(inst.Envelope.AFrames),
		byte(inst.Envelope.AVolume),
		byte(inst.Envelope.DFrames),
		byte(inst.Envelope.DVolume),
		byte(inst.Envelope.SFrames),
		byte(inst.Envelope.RFrames),
		byte(inst.Envelope.RVolume),
		0, 0, 0,
		byte(inst.FilterLowerLimit&0x7f) | byte((inst.FilterSpeed&0x20)<<2),
		byte(inst.VibratoDelay),
		byte((inst.HardCutRelease&1)<<7 | (inst.HardCutReleaseFrames&7)<<4 | inst.VibratoDepth&0x0f),
		byte(inst.VibratoSpeed),
		byte(inst.SquareLowerLimit),
		byte(inst.SquareUpperLimit),
		byte(inst.SquareSpeed),
		byte(inst.FilterUpperLimit&0x3f) | byte((inst.FilterSpeed&0x40)<<1),
		byte(inst.PList.Speed),
		byte(plistLen),
	}
}

func runAHXFrames(r *AHXReplayer, n int) {
	for range n {
		r.PlayIRQ()
	}
}
