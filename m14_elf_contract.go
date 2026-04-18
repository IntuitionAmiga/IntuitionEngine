package main

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	m14ELFClass64     = 2
	m14ELFDataLSB     = 1
	m14ELFVersion     = 1
	m14ELFOSABISysV   = 0
	m14ELFTypeExec    = 2
	m14ELFMachineIE64 = 0x4945
	m14ELFHeaderSize  = 64
	m14ELFProgHdrSize = 56
	m14ELFPTLoad      = 1
	m14ELFPTInterp    = 3
	m14ELFPTNote      = 4
	m14ELFPTPHdr      = 6
	m14ELFPTTLS       = 7
	m14ELFPTDynamic   = 2
	m14ELFSegFlagX    = 0x1
	m14ELFSegFlagW    = 0x2
	m14ELFSegFlagR    = 0x4
	m14ELFPageAlign   = 0x1000
	m14ELFUserBase    = 0x00600000
	m14ELFUserLimit   = 0x02000000
)

type m14ELFProgramHeader struct {
	Type   uint32
	Flags  uint32
	Offset uint64
	Vaddr  uint64
	Paddr  uint64
	Filesz uint64
	Memsz  uint64
	Align  uint64
}

func m14CheckedAdd(a, b uint64) (uint64, bool) {
	if a > math.MaxUint64-b {
		return 0, false
	}
	return a + b, true
}

// validateM14ELFContract freezes the phase-1 IntuitionOS ELF subset for
// DOS-loaded commands/apps. It is host-side contract logic used by tests and
// docs until the in-OS dos.library loader lands in later M14 phases.
func validateM14ELFContract(image []byte) error {
	if len(image) < m14ELFHeaderSize {
		return fmt.Errorf("file too small for ELF64 header")
	}
	if string(image[:4]) != "\x7fELF" {
		return fmt.Errorf("bad ELF magic")
	}
	if image[4] != m14ELFClass64 {
		return fmt.Errorf("unsupported ELF class %d", image[4])
	}
	if image[5] != m14ELFDataLSB {
		return fmt.Errorf("unsupported ELF endianness %d", image[5])
	}
	if image[6] != m14ELFVersion {
		return fmt.Errorf("unsupported ELF ident version %d", image[6])
	}
	if image[7] != m14ELFOSABISysV {
		return fmt.Errorf("unsupported ELF OSABI %d", image[7])
	}

	eType := binary.LittleEndian.Uint16(image[16:18])
	eMachine := binary.LittleEndian.Uint16(image[18:20])
	eVersion := binary.LittleEndian.Uint32(image[20:24])
	eEntry := binary.LittleEndian.Uint64(image[24:32])
	ePhoff := binary.LittleEndian.Uint64(image[32:40])
	eEhsize := binary.LittleEndian.Uint16(image[52:54])
	ePhentsize := binary.LittleEndian.Uint16(image[54:56])
	ePhnum := binary.LittleEndian.Uint16(image[56:58])

	if eType != m14ELFTypeExec {
		return fmt.Errorf("unsupported ELF type %d", eType)
	}
	if eMachine != m14ELFMachineIE64 {
		return fmt.Errorf("unsupported ELF machine 0x%X", eMachine)
	}
	if eVersion != m14ELFVersion {
		return fmt.Errorf("unsupported ELF header version %d", eVersion)
	}
	if eEntry == 0 {
		return fmt.Errorf("entry point is zero")
	}
	if eEhsize != m14ELFHeaderSize {
		return fmt.Errorf("unexpected ELF header size %d", eEhsize)
	}
	if ePhentsize != m14ELFProgHdrSize {
		return fmt.Errorf("unexpected program header size %d", ePhentsize)
	}
	if ePhnum == 0 {
		return fmt.Errorf("no program headers")
	}
	phBytes := uint64(ePhentsize) * uint64(ePhnum)
	phEnd, ok := m14CheckedAdd(ePhoff, phBytes)
	if !ok || ePhoff < m14ELFHeaderSize || phEnd > uint64(len(image)) {
		return fmt.Errorf("program header table out of range")
	}

	headers := make([]m14ELFProgramHeader, 0, ePhnum)
	var entryCovered bool
	for i := uint16(0); i < ePhnum; i++ {
		off := ePhoff + uint64(i)*uint64(ePhentsize)
		offEnd, ok := m14CheckedAdd(off, uint64(ePhentsize))
		if !ok || offEnd > uint64(len(image)) {
			return fmt.Errorf("program header entry out of range")
		}
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
			return fmt.Errorf("unsupported dynamic/auxiliary program header type %d", ph.Type)
		default:
			return fmt.Errorf("unsupported program header type %d", ph.Type)
		}
		if ph.Align != m14ELFPageAlign {
			return fmt.Errorf("segment align 0x%X is not page-sized", ph.Align)
		}
		if ph.Vaddr%m14ELFPageAlign != 0 {
			return fmt.Errorf("segment vaddr 0x%X not page-aligned", ph.Vaddr)
		}
		if ph.Offset%m14ELFPageAlign != ph.Vaddr%m14ELFPageAlign {
			return fmt.Errorf("segment file/va alignment mismatch")
		}
		if ph.Filesz > ph.Memsz {
			return fmt.Errorf("segment filesz exceeds memsz")
		}
		fileEnd, ok := m14CheckedAdd(ph.Offset, ph.Filesz)
		if !ok || fileEnd > uint64(len(image)) {
			return fmt.Errorf("segment file range out of bounds")
		}
		if ph.Memsz == 0 {
			return fmt.Errorf("segment memsz is zero")
		}
		if ph.Flags&^uint32(m14ELFSegFlagR|m14ELFSegFlagW|m14ELFSegFlagX) != 0 {
			return fmt.Errorf("segment flags 0x%X unsupported", ph.Flags)
		}
		if ph.Flags == 0 {
			return fmt.Errorf("segment must request at least one permission")
		}
		if ph.Flags == m14ELFSegFlagW {
			return fmt.Errorf("write-only segment rejected")
		}
		if ph.Flags&(m14ELFSegFlagW|m14ELFSegFlagX) == (m14ELFSegFlagW | m14ELFSegFlagX) {
			return fmt.Errorf("writable+executable segment rejected")
		}
		segEnd, ok := m14CheckedAdd(ph.Vaddr, ph.Memsz)
		if !ok || ph.Vaddr < m14ELFUserBase || segEnd > m14ELFUserLimit {
			return fmt.Errorf("segment outside user image space")
		}

		for _, prev := range headers {
			prevEnd, ok := m14CheckedAdd(prev.Vaddr, prev.Memsz)
			if !ok {
				return fmt.Errorf("previous segment range overflow")
			}
			if ph.Vaddr < prevEnd && prev.Vaddr < segEnd {
				return fmt.Errorf("overlapping PT_LOAD segments")
			}
		}
		if eEntry >= ph.Vaddr && eEntry < segEnd {
			if ph.Flags&m14ELFSegFlagX == 0 {
				return fmt.Errorf("entry point is not in executable segment")
			}
			entryCovered = true
		}
		headers = append(headers, ph)
	}
	if !entryCovered {
		return fmt.Errorf("entry point not covered by executable PT_LOAD")
	}
	return nil
}
