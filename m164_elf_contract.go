package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	m164ELFTypeDyn      = 3
	m164SHTRela         = 4
	m164RelRelative64   = 1
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
	if eShentsize != m164ELFSectionHdrSz || eShnum == 0 || eShstrndx == 0 || eShstrndx >= eShnum {
		return nil, 0, fmt.Errorf("missing bounded section header table")
	}
	phEnd, ok := m14CheckedAdd(ePhoff, uint64(ePhnum)*uint64(ePhentsize))
	if !ok || ePhoff < m14ELFHeaderSize || phEnd > uint64(len(image)) {
		return nil, 0, fmt.Errorf("program header table out of range")
	}
	shEnd, ok := m14CheckedAdd(eShoff, uint64(eShnum)*uint64(eShentsize))
	if !ok || eShoff < m14ELFHeaderSize || shEnd > uint64(len(image)) {
		return nil, 0, fmt.Errorf("section header table out of range")
	}

	var segs []m164LoadSegment
	lowest := uint64(math.MaxUint64)
	var entryCovered bool
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
		case m14ELFPTDynamic, m14ELFPTInterp, m14ELFPTTLS, m14ELFPTNote:
			return nil, 0, fmt.Errorf("unsupported dynamic/auxiliary program header type %d", ph.Type)
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

	f, err := elf.NewFile(bytes.NewReader(image))
	if err != nil {
		return nil, 0, fmt.Errorf("parse sections: %w", err)
	}
	if f.Section(m164IOSMSectionName) == nil {
		return nil, 0, fmt.Errorf("missing %s", m164IOSMSectionName)
	}
	for _, sec := range f.Sections {
		if sec.Type != elf.SHT_RELA {
			continue
		}
		if sec.Entsize != 0 && sec.Entsize != m164ELFRelaEntSize {
			return nil, 0, fmt.Errorf("bad RELA entsize")
		}
		if sec.Size%m164ELFRelaEntSize != 0 {
			return nil, 0, fmt.Errorf("malformed RELA size")
		}
		count := sec.Size / m164ELFRelaEntSize
		if count > m164MaxRelaRecords {
			return nil, 0, fmt.Errorf("too many RELA records")
		}
		data, err := sec.Data()
		if err != nil {
			return nil, 0, fmt.Errorf("read RELA: %w", err)
		}
		for i := uint64(0); i < count; i++ {
			rec := data[i*m164ELFRelaEntSize:]
			rOff := binary.LittleEndian.Uint64(rec[0:8])
			rInfo := binary.LittleEndian.Uint64(rec[8:16])
			rAddend := int64(binary.LittleEndian.Uint64(rec[16:24]))
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
				if rOff+8 > uint64(len(mapped)) {
					return nil, 0, fmt.Errorf("mapped relocation target out of range")
				}
				binary.LittleEndian.PutUint64(mapped[rOff:rOff+8], val)
			}
		}
	}
	return segs, placement.Base + eEntry, nil
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
