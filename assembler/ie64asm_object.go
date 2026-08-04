//go:build ie64

package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

type objectSymbolDecl struct {
	name, section         string
	bind, typ, visibility uint8
	size                  uint64
	sizeEnd               uint64
	sizeFromEnd           bool
	sizeEndLabel          string
	commonAlign           uint64
	defined               bool
}
type pendingObjectReloc struct {
	section, symbol string
	marker          string
	offset          uint64
	typ             uint32
	addend          int64
}
type objectSectionSource struct {
	name         string
	typ          uint32
	flags, align uint64
	lines        []string
}

func parseObjectSectionDirective(rest string) (string, string, error) {
	parts := splitOperands(rest)
	if len(parts) == 0 {
		return "", "", fmt.Errorf(".section requires a name")
	}
	if len(parts) > 2 {
		return "", "", fmt.Errorf(".section accepts only a name and flags")
	}
	name := strings.Trim(strings.TrimSpace(parts[0]), "\"")
	if name == "" {
		return "", "", fmt.Errorf(".section requires a name")
	}
	flags := ""
	if len(parts) == 2 {
		flags = strings.Trim(strings.TrimSpace(parts[1]), "\"")
	}
	return name, flags, nil
}

func objectSectionFlags(spec string) (uint64, error) {
	var flags uint64
	for _, flag := range spec {
		switch flag {
		case 'a':
			flags |= ie64obj.SHFAlloc
		case 'w':
			flags |= ie64obj.SHFWrite
		case 'x':
			flags |= ie64obj.SHFExecInstr
		default:
			return 0, fmt.Errorf("unsupported section flag %q", flag)
		}
	}
	return flags, nil
}

// AssembleIE64Object assembles one source unit as an IE64 V3 ELF relocatable object.
func AssembleIE64Object(source, sourceName string, includePaths []string, defines map[string]uint64) ([]byte, error) {
	probe := NewIE64Assembler()
	probe.basePath = filepath.Dir(sourceName)
	probe.includePaths = includePaths
	for n, v := range defines {
		probe.Predefine(n, v)
	}
	lines, err := probe.preprocess(source, probe.basePath, nil)
	if err != nil {
		return nil, err
	}
	explicitLocals := map[string]bool{}
	for _, raw := range lines {
		clean := strings.TrimSpace(stripComment(raw))
		fields := strings.Fields(clean)
		if len(fields) >= 2 && strings.EqualFold(fields[0], ".local") {
			for _, name := range splitOperands(strings.TrimSpace(clean[len(fields[0]):])) {
				explicitLocals[strings.TrimSpace(name)] = true
			}
		}
	}
	explicitLocalDefinitions := map[string][]string{}
	preSection := ".text"
	preGlobals := map[string]string{}
	preSectionIDs := map[string]int{".text": 1}
	nextPreSectionID := 1
	for lineNo, raw := range lines {
		clean := strings.TrimSpace(stripComment(raw))
		fields := strings.Fields(clean)
		if len(fields) == 0 {
			continue
		}
		head := strings.ToLower(fields[0])
		if strings.HasPrefix(head, ".section") {
			rest := strings.TrimSpace(clean[len(fields[0]):])
			name, _, err := parseObjectSectionDirective(rest)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %v", sourceName, lineNo+1, err)
			}
			preSection = name
			if _, ok := preSectionIDs[preSection]; !ok {
				nextPreSectionID++
				preSectionIDs[preSection] = nextPreSectionID
			}
			continue
		}
		if head == ".text" || head == ".rodata" || head == ".data" || head == ".bss" {
			preSection = head
			if _, ok := preSectionIDs[preSection]; !ok {
				nextPreSectionID++
				preSectionIDs[preSection] = nextPreSectionID
			}
			continue
		}
		if !strings.HasSuffix(fields[0], ":") {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		if strings.HasPrefix(name, ".") {
			if explicitLocals[name] {
				scope := preGlobals[preSection]
				if scope == "" {
					scope = fmt.Sprintf("__ie64_object_scope_%d", preSectionIDs[preSection])
				}
				explicitLocalDefinitions[name] = append(explicitLocalDefinitions[name], scope+name)
			}
		} else if !strings.HasPrefix(name, "__m68kto64_") {
			preGlobals[preSection] = name
		}
	}
	sections := []*objectSectionSource{{name: ".text", typ: ie64obj.SHTProgBits, flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, align: 8}}
	byName := map[string]*objectSectionSource{".text": sections[0]}
	current := sections[0]
	sectionGlobals := map[string]string{}
	sectionFallbacks := map[string]string{".text": "__ie64_object_scope_1"}
	decls := map[string]*objectSymbolDecl{}
	asmLabels := map[string]string{}
	var order []string
	var pending []pendingObjectReloc
	relocID := 0
	scopeID := 1
	decl := func(name string) *objectSymbolDecl {
		name = strings.TrimSpace(name)
		d := decls[name]
		if d == nil {
			d = &objectSymbolDecl{name: name, bind: ie64obj.STBLocal, typ: ie64obj.STTNoType, visibility: ie64obj.STVDefault}
			decls[name] = d
			order = append(order, name)
		}
		return d
	}
	canonicalSymbol := func(name string) string {
		name = strings.TrimSpace(name)
		if definitions := explicitLocalDefinitions[name]; len(definitions) == 1 {
			return definitions[0]
		}
		if strings.HasPrefix(name, ".") && sectionGlobals[current.name] == "" {
			sectionGlobals[current.name] = sectionFallbacks[current.name]
			current.lines = append(current.lines, sectionGlobals[current.name]+":")
		}
		if strings.HasPrefix(name, ".") {
			return sectionGlobals[current.name] + name
		}
		return name
	}
	sectionFor := func(name, sectionFlagSpec string) (*objectSectionSource, error) {
		if s := byName[name]; s != nil {
			if sectionFlagSpec == "" {
				return s, nil
			}
			flags, err := objectSectionFlags(sectionFlagSpec)
			if err != nil {
				return nil, err
			}
			if s.flags != flags {
				return nil, fmt.Errorf("section %s has conflicting flags", name)
			}
			return s, nil
		}
		s := &objectSectionSource{name: name, align: 1}
		switch name {
		case ".text":
			s.typ = ie64obj.SHTProgBits
			s.flags = ie64obj.SHFAlloc | ie64obj.SHFExecInstr
			s.align = 8
		case ".rodata":
			s.typ = ie64obj.SHTProgBits
			s.flags = ie64obj.SHFAlloc
		case ".data", ".preinit_array", ".init_array", ".fini_array", ".fini_array_onexit":
			s.typ = ie64obj.SHTProgBits
			s.flags = ie64obj.SHFAlloc | ie64obj.SHFWrite
		case ".bss":
			s.typ = ie64obj.SHTNoBits
			s.flags = ie64obj.SHFAlloc | ie64obj.SHFWrite
		default:
			s.typ = ie64obj.SHTProgBits
			if sectionFlagSpec != "" {
				flags, err := objectSectionFlags(sectionFlagSpec)
				if err != nil {
					return nil, err
				}
				s.flags = flags
			} else if strings.HasPrefix(name, ".init_array.") || strings.HasPrefix(name, ".fini_array.") {
				s.flags = ie64obj.SHFAlloc | ie64obj.SHFWrite
			} else {
				s.flags = ie64obj.SHFAlloc
			}
		}
		if sectionFlagSpec != "" && (name == ".text" || name == ".rodata" || name == ".data" || name == ".bss" || name == ".preinit_array" || name == ".init_array" || name == ".fini_array" || name == ".fini_array_onexit") {
			flags, err := objectSectionFlags(sectionFlagSpec)
			if err != nil {
				return nil, err
			}
			if s.flags != flags {
				return nil, fmt.Errorf("section %s has conflicting flags", name)
			}
		}
		byName[name] = s
		sections = append(sections, s)
		scopeID++
		sectionFallbacks[name] = fmt.Sprintf("__ie64_object_scope_%d", scopeID)
		return s, nil
	}
	for lineNo, raw := range lines {
		clean := strings.TrimSpace(stripComment(raw))
		if clean == "" {
			continue
		}
		fields := strings.Fields(clean)
		head := strings.ToLower(fields[0])
		if strings.HasPrefix(head, ".section") {
			rest := strings.TrimSpace(clean[len(fields[0]):])
			name, flags, err := parseObjectSectionDirective(rest)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %v", sourceName, lineNo+1, err)
			}
			current, err = sectionFor(name, flags)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %v", sourceName, lineNo+1, err)
			}
			continue
		}
		switch head {
		case ".text", ".rodata", ".data", ".bss":
			current, err = sectionFor(head, "")
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %v", sourceName, lineNo+1, err)
			}
			continue
		case ".global", ".globl", ".weak", ".local", ".hidden":
			if len(fields) < 2 {
				return nil, fmt.Errorf("%s:%d: %s requires a symbol", sourceName, lineNo+1, head)
			}
			for _, n := range splitOperands(strings.TrimSpace(clean[len(fields[0]):])) {
				d := decl(canonicalSymbol(n))
				switch head {
				case ".global", ".globl":
					d.bind = ie64obj.STBGlobal
				case ".weak":
					d.bind = ie64obj.STBWeak
				case ".local":
					d.bind = ie64obj.STBLocal
				case ".hidden":
					d.visibility = ie64obj.STVHidden
				}
			}
			continue
		case ".visibility":
			parts := splitOperands(strings.TrimSpace(clean[len(fields[0]):]))
			if len(parts) != 2 {
				return nil, fmt.Errorf("%s:%d: .visibility requires symbol,value", sourceName, lineNo+1)
			}
			v := strings.ToLower(strings.TrimSpace(parts[1]))
			if v != "default" && v != "hidden" {
				return nil, fmt.Errorf("%s:%d: unsupported visibility %s", sourceName, lineNo+1, v)
			}
			d := decl(canonicalSymbol(parts[0]))
			if v == "hidden" {
				d.visibility = ie64obj.STVHidden
			} else {
				d.visibility = ie64obj.STVDefault
			}
			continue
		case ".type":
			parts := splitOperands(strings.TrimSpace(clean[len(fields[0]):]))
			if len(parts) != 2 {
				return nil, fmt.Errorf("%s:%d: .type requires symbol,type", sourceName, lineNo+1)
			}
			d := decl(canonicalSymbol(parts[0]))
			switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(parts[1])), "@") {
			case "function", "func":
				d.typ = ie64obj.STTFunc
			case "object":
				d.typ = ie64obj.STTObject
			case "notype":
				d.typ = ie64obj.STTNoType
			default:
				return nil, fmt.Errorf("%s:%d: unsupported symbol type %s", sourceName, lineNo+1, parts[1])
			}
			continue
		case ".size":
			parts := splitOperands(strings.TrimSpace(clean[len(fields[0]):]))
			if len(parts) != 2 {
				return nil, fmt.Errorf("%s:%d: .size requires symbol,size", sourceName, lineNo+1)
			}
			d := decl(canonicalSymbol(parts[0]))
			expr := strings.ReplaceAll(strings.TrimSpace(parts[1]), " ", "")
			if expr == ".-"+strings.TrimSpace(parts[0]) {
				d.sizeEndLabel = fmt.Sprintf("__m68kto64_object_size_%d", lineNo+1)
				current.lines = append(current.lines, d.sizeEndLabel+":")
				d.sizeFromEnd = true
			} else {
				n, e := strconv.ParseUint(expr, 0, 64)
				if e != nil {
					return nil, fmt.Errorf("%s:%d: unsupported .size expression %s", sourceName, lineNo+1, parts[1])
				}
				d.size = n
			}
			continue
		case ".align", "align":
			if len(fields) != 2 {
				return nil, fmt.Errorf("%s:%d: align requires one value", sourceName, lineNo+1)
			}
			n, e := strconv.ParseUint(fields[1], 0, 64)
			if e != nil || n == 0 || n&(n-1) != 0 {
				return nil, fmt.Errorf("%s:%d: invalid alignment %s", sourceName, lineNo+1, fields[1])
			}
			if n > current.align {
				current.align = n
			}
			current.lines = append(current.lines, "align "+fields[1])
			continue
		case ".comm":
			parts := splitOperands(strings.TrimSpace(clean[len(fields[0]):]))
			if len(parts) < 2 || len(parts) > 3 {
				return nil, fmt.Errorf("%s:%d: .comm requires symbol,size[,align]", sourceName, lineNo+1)
			}
			sz, e := strconv.ParseUint(parts[1], 0, 64)
			if e != nil {
				return nil, fmt.Errorf("%s:%d: invalid common size", sourceName, lineNo+1)
			}
			al := uint64(1)
			if len(parts) == 3 {
				al, e = strconv.ParseUint(parts[2], 0, 64)
				if e != nil || al == 0 || al&(al-1) != 0 {
					return nil, fmt.Errorf("%s:%d: invalid common alignment", sourceName, lineNo+1)
				}
			}
			d := decl(parts[0])
			d.bind = ie64obj.STBGlobal
			d.section = "COMMON"
			d.defined = true
			d.size = sz
			d.commonAlign = al
			continue
		}
		if strings.HasSuffix(fields[0], ":") {
			name := strings.TrimSuffix(fields[0], ":")
			canonicalName := canonicalSymbol(name)
			d := decl(canonicalName)
			if d.defined {
				return nil, fmt.Errorf("%s:%d: duplicate symbol %s", sourceName, lineNo+1, canonicalName)
			}
			d.defined = true
			d.section = current.name
			if explicitLocals[name] && strings.HasPrefix(name, ".") {
				asmLabels[canonicalName] = fmt.Sprintf("__ie64_explicit_local_%d", len(asmLabels)+1)
				current.lines = append(current.lines, asmLabels[canonicalName]+":")
			} else {
				current.lines = append(current.lines, clean)
			}
			if !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "__m68kto64_") {
				sectionGlobals[current.name] = name
			}
			continue
		}
		rewritten, rels, e := rewriteObjectReferences(clean, current.name, canonicalSymbol, decl)
		if e != nil {
			return nil, fmt.Errorf("%s:%d: %v", sourceName, lineNo+1, e)
		}
		marker := ""
		if len(rels) > 0 {
			relocID++
			marker = fmt.Sprintf("__m68kto64_object_reloc_%d", relocID)
			current.lines = append(current.lines, marker+":")
		}
		current.lines = append(current.lines, rewritten)
		for _, r := range rels {
			r.marker = marker
			pending = append(pending, r)
		}
	}
	obj := &ie64obj.Object{}
	sectionIndex := map[string]uint16{}
	sectionLabels := map[string]map[string]uint32{}
	for _, s := range sections {
		if len(s.lines) == 0 && s.name == ".text" && len(sections) > 1 {
			continue
		}
		a := NewIE64Assembler()
		a.baseAddr = 0
		a.imageBase = 0
		a.basePath = filepath.Dir(sourceName)
		a.includePaths = includePaths
		for n, v := range defines {
			a.Predefine(n, v)
		}
		for n := range decls {
			a.Predefine(n, 0)
		}
		data, e := a.Assemble(strings.Join(s.lines, "\n"))
		if e != nil {
			return nil, fmt.Errorf("%s section %s: %v", sourceName, s.name, e)
		}
		idx := uint16(len(obj.Sections) + 1)
		sectionIndex[s.name] = idx
		sectionLabels[s.name] = cloneLabelMap(a.labels)
		sec := ie64obj.Section{Name: s.name, Type: s.typ, Flags: s.flags, Align: s.align, Data: data}
		if s.typ == ie64obj.SHTNoBits {
			sec.Size = uint64(len(data))
			sec.Data = nil
		}
		obj.Sections = append(obj.Sections, sec)
	}
	for name, d := range decls {
		if !d.defined && d.bind == ie64obj.STBLocal && !explicitLocals[name] && !strings.HasPrefix(name, ".") {
			d.bind = ie64obj.STBGlobal
		}
	}
	// Marshal requires local symbols before non-local symbols; keep source order within each class.
	var names []string
	for _, n := range order {
		if decls[n].bind == ie64obj.STBLocal {
			names = append(names, n)
		}
	}
	for _, n := range order {
		if decls[n].bind != ie64obj.STBLocal {
			names = append(names, n)
		}
	}
	symIndex := map[string]uint32{}
	for _, n := range names {
		d := decls[n]
		sym := ie64obj.Symbol{Name: n, Bind: d.bind, Type: d.typ, Visibility: d.visibility, Size: d.size}
		if d.defined {
			if d.section == "COMMON" {
				sym.Section = ie64obj.SHNCommon
				sym.Value = d.commonAlign
			} else {
				sym.Section = sectionIndex[d.section]
				label := n
				if asmLabels[n] != "" {
					label = asmLabels[n]
				}
				value, ok := sectionLabels[d.section][label]
				if !ok {
					return nil, fmt.Errorf("defined symbol %s has no section label", n)
				}
				sym.Value = uint64(value)
				if d.sizeFromEnd {
					end, ok := sectionLabels[d.section][d.sizeEndLabel]
					if !ok {
						return nil, fmt.Errorf("symbol %s has no size-end label", n)
					}
					d.sizeEnd = uint64(end)
					if d.sizeEnd < sym.Value {
						return nil, fmt.Errorf("symbol %s size expression precedes its definition", n)
					}
					sym.Size = d.sizeEnd - sym.Value
				}
			}
		}
		obj.Symbols = append(obj.Symbols, sym)
		symIndex[n] = uint32(len(obj.Symbols))
	}
	for _, r := range pending {
		idx := sectionIndex[r.section]
		if idx == 0 {
			return nil, fmt.Errorf("relocation references omitted section %s", r.section)
		}
		r.symbol = strings.TrimSpace(r.symbol)
		r.offset += uint64(sectionLabels[r.section][r.marker])
		si := symIndex[r.symbol]
		if si == 0 {
			return nil, fmt.Errorf("relocation references unknown symbol %s", r.symbol)
		}
		obj.Sections[idx-1].Relocations = append(obj.Sections[idx-1].Relocations, ie64obj.Relocation{Offset: r.offset, Symbol: si, Type: r.typ, Addend: r.addend})
	}
	return obj.Marshal()
}

func rewriteObjectReferences(line, section string, canonicalSymbol func(string) string, declare func(string) *objectSymbolDecl) (string, []pendingObjectReloc, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return line, nil, nil
	}
	head := strings.ToLower(fields[0])
	rest := strings.TrimSpace(line[len(fields[0]):])
	var typ uint32
	if head == "jsr" || head == "bra" || strings.HasPrefix(head, "b") && head != "bswap" {
		typ = ie64obj.RPC32
	} else if head == "dc.q" {
		typ = ie64obj.RABS64
	} else if head == "dc.l" {
		typ = ie64obj.RABS32
	} else if head == "move.l" || head == "movt" {
		typ = ie64obj.RLO32
	} else {
		return line, nil, nil
	}
	parts := splitOperands(rest)
	if len(parts) == 0 {
		return line, nil, nil
	}
	if head == "dc.q" || head == "dc.l" {
		width := uint64(8)
		if head == "dc.l" {
			width = 4
		}
		var rels []pendingObjectReloc
		for i, expr := range parts {
			name, add, ok := parseSymbolAddend(expr)
			if !ok {
				continue
			}
			name = canonicalSymbol(name)
			declare(name)
			parts[i] = "0"
			rels = append(rels, pendingObjectReloc{section: section, symbol: name, offset: uint64(i) * width, typ: typ, addend: add})
		}
		return fields[0] + " " + strings.Join(parts, ", "), rels, nil
	}
	at := len(parts) - 1
	expr := strings.TrimSpace(parts[at])
	if head == "move.l" || head == "movt" {
		expr = strings.TrimPrefix(expr, "#")
		want := "lo32("
		if head == "movt" {
			want = "hi32("
			typ = ie64obj.RHI32
		}
		if !strings.HasPrefix(strings.ToLower(expr), want) || !strings.HasSuffix(expr, ")") {
			return line, nil, nil
		}
		expr = expr[len(want) : len(expr)-1]
	}
	name, add, ok := parseSymbolAddend(expr)
	if !ok {
		return line, nil, nil
	}
	name = canonicalSymbol(name)
	declare(name)
	parts[at] = "0"
	if head == "move.l" || head == "movt" {
		parts[at] = "#0"
	}
	return fields[0] + " " + strings.Join(parts, ", "), []pendingObjectReloc{{section: section, symbol: name, typ: typ, addend: add}}, nil
}

func parseSymbolAddend(expr string) (string, int64, bool) {
	expr = strings.TrimSpace(strings.TrimPrefix(expr, "#"))
	if expr == "" {
		return "", 0, false
	}
	cut := -1
	for i := 1; i < len(expr); i++ {
		if expr[i] == '+' || expr[i] == '-' {
			cut = i
			break
		}
	}
	name := expr
	var add int64
	if cut >= 0 {
		name = expr[:cut]
		n, e := strconv.ParseInt(expr[cut:], 0, 64)
		if e != nil {
			return "", 0, false
		}
		add = n
	}
	if name[0] >= '0' && name[0] <= '9' || strings.HasPrefix(name, "$") {
		return "", 0, false
	}
	for _, r := range name {
		if !(r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return "", 0, false
		}
	}
	return name, add, true
}
