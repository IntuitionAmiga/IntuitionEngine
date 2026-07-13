package main

// Phase 5: BASIC instance-aware coprocessor keywords.
//   COSTART cpuType[,instance],"svc"
//   COSTOP  cpuType[,instance]
//   COCALL(cpuType[,instance],op,req,len,resp,cap)
//   COCAPS(cpuType)  COINSTANCE()  COSELSTATE()

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEhBASIC_CoprocInstanceRoundTrip is the headline Phase 5 check: from BASIC,
// start the SAME M68K service image as instance 0 and instance 1, drive a COCALL
// to each, and confirm both answer independently. M68K is the type with a
// shipped service image to hand, and the bootstrap seed lets one image serve
// either instance ring.
func TestEhBASIC_CoprocInstanceRoundTrip(t *testing.T) {
	asmBin := buildAssembler(t)
	data := assembleService(t, []string{
		"vasmm68k_mot", "-Fbin", "-m68020", "-devpac", "-I", "sdk/include", "-o", "OUTPUT",
	}, "sdk/examples/asm/coproc_service_68k.asm")
	svcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(svcDir, "svc.ie68"), data, 0o644); err != nil {
		t.Fatalf("write svc: %v", err)
	}

	// COSTART both instances; COCALL op=1 (add) to each; COWAIT + COSTATUS.
	// The response status descriptor lives in the mailbox (no M68K byte-swap),
	// so COSTATUS==2 confirms each instance processed its own request. The old
	// 6-arg COCALL form is exercised alongside the 7-arg instance form.
	prog := `10 COSTART 4,0,"svc.ie68"
20 COSTART 4,1,"svc.ie68"
30 REQ=MEMALLOC(8,4):RESP=MEMALLOC(16,4)
40 POKE32 REQ,10:POKE32 REQ+4,20
50 T0=COCALL(4,1,REQ,8,RESP,16)
60 COWAIT T0,2000
70 PRINT "S0 ";COSTATUS(T0)
80 T1=COCALL(4,1,1,REQ,8,RESP,16)
90 COWAIT T1,2000
100 PRINT "S1 ";COSTATUS(T1)`
	out, _, _ := execStmtTestWithCoproc(t, asmBin, prog, svcDir)

	assertLineValue(t, out, "S0", "2") // instance 0 answered OK (6-arg COCALL)
	assertLineValue(t, out, "S1", "2") // instance 1 answered OK (7-arg COCALL)
}

// execStmtTestWithCoprocExt is execStmtTestWithCoproc with the EXT2 discovery
// block mapped, so COCAPS/COSELSTATE resolve.
func execStmtTestWithCoprocExt(t *testing.T, asmBin, program, baseDir string) (string, *ehbasicTestHarness, *CoprocessorManager) {
	t.Helper()
	var mgr *CoprocessorManager
	out, h := execStmtTestCore(t, asmBin, program, func(h *ehbasicTestHarness) {
		mgr = NewCoprocessorManager(h.bus, baseDir)
		h.bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)
		h.bus.MapIO(COPROC_EXT2_BASE, COPROC_EXT2_END, mgr.HandleRead, mgr.HandleWrite)
	})
	return out, h, mgr
}

// TestEhBASIC_CoprocCapabilityFuncs pins COCAPS/COINSTANCE/COSELSTATE, and that
// COCAPS(cpuType) and COINSTANCE() have distinct semantics.
func TestEhBASIC_CoprocCapabilityFuncs(t *testing.T) {
	asmBin := buildAssembler(t)
	prog := `10 PRINT "CAP4 ";COCAPS(4)
20 PRINT "CAP1 ";COCAPS(1)
30 PRINT "INST ";COINSTANCE()`
	out, _, _ := execStmtTestWithCoprocExt(t, asmBin, prog, t.TempDir())

	// M68K (type 4) supports 2 instances; IE32 (type 1) supports 1; the
	// currently selected instance is 0. COCAPS(4)=2 differs from COINSTANCE()=0.
	assertLineValue(t, out, "CAP4", "2")
	assertLineValue(t, out, "CAP1", "1")
	assertLineValue(t, out, "INST", "0")
}

// TestEhBASIC_CocallArgForms verifies the 6-arg (instance 0) and 7-arg
// (explicit instance) COCALL forms both parse and write COPROC_INSTANCE.
func TestEhBASIC_CocallArgForms(t *testing.T) {
	asmBin := buildAssembler(t)
	// No workers are started, so each COCALL fails with NO_WORKER and returns 0,
	// but the instance selector it wrote is observable at COPROC_INSTANCE.
	prog := `10 REQ=MEMALLOC(8,4):RESP=MEMALLOC(16,4)
20 A=COCALL(4,1,1,REQ,4,RESP,4)
30 PRINT "I7 ";PEEK32(&H000F238C)
40 B=COCALL(3,1,REQ,4,RESP,4)
50 PRINT "I6 ";PEEK32(&H000F238C)
60 PRINT "AB ";A;" ";B`
	out, _, _ := execStmtTestWithCoprocExt(t, asmBin, prog, t.TempDir())

	assertLineValue(t, out, "I7", "1") // 7-arg wrote instance 1
	assertLineValue(t, out, "I6", "0") // 6-arg wrote instance 0
	if !strings.Contains(out, "AB  0  0") && !strings.Contains(out, "AB 0 0") {
		t.Errorf("expected both COCALL forms to return 0 (no worker), got %q", out)
	}
}

// TestEhBASIC_CocallWrongArgCountFC asserts a malformed COCALL argument count
// raises ?FC.
func TestEhBASIC_CocallWrongArgCountFC(t *testing.T) {
	asmBin := buildAssembler(t)
	prog := `10 A=COCALL(4,1)`
	out, _, _ := execStmtTestWithCoprocExt(t, asmBin, prog, t.TempDir())
	if !strings.Contains(out, "FC") && !strings.Contains(strings.ToUpper(out), "ERROR") {
		t.Errorf("expected ?FC error for 2-arg COCALL, got %q", out)
	}
}

// TestEhBASIC_CostartInstanceParsing pins that COSTART parses the optional
// instance and writes COPROC_CPU_TYPE / COPROC_INSTANCE before START, for both
// the 2-arg and 3-arg forms.
func TestEhBASIC_CostartInstanceParsing(t *testing.T) {
	asmBin := buildAssembler(t)

	// 3-arg form: COSTART cpuType, instance, "file". File is absent so START
	// fails, but the selectors are written first.
	out3, h3, _ := execStmtTestWithCoprocExt(t, asmBin, `10 COSTART 4,1,"nofile.ie68"`, t.TempDir())
	_ = out3
	if got := h3.bus.Read32(COPROC_CPU_TYPE); got != 4 {
		t.Errorf("3-arg COSTART CPU_TYPE = %d, want 4", got)
	}
	if got := h3.bus.Read32(COPROC_INSTANCE); got != 1 {
		t.Errorf("3-arg COSTART INSTANCE = %d, want 1", got)
	}

	// 2-arg form: COSTART cpuType, "file". Instance defaults to 0.
	_, h2, _ := execStmtTestWithCoprocExt(t, asmBin, `10 COSTART 4,"nofile.ie68"`, t.TempDir())
	if got := h2.bus.Read32(COPROC_CPU_TYPE); got != 4 {
		t.Errorf("2-arg COSTART CPU_TYPE = %d, want 4", got)
	}
	if got := h2.bus.Read32(COPROC_INSTANCE); got != 0 {
		t.Errorf("2-arg COSTART INSTANCE = %d, want 0", got)
	}
}

// assertLineValue checks that the output contains a line beginning with label
// followed by the expected numeric value (BASIC prints numbers with a leading
// space and trailing space).
func assertLineValue(t *testing.T, out, label, want string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), label) {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[len(fields)-1] == want {
				return
			}
			t.Errorf("line %q: got fields %v, want last = %q", line, fields, want)
			return
		}
	}
	t.Errorf("output missing line %q (want %s): %q", label, want, out)
}
