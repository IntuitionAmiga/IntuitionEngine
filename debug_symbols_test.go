package main

import (
	"strings"
	"testing"
)

func TestSymbols_Vice_ParsesLabels(t *testing.T) {
	st := NewSymbolTable()
	input := `
al 00:C000 .irq_handler
al c010 .tick
`
	if err := st.LoadVICELabels("6502", strings.NewReader(input), 0x1000); err != nil {
		t.Fatalf("LoadVICELabels: %v", err)
	}
	if got, ok := st.Lookup("6502", "irq_handler"); !ok || got != 0xD000 {
		t.Fatalf("irq_handler = %#x, %v; want %#x, true", got, ok, uint64(0xD000))
	}
	if got, ok := st.Lookup("6502", ".tick"); !ok || got != 0xD010 {
		t.Fatalf("tick = %#x, %v; want %#x, true", got, ok, uint64(0xD010))
	}
}

func TestSymbols_Resolve_NearestPriorWithOffset_AllCPUs(t *testing.T) {
	st := NewSymbolTable()
	cpus := []string{"6502", "Z80", "M68K", "IE32", "IE64", "X86"}
	for _, cpu := range cpus {
		st.Add(cpu, 0x1000, "start", 0x20, SymbolFunc)
		st.Add(cpu, 0x2000, "data", 4, SymbolObject)
		t.Run(cpu, func(t *testing.T) {
			res, ok := st.Resolve(cpu, 0x1004)
			if !ok {
				t.Fatal("Resolve returned false")
			}
			if res.Symbol.Name != "start" || res.Offset != 4 {
				t.Fatalf("Resolve = %s+%#x, want start+0x4", res.Symbol.Name, res.Offset)
			}
			if _, ok := st.Resolve(cpu, 0x1030); ok {
				t.Fatal("Resolve outside sized symbol returned true")
			}
		})
	}
}

func TestExprParser_AcceptsSymbolName_AllCPUs(t *testing.T) {
	st := NewSymbolTable()
	cpus := []string{"6502", "Z80", "M68K", "IE32", "IE64", "X86"}
	for _, cpu := range cpus {
		st.Add(cpu, 0x4000, "main", 0, SymbolFunc)
		t.Run(cpu, func(t *testing.T) {
			got, ok := EvalAddressWithSymbols("main+0x10", nil, st, cpu)
			if !ok {
				t.Fatal("EvalAddressWithSymbols returned false")
			}
			if got != 0x4010 {
				t.Fatalf("address = %#x, want %#x", got, uint64(0x4010))
			}
		})
	}
}

func TestSymCommand_AddLookupAndBreakpoint(t *testing.T) {
	mon, _ := newTestMonitor()
	mon.ExecuteCommand("sym add main $2000 func")
	mon.ExecuteCommand("b main+0x10")

	entry := mon.cpus[mon.focusedID]
	if entry == nil {
		t.Fatal("no focused CPU")
	}
	if !entry.CPU.HasBreakpoint(0x2010) {
		t.Fatal("expected breakpoint at symbol-relative address 0x2010")
	}
}

func TestBacktrace_SymbolAwareOutput(t *testing.T) {
	mon, cpu := newTestMonitor()

	sp := uint64(0x10000 - 8)
	cpu.regs[31] = sp
	ret := uint64(0x2010)
	for i := range 8 {
		cpu.memory[sp+uint64(i)] = byte(ret >> (i * 8))
	}

	mon.ExecuteCommand("sym add main $2000 func")
	_, out := mon.ExecuteCommandResult("bt 1")
	if len(out) == 0 {
		t.Fatal("bt produced no output")
	}
	if !strings.Contains(out[len(out)-1].Text, "main+0x10") {
		t.Fatalf("bt output = %q, want symbolic frame", out[len(out)-1].Text)
	}
}
