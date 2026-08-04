// Package ie64obj implements the frozen IE64 V3 relocatable object format.
package ie64obj

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	EMIE64 = 0x4945
	// EFIE64ABIV3 identifies the V3 compiler ABI. Earlier objects must be rebuilt.
	// The ELF machine value remains the IE64 machine value.
	EFIE64ABIV3  = 0x00000003
	RNone        = 0
	RRelative64  = 1
	RABS64       = 2
	RABS32       = 3
	RPC32        = 4
	RLO32        = 5
	RHI32        = 6
	SHNUndef     = 0
	SHNCommon    = 0xfff2
	SHTNull      = 0
	SHTProgBits  = 1
	SHTSymTab    = 2
	SHTStrTab    = 3
	SHTRela      = 4
	SHTNoBits    = 8
	SHFWrite     = 1
	SHFAlloc     = 2
	SHFExecInstr = 4
	STBLocal     = 0
	STBGlobal    = 1
	STBWeak      = 2
	STTNoType    = 0
	STTObject    = 1
	STTFunc      = 2
	STTSection   = 3
	STVDefault   = 0
	STVHidden    = 2
)

type Relocation struct {
	Offset uint64
	Symbol uint32 // One-based index into Object.Symbols.
	Type   uint32
	Addend int64
}

type Section struct {
	Name        string
	Type        uint32
	Flags       uint64
	Align       uint64
	Data        []byte
	Size        uint64
	Relocations []Relocation
}

type Symbol struct {
	Name       string
	Bind       uint8
	Type       uint8
	Visibility uint8
	Section    uint16 // One-based index into Object.Sections, SHNUndef or SHNCommon.
	Value      uint64
	Size       uint64
}

type Object struct {
	Flags    uint32
	Sections []Section
	Symbols  []Symbol
}

type stringTable struct {
	b   []byte
	off map[string]uint32
}

func newStringTable() *stringTable { return &stringTable{b: []byte{0}, off: map[string]uint32{"": 0}} }
func (s *stringTable) add(v string) uint32 {
	if off, ok := s.off[v]; ok {
		return off
	}
	off := uint32(len(s.b))
	s.b = append(s.b, v...)
	s.b = append(s.b, 0)
	s.off[v] = off
	return off
}

type outputSection struct {
	name                uint32
	typ                 uint32
	flags, offset, size uint64
	link, info          uint32
	align, entsize      uint64
	data                []byte
}

func alignUp(v, a uint64) uint64 {
	if a <= 1 {
		return v
	}
	return (v + a - 1) &^ (a - 1)
}

func validAlign(a uint64) bool { return a == 0 || a&(a-1) == 0 }

// Marshal returns a deterministic ELF64 little-endian ET_REL object.
func (o *Object) Marshal() ([]byte, error) {
	flags := o.Flags
	if flags == 0 {
		flags = EFIE64ABIV3
	}
	if flags != EFIE64ABIV3 {
		if flags == 0x00000002 {
			return nil, fmt.Errorf("stale IE64 compiler object: rebuild from source for ABI V3")
		}
		return nil, fmt.Errorf("unsupported IE64 ABI flags %#x", flags)
	}
	for i, s := range o.Sections {
		if s.Name == "" {
			return nil, fmt.Errorf("section %d has no name", i+1)
		}
		if s.Type != SHTProgBits && s.Type != SHTNoBits {
			return nil, fmt.Errorf("section %s has unsupported type %d", s.Name, s.Type)
		}
		if !validAlign(s.Align) {
			return nil, fmt.Errorf("section %s has invalid alignment %d", s.Name, s.Align)
		}
	}

	strtab, shstr := newStringTable(), newStringTable()
	outs := make([]outputSection, 1, len(o.Sections)*2+4)
	for _, s := range o.Sections {
		sz := uint64(len(s.Data))
		if s.Type == SHTNoBits && s.Size != 0 {
			sz = s.Size
		}
		outs = append(outs, outputSection{name: shstr.add(s.Name), typ: s.Type, flags: s.Flags, size: sz, align: max64(1, s.Align), data: append([]byte(nil), s.Data...)})
	}

	order := make([]int, 0, len(o.Symbols))
	for i, s := range o.Symbols {
		if s.Bind == STBLocal {
			order = append(order, i)
		}
	}
	localCount := len(order)
	for i, s := range o.Symbols {
		if s.Bind != STBLocal {
			order = append(order, i)
		}
	}
	oldToNew := make([]uint32, len(o.Symbols)+1)
	symData := make([]byte, 24*(len(o.Symbols)+1))
	for fileIndex, oldIndex := range order {
		s := o.Symbols[oldIndex]
		if s.Bind > STBWeak || s.Type > STTSection || (s.Visibility != STVDefault && s.Visibility != STVHidden) {
			return nil, fmt.Errorf("symbol %q has unsupported attributes", s.Name)
		}
		if s.Section != SHNUndef && s.Section != SHNCommon && (s.Section == 0 || int(s.Section) > len(o.Sections)) {
			return nil, fmt.Errorf("symbol %q has invalid section %d", s.Name, s.Section)
		}
		idx := fileIndex + 1
		oldToNew[oldIndex+1] = uint32(idx)
		p := symData[idx*24:]
		binary.LittleEndian.PutUint32(p, strtab.add(s.Name))
		p[4] = s.Bind<<4 | s.Type
		p[5] = s.Visibility
		binary.LittleEndian.PutUint16(p[6:], s.Section)
		binary.LittleEndian.PutUint64(p[8:], s.Value)
		binary.LittleEndian.PutUint64(p[16:], s.Size)
	}

	for sectionIndex, s := range o.Sections {
		if len(s.Relocations) == 0 {
			continue
		}
		data := make([]byte, len(s.Relocations)*24)
		for i, r := range s.Relocations {
			if r.Symbol == 0 || int(r.Symbol) > len(o.Symbols) {
				return nil, fmt.Errorf("relocation in %s has invalid symbol %d", s.Name, r.Symbol)
			}
			if r.Type > RHI32 {
				return nil, fmt.Errorf("relocation in %s has unsupported type %d", s.Name, r.Type)
			}
			p := data[i*24:]
			binary.LittleEndian.PutUint64(p, r.Offset)
			binary.LittleEndian.PutUint64(p[8:], uint64(oldToNew[r.Symbol])<<32|uint64(r.Type))
			binary.LittleEndian.PutUint64(p[16:], uint64(r.Addend))
		}
		outs = append(outs, outputSection{name: shstr.add(".rela" + s.Name), typ: SHTRela, info: uint32(sectionIndex + 1), align: 8, entsize: 24, data: data, size: uint64(len(data))})
	}
	symIndex := len(outs)
	outs = append(outs, outputSection{name: shstr.add(".symtab"), typ: SHTSymTab, info: uint32(localCount + 1), align: 8, entsize: 24, data: symData, size: uint64(len(symData))})
	strIndex := len(outs)
	outs = append(outs, outputSection{name: shstr.add(".strtab"), typ: SHTStrTab, align: 1, data: strtab.b, size: uint64(len(strtab.b))})
	shstrIndex := len(outs)
	shstr.add(".shstrtab")
	outs = append(outs, outputSection{name: shstr.off[".shstrtab"], typ: SHTStrTab, align: 1})
	outs[symIndex].link = uint32(strIndex)
	for i := range outs {
		if outs[i].typ == SHTRela {
			outs[i].link = uint32(symIndex)
		}
	}
	outs[shstrIndex].data = shstr.b
	outs[shstrIndex].size = uint64(len(shstr.b))

	offset := uint64(64)
	for i := 1; i < len(outs); i++ {
		offset = alignUp(offset, outs[i].align)
		outs[i].offset = offset
		if outs[i].typ != SHTNoBits {
			offset += uint64(len(outs[i].data))
		}
	}
	shoff := alignUp(offset, 8)
	if shoff > math.MaxInt || uint64(len(outs))*64 > math.MaxInt {
		return nil, fmt.Errorf("object too large")
	}
	buf := make([]byte, shoff+uint64(len(outs))*64)
	copy(buf[:16], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0})
	binary.LittleEndian.PutUint16(buf[16:], 1)
	binary.LittleEndian.PutUint16(buf[18:], EMIE64)
	binary.LittleEndian.PutUint32(buf[20:], 1)
	binary.LittleEndian.PutUint64(buf[40:], shoff)
	binary.LittleEndian.PutUint32(buf[48:], flags)
	binary.LittleEndian.PutUint16(buf[52:], 64)
	binary.LittleEndian.PutUint16(buf[58:], 64)
	binary.LittleEndian.PutUint16(buf[60:], uint16(len(outs)))
	binary.LittleEndian.PutUint16(buf[62:], uint16(shstrIndex))
	for i := 1; i < len(outs); i++ {
		if outs[i].typ != SHTNoBits {
			copy(buf[outs[i].offset:], outs[i].data)
		}
	}
	for i, s := range outs {
		p := buf[shoff+uint64(i)*64:]
		binary.LittleEndian.PutUint32(p, s.name)
		binary.LittleEndian.PutUint32(p[4:], s.typ)
		binary.LittleEndian.PutUint64(p[8:], s.flags)
		binary.LittleEndian.PutUint64(p[24:], s.offset)
		binary.LittleEndian.PutUint64(p[32:], s.size)
		binary.LittleEndian.PutUint32(p[40:], s.link)
		binary.LittleEndian.PutUint32(p[44:], s.info)
		binary.LittleEndian.PutUint64(p[48:], s.align)
		binary.LittleEndian.PutUint64(p[56:], s.entsize)
	}
	return buf, nil
}

func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

type rawSection struct {
	name, typ        uint32
	flags, off, size uint64
	link, info       uint32
	align, entsize   uint64
}

// Parse validates and decodes an IE64 V3 relocatable object.
func Parse(data []byte) (*Object, error) {
	if len(data) < 64 || !bytes.Equal(data[:4], []byte{0x7f, 'E', 'L', 'F'}) || data[4] != 2 || data[5] != 1 || binary.LittleEndian.Uint16(data[16:]) != 1 || binary.LittleEndian.Uint16(data[18:]) != EMIE64 {
		return nil, fmt.Errorf("not an IE64 ELF64 little-endian relocatable object")
	}
	flags := binary.LittleEndian.Uint32(data[48:])
	if flags != EFIE64ABIV3 {
		if flags == 0x00000002 {
			return nil, fmt.Errorf("stale IE64 compiler object: rebuild from source for ABI V3")
		}
		return nil, fmt.Errorf("unsupported IE64 ABI flags %#x", flags)
	}
	shoff := binary.LittleEndian.Uint64(data[40:])
	count := int(binary.LittleEndian.Uint16(data[60:]))
	shstrIndex := int(binary.LittleEndian.Uint16(data[62:]))
	if count == 0 || shstrIndex <= 0 || shstrIndex >= count || shoff+uint64(count)*64 > uint64(len(data)) {
		return nil, fmt.Errorf("invalid section table")
	}
	raw := make([]rawSection, count)
	for i := range raw {
		p := data[shoff+uint64(i)*64:]
		raw[i] = rawSection{binary.LittleEndian.Uint32(p), binary.LittleEndian.Uint32(p[4:]), binary.LittleEndian.Uint64(p[8:]), binary.LittleEndian.Uint64(p[24:]), binary.LittleEndian.Uint64(p[32:]), binary.LittleEndian.Uint32(p[40:]), binary.LittleEndian.Uint32(p[44:]), binary.LittleEndian.Uint64(p[48:]), binary.LittleEndian.Uint64(p[56:])}
	}
	sectionData := func(i int) ([]byte, error) {
		r := raw[i]
		if r.typ == SHTNoBits {
			return nil, nil
		}
		if r.off+r.size > uint64(len(data)) {
			return nil, fmt.Errorf("section %d outside file", i)
		}
		return data[r.off : r.off+r.size], nil
	}
	shstr, err := sectionData(shstrIndex)
	if err != nil {
		return nil, err
	}
	nameAt := func(tab []byte, off uint32) (string, error) {
		if int(off) >= len(tab) {
			return "", fmt.Errorf("string offset outside table")
		}
		end := bytes.IndexByte(tab[off:], 0)
		if end < 0 {
			return "", fmt.Errorf("unterminated string")
		}
		return string(tab[off:uint32(int(off)+end)]), nil
	}
	out := &Object{Flags: flags}
	inputMap := make(map[int]int)
	for i := 1; i < count; i++ {
		if raw[i].typ != SHTProgBits && raw[i].typ != SHTNoBits {
			continue
		}
		name, e := nameAt(shstr, raw[i].name)
		if e != nil {
			return nil, e
		}
		d, e := sectionData(i)
		if e != nil {
			return nil, e
		}
		inputMap[i] = len(out.Sections) + 1
		out.Sections = append(out.Sections, Section{Name: name, Type: raw[i].typ, Flags: raw[i].flags, Align: raw[i].align, Data: append([]byte(nil), d...), Size: raw[i].size})
	}
	symtab := -1
	for i := 1; i < count; i++ {
		if raw[i].typ == SHTSymTab {
			if symtab != -1 {
				return nil, fmt.Errorf("multiple symbol tables")
			}
			symtab = i
		}
	}
	if symtab >= 0 {
		r := raw[symtab]
		if r.entsize != 24 || r.size%24 != 0 || int(r.link) >= count {
			return nil, fmt.Errorf("invalid symbol table")
		}
		sd, e := sectionData(symtab)
		if e != nil {
			return nil, e
		}
		names, e := sectionData(int(r.link))
		if e != nil {
			return nil, e
		}
		for off := 24; off < len(sd); off += 24 {
			p := sd[off:]
			name, e := nameAt(names, binary.LittleEndian.Uint32(p))
			if e != nil {
				return nil, e
			}
			sec := binary.LittleEndian.Uint16(p[6:])
			if mapped, ok := inputMap[int(sec)]; ok {
				sec = uint16(mapped)
			}
			out.Symbols = append(out.Symbols, Symbol{Name: name, Bind: p[4] >> 4, Type: p[4] & 15, Visibility: p[5] & 3, Section: sec, Value: binary.LittleEndian.Uint64(p[8:]), Size: binary.LittleEndian.Uint64(p[16:])})
		}
	}
	for i := 1; i < count; i++ {
		r := raw[i]
		if r.typ != SHTRela {
			continue
		}
		mapped, ok := inputMap[int(r.info)]
		if !ok || int(r.link) != symtab || r.entsize != 24 || r.size%24 != 0 {
			return nil, fmt.Errorf("invalid relocation section")
		}
		rd, e := sectionData(i)
		if e != nil {
			return nil, e
		}
		for off := 0; off < len(rd); off += 24 {
			p := rd[off:]
			info := binary.LittleEndian.Uint64(p[8:])
			out.Sections[mapped-1].Relocations = append(out.Sections[mapped-1].Relocations, Relocation{Offset: binary.LittleEndian.Uint64(p), Symbol: uint32(info >> 32), Type: uint32(info), Addend: int64(binary.LittleEndian.Uint64(p[16:]))})
		}
	}
	return out, nil
}
