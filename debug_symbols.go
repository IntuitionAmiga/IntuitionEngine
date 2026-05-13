package main

import (
	"debug/elf"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type SymbolKind string

const (
	SymbolFunc   SymbolKind = "func"
	SymbolObject SymbolKind = "object"
	SymbolLabel  SymbolKind = "label"
)

type DebugSymbol struct {
	CPU  string
	Addr uint64
	Name string
	Size uint64
	Kind SymbolKind
}

type SymbolResolution struct {
	Symbol DebugSymbol
	Offset uint64
}

type SymbolTable struct {
	mu     sync.RWMutex
	byCPU  map[string][]DebugSymbol
	byName map[string]map[string]DebugSymbol
}

func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		byCPU:  make(map[string][]DebugSymbol),
		byName: make(map[string]map[string]DebugSymbol),
	}
}

func normalizeSymbolCPU(cpu string) string {
	return strings.ToUpper(strings.TrimSpace(cpu))
}

func (st *SymbolTable) Add(cpu string, addr uint64, name string, size uint64, kind SymbolKind) {
	if st == nil {
		return
	}
	name = strings.TrimSpace(strings.TrimPrefix(name, "."))
	if name == "" {
		return
	}
	key := normalizeSymbolCPU(cpu)
	sym := DebugSymbol{CPU: key, Addr: addr, Name: name, Size: size, Kind: kind}

	st.mu.Lock()
	defer st.mu.Unlock()
	if st.byName[key] == nil {
		st.byName[key] = make(map[string]DebugSymbol)
	}
	st.byName[key][name] = sym
	st.byCPU[key] = append(st.byCPU[key], sym)
	sort.Slice(st.byCPU[key], func(i, j int) bool {
		if st.byCPU[key][i].Addr == st.byCPU[key][j].Addr {
			return st.byCPU[key][i].Name < st.byCPU[key][j].Name
		}
		return st.byCPU[key][i].Addr < st.byCPU[key][j].Addr
	})
}

func (st *SymbolTable) Lookup(cpu, name string) (uint64, bool) {
	if st == nil {
		return 0, false
	}
	key := normalizeSymbolCPU(cpu)
	name = strings.TrimSpace(strings.TrimPrefix(name, "."))
	st.mu.RLock()
	defer st.mu.RUnlock()
	if sym, ok := st.byName[key][name]; ok {
		return sym.Addr, true
	}
	return 0, false
}

func (st *SymbolTable) Resolve(cpu string, addr uint64) (SymbolResolution, bool) {
	if st == nil {
		return SymbolResolution{}, false
	}
	key := normalizeSymbolCPU(cpu)
	st.mu.RLock()
	syms := append([]DebugSymbol(nil), st.byCPU[key]...)
	st.mu.RUnlock()
	if len(syms) == 0 {
		return SymbolResolution{}, false
	}
	idx := sort.Search(len(syms), func(i int) bool {
		return syms[i].Addr > addr
	}) - 1
	if idx < 0 {
		return SymbolResolution{}, false
	}
	sym := syms[idx]
	offset := addr - sym.Addr
	if sym.Size != 0 && offset >= sym.Size {
		return SymbolResolution{}, false
	}
	return SymbolResolution{Symbol: sym, Offset: offset}, true
}

func (st *SymbolTable) List(cpu string) []DebugSymbol {
	if st == nil {
		return nil
	}
	key := normalizeSymbolCPU(cpu)
	st.mu.RLock()
	defer st.mu.RUnlock()
	return append([]DebugSymbol(nil), st.byCPU[key]...)
}

func (st *SymbolTable) LoadELF(cpu, path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	syms, err := f.Symbols()
	if err != nil {
		return err
	}
	for _, sym := range syms {
		typ := elf.ST_TYPE(sym.Info)
		switch typ {
		case elf.STT_FUNC:
			st.Add(cpu, sym.Value, sym.Name, sym.Size, SymbolFunc)
		case elf.STT_OBJECT:
			st.Add(cpu, sym.Value, sym.Name, sym.Size, SymbolObject)
		}
	}
	return nil
}

func formatSymbolResolution(res SymbolResolution) string {
	if res.Offset == 0 {
		return res.Symbol.Name
	}
	return fmt.Sprintf("%s+0x%X", res.Symbol.Name, res.Offset)
}
