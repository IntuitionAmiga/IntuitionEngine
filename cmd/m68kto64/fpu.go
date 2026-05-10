package main

import (
	"fmt"
	"strings"
)

// emitFPU is the Phase-7 dispatch shim. It returns (handled=true, err) when the
// mnemonic is recognised as an FPU op, otherwise (handled=false, nil) so the
// integer dispatch can take over.
func (c *Converter) emitFPU(e *Emit, l Line) (bool, error) {
	m := l.Mnemonic
	if m == "" || m[0] != 'f' {
		return false, nil
	}
	switch m {
	case "fmove":
		return true, c.emitFMove(e, l)
	case "fmovem":
		return true, c.emitFMovem(e, l)
	case "fmovecr":
		return true, c.emitFMoveCR(e, l)
	case "fadd", "fsub", "fmul", "fdiv", "fmod", "frem",
		"fsglmul", "fsgldiv":
		return true, c.emitFArith(e, l, m)
	case "fneg", "fabs", "fsqrt", "fint", "fintrz",
		"fgetexp", "fgetman":
		return true, c.emitFUnary(e, l, m)
	case "fscale":
		return true, c.emitFScale(e, l)
	case "fsin", "fcos", "ftan", "fatan", "facos", "fasin",
		"fcosh", "fsinh", "ftanh", "fatanh",
		"fetox", "fetoxm1",
		"flogn", "flog10", "flog2", "flognp1",
		"ftentox", "ftwotox":
		return true, c.emitFTranscendental(e, l, m)
	case "fcmp":
		return true, c.emitFCmp(e, l)
	case "ftst":
		return true, c.emitFTst(e, l)
	case "fnop":
		e.L("; nop (FPU)")
		return true, nil
	case "fsave", "frestore":
		if c.strict {
			return true, fmt.Errorf("%s not modelled (FPU state context-switch)", m)
		}
		e.Lf("; m68kto64: stripped %s (FPU state save not modeled)", m)
		return true, nil
	}
	if strings.HasPrefix(m, "fb") {
		return true, c.emitFBcc(e, l, m)
	}
	if strings.HasPrefix(m, "fdb") {
		return true, c.emitFDBcc(e, l, m)
	}
	if strings.HasPrefix(m, "fs") {
		return true, c.emitFScc(e, l, m)
	}
	if strings.HasPrefix(m, "ftrap") {
		return true, c.emitFTrapcc(e, l, m)
	}
	return false, nil
}

// =====================================================================
// FP register / addressing-mode resolution helpers
// =====================================================================

// fpRegFromToken resolves an "fpN" token (case-insensitive) to its IE64
// even-numbered register name. Returns ("", false) if not an FPn.
func fpRegFromToken(tok string) (string, bool) {
	r, ok := LookupFPRegister(strings.TrimSpace(tok))
	if !ok || r.Class != FPRegData {
		return "", false
	}
	return r.IE64, true
}

// fpControlFromToken returns the FPRegClass for FPCR/FPSR/FPIAR tokens; for FPn
// or non-FP tokens it returns FPRegUnknown.
func fpControlFromToken(tok string) FPRegClass {
	r, ok := LookupFPRegister(strings.TrimSpace(tok))
	if !ok {
		return FPRegUnknown
	}
	switch r.Class {
	case FPRegFPCR, FPRegFPSR, FPRegFPIAR:
		return r.Class
	}
	return FPRegUnknown
}

// fpStepBytes returns the per-element step for FMOVEM/predec/postinc on an
// FPMem operand. Single-precision (.S) is 4 bytes; double / extended (.D /
// degraded .X) is 8 bytes.
func fpStepBytes(size string) int {
	switch strings.ToLower(size) {
	case ".s":
		return 4
	case ".d", ".x":
		return 8
	}
	return 8
}

// fpIsDouble reports whether the size suffix selects an 8-byte transfer
// (`dload`/`dstore` opcode 0x81/0x82). `.D` is double; `.X` degrades to
// double; everything else (including the int suffixes) is treated as
// single-precision for routing.
func fpIsDouble(size string) bool {
	s := strings.ToLower(size)
	return s == ".d" || s == ".x"
}

// emitFPMemLoad loads a single- or double-precision value from `op` into FP
// register `fpDst` (an IE64 even f-reg name).
func (c *Converter) emitFPMemLoad(e *Emit, op Operand, fpDst string, size string) error {
	step := fpStepBytes(size)
	op2 := op
	switch op2.Mode {
	case AMIndirect:
		e.Lf("%s %s, (%s)", fpLoadOp(size), fpDst, op2.Reg.IE64)
		return nil
	case AMDispAn:
		e.Lf("%s %s, %s(%s)", fpLoadOp(size), fpDst, dispOrZero(op2.Disp), op2.Reg.IE64)
		return nil
	case AMPostInc:
		e.Lf("%s %s, (%s)", fpLoadOp(size), fpDst, op2.Reg.IE64)
		e.Lf("add.l %s, %s, #%d", op2.Reg.IE64, op2.Reg.IE64, step)
		return nil
	case AMPreDec:
		e.Lf("sub.l %s, %s, #%d", op2.Reg.IE64, op2.Reg.IE64, step)
		e.Lf("%s %s, (%s)", fpLoadOp(size), fpDst, op2.Reg.IE64)
		return nil
	case AMIndexAn:
		c.emitIndexAddr(e, op2, ScrEA)
		e.Lf("%s %s, (%s)", fpLoadOp(size), fpDst, ScrEA)
		return nil
	case AMAbsW, AMAbsL, AMDispPC, AMIndexPC:
		e.Lf("la %s, %s", ScrEA, op2.Disp)
		if op2.Mode == AMIndexPC {
			c.emitIndexCombine(e, op2.Index, ScrEA)
		}
		e.Lf("%s %s, (%s)", fpLoadOp(size), fpDst, ScrEA)
		return nil
	}
	return fmt.Errorf("emitFPMemLoad: unsupported EA mode %v", op2.Mode)
}

// emitFPMemStore stores FP register `fpSrc` to `op`.
func (c *Converter) emitFPMemStore(e *Emit, op Operand, fpSrc string, size string) error {
	step := fpStepBytes(size)
	switch op.Mode {
	case AMIndirect:
		e.Lf("%s %s, (%s)", fpStoreOp(size), fpSrc, op.Reg.IE64)
		return nil
	case AMDispAn:
		e.Lf("%s %s, %s(%s)", fpStoreOp(size), fpSrc, dispOrZero(op.Disp), op.Reg.IE64)
		return nil
	case AMPostInc:
		e.Lf("%s %s, (%s)", fpStoreOp(size), fpSrc, op.Reg.IE64)
		e.Lf("add.l %s, %s, #%d", op.Reg.IE64, op.Reg.IE64, step)
		return nil
	case AMPreDec:
		e.Lf("sub.l %s, %s, #%d", op.Reg.IE64, op.Reg.IE64, step)
		e.Lf("%s %s, (%s)", fpStoreOp(size), fpSrc, op.Reg.IE64)
		return nil
	case AMIndexAn:
		c.emitIndexAddr(e, op, ScrEA)
		e.Lf("%s %s, (%s)", fpStoreOp(size), fpSrc, ScrEA)
		return nil
	case AMAbsW, AMAbsL:
		e.Lf("la %s, %s", ScrEA, op.Disp)
		e.Lf("%s %s, (%s)", fpStoreOp(size), fpSrc, ScrEA)
		return nil
	}
	return fmt.Errorf("emitFPMemStore: unsupported EA mode %v", op.Mode)
}

func fpLoadOp(size string) string {
	if fpIsDouble(size) {
		return "dload"
	}
	return "fload"
}

func fpStoreOp(size string) string {
	if fpIsDouble(size) {
		return "dstore"
	}
	return "fstore"
}

// =====================================================================
// FMOVE
// =====================================================================

func (c *Converter) emitFMove(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("fmove requires 2 operands")
	}
	src := strings.TrimSpace(l.Operands[0])
	dst := strings.TrimSpace(l.Operands[1])
	size := l.Size
	if size == "" {
		size = ".x"
	}
	if strings.ToLower(size) == ".p" {
		if c.strict {
			return fmt.Errorf("fmove.p (packed BCD) unsupported")
		}
		e.L("; ERROR: .P unsupported")
		return nil
	}
	if strings.ToLower(size) == ".x" {
		e.L("; FPU: .X degraded to .D")
	}

	// Control-register forms: FMOVE FPSR/FPCR/FPIAR ↔ Dn.
	if cls := fpControlFromToken(src); cls != FPRegUnknown {
		return c.emitFMoveControlToDn(e, cls, dst)
	}
	if cls := fpControlFromToken(dst); cls != FPRegUnknown {
		return c.emitFMoveDnToControl(e, src, cls)
	}

	srcFP, srcIsFP := fpRegFromToken(src)
	dstFP, dstIsFP := fpRegFromToken(dst)

	// FMOVE FPm,FPn → dmov f(2n), f(2m). Always treat as double-precision
	// copy; .X / .D / no-suffix all preserve the 64-bit pair.
	if srcIsFP && dstIsFP {
		e.Lf("dmov %s, %s", dstFP, srcFP)
		return nil
	}

	// FMOVE.<size> <ea>,FPn — load into an FP reg.
	if !srcIsFP && dstIsFP {
		return c.emitFMoveLoadToFP(e, src, dstFP, size)
	}

	// FMOVE.<size> FPn,<ea> — store FP reg to memory or convert to int.
	if srcIsFP && !dstIsFP {
		return c.emitFMoveStoreFromFP(e, srcFP, dst, size)
	}
	return fmt.Errorf("fmove: neither operand is an FPn or FP control reg")
}

func (c *Converter) emitFMoveLoadToFP(e *Emit, src string, fpDst string, size string) error {
	op, err := ParseOperand(src)
	if err != nil {
		return fmt.Errorf("fmove src: %v", err)
	}
	sz := strings.ToLower(size)
	switch sz {
	case ".s", ".d", ".x":
		// Floating-point bit pattern in memory or constant pool.
		if op.Mode == AMImmediate {
			e.Lf("la %s, %s", ScrEA, op.Imm)
			e.Lf("%s %s, (%s)", fpLoadOp(sz), fpDst, ScrEA)
			return nil
		}
		return c.emitFPMemLoad(e, op, fpDst, sz)
	case ".b", ".w", ".l":
		// Integer source → FP via fcvtif. Width-correct sign-extend first.
		szBytes := SizeBytes(sz)
		reg, imm, err := c.loadValue(e, op, szBytes, ScrV1)
		if err != nil {
			return err
		}
		var src string
		if reg != "" {
			src = reg
			if szBytes == 1 {
				e.Lf("sext.b %s, %s", ScrV1, src)
				src = ScrV1
			} else if szBytes == 2 {
				e.Lf("sext.w %s, %s", ScrV1, src)
				src = ScrV1
			}
		} else {
			e.Lf("move.l %s, #%s", ScrV1, imm)
			src = ScrV1
		}
		e.Lf("dcvtif %s, %s", fpDst, src)
		return nil
	}
	return fmt.Errorf("fmove: unrecognised size %q", size)
}

func (c *Converter) emitFMoveStoreFromFP(e *Emit, fpSrc string, dst string, size string) error {
	op, err := ParseOperand(dst)
	if err != nil {
		return fmt.Errorf("fmove dst: %v", err)
	}
	sz := strings.ToLower(size)
	switch sz {
	case ".s", ".d", ".x":
		return c.emitFPMemStore(e, op, fpSrc, sz)
	case ".b", ".w", ".l":
		// FP → int. Convert to GPR ScrV1, then mask/store at byte width.
		e.Lf("dcvtfi %s, %s", ScrV1, fpSrc)
		szBytes := SizeBytes(sz)
		return c.storeValue(e, op, szBytes, ScrV1)
	}
	return fmt.Errorf("fmove: unrecognised size %q", size)
}

// FMOVE.L FPSR/FPCR/FPIAR,Dn (read into Dn).
func (c *Converter) emitFMoveControlToDn(e *Emit, cls FPRegClass, dst string) error {
	r, ok := LookupRegister(dst)
	if !ok || r.Class != RegData {
		return fmt.Errorf("fmove control reg dst must be Dn, got %q", dst)
	}
	switch cls {
	case FPRegFPCR:
		e.Lf("fmovcr %s", r.IE64)
		return nil
	case FPRegFPSR:
		// Compose: hardware sticky bits 3:0 + ShadowFPCC at bits 27:24.
		e.Lf("fmovsr %s", ScrV1)
		e.Lf("lsl.l %s, %s, #24", ScrV2, ShadowFPCC)
		e.Lf("or.l %s, %s, %s", r.IE64, ScrV1, ScrV2)
		return nil
	case FPRegFPIAR:
		e.Lf("move.l %s, #0 ; FPIAR read returns 0", r.IE64)
		return nil
	}
	return fmt.Errorf("fmove: unrecognised control reg class %v", cls)
}

// FMOVE.L Dn,FPSR/FPCR/FPIAR (write from Dn).
func (c *Converter) emitFMoveDnToControl(e *Emit, src string, cls FPRegClass) error {
	r, ok := LookupRegister(src)
	if !ok || (r.Class != RegData && r.Class != RegAddr && r.Class != RegSP) {
		return fmt.Errorf("fmove ctrl src must be Dn/An, got %q", src)
	}
	switch cls {
	case FPRegFPCR:
		e.Lf("fmovcc %s", r.IE64)
		return nil
	case FPRegFPSR:
		// Split: cc bits 27:24 → ShadowFPCC; sticky bits via fmovsc which
		// hardware-masks the input to bits 3:0 per ISA §4.6.5.
		e.Lf("lsr.l %s, %s, #24", ScrV1, r.IE64)
		e.Lf("and.l %s, %s, #$F", ShadowFPCC, ScrV1)
		e.Lf("fmovsc %s", r.IE64)
		return nil
	case FPRegFPIAR:
		e.L("; FPIAR write ignored")
		return nil
	}
	return fmt.Errorf("fmove: unrecognised control reg class %v", cls)
}

// =====================================================================
// FMOVECR
// =====================================================================

// fmovecrROMTable maps the m68k 7-bit ROM offset to the IE64 4-bit fmovecr
// index when an equivalent ROM entry exists, OR to a constant-pool symbol
// emitted as a `dc.d` literal. m68k ROM offsets are documented in the 68881
// programmer's reference (§3.4); Pi, e, log2(e), log10(e), ln(2), ln(10) and
// 0/1/10**n are the common ones. IE64 ROM coverage is narrower.
var fmovecrROMMap = map[int]struct {
	IE64Idx int    // -1 if no IE64 ROM entry; use ConstPool fallback
	ConstD  string // dc.d literal text, used when IE64Idx == -1
	Comment string
}{
	0x00: {0, "", "Pi"},
	0x0B: {1, "", "log10(2)"},
	0x0C: {2, "", "e"},
	0x0D: {3, "", "log2(e)"},
	0x0E: {4, "", "log10(e)"},
	0x0F: {-1, "0.0", "+0.0"},
	0x30: {5, "", "ln(2)"},
	0x31: {6, "", "ln(10)"},
	0x32: {-1, "1.0", "1.0"},
	0x33: {-1, "10.0", "1.0e1"},
	0x34: {-1, "100.0", "1.0e2"},
	0x35: {-1, "10000.0", "1.0e4"},
	0x36: {-1, "100000000.0", "1.0e8"},
	0x37: {-1, "10000000000000000.0", "1.0e16"},
	0x38: {-1, "1.0e32", "1.0e32"},
	0x39: {-1, "1.0e64", "1.0e64"},
	0x3A: {-1, "1.0e128", "1.0e128"},
	0x3B: {-1, "1.0e256", "1.0e256"},
}

// fpConstPoolEntries collects unique `dc.d` literals required by FMOVECR /
// FMOVE.D #imm fallback. Emitted at end-of-file by Converter.finishFooter.
type fpConstEntry struct {
	Label   string
	DCD     string
	Comment string
}

func (c *Converter) addFPConst(d string, comment string) string {
	if c.fpConsts == nil {
		c.fpConsts = map[string]fpConstEntry{}
	}
	if e, ok := c.fpConsts[d]; ok {
		return e.Label
	}
	label := fmt.Sprintf("%s_%d", FPSlotConstPool, len(c.fpConsts))
	c.fpConsts[d] = fpConstEntry{Label: label, DCD: d, Comment: comment}
	return label
}

func (c *Converter) emitFMoveCR(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("fmovecr requires 2 operands")
	}
	dst := strings.TrimSpace(l.Operands[0])
	imm := strings.TrimSpace(l.Operands[1])
	fpDst, ok := fpRegFromToken(dst)
	if !ok {
		return fmt.Errorf("fmovecr dst must be FPn, got %q", dst)
	}
	if !strings.HasPrefix(imm, "#") {
		return fmt.Errorf("fmovecr: source must be #offset, got %q", imm)
	}
	off, err := parseInt(strings.TrimPrefix(imm, "#"))
	if err != nil {
		return fmt.Errorf("fmovecr: bad offset %q: %v", imm, err)
	}
	entry, known := fmovecrROMMap[off]
	if !known {
		// Unknown ROM offset — emit a 0.0 literal placeholder + diagnostic.
		label := c.addFPConst("0.0", fmt.Sprintf("fmovecr unknown offset %#x", off))
		e.Lf("la %s, %s ; fmovecr offset %#x unknown — substituted 0.0", ScrEA, label, off)
		e.Lf("dload %s, (%s)", fpDst, ScrEA)
		c.usesFPConstPool = true
		return nil
	}
	if entry.IE64Idx >= 0 {
		e.Lf("fmovecr %s, #%d ; %s", fpDst, entry.IE64Idx, entry.Comment)
		c.markFPInUse()
		return nil
	}
	label := c.addFPConst(entry.ConstD, entry.Comment)
	e.Lf("la %s, %s", ScrEA, label)
	e.Lf("dload %s, (%s)", fpDst, ScrEA)
	c.usesFPConstPool = true
	c.markFPInUse()
	return nil
}

func (c *Converter) markFPInUse() { c.fpUsed = true }

// =====================================================================
// FMOVEM
// =====================================================================

func (c *Converter) emitFMovem(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("fmovem requires 2 operands")
	}
	first := strings.TrimSpace(l.Operands[0])
	second := strings.TrimSpace(l.Operands[1])
	size := l.Size
	if size == "" {
		size = ".x"
	}
	if strings.ToLower(size) == ".x" {
		e.L("; FPU: .X degraded to .D for fmovem")
	}
	step := fpStepBytes(size)

	// Direction: <list>,<ea>  (store)  vs  <ea>,<list>  (load)
	firstList, firstOk := fpRegList(first)
	secondList, secondOk := fpRegList(second)

	switch {
	case firstOk && !secondOk:
		// store list to ea
		ea, err := ParseOperand(second)
		if err != nil {
			return err
		}
		return c.emitFMovemStore(e, firstList, ea, size, step)
	case !firstOk && secondOk:
		// load ea into list
		ea, err := ParseOperand(first)
		if err != nil {
			return err
		}
		return c.emitFMovemLoad(e, secondList, ea, size, step)
	}
	return fmt.Errorf("fmovem: cannot identify list+ea operands")
}

// fpRegList parses an FMOVEM register list (e.g. "fp0-fp3/fp5") into its
// expanded ordered IE64 reg list. Returns ok=false if the token doesn't look
// like an FP list.
func fpRegList(s string) ([]string, bool) {
	t := strings.ToLower(strings.TrimSpace(s))
	if !strings.Contains(t, "fp") {
		return nil, false
	}
	parts := strings.Split(t, "/")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if i := strings.Index(p, "-"); i > 0 {
			lo, ok1 := fpRegFromToken(p[:i])
			hi, ok2 := fpRegFromToken(p[i+1:])
			if !ok1 || !ok2 {
				return nil, false
			}
			loIdx, hiIdx := evenRegIndex(lo), evenRegIndex(hi)
			if loIdx < 0 || hiIdx < 0 || loIdx > hiIdx {
				return nil, false
			}
			for n := loIdx; n <= hiIdx; n++ {
				out = append(out, FPGuestRegToHost(n))
			}
			continue
		}
		r, ok := fpRegFromToken(p)
		if !ok {
			return nil, false
		}
		out = append(out, r)
	}
	// `out` is guaranteed non-empty here: the outer guard above requires
	// the input to contain "fp"; any token that survives the per-element
	// parse appends at least one entry, and parse failures return early.
	return out, true
}

func evenRegIndex(ie64 string) int {
	// Reverse: f0→0, f2→1, ..., f14→7.
	if !strings.HasPrefix(ie64, "f") {
		return -1
	}
	n, err := parseInt(ie64[1:])
	if err != nil || n < 0 || n > 14 || n%2 != 0 {
		return -1
	}
	return n / 2
}

func (c *Converter) emitFMovemStore(e *Emit, regs []string, ea Operand, size string, step int) error {
	op := fpStoreOp(size)
	switch ea.Mode {
	case AMPreDec:
		// m68k FMOVEM -(An) stores in reverse mask order (FP7 first → FP0 last
		// at lowest addr). Emit reverse.
		for i := len(regs) - 1; i >= 0; i-- {
			e.Lf("sub.l %s, %s, #%d", ea.Reg.IE64, ea.Reg.IE64, step)
			e.Lf("%s %s, (%s)", op, regs[i], ea.Reg.IE64)
		}
		return nil
	case AMPostInc:
		for _, r := range regs {
			e.Lf("%s %s, (%s)", op, r, ea.Reg.IE64)
			e.Lf("add.l %s, %s, #%d", ea.Reg.IE64, ea.Reg.IE64, step)
		}
		return nil
	case AMIndirect, AMDispAn, AMIndexAn, AMAbsW, AMAbsL:
		// emitEABase only errors on modes outside this case's whitelist,
		// so no err check needed here.
		_ = c.emitEABase(e, ea, ScrEA)
		for i, r := range regs {
			if i == 0 {
				e.Lf("%s %s, (%s)", op, r, ScrEA)
			} else {
				e.Lf("%s %s, %d(%s)", op, r, i*step, ScrEA)
			}
		}
		return nil
	}
	return fmt.Errorf("fmovem store: unsupported EA %v", ea.Mode)
}

func (c *Converter) emitFMovemLoad(e *Emit, regs []string, ea Operand, size string, step int) error {
	op := fpLoadOp(size)
	switch ea.Mode {
	case AMPostInc:
		for _, r := range regs {
			e.Lf("%s %s, (%s)", op, r, ea.Reg.IE64)
			e.Lf("add.l %s, %s, #%d", ea.Reg.IE64, ea.Reg.IE64, step)
		}
		return nil
	case AMPreDec:
		for i := len(regs) - 1; i >= 0; i-- {
			e.Lf("sub.l %s, %s, #%d", ea.Reg.IE64, ea.Reg.IE64, step)
			e.Lf("%s %s, (%s)", op, regs[i], ea.Reg.IE64)
		}
		return nil
	case AMIndirect, AMDispAn, AMIndexAn, AMAbsW, AMAbsL, AMDispPC, AMIndexPC:
		_ = c.emitEABase(e, ea, ScrEA)
		for i, r := range regs {
			if i == 0 {
				e.Lf("%s %s, (%s)", op, r, ScrEA)
			} else {
				e.Lf("%s %s, %d(%s)", op, r, i*step, ScrEA)
			}
		}
		return nil
	}
	return fmt.Errorf("fmovem load: unsupported EA %v", ea.Mode)
}

// =====================================================================
// Footer — emits accumulated FP memory slots / constant pool.
// =====================================================================

// emitFPFooter dumps the per-output-file FP memory-slot reservations.
// Called by ConvertSource after emitting all routine code. Single-call,
// idempotent: skipped if no FPU op was lowered.
func (c *Converter) emitFPFooter(e *Emit) {
	if !c.fpUsed && c.fpConsts == nil {
		return
	}
	e.L("")
	e.L("; ---- m68kto64 FPU footer ----")
	if c.fpUsed {
		e.Label(FPSlotFPCRSave)
		e.L("dc.q 0    ; FINTRZ FPCR save/restore slot")
		e.Label(FPSlotScratchQ)
		e.L("dc.q 0    ; FSCALE bit-pattern round-trip slot")
	}
	for _, ent := range c.fpConsts {
		e.Label(ent.Label)
		e.Lf("dc.d %s   ; %s", ent.DCD, ent.Comment)
	}
}
