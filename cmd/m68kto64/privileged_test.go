package main

import (
	"strings"
	"testing"
)

// ILLEGAL — m68k vector 4. Maps to syscall #19 (newly registered).
func TestIllegal(t *testing.T) {
	out := convertOneInstr(t, "\tillegal")
	mustContain(t, out, "syscall #19")
}

// RTE — privileged return from exception. Lowered as RTS (SR pop dropped).
func TestRTE_LowersToRtsWithDiag(t *testing.T) {
	out := convertOneInstr(t, "\trte")
	mustContain(t, out, "RTE")
	mustContain(t, out, "SR pop dropped")
	mustContain(t, out, "load.l ") // RTS pop
	mustContain(t, out, "jmp ")
}

// STOP — privileged. Stripped with diagnostic.
func TestSTOP_Stripped(t *testing.T) {
	out := convertOneInstr(t, "\tstop #$2700")
	mustContain(t, out, "stripped STOP")
	if strings.Contains(out, "syscall") || strings.Contains(out, "jmp ") {
		t.Errorf("STOP must not emit syscall or jmp:\n%s", out)
	}
}

// RESET — privileged. Stripped with diagnostic.
func TestRESET_Stripped(t *testing.T) {
	out := convertOneInstr(t, "\treset")
	mustContain(t, out, "stripped RESET")
}
