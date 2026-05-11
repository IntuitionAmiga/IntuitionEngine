// m68kto64_shadow_ccr_runtime_test.go
//
// End-to-end audit of m68kto64's shadow-CCR lowering. Each case feeds a
// minimal m68k source through the deployed transpile+assemble pipeline
// (sdk/bin/m68kto64 → sdk/bin/ie64asm), runs the resulting .ie64 on a
// real CPU64, and asserts the shadow registers (r24..r28) end up with
// the m68k-spec N/Z/V/C/X values for the given inputs.
//
// Skips if sdk/bin/{m68kto64,ie64asm} are not built.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func findM68KTo64ForTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Dir(thisFile)
	cand := filepath.Join(repoRoot, "sdk", "bin", "m68kto64")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return ""
}

// transpileAndAssemble runs the m68k source through m68kto64 (no header
// noise) and ie64asm, returning the assembled binary. Skips the calling
// test if either tool is missing.
func transpileAndAssemble(t *testing.T, m68kSrc string) []byte {
	t.Helper()
	m68 := findM68KTo64ForTest(t)
	asm := findIE64AsmForTest(t)
	if m68 == "" || asm == "" {
		t.Skip("sdk/bin/m68kto64 or sdk/bin/ie64asm missing (run `make m68kto64 ie64asm`)")
	}

	dir := t.TempDir()
	mPath := filepath.Join(dir, "in.s")
	iePath := filepath.Join(dir, "out_ie64.s")
	binPath := filepath.Join(dir, "out.bin")

	if err := os.WriteFile(mPath, []byte(m68kSrc), 0o644); err != nil {
		t.Fatalf("write m68k source: %v", err)
	}

	cmd := exec.Command(m68, "-no-header", "-o", iePath, mPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("m68kto64 failed: %v\n%s", err, out)
	}
	body, err := os.ReadFile(iePath)
	if err != nil {
		t.Fatalf("read transpiled: %v", err)
	}

	// Prepend `org $1000` and append HALT so the assembled image is
	// self-contained.
	wrapped := "\torg $1000\ntest_entry:\n" + string(body) + "\n\thalt\n"
	wrapPath := filepath.Join(dir, "wrapped.s")
	if err := os.WriteFile(wrapPath, []byte(wrapped), 0o644); err != nil {
		t.Fatalf("write wrapped source: %v", err)
	}

	cmd = exec.Command(asm, "-o", binPath, wrapPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ie64asm failed: %v\n--- m68kto64 output ---\n%s\n--- asm err ---\n%s",
			err, body, out)
	}
	bin, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read bin: %v", err)
	}
	_ = strings.TrimSpace // keep import
	return bin
}

func runToHalt(t *testing.T, bin []byte, maxSteps int) *CPU64 {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.LoadProgramBytes(bin)
	cpu.PC = PROG_START

	for i := 0; i < maxSteps; i++ {
		if cpu.PC == 0 {
			break
		}
		if cpu.memory[cpu.PC] == OP_HALT64 {
			break
		}
		cpu.StepOne()
	}
	return cpu
}

// shadowCase captures one m68k operation and the expected N/Z/C/V/X
// values after lowering. Inputs are written into d0 (dst) and d1
// (src); the producer runs against (d0, d1) at the given size.
type shadowCase struct {
	name string
	op   string // m68k op like "sub.w", "add.w"
	d0   uint32 // dst preload
	d1   uint32 // src preload
	n, z, c, v, x byte // expected shadow values (0 or 1)
}

// runShadowCase emits `move.l #d0,d0; move.l #d1,d1; <op> d1,d0` and
// asserts the shadow registers post-op.
func runShadowCase(t *testing.T, c shadowCase) {
	t.Helper()
	src := "\tmove.l\t#$" + hexU32(c.d0) + ",d0\n" +
		"\tmove.l\t#$" + hexU32(c.d1) + ",d1\n" +
		"\t" + c.op + "\td1,d0\n"
	bin := transpileAndAssemble(t, src)
	cpu := runToHalt(t, bin, 200)

	// Per cmd/m68kto64/ccr_shadow.go (canonical):
	//   r24 = ShadowN (sext result; sign-bit reflects N)
	//   r25 = ShadowZ (raw masked result; zero ⇔ Z=1)
	//   r26 = ShadowC (0/1)
	//   r27 = ShadowV (0/1)
	//   r28 = ShadowX (0/1)
	gotN := boolBit(int64(cpu.regs[24]) < 0)
	gotZ := boolBit(cpu.regs[25] == 0)
	gotC := boolBit(cpu.regs[26] != 0)
	gotV := boolBit(cpu.regs[27] != 0)
	gotX := boolBit(cpu.regs[28] != 0)

	if gotN != c.n || gotZ != c.z || gotC != c.c || gotV != c.v || gotX != c.x {
		t.Errorf("%s d0=%#x d1=%#x:\n  got  N=%d Z=%d C=%d V=%d X=%d\n  want N=%d Z=%d C=%d V=%d X=%d\n  r24=%#x r25=%#x r26=%#x r27=%#x r28=%#x",
			c.name, c.d0, c.d1, gotN, gotZ, gotC, gotV, gotX,
			c.n, c.z, c.c, c.v, c.x,
			cpu.regs[24], cpu.regs[25], cpu.regs[26], cpu.regs[27], cpu.regs[28])
	}
}

func hexU32(v uint32) string {
	const digits = "0123456789ABCDEF"
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = digits[v&0xF]
		v >>= 4
	}
	return string(b)
}

func boolBit(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// TestShadowCCR_SubW exercises sub.w d1,d0 truth-table cases.
// m68k semantic: result = dst - src, then N=sign(result), Z=(result==0),
// C=borrow, V=signed overflow, X=C.
func TestShadowCCR_SubW(t *testing.T) {
	cases := []shadowCase{
		{"1-1=0",        "sub.w", 0x00000001, 0x00000001, 0, 1, 0, 0, 0},
		{"0-1=-1",       "sub.w", 0x00000000, 0x00000001, 1, 0, 1, 0, 1},
		{"-32768-1",     "sub.w", 0x00008000, 0x00000001, 0, 0, 0, 1, 0},
		{"32767-(-1)",   "sub.w", 0x00007FFF, 0x0000FFFF, 1, 0, 1, 1, 1},
		{"-1 - -1 = 0",  "sub.w", 0x0000FFFF, 0x0000FFFF, 0, 1, 0, 0, 0},
		{"$1234-$5678",  "sub.w", 0x00001234, 0x00005678, 1, 0, 1, 0, 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { runShadowCase(t, c) })
	}
}

// TestShadowCCR_AddW exercises add.w d1,d0 truth-table cases.
// m68k: result = dst + src, N=sign, Z=(result==0), C=carry,
// V=signed overflow, X=C.
func TestShadowCCR_AddW(t *testing.T) {
	cases := []shadowCase{
		{"1+1=2",        "add.w", 0x00000001, 0x00000001, 0, 0, 0, 0, 0},
		{"32767+1",      "add.w", 0x00007FFF, 0x00000001, 1, 0, 0, 1, 0},
		{"-1+1=0",       "add.w", 0x0000FFFF, 0x00000001, 0, 1, 1, 0, 1},
		{"-32768+-32768","add.w", 0x00008000, 0x00008000, 0, 1, 1, 1, 1},
		{"$1234+$5678",  "add.w", 0x00001234, 0x00005678, 0, 0, 0, 0, 0},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { runShadowCase(t, c) })
	}
}

// TestShadowCCR_SubL exercises sub.l d1,d0 — full 32-bit width.
func TestShadowCCR_SubL(t *testing.T) {
	cases := []shadowCase{
		{"1-1=0",         "sub.l", 0x00000001, 0x00000001, 0, 1, 0, 0, 0},
		{"0-1=-1",        "sub.l", 0x00000000, 0x00000001, 1, 0, 1, 0, 1},
		{"INT32_MIN-1",   "sub.l", 0x80000000, 0x00000001, 0, 0, 0, 1, 0},
		{"INT32_MAX-(-1)","sub.l", 0x7FFFFFFF, 0xFFFFFFFF, 1, 0, 1, 1, 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { runShadowCase(t, c) })
	}
}

// TestShadowCCR_AddL exercises add.l d1,d0 — full 32-bit width.
func TestShadowCCR_AddL(t *testing.T) {
	cases := []shadowCase{
		{"1+1=2",         "add.l", 0x00000001, 0x00000001, 0, 0, 0, 0, 0},
		{"INT32_MAX+1",   "add.l", 0x7FFFFFFF, 0x00000001, 1, 0, 0, 1, 0},
		{"-1+1=0",        "add.l", 0xFFFFFFFF, 0x00000001, 0, 1, 1, 0, 1},
		{"INT32_MIN+INT32_MIN", "add.l", 0x80000000, 0x80000000, 0, 1, 1, 1, 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { runShadowCase(t, c) })
	}
}
