package main

import (
	"testing"
	"unsafe"
)

func TestIE32JITContextABIHasStableWordAlignment(t *testing.T) {
	for name, offset := range map[string]uintptr{
		"PC":               ie32JITABIPC,
		"SP":               ie32JITABISP,
		"A":                ie32JITABIA,
		"interruptEnabled": ie32JITABIInterruptEnabled,
		"inInterrupt":      ie32JITABIInInterrupt,
		"memory":           ie32JITABIMemory,
		"memBase":          ie32JITABIMemBase,
	} {
		if offset%4 != 0 {
			t.Fatalf("IE32 JIT ABI %s offset %#x is not word aligned", name, offset)
		}
	}
	if ie32JITABIMemBase%unsafe.Alignof(uintptr(0)) != 0 {
		t.Fatalf("IE32 JIT ABI memBase offset %#x is not pointer aligned", ie32JITABIMemBase)
	}
}
