package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	elfMachineIE64 = 0x4945
	elfTypeDyn     = 3
	elfPTLoad      = 1
	elfPTNote      = 4
	pageSize       = 0x1000
	baseVA         = 0x600000
	listingBias    = 0x1000

	elfSectionHeaderSize = 64

	iosmMagic           = 0x4D534F49
	iosmSchemaVersion   = 1
	iosmNoteType        = 0x494F5331
	iosmKindLibrary     = 1
	iosmKindDevice      = 2
	iosmKindHandler     = 3
	iosmKindResource    = 4
	iosmKindCommand     = 5
	iosmModfCompatPort  = 0x00000002
	iosmModfASLRCapable = 0x00000004
	iosmSize            = 128
	iosmCopyright       = "Copyright \xA9 2026 Zayn Otley"

	iosmSectionName = ".ios.manifest"
	iosmNoteName    = "IOS-MOD"

	m16LibManifestMagic       = iosmMagic
	m16LibManifestDescVersion = iosmSchemaVersion
	m16LibManifestNoteType    = iosmNoteType
	m16LibManifestTypeLibrary = iosmKindLibrary
	m16ModfCompatPort         = iosmModfCompatPort
)

var labelRe = regexp.MustCompile(`^([0-9A-F]{8})\s+.*$`)

type libManifestSpec struct {
	Name          string
	Kind          uint8
	Version       uint16
	Revision      uint16
	Patch         uint16
	Type          uint32
	Flags         uint32
	MsgABIVersion uint32
	BuildDate     string
	Copyright     string
}

type manifestExprParser struct {
	input   string
	pos     int
	symbols map[string]uint64
	global  string
}

func main() {
	listingPath := flag.String("listing", "", "path to ie64asm listing file")
	imagePath := flag.String("image", "", "path to assembled iexec.ie64 image")
	outPath := flag.String("out", "", "output path for boot_dos_library.elf")
	label := flag.String("label", "prog_doslib", "flat program label to export as ELF")
	buildDate := flag.String("build-date", "", "manifest build date (YYYY-MM-DD); defaults to SOURCE_DATE_EPOCH or current UTC date")
	flag.Parse()

	if *listingPath == "" || *imagePath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: rebuild_boot_dos_elf -listing iexec.lst -image iexec.ie64 -out out.elf [-label prog_doslib]")
		os.Exit(2)
	}

	listing, err := os.ReadFile(*listingPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read listing: %v\n", err)
		os.Exit(1)
	}

	progStart, err := parseProgramStartFromListing(listing, *label)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	image, err := os.ReadFile(*imagePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read image: %v\n", err)
		os.Exit(1)
	}
	if uint64(len(image)) < progStart+32 {
		fmt.Fprintf(os.Stderr, "image too small for prog_doslib header at 0x%X\n", progStart)
		os.Exit(1)
	}

	codeSize := binary.LittleEndian.Uint32(image[progStart+8 : progStart+12])
	dataSize := binary.LittleEndian.Uint32(image[progStart+12 : progStart+16])
	codeStart := progStart + 32
	codeEnd := codeStart + uint64(codeSize)
	dataStart := codeEnd
	dataEnd := dataStart + uint64(dataSize)
	if uint64(len(image)) < dataEnd {
		fmt.Fprintf(os.Stderr, "image too small: len=0x%X need dataEnd=0x%X\n", len(image), dataEnd)
		os.Exit(1)
	}

	code := image[codeStart:codeEnd]
	data := image[dataStart:dataEnd]
	spec, hasManifest, err := manifestSpecForLabel(listing, *label)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	resolvedBuildDate, err := resolveBuildDate(*buildDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve build date: %v\n", err)
		os.Exit(1)
	}
	spec.BuildDate = resolvedBuildDate
	spec.Copyright = iosmCopyright
	elf := buildELF(code, data, spec, hasManifest)

	if err := os.WriteFile(*outPath, elf, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write output: %v\n", err)
		os.Exit(1)
	}
}

func resolveBuildDate(explicit string) (string, error) {
	if explicit != "" {
		if _, err := time.Parse("2006-01-02", explicit); err != nil {
			return "", fmt.Errorf("invalid -build-date %q: %w", explicit, err)
		}
		return explicit, nil
	}
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		seconds, err := strconv.ParseInt(epoch, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid SOURCE_DATE_EPOCH %q: %w", epoch, err)
		}
		return time.Unix(seconds, 0).UTC().Format("2006-01-02"), nil
	}
	return time.Now().UTC().Format("2006-01-02"), nil
}

func parseProgramStartFromListing(body []byte, label string) (uint64, error) {
	lines := bytes.Split(body, []byte{'\n'})
	for i, raw := range lines {
		line := string(raw)
		if strings.Contains(line, label+":") {
			return parseNextAddress(lines, i+1)
		}
	}
	return 0, fmt.Errorf("missing %s start in listing", label)
}

func parseLibManifestSpecFromListing(body []byte, label string) (libManifestSpec, bool, error) {
	lines := bytes.Split(body, []byte{'\n'})
	symbols := parseListingSymbols(lines)
	inLabel := false
	for _, raw := range lines {
		line := string(raw)
		trimmed := strings.TrimSpace(line)
		if !inLabel {
			if trimmed == label+":" {
				inLabel = true
			}
			continue
		}
		if trimmed == "" {
			continue
		}
		if idx := strings.Index(trimmed, ".libmanifest"); idx >= 0 {
			spec, err := parseLibManifestDirective(trimmed[idx+len(".libmanifest"):], symbols, label)
			if err != nil {
				return libManifestSpec{}, false, fmt.Errorf("parse %s .libmanifest: %w", label, err)
			}
			return spec, true, nil
		}
		if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(strings.TrimSuffix(trimmed, ":"), ".") {
			break
		}
	}
	return libManifestSpec{}, false, nil
}

func parseListingSymbols(lines [][]byte) map[string]uint64 {
	symbols := make(map[string]uint64)
	var pendingLabels []string
	var lastGlobal string
	for _, raw := range lines {
		line := string(raw)
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		if len(fields) >= 4 && fields[0] == "=" && (strings.EqualFold(fields[3], "equ") || strings.EqualFold(fields[3], "set")) {
			val, err := strconv.ParseUint(fields[1], 16, 64)
			if err == nil {
				symbols[fields[2]] = val
			}
		}
		if strings.HasSuffix(trimmed, ":") {
			label := strings.TrimSuffix(trimmed, ":")
			if strings.HasPrefix(label, ".") {
				if lastGlobal != "" {
					pendingLabels = append(pendingLabels, lastGlobal+label)
				} else {
					pendingLabels = append(pendingLabels, label)
				}
			} else {
				lastGlobal = label
				pendingLabels = append(pendingLabels, label)
			}
			continue
		}
		if len(pendingLabels) > 0 {
			if m := labelRe.FindSubmatch(raw); m != nil {
				addr, err := strconv.ParseUint(string(m[1]), 16, 64)
				if err == nil && addr >= listingBias {
					for _, label := range pendingLabels {
						symbols[label] = addr - listingBias
					}
				}
				pendingLabels = pendingLabels[:0]
			}
		}
	}
	return symbols
}

func parseLibManifestDirective(rest string, symbols map[string]uint64, global string) (libManifestSpec, error) {
	parts := splitDirectiveOperands(rest)
	spec := libManifestSpec{}
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.Index(part, "=")
		if eq <= 0 || eq == len(part)-1 {
			return libManifestSpec{}, fmt.Errorf("invalid field %q", part)
		}
		key := strings.ToLower(strings.TrimSpace(part[:eq]))
		val := strings.TrimSpace(part[eq+1:])
		if seen[key] {
			return libManifestSpec{}, fmt.Errorf("duplicate key %q", key)
		}
		seen[key] = true
		switch key {
		case "name":
			name, err := parseQuotedDirectiveString(val)
			if err != nil {
				return libManifestSpec{}, err
			}
			spec.Name = name
		case "version":
			n, err := evalManifestDirectiveExpr(val, symbols, global)
			if err != nil {
				return libManifestSpec{}, fmt.Errorf("version: %w", err)
			}
			spec.Version = uint16(n)
		case "revision":
			n, err := evalManifestDirectiveExpr(val, symbols, global)
			if err != nil {
				return libManifestSpec{}, fmt.Errorf("revision: %w", err)
			}
			spec.Revision = uint16(n)
		case "patch":
			n, err := evalManifestDirectiveExpr(val, symbols, global)
			if err != nil {
				return libManifestSpec{}, fmt.Errorf("patch: %w", err)
			}
			spec.Patch = uint16(n)
		case "type":
			n, err := evalManifestDirectiveExpr(val, symbols, global)
			if err != nil {
				return libManifestSpec{}, fmt.Errorf("type: %w", err)
			}
			spec.Type = uint32(n)
			spec.Kind = uint8(n)
		case "flags":
			n, err := evalManifestDirectiveExpr(val, symbols, global)
			if err != nil {
				return libManifestSpec{}, fmt.Errorf("flags: %w", err)
			}
			spec.Flags = uint32(n)
		case "msg_abi":
			n, err := evalManifestDirectiveExpr(val, symbols, global)
			if err != nil {
				return libManifestSpec{}, fmt.Errorf("msg_abi: %w", err)
			}
			spec.MsgABIVersion = uint32(n)
		default:
			return libManifestSpec{}, fmt.Errorf("unknown key %q", key)
		}
	}
	required := []string{"name", "version", "revision", "type", "flags", "msg_abi"}
	for _, key := range required {
		if !seen[key] {
			return libManifestSpec{}, fmt.Errorf("missing key %q", key)
		}
	}
	return spec, nil
}

func manifestSpecForLabel(listing []byte, label string) (libManifestSpec, bool, error) {
	spec, ok, err := parseLibManifestSpecFromListing(listing, label)
	if err != nil {
		return libManifestSpec{}, false, err
	}
	if ok {
		if spec.Kind == 0 {
			spec.Kind = uint8(spec.Type)
		}
		return spec, ok, nil
	}
	if requiresSourceManifest(label) {
		return libManifestSpec{}, false, fmt.Errorf("missing %s .libmanifest in source listing", label)
	}
	if spec, ok := manifestSpecsByLabel[buildLabelAlias(label)]; ok {
		if spec.Type == 0 {
			spec.Type = uint32(spec.Kind)
		}
		return spec, true, nil
	}
	return spec, ok, nil
}

func buildLabelAlias(label string) string {
	if strings.HasPrefix(label, "prog_") {
		return label
	}
	return "prog_" + label
}

func requiresSourceManifest(label string) bool {
	switch buildLabelAlias(label) {
	case "prog_doslib", "prog_graphics_library", "prog_intuition_library":
		return true
	default:
		return false
	}
}

var manifestSpecsByLabel = map[string]libManifestSpec{
	"prog_console":      {Name: "console.handler", Kind: iosmKindHandler, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfCompatPort | iosmModfASLRCapable},
	"prog_shell":        {Name: "Shell", Kind: iosmKindHandler, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfCompatPort | iosmModfASLRCapable},
	"prog_hwres":        {Name: "hardware.resource", Kind: iosmKindResource, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfCompatPort | iosmModfASLRCapable},
	"prog_input_device": {Name: "input.device", Kind: iosmKindDevice, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfCompatPort | iosmModfASLRCapable},
	"prog_version":      {Name: "Version", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_avail":        {Name: "Avail", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_dir":          {Name: "Dir", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_type":         {Name: "Type", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_echo_cmd":     {Name: "Echo", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_resident_cmd": {Name: "Resident", Kind: iosmKindCommand, Version: 1, Revision: 2, Patch: 0, Flags: iosmModfASLRCapable},
	"prog_assign_cmd":   {Name: "Assign", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_list_cmd":     {Name: "List", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_which_cmd":    {Name: "Which", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_help_app":     {Name: "Help", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_gfxdemo":      {Name: "GfxDemo", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_about":        {Name: "About", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
	"prog_elfseg":       {Name: "ElfSeg", Kind: iosmKindCommand, Version: 1, Revision: 0, Patch: 1, Flags: iosmModfASLRCapable},
}

func splitDirectiveOperands(rest string) []string {
	var parts []string
	var current strings.Builder
	inString := false
	for i := 0; i < len(rest); i++ {
		ch := rest[i]
		switch ch {
		case '"':
			current.WriteByte(ch)
			if i == 0 || rest[i-1] != '\\' {
				inString = !inString
			}
		case ',':
			if inString {
				current.WriteByte(ch)
				continue
			}
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func parseQuotedDirectiveString(val string) (string, error) {
	if len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
		return "", fmt.Errorf("name must be a quoted string")
	}
	var b strings.Builder
	body := val[1 : len(val)-1]
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' {
			if i+1 >= len(body) {
				return "", fmt.Errorf("invalid escape")
			}
			b.WriteByte(unescapeDirectiveChar(body[i+1]))
			i++
			continue
		}
		b.WriteByte(body[i])
	}
	return b.String(), nil
}

func unescapeDirectiveChar(ch byte) byte {
	switch ch {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case '\\':
		return '\\'
	case '"':
		return '"'
	case '0':
		return 0
	default:
		return ch
	}
}

func evalManifestDirectiveExpr(val string, symbols map[string]uint64, global string) (uint64, error) {
	p := &manifestExprParser{
		input:   strings.TrimSpace(val),
		symbols: symbols,
		global:  global,
	}
	n, err := p.parseExprCompare()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.pos < len(p.input) {
		return 0, fmt.Errorf("unexpected character %q", p.input[p.pos])
	}
	return uint64(n), nil
}

func (p *manifestExprParser) skipSpaces() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *manifestExprParser) peek() byte {
	p.skipSpaces()
	if p.pos < len(p.input) {
		return p.input[p.pos]
	}
	return 0
}

func (p *manifestExprParser) peekTwo() string {
	p.skipSpaces()
	if p.pos+1 < len(p.input) {
		return p.input[p.pos : p.pos+2]
	}
	return ""
}

func (p *manifestExprParser) parseExprCompare() (int64, error) {
	left, err := p.parseExprOr()
	if err != nil {
		return 0, err
	}
	for {
		tw := p.peekTwo()
		switch tw {
		case "==":
			p.pos += 2
			right, err := p.parseExprOr()
			if err != nil {
				return 0, err
			}
			if left == right {
				left = 1
			} else {
				left = 0
			}
		case "!=":
			p.pos += 2
			right, err := p.parseExprOr()
			if err != nil {
				return 0, err
			}
			if left != right {
				left = 1
			} else {
				left = 0
			}
		case "<=":
			p.pos += 2
			right, err := p.parseExprOr()
			if err != nil {
				return 0, err
			}
			if left <= right {
				left = 1
			} else {
				left = 0
			}
		case ">=":
			p.pos += 2
			right, err := p.parseExprOr()
			if err != nil {
				return 0, err
			}
			if left >= right {
				left = 1
			} else {
				left = 0
			}
		default:
			ch := p.peek()
			if ch == '<' && tw != "<<" {
				p.pos++
				right, err := p.parseExprOr()
				if err != nil {
					return 0, err
				}
				if left < right {
					left = 1
				} else {
					left = 0
				}
				continue
			}
			if ch == '>' && tw != ">>" {
				p.pos++
				right, err := p.parseExprOr()
				if err != nil {
					return 0, err
				}
				if left > right {
					left = 1
				} else {
					left = 0
				}
				continue
			}
			return left, nil
		}
	}
}

func (p *manifestExprParser) parseExprOr() (int64, error) {
	left, err := p.parseExprXor()
	if err != nil {
		return 0, err
	}
	for p.peek() == '|' {
		p.pos++
		right, err := p.parseExprXor()
		if err != nil {
			return 0, err
		}
		left = left | right
	}
	return left, nil
}

func (p *manifestExprParser) parseExprXor() (int64, error) {
	left, err := p.parseExprAnd()
	if err != nil {
		return 0, err
	}
	for p.peek() == '^' {
		p.pos++
		right, err := p.parseExprAnd()
		if err != nil {
			return 0, err
		}
		left = left ^ right
	}
	return left, nil
}

func (p *manifestExprParser) parseExprAnd() (int64, error) {
	left, err := p.parseExprShift()
	if err != nil {
		return 0, err
	}
	for p.peek() == '&' {
		p.pos++
		right, err := p.parseExprShift()
		if err != nil {
			return 0, err
		}
		left = left & right
	}
	return left, nil
}

func (p *manifestExprParser) parseExprShift() (int64, error) {
	left, err := p.parseExprAdd()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peekTwo() {
		case "<<":
			p.pos += 2
			right, err := p.parseExprAdd()
			if err != nil {
				return 0, err
			}
			left = left << uint(right)
		case ">>":
			p.pos += 2
			right, err := p.parseExprAdd()
			if err != nil {
				return 0, err
			}
			left = left >> uint(right)
		default:
			return left, nil
		}
	}
}

func (p *manifestExprParser) parseExprAdd() (int64, error) {
	left, err := p.parseExprMul()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '+':
			p.pos++
			right, err := p.parseExprMul()
			if err != nil {
				return 0, err
			}
			left = left + right
		case '-':
			p.pos++
			right, err := p.parseExprMul()
			if err != nil {
				return 0, err
			}
			left = left - right
		default:
			return left, nil
		}
	}
}

func (p *manifestExprParser) parseExprMul() (int64, error) {
	left, err := p.parseExprUnary()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '*':
			p.pos++
			right, err := p.parseExprUnary()
			if err != nil {
				return 0, err
			}
			left = left * right
		case '/':
			p.pos++
			right, err := p.parseExprUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left = left / right
		default:
			return left, nil
		}
	}
}

func (p *manifestExprParser) parseExprUnary() (int64, error) {
	p.skipSpaces()
	if p.pos < len(p.input) {
		switch p.input[p.pos] {
		case '-':
			p.pos++
			val, err := p.parseExprUnary()
			if err != nil {
				return 0, err
			}
			return -val, nil
		case '+':
			p.pos++
			return p.parseExprUnary()
		case '~':
			p.pos++
			val, err := p.parseExprUnary()
			if err != nil {
				return 0, err
			}
			return ^val, nil
		}
	}
	return p.parseExprAtom()
}

func (p *manifestExprParser) parseExprAtom() (int64, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	ch := p.input[p.pos]
	if ch == '(' {
		p.pos++
		val, err := p.parseExprCompare()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}
	if ch == '$' {
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && (isHexDigit(p.input[p.pos]) || p.input[p.pos] == '_') {
			p.pos++
		}
		numStr := strings.ReplaceAll(p.input[start:p.pos], "_", "")
		val, err := strconv.ParseUint(numStr, 16, 64)
		if err != nil {
			return 0, err
		}
		return int64(val), nil
	}
	if ch == '0' && p.pos+1 < len(p.input) && (p.input[p.pos+1] == 'x' || p.input[p.pos+1] == 'X') {
		p.pos += 2
		start := p.pos
		for p.pos < len(p.input) && (isHexDigit(p.input[p.pos]) || p.input[p.pos] == '_') {
			p.pos++
		}
		numStr := strings.ReplaceAll(p.input[start:p.pos], "_", "")
		val, err := strconv.ParseUint(numStr, 16, 64)
		if err != nil {
			return 0, err
		}
		return int64(val), nil
	}
	if ch >= '0' && ch <= '9' {
		start := p.pos
		for p.pos < len(p.input) && ((p.input[p.pos] >= '0' && p.input[p.pos] <= '9') || p.input[p.pos] == '_') {
			p.pos++
		}
		numStr := strings.ReplaceAll(p.input[start:p.pos], "_", "")
		val, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return 0, err
		}
		return val, nil
	}
	if isIdentStart(ch) || ch == '.' {
		start := p.pos
		for p.pos < len(p.input) && (isIdentChar(p.input[p.pos]) || p.input[p.pos] == '.') {
			p.pos++
		}
		name := p.input[start:p.pos]
		if strings.HasPrefix(name, ".") && p.global != "" {
			if val, ok := p.symbols[p.global+name]; ok {
				return int64(val), nil
			}
		}
		val, ok := p.symbols[name]
		if !ok {
			return 0, fmt.Errorf("undefined symbol: %s", name)
		}
		return int64(val), nil
	}
	return 0, fmt.Errorf("unexpected character %q in expression", ch)
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func parseNextAddress(lines [][]byte, start int) (uint64, error) {
	for i := start; i < len(lines); i++ {
		if m := labelRe.FindSubmatch(lines[i]); m != nil {
			addr, err := strconv.ParseUint(string(m[1]), 16, 64)
			if err != nil {
				return 0, err
			}
			if addr < listingBias {
				return 0, fmt.Errorf("listing address 0x%X below bias", addr)
			}
			return addr - listingBias, nil
		}
	}
	return 0, fmt.Errorf("could not find next address in listing")
}

func buildELF(code []byte, data []byte, manifest libManifestSpec, withManifest bool) []byte {
	codeFileOff := uint64(pageSize)
	codeFileSize := uint64(len(code))
	codeMemSize := roundUp(codeFileSize, pageSize)
	if codeMemSize == 0 {
		codeMemSize = pageSize
	}
	dataVA := codeMemSize
	dataFileOff := codeFileOff + codeMemSize
	dataFileSize := uint64(len(data))
	dataMemSize := roundUp(dataFileSize, pageSize)
	if dataMemSize == 0 {
		dataMemSize = pageSize
	}

	manifestBytes := []byte(nil)
	manifestFileOff := dataFileOff + dataFileSize
	if withManifest {
		manifestBytes = buildManifestNote(manifest)
	}

	outLen := dataFileOff + dataFileSize
	if withManifest {
		outLen = manifestFileOff + uint64(len(manifestBytes))
	}
	out := make([]byte, outLen)
	copy(out[codeFileOff:], code)
	copy(out[dataFileOff:], data)
	if withManifest {
		copy(out[manifestFileOff:], manifestBytes)
	}

	copy(out[0:16], []byte{0x7F, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(out[16:18], elfTypeDyn)
	binary.LittleEndian.PutUint16(out[18:20], elfMachineIE64)
	binary.LittleEndian.PutUint32(out[20:24], 1)
	binary.LittleEndian.PutUint64(out[24:32], 0)
	binary.LittleEndian.PutUint64(out[32:40], 64)
	binary.LittleEndian.PutUint64(out[40:48], 0)
	binary.LittleEndian.PutUint32(out[48:52], 0)
	binary.LittleEndian.PutUint16(out[52:54], 64)
	binary.LittleEndian.PutUint16(out[54:56], 56)
	phnum := uint16(2)
	if withManifest {
		phnum = 3
	}
	binary.LittleEndian.PutUint16(out[56:58], phnum)
	binary.LittleEndian.PutUint16(out[58:60], 0)
	binary.LittleEndian.PutUint16(out[60:62], 0)
	binary.LittleEndian.PutUint16(out[62:64], 0)

	ph0 := 64
	binary.LittleEndian.PutUint32(out[ph0+0:ph0+4], elfPTLoad)
	binary.LittleEndian.PutUint32(out[ph0+4:ph0+8], 5)
	binary.LittleEndian.PutUint64(out[ph0+8:ph0+16], codeFileOff)
	binary.LittleEndian.PutUint64(out[ph0+16:ph0+24], 0)
	binary.LittleEndian.PutUint64(out[ph0+24:ph0+32], 0)
	binary.LittleEndian.PutUint64(out[ph0+32:ph0+40], codeFileSize)
	binary.LittleEndian.PutUint64(out[ph0+40:ph0+48], codeMemSize)
	binary.LittleEndian.PutUint64(out[ph0+48:ph0+56], pageSize)

	ph1 := ph0 + 56
	binary.LittleEndian.PutUint32(out[ph1+0:ph1+4], elfPTLoad)
	binary.LittleEndian.PutUint32(out[ph1+4:ph1+8], 6)
	binary.LittleEndian.PutUint64(out[ph1+8:ph1+16], dataFileOff)
	binary.LittleEndian.PutUint64(out[ph1+16:ph1+24], dataVA)
	binary.LittleEndian.PutUint64(out[ph1+24:ph1+32], dataVA)
	binary.LittleEndian.PutUint64(out[ph1+32:ph1+40], dataFileSize)
	binary.LittleEndian.PutUint64(out[ph1+40:ph1+48], dataMemSize)
	binary.LittleEndian.PutUint64(out[ph1+48:ph1+56], pageSize)

	if withManifest {
		ph2 := ph1 + 56
		binary.LittleEndian.PutUint32(out[ph2+0:ph2+4], elfPTNote)
		binary.LittleEndian.PutUint32(out[ph2+4:ph2+8], 4)
		binary.LittleEndian.PutUint64(out[ph2+8:ph2+16], manifestFileOff)
		binary.LittleEndian.PutUint64(out[ph2+16:ph2+24], 0)
		binary.LittleEndian.PutUint64(out[ph2+24:ph2+32], 0)
		binary.LittleEndian.PutUint64(out[ph2+32:ph2+40], uint64(len(manifestBytes)))
		binary.LittleEndian.PutUint64(out[ph2+40:ph2+48], uint64(len(manifestBytes)))
		binary.LittleEndian.PutUint64(out[ph2+48:ph2+56], 4)
	}

	return out
}

func buildManifestNote(spec libManifestSpec) []byte {
	noteName := []byte(iosmNoteName + "\x00")
	desc := make([]byte, iosmSize)
	typ := spec.Type
	if typ == 0 {
		typ = uint32(spec.Kind)
	}
	binary.LittleEndian.PutUint32(desc[0:4], iosmMagic)
	binary.LittleEndian.PutUint32(desc[4:8], iosmSchemaVersion)
	desc[8] = uint8(typ)
	binary.LittleEndian.PutUint16(desc[10:12], spec.Version)
	binary.LittleEndian.PutUint16(desc[12:14], spec.Revision)
	binary.LittleEndian.PutUint16(desc[14:16], spec.Patch)
	copy(desc[16:48], []byte(spec.Name))
	binary.LittleEndian.PutUint32(desc[48:52], spec.Flags)
	binary.LittleEndian.PutUint32(desc[52:56], spec.MsgABIVersion)
	copy(desc[56:72], []byte(spec.BuildDate))
	copy(desc[72:120], []byte(spec.Copyright))

	namePaddedLen := roundUp(uint64(len(noteName)), 4)
	descPaddedLen := roundUp(uint64(len(desc)), 4)
	out := make([]byte, 12+namePaddedLen+descPaddedLen)
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(noteName)))
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(desc)))
	binary.LittleEndian.PutUint32(out[8:12], iosmNoteType)
	copy(out[12:12+len(noteName)], noteName)
	copy(out[12+namePaddedLen:], desc)
	return out
}

func roundUp(v, align uint64) uint64 {
	if v == 0 {
		return 0
	}
	return (v + align - 1) &^ (align - 1)
}
