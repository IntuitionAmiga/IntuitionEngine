package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// runtimeBlobForTests returns the standalone COMPILE runtime blob, generated once and
// cached. Test File I/O harnesses install it on the device (SetRuntimeBlob), mirroring
// the host serving the embedded blob in production, so COMPILE never depends on a
// sidecar file in the working directory.
var (
	rtBlobOnce sync.Once
	rtBlobData []byte
)

func runtimeBlobForTests(t *testing.T) []byte {
	t.Helper()
	rtBlobOnce.Do(func() {
		rtBlobData = buildRuntimeBlobBin(t)
	})
	return rtBlobData
}

// Phase 0 of COMPILE_RUNTIME_BUNDLE_PLAN.md: the standalone COMPILE runtime blob.
//
// The blob bundles the EhBASIC expression/variable/string/maths/exec closure as a
// position-fixed image linked at AOT_RT_BASE (placement B, the program-text gap).
// COMPILE copies it into standalone .ie64 images. The blob is deterministic from the
// committed runtime sources, so it is generated (assembled + trimmed) rather than
// committed as a binary - the repo .gitignore excludes *.bin and a clean checkout
// would otherwise fail. These tests lock the Phase 0 findings:
//
//   - the blob assembles cleanly from its committed sources,
//   - the org padding is exactly [PROGRAM_START, AOT_RT_BASE) and trims cleanly,
//   - the trimmed blob fits the placement-B budget (ends at or below
//     AOT_RT_LIMIT 0x070000).
//
// buildRuntimeBlobBin generates the trimmed blob bytes the COMPILE path will bundle;
// the build/File I/O wiring (Phase 1) produces aot_runtime_blob.bin from the same
// assemble + trim, so there is no committed binary to drift from.

const (
	aotRTBase        = 0x043000 // AOT_RT_BASE
	aotRTLimit       = 0x070000 // AOT_RT_LIMIT
	aotRTBlobMax     = 0x10000  // AOT_RT_BLOB_MAX: compile-time staging size / hard cap
	aotProgramStart  = 0x001000 // PROG_START
	aotRTOrgPadBytes = aotRTBase - aotProgramStart
)

// assembleRuntimeBlob assembles aot_runtime_blob.asm and returns the raw output
// (which includes the [PROGRAM_START, AOT_RT_BASE) origin padding ie64asm keeps).
func assembleRuntimeBlob(t *testing.T) []byte {
	t.Helper()
	asmBin := buildAssembler(t)
	incDir := filepath.Join(repoRootDir(t), "sdk", "include")
	srcPath := filepath.Join(incDir, "aot_runtime_blob.asm")
	outPath := filepath.Join(t.TempDir(), "aot_runtime_blob.ie64")
	cmd := exec.Command(asmBin, "-I", incDir, "-o", outPath, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		// ie64asm writes beside the source when -o is unsupported; fall back.
		def := filepath.Join(incDir, "aot_runtime_blob.ie64")
		if b, rerr := os.ReadFile(def); rerr == nil {
			defer os.Remove(def)
			return b
		}
		t.Fatalf("assemble runtime blob: %v\n%s", err, out)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		// Older ie64asm ignores -o and writes beside the source.
		def := filepath.Join(incDir, "aot_runtime_blob.ie64")
		b, err = os.ReadFile(def)
		if err != nil {
			t.Fatalf("read assembled runtime blob: %v", err)
		}
		os.Remove(def)
	}
	return b
}

func trimRuntimeBlob(t *testing.T, raw []byte) []byte {
	t.Helper()
	if len(raw) <= aotRTOrgPadBytes {
		t.Fatalf("runtime blob raw size %d <= org pad %d: nothing emitted at AOT_RT_BASE",
			len(raw), aotRTOrgPadBytes)
	}
	// The org pad must be all zero, and the first emitted byte must land exactly at
	// AOT_RT_BASE, otherwise the trim would place the blob at the wrong address.
	for i := 0; i < aotRTOrgPadBytes; i++ {
		if raw[i] != 0 {
			t.Fatalf("non-zero byte 0x%02X at offset %#x inside the org pad: org padding not clean",
				raw[i], i)
		}
	}
	if raw[aotRTOrgPadBytes] == 0 {
		t.Fatalf("first byte at AOT_RT_BASE offset %#x is zero: jump table missing", aotRTOrgPadBytes)
	}
	return raw[aotRTOrgPadBytes:]
}

// buildRuntimeBlobBin assembles the runtime blob unit and returns the trimmed
// bytes (byte 0 == guest AOT_RT_BASE) that the COMPILE path bundles into standalone
// images. Phase 1 build/File I/O wiring uses the same assemble + trim to materialise
// aot_runtime_blob.bin, so nothing about the blob is committed as a binary.
func buildRuntimeBlobBin(t *testing.T) []byte {
	t.Helper()
	return trimRuntimeBlob(t, assembleRuntimeBlob(t))
}

// TestAOTRuntimeBlob_StandaloneCallsExprEval is the Phase 1 architecture proof: a
// hand-written standalone image copies the generated blob to AOT_RT_BASE, then calls
// into it through the jump table (RT_VAR_INIT, RT_EXPR_EVAL) on a tokenised
// expression, with no resident interpreter present. It proves the blob is
// position-correct at its link address, the jump-table ABI resolves, and the bundled
// expr_eval runs standalone. The expression "5"+TK_PLUS+"3" evaluates to FP32 8.0
// (0x41000000), stored to a scratch address the test reads back.
func TestAOTRuntimeBlob_StandaloneCallsExprEval(t *testing.T) {
	asmBin := buildAssembler(t)
	blob := buildRuntimeBlobBin(t)

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "blob.bin"), blob, 0644); err != nil {
		t.Fatal(err)
	}

	// Standalone image: bootstrap (stack/state/terminal regs) -> copy blob payload to
	// AOT_RT_BASE -> var_init -> expr_eval on tokenised "5+3" -> store FP32 result at
	// 0x000800 -> halt. The blob payload is incbin'd after the code; its load address
	// (blob_payload, ~0x1060) is well below AOT_RT_BASE (0x043000), so the forward
	// copy does not overlap its destination.
	asm := `include "ie64.inc"
include "ehbasic_tokens.inc"
    org 0x1000
start:
    move.l  r31, #STACK_TOP
    move.l  r16, #BASIC_STATE
    move.l  r26, #TERM_OUT
    move.l  r27, #TERM_STATUS
    move.l  r1, #blob_payload
    move.l  r2, #AOT_RT_BASE
    move.l  r3, #` + fmt.Sprintf("%d", len(blob)) + `
.copy:
    beqz    r3, .copydone
    load.b  r4, (r1)
    store.b r4, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    sub.q   r3, r3, #1
    bra     .copy
.copydone:
    move.l  r6, #RT_VAR_INIT
    load.q  r6, (r6)
    jsr     (r6)
    move.l  r17, #expr_tokens
    move.l  r6, #RT_EXPR_EVAL
    load.q  r6, (r6)
    jsr     (r6)
    move.l  r1, #0x800
    store.q r8, (r1)
    store.q r9, 8(r1)
    halt
    align 8
expr_tokens:
    dc.b    "5", TK_PLUS, "3", 0
    align 8
blob_payload:
    incbin  "blob.bin"
blob_end:
`
	srcPath := filepath.Join(tmpDir, "proof.asm")
	if err := os.WriteFile(srcPath, []byte(asm), 0644); err != nil {
		t.Fatal(err)
	}
	incDir := filepath.Join(repoRootDir(t), "sdk", "include")
	outPath := filepath.Join(tmpDir, "proof.ie64")
	cmd := exec.Command(asmBin, "-I", incDir, "-o", outPath, "proof.asm")
	cmd.Dir = tmpDir // so incbin "blob.bin" resolves
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assemble proof image: %v\n%s", err, out)
	}
	img, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read proof image: %v", err)
	}

	run := newEhbasicHarness(t)
	run.loadBytes(img)
	run.runCycles(8_000_000)

	const scratch = 0x800
	gotPayload := uint64(run.cpu.memory[scratch]) |
		uint64(run.cpu.memory[scratch+1])<<8 |
		uint64(run.cpu.memory[scratch+2])<<16 |
		uint64(run.cpu.memory[scratch+3])<<24 |
		uint64(run.cpu.memory[scratch+4])<<32 |
		uint64(run.cpu.memory[scratch+5])<<40 |
		uint64(run.cpu.memory[scratch+6])<<48 |
		uint64(run.cpu.memory[scratch+7])<<56
	gotTag := uint64(run.cpu.memory[scratch+8]) |
		uint64(run.cpu.memory[scratch+9])<<8 |
		uint64(run.cpu.memory[scratch+10])<<16 |
		uint64(run.cpu.memory[scratch+11])<<24 |
		uint64(run.cpu.memory[scratch+12])<<32 |
		uint64(run.cpu.memory[scratch+13])<<40 |
		uint64(run.cpu.memory[scratch+14])<<48 |
		uint64(run.cpu.memory[scratch+15])<<56
	if gotPayload != 8 || gotTag != 2 {
		t.Fatalf("standalone expr_eval(5+3) = payload %#x tag %#x, want payload 8 tag VAL_I64", gotPayload, gotTag)
	}
}

// TestAOTRuntimeBlob_EmbeddedUnconditionally proves the host embeds the runtime blob
// regardless of the embed_basic build tag. COMPILE needs the blob to bundle standalone
// images, and -basic runs without embed_basic (a custom -basic-image or the prebuilt
// fallback), so gating the embed on embed_basic would leave non-embedded builds unable
// to serve the blob (COMPILE would fail with a file error). This test binary is built
// without embed_basic, so a non-empty embeddedRuntimeBlob here confirms the embed is
// unconditional; it must also equal the committed/fresh blob bytes.
func TestAOTRuntimeBlob_EmbeddedUnconditionally(t *testing.T) {
	if len(embeddedRuntimeBlob) == 0 {
		t.Fatal("embeddedRuntimeBlob is empty: the runtime blob must be embedded unconditionally " +
			"(not gated on embed_basic), else non-embedded -basic builds cannot serve it to COMPILE")
	}
	fresh := buildRuntimeBlobBin(t)
	if len(embeddedRuntimeBlob) != len(fresh) {
		t.Fatalf("embedded blob %d bytes, fresh %d: regenerate aot_runtime_blob.bin", len(embeddedRuntimeBlob), len(fresh))
	}
	for i := range fresh {
		if embeddedRuntimeBlob[i] != fresh[i] {
			t.Fatalf("embedded blob differs at offset %#x (%#02x != %#02x): regenerate aot_runtime_blob.bin",
				i, embeddedRuntimeBlob[i], fresh[i])
		}
	}
}

// TestAOTRuntimeBlob_MatchesCommitted guards the committed/embedded blob against
// drifting from the runtime sources. sdk/include/aot_runtime_blob.bin is generated by
// `make basic` (tools/gen_runtime_blob) and go:embedded into the host unconditionally /
// served by the File I/O device, so it must equal a fresh assemble + trim. If this
// fails, run `make aot-runtime-blob` (or `go run ./tools/gen_runtime_blob`).
func TestAOTRuntimeBlob_MatchesCommitted(t *testing.T) {
	binPath := filepath.Join(repoRootDir(t), "sdk", "include", "aot_runtime_blob.bin")
	committed, err := os.ReadFile(binPath)
	if err != nil {
		t.Skipf("committed runtime blob not present (run make aot-runtime-blob): %v", err)
	}
	fresh := buildRuntimeBlobBin(t)
	if len(committed) != len(fresh) {
		t.Fatalf("committed blob %d bytes, fresh %d: regenerate aot_runtime_blob.bin", len(committed), len(fresh))
	}
	for i := range committed {
		if committed[i] != fresh[i] {
			t.Fatalf("committed blob differs at offset %#x (%#02x != %#02x): regenerate aot_runtime_blob.bin",
				i, committed[i], fresh[i])
		}
	}
}

// TestAOTRuntimeBlob_DirectCallClosure is the automated direct-closure audit the plan
// requires: every direct jsr/bsr/jmp/branch target reachable from the bundled runtime
// must resolve to a label defined inside the bundle. A direct branch to a routine left
// in the interpreter (not bundled) would be a jump into absent memory at run time. The
// in-guest assembler already errors on an undefined symbol, but this test is an
// independent, source-level guard that no bundled routine calls outside the closure
// (and pinpoints the offending site if one is introduced). Register-indirect calls
// (jsr (rN)) are excluded here; their policy (USR supported, CO* rejected) is covered
// by the transpiler tests.
func TestAOTRuntimeBlob_DirectCallClosure(t *testing.T) {
	incDir := filepath.Join(repoRootDir(t), "sdk", "include")
	// The code-bearing units bundled by aot_runtime_blob.asm (ie64.inc / tokens are
	// constant-only). Keep in lockstep with the blob's include list.
	bundled := []string{
		"ie64_fp.inc", "ehbasic_expr.inc", "ehbasic_vars.inc", "ehbasic_strings.inc",
		"ehbasic_io.inc", "ehbasic_exec.inc", "ehbasic_lineeditor.inc",
		"ehbasic_file_io.inc", "aot_runtime_stubs.inc", "aot_runtime_blob.asm",
	}
	labelRe := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):`)
	branchRe := regexp.MustCompile(`^(jsr|bsr|jmp|bra|beq|bne|beqz|bnez|blt|bgt|ble|bge|bgez|blez|bltz|bgtz)\b(.*)$`)
	identRe := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

	globals := map[string]bool{}
	type site struct{ file, target, line string }
	var targets []site

	for _, name := range bundled {
		data, err := os.ReadFile(filepath.Join(incDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, raw := range strings.Split(string(data), "\n") {
			if i := strings.IndexByte(raw, ';'); i >= 0 { // strip the comment
				raw = raw[:i]
			}
			if m := labelRe.FindStringSubmatch(strings.TrimRight(raw, " \t")); m != nil {
				globals[m[1]] = true
			}
			body := strings.TrimSpace(raw)
			m := branchRe.FindStringSubmatch(body)
			if m == nil {
				continue
			}
			// The branch target is the last comma/space-separated operand.
			ops := strings.FieldsFunc(m[2], func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
			if len(ops) == 0 {
				continue
			}
			tgt := ops[len(ops)-1]
			if strings.HasPrefix(tgt, "(") || strings.HasPrefix(tgt, ".") || !identRe.MatchString(tgt) {
				continue // register-indirect, file-local label, or numeric/immediate
			}
			targets = append(targets, site{name, tgt, body})
		}
	}

	if len(targets) == 0 {
		t.Fatal("no direct branch targets found: audit parsed nothing (check the mnemonic set)")
	}
	for _, s := range targets {
		if !globals[s.target] {
			t.Errorf("un-bundled direct branch target %q in %s (%q): the call leaves the runtime "+
				"closure. Bundle the routine or add a stub in aot_runtime_stubs.inc.", s.target, s.file, s.line)
		}
	}
	t.Logf("direct-call closure audit: %d global labels, %d direct branch targets, all resolved",
		len(globals), len(targets))
}

func TestAOTRuntimeBlob_AssemblesAndTrims(t *testing.T) {
	blob := buildRuntimeBlobBin(t)
	top := aotRTBase + len(blob)
	if top > aotRTLimit {
		t.Fatalf("trimmed blob is %d bytes; top %#x exceeds AOT_RT_LIMIT %#x",
			len(blob), top, aotRTLimit)
	}
	// The blob must fit the compile-time low-32 staging buffer (aot_read_rt_blob
	// allocates AOT_RT_BLOB_MAX). A File I/O read stages the whole file before
	// FILE_RESULT_LEN is checked, so a blob over this cap would overflow neighbouring
	// AOT workspace. Keep this in lockstep with the staging allocation and
	// tools/gen_runtime_blob.
	if len(blob) > aotRTBlobMax {
		t.Fatalf("trimmed blob is %d bytes, exceeds AOT_RT_BLOB_MAX %#x (compile-time staging size); "+
			"raise AOT_RT_BLOB_MAX + the aot_read_rt_blob staging allocation together", len(blob), aotRTBlobMax)
	}
	// The blob is determinate; a second assemble must reproduce it byte for byte
	// (guards the generate-on-demand contract the COMPILE bundling relies on).
	again := buildRuntimeBlobBin(t)
	if len(again) != len(blob) {
		t.Fatalf("non-determinate blob size: %d then %d", len(blob), len(again))
	}
	for i := range blob {
		if blob[i] != again[i] {
			t.Fatalf("non-determinate blob byte at %#x: %#02x != %#02x", i, blob[i], again[i])
		}
	}
	t.Logf("trimmed runtime blob: %d bytes, occupies [%#x, %#x), limit %#x",
		len(blob), aotRTBase, top, aotRTLimit)
}
