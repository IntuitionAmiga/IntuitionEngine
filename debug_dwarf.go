package main

import (
	"debug/dwarf"
	"debug/elf"
	"sort"
	"sync"
)

type SourceLine struct {
	CPU  string
	Addr uint64
	File string
	Line int
}

type SourceLineTable struct {
	mu    sync.RWMutex
	byCPU map[string][]SourceLine
}

func NewSourceLineTable() *SourceLineTable {
	return &SourceLineTable{byCPU: make(map[string][]SourceLine)}
}

func (st *SourceLineTable) Add(cpu string, addr uint64, file string, line int) {
	if st == nil || file == "" || line <= 0 {
		return
	}
	key := normalizeSymbolCPU(cpu)
	st.mu.Lock()
	defer st.mu.Unlock()
	st.byCPU[key] = append(st.byCPU[key], SourceLine{CPU: key, Addr: addr, File: file, Line: line})
	sort.Slice(st.byCPU[key], func(i, j int) bool {
		return st.byCPU[key][i].Addr < st.byCPU[key][j].Addr
	})
}

func (st *SourceLineTable) Resolve(cpu string, addr uint64) (SourceLine, bool) {
	if st == nil {
		return SourceLine{}, false
	}
	key := normalizeSymbolCPU(cpu)
	st.mu.RLock()
	lines := append([]SourceLine(nil), st.byCPU[key]...)
	st.mu.RUnlock()
	if len(lines) == 0 {
		return SourceLine{}, false
	}
	idx := sort.Search(len(lines), func(i int) bool { return lines[i].Addr > addr }) - 1
	if idx < 0 {
		return SourceLine{}, false
	}
	return lines[idx], true
}

func (st *SourceLineTable) LoadELF(cpu, path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dw, err := f.DWARF()
	if err != nil {
		return nil
	}
	r := dw.Reader()
	for {
		ent, err := r.Next()
		if err != nil {
			return err
		}
		if ent == nil {
			return nil
		}
		if ent.Tag != dwarf.TagCompileUnit {
			r.SkipChildren()
			continue
		}
		lr, err := dw.LineReader(ent)
		if err != nil || lr == nil {
			continue
		}
		var le dwarf.LineEntry
		for {
			err = lr.Next(&le)
			if err != nil {
				break
			}
			if le.EndSequence || le.File == nil || le.Address == 0 {
				continue
			}
			st.Add(cpu, le.Address, le.File.Name, le.Line)
		}
	}
}
