//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
)

// TestM68KMemCopyLoopFires verifies the MemCopy classifier matches the
// canonical bench shape and that the compile path actually invokes the
// specialized fast path.
func TestM68KMemCopyLoopFires(t *testing.T) {
	mem := make([]byte, 0x100000)
	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			mem[pc] = byte(op >> 8)
			mem[pc+1] = byte(op)
			pc += 2
		}
	}
	w(0x41F9, 0, 0x4000) // LEA src,A0
	w(0x43F9, 0, 0x5000) // LEA dest,A1
	w(0x3E3C, 9999)      // MOVE.W #9999,D7
	loopTop := pc
	w(0x22D8) // MOVE.L (A0)+,(A1)+
	disp := int16(int32(loopTop) - int32(pc) - 2)
	w(0x51CF, uint16(disp)) // DBRA D7,loop
	w(0x4E72, 0x2700)       // STOP

	instrs := m68kScanBlock(mem, loopTop)
	if len(instrs) < 2 {
		t.Fatalf("expected >=2 instrs at loopTop, got %d", len(instrs))
	}
	srcAn, dstAn, ctrDn, size, nextPC, ok := m68kIsMemCopyLoopBlock(instrs, loopTop, mem)
	if !ok {
		t.Fatal("classifier rejected canonical MemCopy loop")
	}
	if srcAn != 0 || dstAn != 1 || ctrDn != 7 {
		t.Errorf("classifier returned wrong regs: srcAn=%d dstAn=%d ctrDn=%d", srcAn, dstAn, ctrDn)
	}
	if size != M68K_SIZE_LONG {
		t.Errorf("size = %d, want LONG", size)
	}
	if nextPC != loopTop+6 {
		t.Errorf("nextPC = 0x%X, want 0x%X", nextPC, loopTop+6)
	}

	hitsBefore := m68kMemCopyLoopHits
	em, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer em.Free()
	if _, err := m68kCompileBlockWithMem(instrs, loopTop, em, mem); err != nil {
		t.Fatalf("compile: %v", err)
	}
	if m68kMemCopyLoopHits == hitsBefore {
		t.Fatal("compile did not fire MemCopy loop spec")
	}
}

// TestM68KMemCopyLoopRejects ensures the classifier turns down shapes that
// are not the exact MemCopy canonical form.
func TestM68KMemCopyLoopRejects(t *testing.T) {
	mem := make([]byte, 0x100000)
	cases := []struct {
		name  string
		setup func(w func(ops ...uint16)) (loopTop uint32)
	}{
		{
			"src and dst same An",
			func(w func(ops ...uint16)) uint32 {
				w(0x21D0)         // MOVE.L (A0)+,(A0)+ — sReg=0 dReg=0
				w(0x51CF, 0xFFFC) // DBRA D7,loop
				return 0x1000
			},
		},
		{
			"DBRA target not block start",
			func(w func(ops ...uint16)) uint32 {
				w(0x22D8)         // MOVE.L (A0)+,(A1)+
				w(0x51CF, 0xFFF0) // DBRA D7, far back
				return 0x1000
			},
		},
		{
			"first instr is not MOVE.L postinc-postinc",
			func(w func(ops ...uint16)) uint32 {
				w(0x2018) // MOVE.L (A0)+,D0 — dstMode=0
				w(0x51CF, 0xFFFC)
				return 0x1000
			},
		},
		{
			"second instr is not DBRA",
			func(w func(ops ...uint16)) uint32 {
				w(0x22D8) // MOVE.L (A0)+,(A1)+
				w(0x60FE) // BRA -2
				return 0x1000
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for i := range mem {
				mem[i] = 0
			}
			pc := uint32(0x1000)
			w := func(ops ...uint16) {
				for _, op := range ops {
					mem[pc] = byte(op >> 8)
					mem[pc+1] = byte(op)
					pc += 2
				}
			}
			loopTop := c.setup(w)
			instrs := m68kScanBlock(mem, loopTop)
			if _, _, _, _, _, ok := m68kIsMemCopyLoopBlock(instrs, loopTop, mem); ok {
				t.Fatalf("%s: classifier should have rejected", c.name)
			}
		})
	}
}
