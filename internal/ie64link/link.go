// Package ie64link implements the fixed-layout IE64 V3 static linker.
package ie64link

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64archive"
	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

const (
	ProgStart = uint64(0x1000)
	StackLow  = uint64(0x8f000)
	StackTop  = uint64(0x9f000)
	HeapEnd   = StackLow
)

type Input struct {
	Name   string
	Object *ie64obj.Object
}
type Argument struct {
	Name    string
	Object  *ie64obj.Object
	Archive *ie64archive.Archive
}
type Options struct{ Entry string }
type Result struct {
	Image      []byte
	Symbols    map[string]uint64
	Map        string
	RuntimeEnd uint64
}

// ResolveArguments performs Unix archive extraction in command-line order.
func ResolveArguments(args []Argument) ([]Input, error) {
	var out []Input
	defined := map[string]bool{}
	undefined := map[string]bool{}
	add := func(in Input) {
		out = append(out, in)
		for _, s := range in.Object.Symbols {
			if s.Bind == ie64obj.STBLocal || s.Name == "" {
				continue
			}
			if s.Section == ie64obj.SHNUndef {
				if s.Bind != ie64obj.STBWeak && !defined[s.Name] {
					undefined[s.Name] = true
				}
			} else {
				defined[s.Name] = true
				delete(undefined, s.Name)
			}
		}
	}
	for _, arg := range args {
		if arg.Object != nil {
			add(Input{Name: arg.Name, Object: arg.Object})
			continue
		}
		if arg.Archive == nil {
			return nil, fmt.Errorf("%s: missing object or archive", arg.Name)
		}
		extracted := make([]bool, len(arg.Archive.Members))
		for {
			changed := false
			for i, m := range arg.Archive.Members {
				if extracted[i] {
					continue
				}
				obj, err := ie64obj.Parse(m.Data)
				if err != nil {
					return nil, fmt.Errorf("%s(%s): %w", arg.Name, m.Name, err)
				}
				needed := false
				for _, section := range obj.Sections {
					if strings.Trim(section.Name, "\"") == ".fini_array_onexit" {
						needed = true
						break
					}
				}
				for _, s := range obj.Symbols {
					if s.Section != ie64obj.SHNUndef && s.Bind != ie64obj.STBLocal && undefined[s.Name] {
						needed = true
						break
					}
				}
				if needed {
					extracted[i] = true
					add(Input{Name: fmt.Sprintf("%s(%s)", arg.Name, m.Name), Object: obj})
					changed = true
				}
			}
			if !changed {
				break
			}
		}
	}
	return out, nil
}

type placedSection struct {
	input      int
	section    int
	address    uint64
	fileOffset uint64
	size       uint64
	rank       int
	priority   int
}

func arrayPriority(name, prefix string) (int, error) {
	if name == prefix {
		return int(^uint(0) >> 1), nil
	}
	suffix := strings.TrimPrefix(name, prefix+".")
	if suffix == name || suffix == "" {
		return 0, fmt.Errorf("invalid array section %s", name)
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("array priority in %s is not decimal", name)
		}
	}
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, fmt.Errorf("invalid array priority in %s", name)
	}
	return n, nil
}

type definition struct {
	input   int
	symbol  int
	address uint64
	weak    bool
	common  bool
}

func sectionRank(name string, flags uint64, typ uint32) (int, error) {
	switch {
	case name == ".interrupt_vector":
		return 0, fmt.Errorf("unsupported input section %s", name)
	case name == ".text":
		return 0, nil
	case name == ".rodata":
		return 1, nil
	case name == ".preinit_array":
		return 2, nil
	case name == ".init_array" || strings.HasPrefix(name, ".init_array."):
		return 3, nil
	case name == ".fini_array" || strings.HasPrefix(name, ".fini_array."):
		return 4, nil
	case name == ".fini_array_onexit":
		return 5, nil
	case name == ".data":
		return 6, nil
	case name == ".bss":
		return 7, nil
	default:
		if flags&ie64obj.SHFAlloc == 0 {
			return 0, fmt.Errorf("input section %s is not allocatable", name)
		}
		if flags&ie64obj.SHFExecInstr != 0 {
			return 0, nil
		}
		if flags&ie64obj.SHFWrite != 0 {
			if typ == ie64obj.SHTNoBits {
				return 7, nil
			}
			return 6, nil
		}
		return 1, nil
	}
}

func alignUp(v, a uint64) uint64 {
	if a <= 1 {
		return v
	}
	return (v + a - 1) &^ (a - 1)
}

func Link(inputs []Input, opts Options) (*Result, error) {
	var placed []placedSection
	for ii, in := range inputs {
		if in.Object == nil {
			return nil, fmt.Errorf("%s: nil object", in.Name)
		}
		if in.Object.Flags != ie64obj.EFIE64ABIV3 {
			if in.Object.Flags == 0x00000002 {
				return nil, fmt.Errorf("%s: stale IE64 compiler object: rebuild from source for ABI V3", in.Name)
			}
			return nil, fmt.Errorf("%s: unsupported IE64 ABI flags %#x", in.Name, in.Object.Flags)
		}
		for si, s := range in.Object.Sections {
			sectionName := strings.Trim(s.Name, "\"")
			rank, err := sectionRank(sectionName, s.Flags, s.Type)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", in.Name, err)
			}
			size := uint64(len(s.Data))
			if s.Type == ie64obj.SHTNoBits {
				size = s.Size
			}
			priority := 0
			if strings.HasPrefix(sectionName, ".init_array") {
				priority, err = arrayPriority(sectionName, ".init_array")
				if err != nil {
					return nil, fmt.Errorf("%s: %w", in.Name, err)
				}
			}
			if strings.HasPrefix(sectionName, ".fini_array") && sectionName != ".fini_array_onexit" {
				priority, err = arrayPriority(sectionName, ".fini_array")
				if err != nil {
					return nil, fmt.Errorf("%s: %w", in.Name, err)
				}
			}
			if rank >= 2 && rank <= 5 {
				if size%8 != 0 {
					return nil, fmt.Errorf("%s: callable array section %s size %d is not divisible by eight", in.Name, s.Name, size)
				}
				if s.Align != 0 && s.Align != 1 && s.Align != 2 && s.Align != 4 && s.Align != 8 {
					return nil, fmt.Errorf("%s: callable array section %s has invalid alignment %d", in.Name, s.Name, s.Align)
				}
			}
			placed = append(placed, placedSection{input: ii, section: si, size: size, rank: rank, priority: priority})
		}
	}
	sort.SliceStable(placed, func(i, j int) bool {
		if placed[i].rank != placed[j].rank {
			return placed[i].rank < placed[j].rank
		}
		return placed[i].priority < placed[j].priority
	})
	cursor := ProgStart
	previousRank := -1
	for i := range placed {
		s := inputs[placed[i].input].Object.Sections[placed[i].section]
		a := s.Align
		if a == 0 {
			a = 1
		}
		if placed[i].rank >= 2 && placed[i].rank <= 5 {
			a = 1
			if (placed[i].rank == 2 || placed[i].rank == 3) && previousRank < 2 {
				a = 8
			}
			if (placed[i].rank == 4 || placed[i].rank == 5) && previousRank < 4 {
				a = 8
			}
		}
		cursor = alignUp(cursor, a)
		placed[i].address = cursor
		placed[i].fileOffset = cursor - ProgStart
		cursor += placed[i].size
		previousRank = placed[i].rank
	}
	if cursor > StackLow {
		return nil, fmt.Errorf("baremetal-low layout exceeds 0x%x by %d bytes", StackLow, cursor-StackLow)
	}
	fileEnd := ProgStart
	for _, p := range placed {
		if inputs[p.input].Object.Sections[p.section].Type != ie64obj.SHTNoBits && p.address+p.size > fileEnd {
			fileEnd = p.address + p.size
		}
	}
	placeByInput := make(map[[2]int]placedSection)
	for _, p := range placed {
		placeByInput[[2]int{p.input, p.section}] = p
	}

	defs := make(map[string]definition)
	unresolved := make(map[string]bool)
	type commonDef struct {
		input, symbol int
		size, align   uint64
		weak          bool
	}
	commons := make(map[string]commonDef)
	for ii, in := range inputs {
		for si, s := range in.Object.Symbols {
			if s.Bind == ie64obj.STBLocal {
				continue
			}
			if s.Section == ie64obj.SHNUndef {
				if s.Bind != ie64obj.STBWeak {
					unresolved[s.Name] = true
				}
				continue
			}
			if s.Section == ie64obj.SHNCommon {
				align := s.Value
				if align == 0 {
					align = 1
				}
				if align&(align-1) != 0 {
					return nil, fmt.Errorf("%s: common symbol %s has invalid alignment %d", in.Name, s.Name, align)
				}
				c := commonDef{input: ii, symbol: si, size: s.Size, align: align, weak: s.Bind == ie64obj.STBWeak}
				if old, ok := commons[s.Name]; !ok || c.size > old.size || (c.size == old.size && c.align > old.align) {
					commons[s.Name] = c
				}
				continue
			}
			p, ok := placeByInput[[2]int{ii, int(s.Section) - 1}]
			if !ok {
				return nil, fmt.Errorf("%s: symbol %s has invalid section", in.Name, s.Name)
			}
			if s.Value > p.size || s.Size > p.size-s.Value {
				return nil, fmt.Errorf("%s: symbol %s range is outside section", in.Name, s.Name)
			}
			d := definition{input: ii, symbol: si, address: p.address + s.Value, weak: s.Bind == ie64obj.STBWeak}
			if old, ok := defs[s.Name]; ok {
				if !old.weak && !d.weak {
					return nil, fmt.Errorf("duplicate strong symbol %s in %s and %s", s.Name, inputs[old.input].Name, in.Name)
				}
				if old.weak && !d.weak {
					defs[s.Name] = d
				}
			} else {
				defs[s.Name] = d
			}
			delete(unresolved, s.Name)
		}
	}
	commonStart := cursor
	commonNames := make([]string, 0, len(commons))
	for name := range commons {
		if _, ok := defs[name]; !ok {
			commonNames = append(commonNames, name)
		}
	}
	sort.Strings(commonNames)
	for _, name := range commonNames {
		c := commons[name]
		cursor = alignUp(cursor, c.align)
		defs[name] = definition{input: c.input, symbol: c.symbol, address: cursor, weak: c.weak, common: true}
		delete(unresolved, name)
		cursor += c.size
	}
	if cursor > StackLow {
		return nil, fmt.Errorf("baremetal-low layout exceeds 0x%x by %d bytes", StackLow, cursor-StackLow)
	}
	boundaryNow := func(rank int, end bool) uint64 {
		v := cursor
		found := false
		for _, p := range placed {
			if p.rank == rank {
				at := p.address
				if end {
					at += p.size
				}
				if !found || !end && at < v || end && at > v {
					v = at
				}
				found = true
			}
		}
		if !found {
			return firstAddressAfter(placed, rank, cursor)
		}
		return v
	}
	arrayNow := func(first, last int) (uint64, uint64, bool) {
		start := firstAddressAfter(placed, first-1, cursor)
		end := start
		found := false
		for _, p := range placed {
			if p.rank >= first && p.rank <= last {
				if !found {
					start = p.address
					found = true
				}
				end = p.address + p.size
			}
		}
		return start, end, found
	}
	preStartNow, preEndNow, _ := arrayNow(2, 2)
	initStartNow, initEndNow, hasInit := arrayNow(3, 3)
	if preEndNow != preStartNow {
		initStartNow = preEndNow
		if !hasInit {
			initEndNow = preEndNow
		}
	} else {
		preStartNow = initStartNow
		preEndNow = initStartNow
	}
	finiStartNow, finiEndNow, _ := arrayNow(4, 5)
	linked := map[string]uint64{"__image_start": ProgStart, "__text_start": boundaryNow(0, false), "__text_end": boundaryNow(0, true), "__rodata_start": boundaryNow(1, false), "__rodata_end": boundaryNow(1, true), "__data_start": boundaryNow(6, false), "__data_end": boundaryNow(6, true), "__bss_start": boundaryNow(7, false), "__bss_end": boundaryNow(7, true), "__heap_start": cursor, "__heap_end": HeapEnd, "__stack_bottom": StackLow, "__stack_top": StackTop, "__preinit_array_start": preStartNow, "__preinit_array_end": preEndNow, "__init_array_start": initStartNow, "__init_array_end": initEndNow, "__bothinit_array_start": preStartNow, "__bothinit_array_end": initEndNow, "__fini_array_start": finiStartNow, "__fini_array_end": finiEndNow}
	linked["__image_end"] = fileEnd
	linked["__ie64_heap_start"] = cursor
	linked["__ie64_heap_end"] = HeapEnd
	if len(commonNames) > 0 {
		linked["__bss_end"] = cursor
		if linked["__bss_start"] > commonStart {
			linked["__bss_start"] = commonStart
		}
	}
	for name, address := range linked {
		if old, ok := defs[name]; ok {
			return nil, fmt.Errorf("symbol %s conflicts with linker-defined symbol in %s", name, inputs[old.input].Name)
		}
		defs[name] = definition{input: -1, symbol: -1, address: address}
		delete(unresolved, name)
	}
	for name := range unresolved {
		if _, ok := defs[name]; !ok {
			return nil, fmt.Errorf("undefined symbol %s", name)
		}
	}

	image := make([]byte, fileEnd-ProgStart)
	for _, p := range placed {
		s := inputs[p.input].Object.Sections[p.section]
		if s.Type != ie64obj.SHTNoBits {
			copy(image[p.fileOffset:], s.Data)
		}
	}
	resolve := func(ii int, symIndex uint32) (uint64, error) {
		if symIndex == 0 || int(symIndex) > len(inputs[ii].Object.Symbols) {
			return 0, fmt.Errorf("invalid relocation symbol %d", symIndex)
		}
		s := inputs[ii].Object.Symbols[symIndex-1]
		if s.Bind == ie64obj.STBLocal && s.Section != ie64obj.SHNUndef {
			p := placeByInput[[2]int{ii, int(s.Section) - 1}]
			return p.address + s.Value, nil
		}
		d, ok := defs[s.Name]
		if !ok {
			if s.Bind == ie64obj.STBWeak {
				return 0, nil
			}
			return 0, fmt.Errorf("undefined symbol %s", s.Name)
		}
		return d.address, nil
	}
	for _, p := range placed {
		s := inputs[p.input].Object.Sections[p.section]
		for _, r := range s.Relocations {
			var width uint64
			switch r.Type {
			case ie64obj.RNone:
				width = 0
			case ie64obj.RRelative64:
				width = 8
			case ie64obj.RABS32:
				width = 4
			case ie64obj.RABS64, ie64obj.RPC32, ie64obj.RLO32, ie64obj.RHI32:
				width = 8
			default:
				return nil, fmt.Errorf("unsupported relocation type %d", r.Type)
			}
			if r.Offset > p.size || width > p.size-r.Offset {
				return nil, fmt.Errorf("%s: relocation outside section %s", inputs[p.input].Name, s.Name)
			}
			P := p.address + r.Offset
			target := P - ProgStart
			if r.Type == ie64obj.RRelative64 {
				if r.Symbol != 0 {
					return nil, fmt.Errorf("RELATIVE64 relocation has nonzero symbol index")
				}
				if r.Addend < -int64(ProgStart) || r.Addend > math.MaxInt64-int64(ProgStart) {
					return nil, fmt.Errorf("RELATIVE64 relocation overflow")
				}
				binary.LittleEndian.PutUint64(image[target:], uint64(int64(ProgStart)+r.Addend))
				continue
			}
			S, err := resolve(p.input, r.Symbol)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", inputs[p.input].Name, err)
			}
			value := int64(S) + r.Addend
			switch r.Type {
			case ie64obj.RNone:
				continue
			case ie64obj.RABS64:
				if target+8 > uint64(len(image)) {
					return nil, fmt.Errorf("ABS64 relocation outside file data")
				}
				binary.LittleEndian.PutUint64(image[target:], uint64(value))
			case ie64obj.RABS32:
				if value < 0 || uint64(value) > math.MaxUint32 {
					return nil, fmt.Errorf("ABS32 relocation overflow for %s", inputs[p.input].Object.Symbols[r.Symbol-1].Name)
				}
				if target+4 > uint64(len(image)) {
					return nil, fmt.Errorf("ABS32 relocation outside file data")
				}
				binary.LittleEndian.PutUint32(image[target:], uint32(value))
			case ie64obj.RPC32:
				disp := value - int64(P)
				if disp < math.MinInt32 || disp > math.MaxInt32 {
					return nil, fmt.Errorf("PC32 relocation overflow")
				}
				if target+8 > uint64(len(image)) {
					return nil, fmt.Errorf("PC32 relocation outside file data")
				}
				binary.LittleEndian.PutUint32(image[target+4:], uint32(int32(disp)))
			case ie64obj.RLO32, ie64obj.RHI32:
				if target+8 > uint64(len(image)) {
					return nil, fmt.Errorf("split relocation outside file data")
				}
				v := uint64(value)
				if r.Type == ie64obj.RHI32 {
					v >>= 32
				}
				binary.LittleEndian.PutUint32(image[target+4:], uint32(v))
			}
		}
	}

	symbols := map[string]uint64{}
	for n, d := range defs {
		symbols[n] = d.address
	}
	boundary := func(name string, rank int, end bool) uint64 {
		v := cursor
		if !end {
			v = cursor
		}
		found := false
		for _, p := range placed {
			if p.rank == rank {
				at := p.address
				if end {
					at += p.size
				}
				if !found || (!end && at < v) || (end && at > v) {
					v = at
				}
				found = true
			}
		}
		if !found {
			return firstAddressAfter(placed, rank, cursor)
		}
		return v
	}
	symbols["__text_start"] = boundary("", 0, false)
	symbols["__text_end"] = boundary("", 0, true)
	symbols["__rodata_start"] = boundary("", 1, false)
	symbols["__rodata_end"] = boundary("", 1, true)
	symbols["__data_start"] = boundary("", 6, false)
	symbols["__data_end"] = boundary("", 6, true)
	symbols["__bss_start"] = boundary("", 7, false)
	symbols["__bss_end"] = boundary("", 7, true)
	if len(commonNames) > 0 {
		if symbols["__bss_start"] > commonStart {
			symbols["__bss_start"] = commonStart
		}
		symbols["__bss_end"] = cursor
	}
	symbols["__heap_start"] = cursor
	symbols["__heap_end"] = HeapEnd
	symbols["__stack_bottom"] = StackLow
	symbols["__stack_top"] = StackTop
	symbols["__image_start"] = ProgStart
	symbols["__image_end"] = fileEnd
	arrayBoundary := func(first, last int) (uint64, uint64, bool) {
		start := firstAddressAfter(placed, first-1, cursor)
		end := start
		found := false
		for _, p := range placed {
			if p.rank >= first && p.rank <= last {
				if !found {
					start = p.address
					found = true
				}
				end = p.address + p.size
			}
		}
		return start, end, found
	}
	preStart, preEnd, _ := arrayBoundary(2, 2)
	initStart, initEnd, hasInit := arrayBoundary(3, 3)
	if preEnd != preStart {
		initStart = preEnd
		if !hasInit {
			initEnd = preEnd
		}
	} else {
		preStart = initStart
		preEnd = initStart
	}
	bothStart := preStart
	bothEnd := initEnd
	finiStart, finiEnd, _ := arrayBoundary(4, 5)
	symbols["__preinit_array_start"] = preStart
	symbols["__preinit_array_end"] = preEnd
	symbols["__init_array_start"] = initStart
	symbols["__init_array_end"] = initEnd
	symbols["__bothinit_array_start"] = bothStart
	symbols["__bothinit_array_end"] = bothEnd
	symbols["__fini_array_start"] = finiStart
	symbols["__fini_array_end"] = finiEnd
	entry := opts.Entry
	if entry == "" {
		entry = "_start"
	}
	entryAddr, ok := symbols[entry]
	if !ok {
		return nil, fmt.Errorf("entry symbol %s is undefined", entry)
	}
	if entryAddr != ProgStart {
		return nil, fmt.Errorf("entry symbol %s must resolve to 0x%x, got 0x%x", entry, ProgStart, entryAddr)
	}
	var mb strings.Builder
	fmt.Fprintf(&mb, "IE64 baremetal-low map\nentry %s 0x%016x\n", entry, entryAddr)
	names := make([]string, 0, len(symbols))
	for n := range symbols {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(&mb, "0x%016x %s\n", symbols[n], n)
	}
	return &Result{Image: image, Symbols: symbols, Map: mb.String(), RuntimeEnd: cursor}, nil
}

func firstAddressAfter(placed []placedSection, rank int, fallback uint64) uint64 {
	for _, p := range placed {
		if p.rank > rank {
			return p.address
		}
	}
	return fallback
}
