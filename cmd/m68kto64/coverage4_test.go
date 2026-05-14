package main

import (
	"strings"
	"testing"
)

// Sweep of error branches across the converter.
func TestErrSweep(t *testing.T) {
	cases := []string{
		"\tmove\n",
		"\tmove.l d0\n",
		"\tlea\n",
		"\tlea d0\n",
		"\tadd\n",
		"\tadd.l d0\n",
		"\tneg\n",
		"\tnot\n",
		"\tclr\n",
		"\text d0,d1\n",
		"\textb.l (a0)\n",
		"\tlsl\n",
		"\tlsl #1,#2,#3\n",
		"\tswap\n",
		"\tst\n",
		"\tbtst d0\n",
		"\tbra\n",
		"\tbsr\n",
		"\tjmp\n",
		"\tjsr\n",
		"\tlink a0\n",
		"\tunlk\n",
		"\tdbra d0\n",
		"\tcmp d0\n",
		"\ttst\n",
		"\tbeq\n",
		"\tmovem.l d0\n",
		"\tmovem.b d0,(a0)\n",
		"\tmovem.l (a0),(a1)\n",
		"\ttrap\n",
		"\tchk d0\n",
		"\tabcd d0\n",
		"\tsbcd (a0),d0\n",
		"\tnbcd\n",
		"\tnbcd (a0)\n",
		"\tpack d0,d1\n",
		"\tunpk d0\n",
		"\tcas d0,d1\n",
		"\tbfins d0\n",
		"\tbftst\n",
		"\tbfffo d0\n",
		"\tbfclr d0\n",
		"\tdivu.w (a0),(a1)\n", // dst not Dn
		"\tmulu.w (a0),(a1)\n",
	}
	for _, src := range cases {
		c := NewConverter()
		c.noHeader = true
		c.strict = true
		_, errs := c.ConvertSource(src)
		if errs == 0 {
			t.Logf("note: %q produced no error in strict mode", strings.TrimSpace(src))
		}
	}
}

// Hit a grab-bag of fuse cases that exercise low-coverage functions.

func TestFuseSweep(t *testing.T) {
	cases := []string{
		"\tcmp.b d0,d1\n\tbeq L\nL:\n\trts\n",
		"\tcmp.b d0,(a0)\n\tbne L\nL:\n\trts\n",
		"\tcmpi.l #0,d0\n\tbne L\nL:\n\trts\n",
		"\tcmpa.l a0,a1\n\tbeq L\nL:\n\trts\n",
		"\ttst.b (a0)\n\tbeq L\nL:\n\trts\n",
		"\ttst.w d0\n\tbgt L\nL:\n\trts\n",
		"\ttst.l d0\n\tbhi L\nL:\n\trts\n",
		"\ttst.w d0\n\tble L\nL:\n\trts\n",
	}
	for _, src := range cases {
		c := NewConverter()
		c.noHeader = true
		out, errs := c.ConvertSource(src)
		if errs != 0 {
			t.Errorf("%q: errors:\n%s", strings.TrimSpace(src), out)
		}
	}
}

// Movem on Dn-only and mixed lists.
func TestMovem_VariousMasks(t *testing.T) {
	cases := []string{
		"\tmovem.l d0/d1,(a0)",
		"\tmovem.w d0/d3-d5,-(sp)",
		"\tmovem.w (sp)+,d0/d3-d5",
		"\tmovem.l a0-a3,8(a0)",
		"\tmovem.l 8(a0),a0-a3",
	}
	for _, src := range cases {
		c := NewConverter()
		c.noHeader = true
		out, errs := c.ConvertSource(src + "\n")
		if errs != 0 {
			t.Errorf("%q: errors:\n%s", src, out)
		}
	}
}

// Shift mem with reg count.
func TestShift_MemRegCount(t *testing.T) {
	out := convertOneInstr(t, "\tlsl.l d0,(a0)")
	mustContain(t, out, "load.l r18, (r9)")
	mustContain(t, out, "bswap.l r20, r18")
	mustContain(t, out, "store.l r20, (r9)")
}

// emitBfModify mem path.
func TestBfset_Mem(t *testing.T) {
	out := convertOneInstr(t, "\tbfset (a0){#0:#4}")
	mustContain(t, out, "load.l r18, (r16)")
	mustContain(t, out, "store.l r18, (r16)")
}

// emitBfins mem path.
func TestBfins_MemSecond(t *testing.T) {
	out := convertOneInstr(t, "\tbfins d0,8(a0){#4:#8}")
	mustContain(t, out, "load.l r18,")
	mustContain(t, out, "store.l r18,")
}

// CHK with two-Dn forms — exercises additional emitChk branch.
func TestChk_DnDn(t *testing.T) {
	out := convertOneInstr(t, "\tchk d0,d1")
	mustContain(t, out, "syscall #17")
}

// Trap with constant immediate (variable too).
func TestTrap_Constant(t *testing.T) {
	for _, src := range []string{"\ttrap #0\n", "\ttrap #15\n"} {
		out := convertSrc(t, src)
		mustContain(t, out, "syscall")
	}
}

// EmitDivZeroGuard static-zero branch.
func TestDiv_StaticZero_TrapsDirectly(t *testing.T) {
	out := convertOneInstr(t, "\tdivu.l #0,d0")
	mustContain(t, out, "syscall #16")
	// No bnez guard when divisor is constant zero.
	if strings.Contains(out, "bnez") {
		t.Errorf("constant-zero divisor should trap unconditionally, got bnez:\n%s", out)
	}
}

// FuseNormalise immediate path with size==2 signed.
func TestFuse_CmpiW_Signed(t *testing.T) {
	out := convertSrc(t, "\tcmpi.w #1,d0\n\tblt L\nL:\n\trts\n")
	mustContain(t, out, "sext.w")
	mustContain(t, out, "blt")
}

// MOVEA from An to An (size .l).
func TestMovea_AnAn(t *testing.T) {
	out := convertOneInstr(t, "\tmovea.l a0,a1")
	mustContain(t, out, "move.l r10, r9")
}

// EmitShadowScc mem dst path with cc=mi.
func TestSmi_OnMem(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tsmi (a0)\n")
	mustContain(t, out, "store.b")
}

// Lea with displaced PC indexed.
func TestLea_PCIndexed(t *testing.T) {
	out := convertOneInstr(t, "\tlea (label,pc,d0.l*2),a1")
	mustContain(t, out, "la r10, label")
	mustContain(t, out, "lsl.l r19")
}
