package main

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	m164ELFTypeDyn      = 3
	m164SHTRela         = 4
	m164RelRelative64   = 1
	m164IOSRelMagic     = 0x52534F49
	m164IOSRelNoteType  = 0x494F5231
	m164ModfCompatPort  = 0x00000002
	m164ModfASLRCapable = 0x00000004
	m164RuntimeBaseMin  = m14ELFUserBase
	m164RuntimeBaseMax  = m14ELFUserLimit
	m164MaxRelaRecords  = 65536
	m164ELFRelaEntSize  = 24
	m164ELFSectionHdrSz = 64
	m164IOSMSectionName = ".ios.manifest"
)

type m164Placement struct {
	Base uint64
}

type m164LoadSegment struct {
	Vaddr  uint64
	Memsz  uint64
	Filesz uint64
	Offset uint64
	Flags  uint32
}

type m164RelaRecord struct {
	Offset uint64
	Info   uint64
	Addend int64
}

func validateM164RuntimeELFContract(image []byte, placement m164Placement) error {
	_, _, err := m164LoadRuntimeELF(image, placement, nil)
	return err
}

func m164LoadRuntimeELF(image []byte, placement m164Placement, mapped []byte) ([]m164LoadSegment, uint64, error) {
	if len(image) < m14ELFHeaderSize {
		return nil, 0, fmt.Errorf("file too small for ELF64 header")
	}
	if string(image[:4]) != "\x7fELF" {
		return nil, 0, fmt.Errorf("bad ELF magic")
	}
	if image[4] != m14ELFClass64 || image[5] != m14ELFDataLSB || image[6] != m14ELFVersion || image[7] != m14ELFOSABISysV {
		return nil, 0, fmt.Errorf("unsupported ELF ident")
	}
	eType := binary.LittleEndian.Uint16(image[16:18])
	eMachine := binary.LittleEndian.Uint16(image[18:20])
	eVersion := binary.LittleEndian.Uint32(image[20:24])
	eEntry := binary.LittleEndian.Uint64(image[24:32])
	ePhoff := binary.LittleEndian.Uint64(image[32:40])
	eShoff := binary.LittleEndian.Uint64(image[40:48])
	eEhsize := binary.LittleEndian.Uint16(image[52:54])
	ePhentsize := binary.LittleEndian.Uint16(image[54:56])
	ePhnum := binary.LittleEndian.Uint16(image[56:58])
	eShentsize := binary.LittleEndian.Uint16(image[58:60])
	eShnum := binary.LittleEndian.Uint16(image[60:62])
	eShstrndx := binary.LittleEndian.Uint16(image[62:64])

	if eType != m164ELFTypeDyn {
		return nil, 0, fmt.Errorf("unsupported ELF type %d", eType)
	}
	if eMachine != m14ELFMachineIE64 || eVersion != m14ELFVersion {
		return nil, 0, fmt.Errorf("unsupported ELF machine/version")
	}
	if eEhsize != m14ELFHeaderSize || ePhentsize != m14ELFProgHdrSize || ePhnum == 0 {
		return nil, 0, fmt.Errorf("bad ELF header sizes")
	}
	phEnd, ok := m14CheckedAdd(ePhoff, uint64(ePhnum)*uint64(ePhentsize))
	if !ok || ePhoff < m14ELFHeaderSize || phEnd > uint64(len(image)) {
		return nil, 0, fmt.Errorf("program header table out of range")
	}
	if eShoff != 0 || eShentsize != 0 || eShnum != 0 || eShstrndx != 0 {
		return nil, 0, fmt.Errorf("section headers are rejected by M16.4.1 runtime contract")
	}

	var segs []m164LoadSegment
	lowest := uint64(math.MaxUint64)
	var entryCovered bool
	var note []byte
	for i := uint16(0); i < ePhnum; i++ {
		off := ePhoff + uint64(i)*uint64(ePhentsize)
		ph := m14ELFProgramHeader{
			Type:   binary.LittleEndian.Uint32(image[off : off+4]),
			Flags:  binary.LittleEndian.Uint32(image[off+4 : off+8]),
			Offset: binary.LittleEndian.Uint64(image[off+8 : off+16]),
			Vaddr:  binary.LittleEndian.Uint64(image[off+16 : off+24]),
			Paddr:  binary.LittleEndian.Uint64(image[off+24 : off+32]),
			Filesz: binary.LittleEndian.Uint64(image[off+32 : off+40]),
			Memsz:  binary.LittleEndian.Uint64(image[off+40 : off+48]),
			Align:  binary.LittleEndian.Uint64(image[off+48 : off+56]),
		}
		switch ph.Type {
		case m14ELFPTLoad:
		case m14ELFPTNote:
			if note != nil {
				return nil, 0, fmt.Errorf("multiple PT_NOTE program headers")
			}
			noteEnd, ok := m14CheckedAdd(ph.Offset, ph.Filesz)
			if !ok || noteEnd > uint64(len(image)) {
				return nil, 0, fmt.Errorf("PT_NOTE file range out of bounds")
			}
			note = image[ph.Offset:noteEnd]
			continue
		case m14ELFPTDynamic, m14ELFPTInterp, m14ELFPTTLS:
			return nil, 0, fmt.Errorf("dynamic-linker program header rejected: %d", ph.Type)
		default:
			return nil, 0, fmt.Errorf("unsupported program header type %d", ph.Type)
		}
		if ph.Align != m14ELFPageAlign || ph.Vaddr%m14ELFPageAlign != 0 || ph.Offset%m14ELFPageAlign != ph.Vaddr%m14ELFPageAlign {
			return nil, 0, fmt.Errorf("bad segment alignment")
		}
		if ph.Filesz > ph.Memsz || ph.Memsz == 0 {
			return nil, 0, fmt.Errorf("bad segment sizes")
		}
		fileEnd, ok := m14CheckedAdd(ph.Offset, ph.Filesz)
		if !ok || fileEnd > uint64(len(image)) {
			return nil, 0, fmt.Errorf("segment file range out of bounds")
		}
		if ph.Flags&^uint32(m14ELFSegFlagR|m14ELFSegFlagW|m14ELFSegFlagX) != 0 || ph.Flags == 0 || ph.Flags == m14ELFSegFlagW || ph.Flags&(m14ELFSegFlagW|m14ELFSegFlagX) == (m14ELFSegFlagW|m14ELFSegFlagX) {
			return nil, 0, fmt.Errorf("bad segment flags")
		}
		segEnd, ok := m14CheckedAdd(ph.Vaddr, ph.Memsz)
		if !ok {
			return nil, 0, fmt.Errorf("segment range overflow")
		}
		placedEnd, ok := m14CheckedAdd(placement.Base, segEnd)
		if !ok || placement.Base+ph.Vaddr < m164RuntimeBaseMin || placedEnd > m164RuntimeBaseMax {
			return nil, 0, fmt.Errorf("placed segment outside user image space")
		}
		for _, prev := range segs {
			prevEnd, _ := m14CheckedAdd(prev.Vaddr, prev.Memsz)
			if ph.Vaddr < prevEnd && prev.Vaddr < segEnd {
				return nil, 0, fmt.Errorf("overlapping PT_LOAD segments")
			}
		}
		if ph.Vaddr < lowest {
			lowest = ph.Vaddr
		}
		if eEntry >= ph.Vaddr && eEntry < segEnd {
			if ph.Flags&m14ELFSegFlagX == 0 {
				return nil, 0, fmt.Errorf("entry point is not in executable segment")
			}
			entryCovered = true
		}
		segs = append(segs, m164LoadSegment{Vaddr: ph.Vaddr, Memsz: ph.Memsz, Filesz: ph.Filesz, Offset: ph.Offset, Flags: ph.Flags})
	}
	if lowest != 0 {
		return nil, 0, fmt.Errorf("lowest PT_LOAD p_vaddr is 0x%X, want zero", lowest)
	}
	if !entryCovered {
		return nil, 0, fmt.Errorf("entry point not covered by executable PT_LOAD")
	}
	if len(segs) < 2 {
		return nil, 0, fmt.Errorf("runtime ELF needs at least two PT_LOAD segments")
	}
	if note == nil {
		return nil, 0, fmt.Errorf("missing IOS metadata PT_NOTE")
	}
	relocs, err := m164ParseIOSNotes(note)
	if err != nil {
		return nil, 0, err
	}
	for _, rel := range relocs {
		rOff := rel.Offset
		rInfo := rel.Info
		rAddend := rel.Addend
		if rOff%8 != 0 {
			return nil, 0, fmt.Errorf("unaligned relocation target")
		}
		if uint32(rInfo) != m164RelRelative64 {
			return nil, 0, fmt.Errorf("unknown relocation type %d", uint32(rInfo))
		}
		if rInfo>>32 != 0 {
			return nil, 0, fmt.Errorf("external-symbol relocation rejected")
		}
		if rAddend < 0 {
			return nil, 0, fmt.Errorf("negative relocation addend")
		}
		if !m164RangeInSegment(segs, rOff, 8, true, false) {
			return nil, 0, fmt.Errorf("relocation target outside writable non-executable segment")
		}
		add := uint64(rAddend)
		if !m164RangeInSegment(segs, add, 1, false, false) {
			return nil, 0, fmt.Errorf("relocation addend outside loaded segment")
		}
		val, ok := m14CheckedAdd(placement.Base, add)
		if !ok {
			return nil, 0, fmt.Errorf("relocation arithmetic overflow")
		}
		if mapped != nil {
			if rOff > uint64(len(mapped)) || uint64(len(mapped))-rOff < 8 {
				return nil, 0, fmt.Errorf("mapped relocation target out of range")
			}
			binary.LittleEndian.PutUint64(mapped[rOff:rOff+8], val)
		}
	}
	return segs, placement.Base + eEntry, nil
}

func m164ParseIOSNotes(note []byte) ([]m164RelaRecord, error) {
	var sawMod, sawRel bool
	var relocs []m164RelaRecord
	for off := uint64(0); off < uint64(len(note)); {
		if uint64(len(note))-off < 12 {
			return nil, fmt.Errorf("malformed note header")
		}
		namesz := uint64(binary.LittleEndian.Uint32(note[off : off+4]))
		descsz := uint64(binary.LittleEndian.Uint32(note[off+4 : off+8]))
		typ := binary.LittleEndian.Uint32(note[off+8 : off+12])
		namePad, ok := m164NotePad(namesz)
		if !ok {
			return nil, fmt.Errorf("note name padding overflow")
		}
		descPad, ok := m164NotePad(descsz)
		if !ok {
			return nil, fmt.Errorf("note desc padding overflow")
		}
		nameStart, ok := m14CheckedAdd(off, 12)
		if !ok {
			return nil, fmt.Errorf("note pointer overflow")
		}
		descStart, ok := m14CheckedAdd(nameStart, namePad)
		if !ok {
			return nil, fmt.Errorf("note pointer overflow")
		}
		next, ok := m14CheckedAdd(descStart, descPad)
		if !ok || next > uint64(len(note)) || nameStart+namesz > uint64(len(note)) || descStart+descsz > uint64(len(note)) {
			return nil, fmt.Errorf("note entry out of bounds")
		}
		nameBytes := note[nameStart : nameStart+namesz]
		desc := note[descStart : descStart+descsz]
		switch string(nameBytes) {
		case "IOS-MOD\x00":
			if typ != 0x494F5331 {
				return nil, fmt.Errorf("unknown IOS-MOD note type")
			}
			if sawMod {
				return nil, fmt.Errorf("duplicate IOS-MOD note")
			}
			if err := m164ValidateIOSM(desc); err != nil {
				return nil, err
			}
			sawMod = true
		case "IOS-REL\x00":
			if typ != m164IOSRelNoteType {
				return nil, fmt.Errorf("unknown IOS-REL note type")
			}
			if sawRel {
				return nil, fmt.Errorf("duplicate IOS-REL note")
			}
			parsed, err := m164ParseIOSRel(desc)
			if err != nil {
				return nil, err
			}
			relocs = parsed
			sawRel = true
		default:
			return nil, fmt.Errorf("unknown IOS note %q", string(nameBytes))
		}
		off = next
	}
	if !sawMod {
		return nil, fmt.Errorf("missing IOS-MOD note")
	}
	return relocs, nil
}

func m164NotePad(v uint64) (uint64, bool) {
	if v > math.MaxUint64-3 {
		return 0, false
	}
	return (v + 3) &^ uint64(3), true
}

func m164ValidateIOSM(desc []byte) error {
	if len(desc) != 128 {
		return fmt.Errorf("bad IOSM descriptor size")
	}
	if binary.LittleEndian.Uint32(desc[0:4]) != 0x4D534F49 {
		return fmt.Errorf("bad IOSM magic")
	}
	if binary.LittleEndian.Uint32(desc[4:8]) != 1 {
		return fmt.Errorf("bad IOSM schema")
	}
	if desc[9] != 0 {
		return fmt.Errorf("bad IOSM reserved0")
	}
	for _, b := range desc[120:128] {
		if b != 0 {
			return fmt.Errorf("bad IOSM reserved2")
		}
	}
	kind := desc[8]
	flags := binary.LittleEndian.Uint32(desc[48:52])
	switch kind {
	case 1, 2, 3, 4:
		if flags != m164ModfCompatPort|m164ModfASLRCapable {
			return fmt.Errorf("bad IOSM service flags")
		}
	case 5:
		if flags != m164ModfASLRCapable {
			return fmt.Errorf("bad IOSM command flags")
		}
	default:
		return fmt.Errorf("bad IOSM kind")
	}
	return nil
}

func m164ParseIOSRel(desc []byte) ([]m164RelaRecord, error) {
	if len(desc) < 32 {
		return nil, fmt.Errorf("short IOS-REL descriptor")
	}
	if binary.LittleEndian.Uint32(desc[0:4]) != m164IOSRelMagic {
		return nil, fmt.Errorf("bad IOS-REL magic")
	}
	if binary.LittleEndian.Uint16(desc[4:6]) != 1 || binary.LittleEndian.Uint16(desc[6:8]) != 32 || binary.LittleEndian.Uint16(desc[8:10]) != m164ELFRelaEntSize {
		return nil, fmt.Errorf("bad IOS-REL header")
	}
	if binary.LittleEndian.Uint16(desc[10:12]) != 0 || binary.LittleEndian.Uint64(desc[16:24]) != 0 || binary.LittleEndian.Uint64(desc[24:32]) != 0 {
		return nil, fmt.Errorf("bad IOS-REL reserved fields")
	}
	count := uint64(binary.LittleEndian.Uint32(desc[12:16]))
	if count > m164MaxRelaRecords {
		return nil, fmt.Errorf("too many IOS-REL records")
	}
	recordsSize := count * m164ELFRelaEntSize
	want, ok := m14CheckedAdd(32, recordsSize)
	if !ok || want != uint64(len(desc)) {
		return nil, fmt.Errorf("bad IOS-REL descriptor size")
	}
	relocs := make([]m164RelaRecord, 0, count)
	for i := uint64(0); i < count; i++ {
		off := 32 + i*m164ELFRelaEntSize
		relocs = append(relocs, m164RelaRecord{
			Offset: binary.LittleEndian.Uint64(desc[off : off+8]),
			Info:   binary.LittleEndian.Uint64(desc[off+8 : off+16]),
			Addend: int64(binary.LittleEndian.Uint64(desc[off+16 : off+24])),
		})
	}
	return relocs, nil
}

func m164RangeInSegment(segs []m164LoadSegment, off, size uint64, requireWritable, requireExecutable bool) bool {
	end, ok := m14CheckedAdd(off, size)
	if !ok {
		return false
	}
	for _, seg := range segs {
		segEnd, ok := m14CheckedAdd(seg.Vaddr, seg.Memsz)
		if !ok || off < seg.Vaddr || end > segEnd {
			continue
		}
		if requireWritable && seg.Flags&m14ELFSegFlagW == 0 {
			return false
		}
		if !requireExecutable && seg.Flags&m14ELFSegFlagX != 0 && requireWritable {
			return false
		}
		if requireExecutable && seg.Flags&m14ELFSegFlagX == 0 {
			return false
		}
		return true
	}
	return false
}
