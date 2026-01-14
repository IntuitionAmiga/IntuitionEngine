package main

import "testing"

func TestZ80RRegisterIncrementsWithPrefixes(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0xCB, 0x01, 0x06, // RLC (IX+1)
	})
	rig.cpu.IX = 0x1000
	rig.bus.mem[0x1001] = 0x80

	rig.cpu.Step()

	if rig.cpu.R&0x7F != 3 {
		t.Fatalf("R = 0x%02X, want low 7 bits = 3", rig.cpu.R)
	}
}
