package main

import (
	"testing"
	"unsafe"
)

func TestCPU6502CacheLineLayout(t *testing.T) {
	var cpu CPU_6502

	offRunning := unsafe.Offsetof(cpu.running)
	offCycles := unsafe.Offsetof(cpu.Cycles)

	if offRunning%64 != 0 {
		t.Fatalf("running offset %d is not 64-byte aligned", offRunning)
	}
	if offCycles%64 != 0 {
		t.Fatalf("Cycles offset %d is not 64-byte aligned", offCycles)
	}
}
