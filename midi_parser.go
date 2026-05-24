package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

const midiDefaultTempoUSPerQuarter = 500000

type MIDIEventKind uint8

const (
	MIDIEventNoteOff MIDIEventKind = iota
	MIDIEventNoteOn
	MIDIEventProgramChange
	MIDIEventControlChange
	MIDIEventPitchBend
)

type MIDIEvent struct {
	Tick       uint32
	SampleTime int64
	Kind       MIDIEventKind
	Channel    uint8
	Note       uint8
	Velocity   uint8
	Program    uint8
	Controller uint8
	Value      int
}

type MIDITempoEvent struct {
	Tick       uint32
	SampleTime int64
	USPerQN    int
}

type MIDIFile struct {
	Format          uint16
	FormatName      string
	TrackCount      uint16
	Division        uint16
	Events          []MIDIEvent
	TempoEvents     []MIDITempoEvent
	DurationSamples int64
	Metadata        MusicMetadata
	PatchTableName  string
	IsMUS           bool
	raw             []byte
}

func (m *MIDIFile) GetMetadata() MusicMetadata { return m.Metadata }
func (m *MIDIFile) GetData() []byte            { return m.raw }

func (m *MIDIFile) TempoBPMAtSample(sample int64) int {
	us := midiDefaultTempoUSPerQuarter
	for _, ev := range m.TempoEvents {
		if ev.SampleTime > sample {
			break
		}
		us = ev.USPerQN
	}
	if us <= 0 {
		return 120
	}
	return int(math.Round(60000000.0 / float64(us)))
}

func ParseMIDIData(data []byte) (*MIDIFile, error) {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{'M', 'U', 'S', 0x1A}) {
		return parseMUSData(data)
	}
	return parseSMFData(data)
}

func parseSMFData(data []byte) (*MIDIFile, error) {
	if len(data) < 14 || string(data[:4]) != "MThd" {
		return nil, fmt.Errorf("midi: missing or truncated MThd header")
	}
	hdrLen := binary.BigEndian.Uint32(data[4:8])
	if hdrLen < 6 || int(8+hdrLen) > len(data) {
		return nil, fmt.Errorf("midi: invalid header length")
	}
	format := binary.BigEndian.Uint16(data[8:10])
	if format == 2 {
		return nil, fmt.Errorf("midi: SMF type 2 is unsupported")
	}
	tracks := binary.BigEndian.Uint16(data[10:12])
	division := binary.BigEndian.Uint16(data[12:14])
	if division&0x8000 != 0 {
		return nil, fmt.Errorf("midi: SMPTE division is unsupported")
	}
	file := &MIDIFile{
		Format:         format,
		FormatName:     fmt.Sprintf("SMF type %d", format),
		TrackCount:     tracks,
		Division:       division,
		PatchTableName: RawlandMiniPatchTableName,
		raw:            append([]byte(nil), data...),
	}

	pos := int(8 + hdrLen)
	var tempos []MIDITempoEvent
	for tr := 0; tr < int(tracks); tr++ {
		if pos+8 > len(data) || string(data[pos:pos+4]) != "MTrk" {
			return nil, fmt.Errorf("midi: missing MTrk chunk")
		}
		ln := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
		pos += 8
		if ln < 0 || pos+ln > len(data) {
			return nil, fmt.Errorf("midi: truncated MTrk chunk")
		}
		events, trackTempos, title, err := parseSMFTrack(data[pos : pos+ln])
		if err != nil {
			return nil, err
		}
		if file.Metadata.Title == "" && title != "" {
			file.Metadata.Title = title
		}
		file.Events = append(file.Events, events...)
		tempos = append(tempos, trackTempos...)
		pos += ln
	}
	if len(tempos) == 0 {
		tempos = append(tempos, MIDITempoEvent{USPerQN: midiDefaultTempoUSPerQuarter})
	}
	finalizeMIDITiming(file, tempos)
	file.Metadata.System = "MIDI"
	file.Metadata.Duration = float64(file.DurationSamples) / SAMPLE_RATE
	return file, nil
}

func parseSMFTrack(data []byte) ([]MIDIEvent, []MIDITempoEvent, string, error) {
	var events []MIDIEvent
	var tempos []MIDITempoEvent
	var title string
	var tick uint32
	var running byte
	for pos := 0; pos < len(data); {
		delta, n, err := readMIDIVarLen(data[pos:])
		if err != nil {
			return nil, nil, "", err
		}
		pos += n
		tick += delta
		if pos >= len(data) {
			return nil, nil, "", fmt.Errorf("midi: truncated event")
		}
		status := data[pos]
		if status >= 0x80 {
			pos++
			if status < 0xF0 {
				running = status
			}
		} else if running != 0 {
			status = running
		} else {
			return nil, nil, "", fmt.Errorf("midi: running status without prior status")
		}

		switch {
		case status == 0xFF:
			if pos >= len(data) {
				return nil, nil, "", fmt.Errorf("midi: truncated meta event")
			}
			meta := data[pos]
			pos++
			ln, used, err := readMIDIVarLen(data[pos:])
			if err != nil {
				return nil, nil, "", err
			}
			pos += used
			if pos+int(ln) > len(data) {
				return nil, nil, "", fmt.Errorf("midi: truncated meta payload")
			}
			payload := data[pos : pos+int(ln)]
			pos += int(ln)
			if meta == 0x2F {
				return events, tempos, title, nil
			}
			if meta == 0x03 && title == "" {
				title = string(payload)
			}
			if meta == 0x51 && len(payload) == 3 {
				tempos = append(tempos, MIDITempoEvent{Tick: tick, USPerQN: int(payload[0])<<16 | int(payload[1])<<8 | int(payload[2])})
			}
		case status == 0xF0 || status == 0xF7:
			ln, used, err := readMIDIVarLen(data[pos:])
			if err != nil {
				return nil, nil, "", err
			}
			pos += used + int(ln)
			if pos > len(data) {
				return nil, nil, "", fmt.Errorf("midi: truncated sysex")
			}
		default:
			kind := status & 0xF0
			ch := status & 0x0F
			need := 2
			if kind == 0xC0 || kind == 0xD0 {
				need = 1
			}
			if pos+need > len(data) {
				return nil, nil, "", fmt.Errorf("midi: truncated channel event")
			}
			a := data[pos]
			var b byte
			if need == 2 {
				b = data[pos+1]
			}
			pos += need
			switch kind {
			case 0x80:
				events = append(events, MIDIEvent{Tick: tick, Kind: MIDIEventNoteOff, Channel: ch, Note: a, Velocity: b})
			case 0x90:
				evKind := MIDIEventNoteOn
				if b == 0 {
					evKind = MIDIEventNoteOff
				}
				events = append(events, MIDIEvent{Tick: tick, Kind: evKind, Channel: ch, Note: a, Velocity: b})
			case 0xB0:
				if a == 7 || a == 11 {
					events = append(events, MIDIEvent{Tick: tick, Kind: MIDIEventControlChange, Channel: ch, Controller: a, Value: int(b)})
				}
			case 0xC0:
				events = append(events, MIDIEvent{Tick: tick, Kind: MIDIEventProgramChange, Channel: ch, Program: a})
			case 0xE0:
				events = append(events, MIDIEvent{Tick: tick, Kind: MIDIEventPitchBend, Channel: ch, Value: int(a) | int(b)<<7})
			}
		}
	}
	return events, tempos, title, nil
}

func finalizeMIDITiming(file *MIDIFile, tempos []MIDITempoEvent) {
	sort.SliceStable(file.Events, func(i, j int) bool { return file.Events[i].Tick < file.Events[j].Tick })
	sort.SliceStable(tempos, func(i, j int) bool { return tempos[i].Tick < tempos[j].Tick })
	if len(tempos) == 0 || tempos[0].Tick != 0 {
		tempos = append([]MIDITempoEvent{{USPerQN: midiDefaultTempoUSPerQuarter}}, tempos...)
	}
	tickToSample := func(tick uint32) int64 {
		var sample int64
		var lastTick uint32
		tempo := midiDefaultTempoUSPerQuarter
		for _, te := range tempos {
			if te.Tick > tick {
				break
			}
			sample += ticksToSamples(te.Tick-lastTick, tempo, int(file.Division))
			lastTick = te.Tick
			tempo = te.USPerQN
		}
		sample += ticksToSamples(tick-lastTick, tempo, int(file.Division))
		return sample
	}
	for i := range tempos {
		tempos[i].SampleTime = tickToSample(tempos[i].Tick)
	}
	for i := range file.Events {
		file.Events[i].SampleTime = tickToSample(file.Events[i].Tick)
		if file.Events[i].SampleTime > file.DurationSamples {
			file.DurationSamples = file.Events[i].SampleTime
		}
	}
	file.TempoEvents = tempos
}

func ticksToSamples(ticks uint32, tempoUS, division int) int64 {
	if ticks == 0 || division <= 0 {
		return 0
	}
	return int64(math.Round(float64(ticks) * float64(tempoUS) * float64(SAMPLE_RATE) / (1000000.0 * float64(division))))
}

func readMIDIVarLen(data []byte) (uint32, int, error) {
	var v uint32
	for i := 0; i < 4; i++ {
		if i >= len(data) {
			return 0, 0, fmt.Errorf("midi: truncated variable length value")
		}
		b := data[i]
		v = (v << 7) | uint32(b&0x7F)
		if b&0x80 == 0 {
			return v, i + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("midi: invalid variable length value")
}

func parseMUSData(data []byte) (*MIDIFile, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("mus: truncated header")
	}
	scoreLen := int(binary.LittleEndian.Uint16(data[4:6]))
	scoreStart := int(binary.LittleEndian.Uint16(data[6:8]))
	if scoreStart < 16 || scoreStart+scoreLen > len(data) {
		return nil, fmt.Errorf("mus: invalid score range")
	}
	file := &MIDIFile{
		FormatName:     "MUS",
		TrackCount:     1,
		Division:       140,
		PatchTableName: RawlandMiniPatchTableName,
		IsMUS:          true,
		raw:            append([]byte(nil), data...),
	}
	pos := scoreStart
	end := scoreStart + scoreLen
	var tick uint32
	lastVelocity := [16]uint8{}
	for i := range lastVelocity {
		lastVelocity[i] = 64
	}
	for pos < end {
		desc := data[pos]
		pos++
		typ := (desc >> 4) & 0x7
		ch := desc & 0x0F
		midiCh := ch
		switch ch {
		case 15:
			midiCh = 9
		case 9:
			midiCh = 15
		}
		switch typ {
		case 0:
			if pos >= end {
				return nil, fmt.Errorf("mus: truncated note off")
			}
			note := data[pos] & 0x7F
			pos++
			file.Events = append(file.Events, MIDIEvent{Tick: tick, Kind: MIDIEventNoteOff, Channel: midiCh, Note: note})
		case 1:
			if pos >= end {
				return nil, fmt.Errorf("mus: truncated note on")
			}
			noteByte := data[pos]
			pos++
			note := noteByte & 0x7F
			vel := lastVelocity[ch]
			if noteByte&0x80 != 0 {
				if pos >= end {
					return nil, fmt.Errorf("mus: truncated velocity")
				}
				vel = data[pos] & 0x7F
				lastVelocity[ch] = vel
				pos++
			}
			file.Events = append(file.Events, MIDIEvent{Tick: tick, Kind: MIDIEventNoteOn, Channel: midiCh, Note: note, Velocity: vel})
		case 2:
			if pos >= end {
				return nil, fmt.Errorf("mus: truncated pitch bend")
			}
			v := int(data[pos])
			pos++
			file.Events = append(file.Events, MIDIEvent{Tick: tick, Kind: MIDIEventPitchBend, Channel: midiCh, Value: v << 6})
		case 3:
			if pos >= end {
				return nil, fmt.Errorf("mus: truncated system event")
			}
			pos++
		case 4:
			if pos+2 > end {
				return nil, fmt.Errorf("mus: truncated controller event")
			}
			ctrl := data[pos]
			val := data[pos+1]
			pos += 2
			switch ctrl {
			case 0:
				file.Events = append(file.Events, MIDIEvent{Tick: tick, Kind: MIDIEventProgramChange, Channel: midiCh, Program: val})
			case 3:
				file.Events = append(file.Events, MIDIEvent{Tick: tick, Kind: MIDIEventControlChange, Channel: midiCh, Controller: 7, Value: int(val)})
			case 4:
				file.Events = append(file.Events, MIDIEvent{Tick: tick, Kind: MIDIEventControlChange, Channel: midiCh, Controller: 11, Value: int(val)})
			}
		case 6:
			pos = end
		default:
			return nil, fmt.Errorf("mus: unsupported event type %d", typ)
		}
		if desc&0x80 != 0 && pos < end {
			delta, used, err := readMIDIVarLen(data[pos:end])
			if err != nil {
				return nil, err
			}
			pos += used
			tick += delta
		}
	}
	for i := range file.Events {
		file.Events[i].SampleTime = int64(file.Events[i].Tick) * SAMPLE_RATE / 140
		if file.Events[i].SampleTime > file.DurationSamples {
			file.DurationSamples = file.Events[i].SampleTime
		}
	}
	file.TempoEvents = []MIDITempoEvent{{USPerQN: 428571, SampleTime: 0}}
	file.Metadata.System = "Doom MUS"
	file.Metadata.Duration = float64(file.DurationSamples) / SAMPLE_RATE
	return file, nil
}
