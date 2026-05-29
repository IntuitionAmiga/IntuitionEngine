//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	iedoomSymRDrawColumn = 0x00040A68
	iedoomSymRDrawSpan   = 0x00040F58

	iedoomSymDSYStep   = 0x001B0208
	iedoomSymDSXStep   = 0x001B020C
	iedoomSymDSYFrac   = 0x001B0210
	iedoomSymDSXFrac   = 0x001B0214
	iedoomSymDSColMap  = 0x001B0218
	iedoomSymDSX2      = 0x001B021C
	iedoomSymDSX1      = 0x001B0220
	iedoomSymDSY       = 0x001B0224
	iedoomSymDSSource  = 0x001B0204
	iedoomSymDCSource  = 0x001B0238
	iedoomSymDCTexMid  = 0x001B023C
	iedoomSymDCIScale  = 0x001B0240
	iedoomSymDCYH      = 0x001B0244
	iedoomSymDCYL      = 0x001B0248
	iedoomSymDCX       = 0x001B024C
	iedoomSymDCColMap  = 0x001B0250
	iedoomSymColumnOfs = 0x001B0560
	iedoomSymYLookup   = 0x001B16E0
	iedoomSymCenterY   = 0x001B9660

	iedoomRendererFB      = 0x00220000
	iedoomRendererSource  = 0x00250000
	iedoomRendererColMap  = 0x00260000
	iedoomRendererStack   = 0x002F0000
	iedoomRendererRetAddr = 0x002F1000
)

func newIEDoomRendererCPU(t *testing.T, jit bool) *CPU_X86 {
	t.Helper()
	rom := filepath.Join("..", "chocolate-doom", "build", "iedoom.ie86")
	data, err := os.ReadFile(rom)
	if err != nil {
		t.Skipf("IEDoom linked image not present: %v", err)
	}
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	if len(data) > len(cpu.memory) {
		t.Fatalf("IEDoom image too large: %d > %d", len(data), len(cpu.memory))
	}
	copy(cpu.memory, data)
	cpu.ESP = iedoomRendererStack
	cpu.memory[iedoomRendererRetAddr] = 0xF4 // HLT
	binaryLE32(cpu.memory[iedoomRendererStack:], iedoomRendererRetAddr)
	cpu.x86JitEnabled = jit
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	return cpu
}

func runIEDoomRendererCPU(t *testing.T, cpu *CPU_X86, jit bool) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.Halted = false
		if jit {
			cpu.X86ExecuteJIT()
		} else {
			cpu.x86RunInterpreter()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("IEDoom renderer kernel timed out")
	}
}

func binaryLE32(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}

func iedoomWrite32(cpu *CPU_X86, addr, v uint32) {
	binaryLE32(cpu.memory[addr:], v)
}

func initIEDoomRendererTables(cpu *CPU_X86) {
	for i := uint32(0); i < 320; i++ {
		iedoomWrite32(cpu, iedoomSymColumnOfs+i*4, i)
	}
	for y := uint32(0); y < 200; y++ {
		iedoomWrite32(cpu, iedoomSymYLookup+y*4, iedoomRendererFB+y*320)
	}
	for i := uint32(0); i < 4096; i++ {
		cpu.memory[iedoomRendererSource+i] = byte((i*37 + i>>3 + 11) & 0xFF)
	}
	for i := uint32(0); i < 256; i++ {
		cpu.memory[iedoomRendererColMap+i] = byte((i*13 + 7) & 0xFF)
	}
	for i := uint32(0); i < 320*200; i++ {
		cpu.memory[iedoomRendererFB+i] = 0
	}
}

func initIEDoomDrawSpan(cpu *CPU_X86) {
	initIEDoomRendererTables(cpu)
	iedoomWrite32(cpu, iedoomSymDSSource, iedoomRendererSource)
	iedoomWrite32(cpu, iedoomSymDSColMap, iedoomRendererColMap)
	iedoomWrite32(cpu, iedoomSymDSX1, 17)
	iedoomWrite32(cpu, iedoomSymDSX2, 103)
	iedoomWrite32(cpu, iedoomSymDSY, 43)
	iedoomWrite32(cpu, iedoomSymDSXFrac, 0x12345000)
	iedoomWrite32(cpu, iedoomSymDSYFrac, 0x0ABC0000)
	iedoomWrite32(cpu, iedoomSymDSXStep, 0x00013000)
	iedoomWrite32(cpu, iedoomSymDSYStep, 0x0000E000)
	cpu.EIP = iedoomSymRDrawSpan
}

func initIEDoomDrawColumn(cpu *CPU_X86) {
	initIEDoomRendererTables(cpu)
	iedoomWrite32(cpu, iedoomSymDCSource, iedoomRendererSource)
	iedoomWrite32(cpu, iedoomSymDCColMap, iedoomRendererColMap)
	iedoomWrite32(cpu, iedoomSymDCX, 77)
	iedoomWrite32(cpu, iedoomSymDCYL, 12)
	iedoomWrite32(cpu, iedoomSymDCYH, 93)
	iedoomWrite32(cpu, iedoomSymDCIScale, 0x00012000)
	iedoomWrite32(cpu, iedoomSymDCTexMid, 0x00340000)
	iedoomWrite32(cpu, iedoomSymCenterY, 100)
	cpu.EIP = iedoomSymRDrawColumn
}

func TestX86JIT_IEDoomDrawSpanMatchesInterpreter(t *testing.T) {
	interp := newIEDoomRendererCPU(t, false)
	jit := newIEDoomRendererCPU(t, true)
	initIEDoomDrawSpan(interp)
	initIEDoomDrawSpan(jit)

	runIEDoomRendererCPU(t, interp, false)
	runIEDoomRendererCPU(t, jit, true)

	want := interp.memory[iedoomRendererFB : iedoomRendererFB+320*200]
	got := jit.memory[iedoomRendererFB : iedoomRendererFB+320*200]
	if !bytes.Equal(got, want) {
		t.Fatalf("R_DrawSpan framebuffer mismatch")
	}
}

func TestX86JIT_IEDoomDrawColumnMatchesInterpreter(t *testing.T) {
	interp := newIEDoomRendererCPU(t, false)
	jit := newIEDoomRendererCPU(t, true)
	initIEDoomDrawColumn(interp)
	initIEDoomDrawColumn(jit)

	runIEDoomRendererCPU(t, interp, false)
	runIEDoomRendererCPU(t, jit, true)

	want := interp.memory[iedoomRendererFB : iedoomRendererFB+320*200]
	got := jit.memory[iedoomRendererFB : iedoomRendererFB+320*200]
	if !bytes.Equal(got, want) {
		t.Fatalf("R_DrawColumn framebuffer mismatch")
	}
}
