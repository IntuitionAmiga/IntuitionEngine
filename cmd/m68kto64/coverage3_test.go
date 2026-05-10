package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Hit error branches across all emit* paths.

func mustErr(t *testing.T, src string, errSubstr string) {
	t.Helper()
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	out, errs := c.ConvertSource(src)
	if errs == 0 {
		t.Errorf("%q: expected error containing %q, got clean output:\n%s", src, errSubstr, out)
	}
}

func TestErr_MoveQ_BadOperands(t *testing.T) {
	mustErr(t, "\tmoveq d0,d1\n", "imm")        // moveq needs #imm,Dn
	mustErr(t, "\tmoveq #5,(a0)\n", "moveq")
}

func TestErr_Movea_NeedsAn(t *testing.T) {
	mustErr(t, "\tmovea.l d0,d1\n", "movea")
}

func TestErr_Lea_DstNotAn(t *testing.T) {
	mustErr(t, "\tlea (a0),d0\n", "lea")
}

func TestErr_Btst_BadBitOperand(t *testing.T) {
	mustErr(t, "\tbtst (a0),d0\n", "btst")
}

func TestErr_Bfins_SrcNotDn(t *testing.T) {
	mustErr(t, "\tbfins #5,d0{#0:#8}\n", "source must be Dn")
}

func TestErr_Bfffo_DstNotDn(t *testing.T) {
	mustErr(t, "\tbfffo d0{#0:#8},(a0)\n", "destination must be Dn")
}

func TestErr_Pack_BadAdj(t *testing.T) {
	mustErr(t, "\tpack d0,d1,d2\n", "imm")
}

func TestErr_Cas_DcNotDn(t *testing.T) {
	mustErr(t, "\tcas.l (a0),d0,(a1)\n", "Dn")
}

func TestErr_Trap_NotImm(t *testing.T) {
	mustErr(t, "\ttrap d0\n", "imm")
}

func TestErr_Link_FirstNotAn(t *testing.T) {
	mustErr(t, "\tlink d0,#-12\n", "An")
}

func TestErr_Unlk_NotAn(t *testing.T) {
	mustErr(t, "\tunlk d0\n", "An")
}

func TestErr_Dbra_FirstNotDn(t *testing.T) {
	mustErr(t, "\tdbra a0,L\n", "Dn")
}

func TestErr_Swap_NotDn(t *testing.T) {
	mustErr(t, "\tswap a0\n", "swap")
}

// Coverage for unconditional Scc forms (st/sf — separate from cc/cs branch).
func TestSt_OnMem(t *testing.T) {
	out := convertOneInstr(t, "\tst (a0)")
	mustContain(t, out, "store.b r17, (r9)")
}

func TestSf_OnMem(t *testing.T) {
	out := convertOneInstr(t, "\tsf (a0)")
	mustContain(t, out, "store.b r0, (r9)")
}

// Scc on memory destination — exercises emitWriteByteConst memory branch.
func TestScc_OnMem(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tseq (a0)\n")
	mustContain(t, out, "store.b") // byte write of 0xFF or 0x00
}

// emitMove with src memory + dst An via MOVEA.l (a0),a1.
func TestMovea_MemSrc(t *testing.T) {
	out := convertOneInstr(t, "\tmovea.l (a0),a1")
	mustContain(t, out, "load.l r17, (r9)")
	mustContain(t, out, "move.l r10, r17")
}

// JSR with disp(An).
func TestJsr_DispAn(t *testing.T) {
	out := convertOneInstr(t, "\tjsr 8(a0)")
	mustContain(t, out, "jmp 8(r9)")
}

// JSR indexed.
func TestJsr_Indexed(t *testing.T) {
	out := convertOneInstr(t, "\tjsr (8,a0,d0.l*4)")
	mustContain(t, out, "jmp (r16)")
}

// CHK against memory bound.
func TestChk_MemBound(t *testing.T) {
	out := convertOneInstr(t, "\tchk.w (a0),d1")
	mustContain(t, out, "load.w r17, (r9)")
	mustContain(t, out, "syscall #17")
}

// MULU.L pair with immediate src — exercises imm-materialise branch.
func TestMuluL_PairImm(t *testing.T) {
	out := convertOneInstr(t, "\tmulu.l #100,d2:d3")
	mustContain(t, out, "move.l r17, #100")
	mustContain(t, out, "mulu.l r4")
}

// DIVU.L pair with immediate src.
func TestDivuL_PairImm(t *testing.T) {
	out := convertOneInstr(t, "\tdivu.l #100,d2:d3")
	mustContain(t, out, "move.l r17, #100")
}

// =====================================================================
// CLI smoke test against a built binary.
// =====================================================================

func TestCLI_Smoke(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot locate repo root: %v", err)
	}
	bin := filepath.Join(repoRoot, "sdk", "bin", "m68kto64")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("sdk/bin/m68kto64 not built: %v", err)
	}
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in.s")
	out := filepath.Join(tmp, "out.s")
	if err := os.WriteFile(in, []byte("\tmove.l #1,d0\n\trts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-no-header", "-o", out, in)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI invocation failed: %v\n%s", err, combined)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// MOVE writes through ScrV1 then to dst, plus emits NZ/CV shadows.
	if !strings.Contains(string(body), "move.l r17, #1") || !strings.Contains(string(body), "move.l r1, r17") {
		t.Errorf("CLI output missing expected instructions:\n%s", body)
	}
}

func TestCLI_ErrorOnMissingFile(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot locate repo root: %v", err)
	}
	bin := filepath.Join(repoRoot, "sdk", "bin", "m68kto64")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("sdk/bin/m68kto64 not built: %v", err)
	}
	cmd := exec.Command(bin, "-o", "/tmp/x.s", "/nonexistent/file.s")
	if err := cmd.Run(); err == nil {
		t.Errorf("CLI should exit nonzero on missing input")
	}
}

func TestCLI_NoArgs_ShowsUsage(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot locate repo root: %v", err)
	}
	bin := filepath.Join(repoRoot, "sdk", "bin", "m68kto64")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("sdk/bin/m68kto64 not built: %v", err)
	}
	cmd := exec.Command(bin)
	combined, _ := cmd.CombinedOutput()
	if !strings.Contains(string(combined), "Usage:") {
		t.Errorf("expected usage output, got:\n%s", combined)
	}
}
