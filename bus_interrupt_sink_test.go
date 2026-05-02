package main

import "testing"

func TestInterruptMaskValues(t *testing.T) {
	if IntMaskVBI == 0 || IntMaskDLI == 0 {
		t.Fatal("interrupt masks must be non-zero")
	}
	if IntMaskVBI&IntMaskDLI != 0 {
		t.Fatal("interrupt masks must be disjoint")
	}
}
