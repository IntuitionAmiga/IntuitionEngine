//go:build headless

package main

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVoodooABIDrift(t *testing.T) {
	consts := parseVoodooGoConstants(t)
	rows := parseVoodooABIRows(t)

	base := consts["VOODOO_BASE"]
	for _, row := range rows {
		got, ok := consts[row.name]
		if !ok {
			t.Fatalf("%s appears in ABI TSV but not in voodoo_constants.go", row.name)
		}
		if want := base + row.offset; got != want {
			t.Fatalf("%s = %#x, want %#x from ABI offset %#x", row.name, got, want, row.offset)
		}
	}

	if got, want := consts["VOODOO_END"], consts["VOODOO_PALETTE_BASE"]+(VOODOO_PALETTE_SIZE*4)-1; got != want {
		t.Fatalf("VOODOO_END = %#x, want final palette entry %#x", got, want)
	}
	if got, want := consts["VOODOO_REG_COUNT"], ((consts["VOODOO_PALETTE_BASE"]-base)/4)+consts["VOODOO_PALETTE_SIZE"]; got != want {
		t.Fatalf("VOODOO_REG_COUNT = %d, want %d", got, want)
	}

	checkIncludeSymbolParity(t, consts)
}

func TestVoodooABIAddressLayout(t *testing.T) {
	consts := parseVoodooGoConstants(t)

	if got, want := consts["VOODOO_BASE"], uint64(0xF8000); got != want {
		t.Fatalf("VOODOO_BASE = %#x, want %#x", got, want)
	}
	if got, want := consts["VOODOO_END"], uint64(0xF87FF); got != want {
		t.Fatalf("VOODOO_END = %#x, want %#x", got, want)
	}
	if got, want := consts["VOODOO_TEXMEM_BASE"], uint64(0xD0000); got != want {
		t.Fatalf("VOODOO_TEXMEM_BASE = %#x, want %#x", got, want)
	}
	if got, want := consts["VOODOO_TEXMEM_SIZE"], uint64(0x10000); got != want {
		t.Fatalf("VOODOO_TEXMEM_SIZE = %#x, want %#x", got, want)
	}

	regStart := uint32(consts["VOODOO_BASE"])
	regEnd := uint32(consts["VOODOO_END"])
	texStart := uint32(consts["VOODOO_TEXMEM_BASE"])
	texEnd := texStart + uint32(consts["VOODOO_TEXMEM_SIZE"]) - 1

	for _, occupied := range []struct {
		name       string
		start, end uint32
	}{
		{"VGA VRAM", VGA_VRAM_WINDOW, VGA_VRAM_WINDOW + VGA_VRAM_SIZE - 1},
		{"VGA text", VGA_TEXT_WINDOW, VGA_TEXT_WINDOW + VGA_TEXT_SIZE - 1},
		{"VideoChip VRAM", VRAM_START, VRAM_START + VRAM_SIZE - 1},
		{"TED video VRAM", TED_V_VRAM_BASE, TED_V_VRAM_BASE + TED_V_VRAM_SIZE - 1},
		{"VideoChip registers", VIDEO_REG_BASE, VIDEO_REG_END},
		{"SYSINFO", SYSINFO_REGION_BASE, SYSINFO_REGION_END},
	} {
		if rangesOverlap(regStart, regEnd, occupied.start, occupied.end) {
			t.Fatalf("Voodoo register range %#x-%#x overlaps %s %#x-%#x", regStart, regEnd, occupied.name, occupied.start, occupied.end)
		}
		if rangesOverlap(texStart, texEnd, occupied.start, occupied.end) {
			t.Fatalf("Voodoo texture range %#x-%#x overlaps %s %#x-%#x", texStart, texEnd, occupied.name, occupied.start, occupied.end)
		}
	}
}

func rangesOverlap(aStart, aEnd, bStart, bEnd uint32) bool {
	return aStart <= bEnd && bStart <= aEnd
}

type voodooABIRow struct {
	name   string
	offset uint64
}

func parseVoodooABIRows(t *testing.T) []voodooABIRow {
	t.Helper()
	f, err := os.Open(filepath.Join("sdk", "docs", "ie_voodoo_abi.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var rows []voodooABIRow
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if line == 1 || text == "" {
			continue
		}
		cols := strings.Split(text, "\t")
		if len(cols) < 2 {
			t.Fatalf("bad ABI TSV line %d: %q", line, text)
		}
		offset, err := strconv.ParseUint(strings.TrimPrefix(cols[1], "0x"), 16, 64)
		if err != nil {
			t.Fatalf("bad ABI offset on line %d: %v", line, err)
		}
		rows = append(rows, voodooABIRow{name: cols[0], offset: offset})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("ABI TSV contains no rows")
	}
	return rows
}

func parseVoodooGoConstants(t *testing.T) map[string]uint64 {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "voodoo_constants.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]uint64)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		var previous []ast.Expr
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			values := vs.Values
			if len(values) == 0 {
				values = previous
			} else {
				previous = values
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "VOODOO_") || i >= len(values) {
					continue
				}
				value, ok := evalConstExpr(values[i], out)
				if ok {
					out[name.Name] = value
				}
			}
		}
	}
	return out
}

func evalConstExpr(expr ast.Expr, consts map[string]uint64) (uint64, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		v, err := strconv.ParseUint(e.Value, 0, 64)
		return v, err == nil
	case *ast.Ident:
		v, ok := consts[e.Name]
		return v, ok
	case *ast.ParenExpr:
		return evalConstExpr(e.X, consts)
	case *ast.UnaryExpr:
		v, ok := evalConstExpr(e.X, consts)
		if !ok {
			return 0, false
		}
		switch e.Op {
		case token.ADD:
			return v, true
		default:
			return 0, false
		}
	case *ast.BinaryExpr:
		left, ok := evalConstExpr(e.X, consts)
		if !ok {
			return 0, false
		}
		right, ok := evalConstExpr(e.Y, consts)
		if !ok {
			return 0, false
		}
		switch e.Op {
		case token.ADD:
			return left + right, true
		case token.SUB:
			return left - right, true
		case token.MUL:
			return left * right, true
		case token.QUO:
			if right == 0 {
				return 0, false
			}
			return left / right, true
		case token.SHL:
			return left << right, true
		default:
			return 0, false
		}
	}
	return 0, false
}

func checkIncludeSymbolParity(t *testing.T, consts map[string]uint64) {
	t.Helper()
	required := []string{
		"VOODOO_BASE",
		"VOODOO_END",
		"VOODOO_STATUS",
		"VOODOO_ENABLE",
		"VOODOO_TRIANGLE_CMD",
		"VOODOO_SWAP_BUFFER_CMD",
		"VOODOO_TEXTURE_MODE",
		"VOODOO_TEXMEM_BASE",
		"VOODOO_TEXMEM_SIZE",
		"VOODOO_PALETTE_BASE",
		"VOODOO_6502_WINDOW_BASE",
		"VOODOO_6502_WINDOW_SIZE",
		"VOODOO_6502_BANK_HI",
		"VOODOO_6502_BANK_PAGE_HI",
	}
	for _, path := range []string{
		"sdk/include/ie32.inc",
		"sdk/include/ie64.inc",
		"sdk/include/ie68.inc",
		"sdk/include/ie80.inc",
		"sdk/include/ie86.inc",
	} {
		symbols := parseIncludeConstants(t, path)
		for _, name := range required {
			got, ok := symbols[name]
			if !ok {
				t.Fatalf("%s missing %s", path, name)
			}
			if got != consts[name] {
				t.Fatalf("%s %s = %#x, want %#x", path, name, got, consts[name])
			}
		}
	}

	ie65Symbols := parseIncludeConstants(t, "sdk/include/ie65.inc")
	for _, name := range []string{
		"VOODOO_6502_WINDOW_BASE",
		"VOODOO_6502_WINDOW_SIZE",
		"VOODOO_6502_BANK_HI",
		"VOODOO_6502_BANK_PAGE_HI",
	} {
		got, ok := ie65Symbols[name]
		if !ok {
			t.Fatalf("sdk/include/ie65.inc missing %s", name)
		}
		if got != consts[name] {
			t.Fatalf("sdk/include/ie65.inc %s = %#x, want %#x", name, got, consts[name])
		}
	}
}

func parseIncludeConstants(t *testing.T, path string) map[string]uint64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]uint64)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.Split(scanner.Text(), ";")[0])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		var name, value string
		switch {
		case len(fields) >= 2 && fields[0] == ".set" && strings.Contains(fields[1], ","):
			parts := strings.SplitN(fields[1], ",", 2)
			name, value = parts[0], parts[1]
		case len(fields) >= 3 && (fields[0] == ".equ" || fields[0] == ".set"):
			name = strings.TrimSuffix(fields[1], ",")
			value = fields[2]
		case len(fields) >= 3 && strings.EqualFold(fields[1], "equ"):
			name, value = fields[0], fields[2]
		case len(fields) >= 3 && fields[1] == "=":
			name, value = fields[0], fields[2]
		default:
			continue
		}
		if !strings.HasPrefix(name, "VOODOO_") {
			continue
		}
		parsed, ok := parseAsmUint(value)
		if ok {
			out[name] = parsed
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func parseAsmUint(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	base := 10
	if strings.HasPrefix(value, "$") {
		base = 16
		value = strings.TrimPrefix(value, "$")
	}
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		base = 16
		value = strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	}
	if strings.HasSuffix(value, "h") || strings.HasSuffix(value, "H") {
		base = 16
		value = strings.TrimSuffix(strings.TrimSuffix(value, "h"), "H")
	}
	if strings.ContainsAny(value, "ABCDEFabcdef") {
		base = 16
	}
	v, err := strconv.ParseUint(value, base, 64)
	return v, err == nil
}
