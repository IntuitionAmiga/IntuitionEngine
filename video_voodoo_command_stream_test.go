//go:build headless

package main

import (
	"encoding/binary"
	"testing"
)

func writeVoodooCommandStream(mem []byte, addr uint32, cmds ...uint32) uint32 {
	if len(cmds)%2 != 0 {
		panic("command stream requires addr/value pairs")
	}
	for i, v := range cmds {
		binary.BigEndian.PutUint32(mem[int(addr)+i*4:int(addr)+i*4+4], v)
	}
	return uint32(len(cmds) / 2)
}

func writeM68KVoodooCommandStream(cpu *M68KCPU, addr uint32, cmds ...uint32) uint32 {
	if len(cmds)%2 != 0 {
		panic("command stream requires addr/value pairs")
	}
	for i, v := range cmds {
		cpu.Write32(addr+uint32(i*4), v)
	}
	return uint32(len(cmds) / 2)
}

func TestVoodoo_CommandStream_ReplaysRegisterWrites(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	stream := uint32(0x2000)
	count := writeVoodooCommandStream(bus.GetMemory(), stream,
		VOODOO_ENABLE, 1,
		VOODOO_COLOR_SELECT, 0,
		VOODOO_VERTEX_AX, 16,
		VOODOO_VERTEX_AY, 32,
		VOODOO_START_R, 0x1000,
		VOODOO_START_W, 0x40000000,
		VOODOO_TRIANGLE_CMD, 0,
	)

	bus.Write32(VOODOO_CMD_PTR, stream)
	bus.Write32(VOODOO_CMD_COUNT, count)
	bus.Write32(VOODOO_CMD_SUBMIT, VOODOO_CMD_SUBMIT_REPLAY)

	if !v.IsEnabled() {
		t.Fatal("command stream did not enable Voodoo")
	}
	if got := len(v.triangleBatch); got != 1 {
		t.Fatalf("triangle batch length = %d, want 1", got)
	}
	tri := v.triangleBatch[0]
	if tri.Vertices[0].X != 1.0 || tri.Vertices[0].Y != 2.0 {
		t.Fatalf("vertex A = (%f,%f), want (1,2)", tri.Vertices[0].X, tri.Vertices[0].Y)
	}
	if tri.Vertices[0].R != 1.0 || tri.Vertices[0].W != 1.0 {
		t.Fatalf("vertex A attrs R=%f W=%f, want 1,1", tri.Vertices[0].R, tri.Vertices[0].W)
	}
}

func TestVoodoo_CommandStream_ReplaysM68KGuestRAMWrites(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	cpu := NewM68KCPU(bus)
	stream := uint32(0x120000)
	count := writeM68KVoodooCommandStream(cpu, stream,
		VOODOO_ENABLE, 1,
		VOODOO_COLOR_SELECT, 0,
		VOODOO_VERTEX_AX, 16,
		VOODOO_VERTEX_AY, 32,
		VOODOO_COLOR_SELECT, 1,
		VOODOO_VERTEX_BX, 48,
		VOODOO_VERTEX_BY, 64,
		VOODOO_COLOR_SELECT, 2,
		VOODOO_VERTEX_CX, 80,
		VOODOO_VERTEX_CY, 96,
		VOODOO_START_R, 0x1000,
		VOODOO_START_W, 0x40000000,
		VOODOO_TRIANGLE_CMD, 0,
	)

	bus.Write32(VOODOO_CMD_PTR, stream)
	bus.Write32(VOODOO_CMD_COUNT, count)
	bus.Write32(VOODOO_CMD_SUBMIT, VOODOO_CMD_SUBMIT_REPLAY)

	if got := len(v.triangleBatch); got != 1 {
		t.Fatalf("triangle batch length = %d, want 1", got)
	}
	tri := v.triangleBatch[0]
	want := [3][2]float32{{1, 2}, {3, 4}, {5, 6}}
	for i := range want {
		if tri.Vertices[i].X != want[i][0] || tri.Vertices[i].Y != want[i][1] {
			t.Fatalf("vertex %d = (%f,%f), want (%f,%f)",
				i, tri.Vertices[i].X, tri.Vertices[i].Y, want[i][0], want[i][1])
		}
	}
}

func TestVoodoo_CommandStream_IgnoresControlRegisterRecursion(t *testing.T) {
	bus, v := newMappedTestVoodoo(t)
	stream := uint32(0x2400)
	count := writeVoodooCommandStream(bus.GetMemory(), stream,
		VOODOO_CMD_SUBMIT, VOODOO_CMD_SUBMIT_REPLAY,
		VOODOO_ENABLE, 1,
	)

	bus.Write32(VOODOO_CMD_PTR, stream)
	bus.Write32(VOODOO_CMD_COUNT, count)
	bus.Write32(VOODOO_CMD_SUBMIT, VOODOO_CMD_SUBMIT_REPLAY)

	if !v.IsEnabled() {
		t.Fatal("command stream stopped after recursive submit entry")
	}
}
