package main

// =====================================================================
// Phase 2 — RTE-walkback handler detection + FP slot save/restore
// =====================================================================
//
// When `-fp-irq-wrap` (Converter.fpIrqWrap) is set, the converter performs
// a pre-emit line-stream scan to partition the input into (label, …, rte)
// regions. Each label that reaches an RTE via straight-line fall-through is
// marked as a handler entry. At emit time:
//
//   * the label emit gets a save stub appended (push FP slots to a7),
//   * the matching RTE emit gets a restore stub prepended (pop FP slots
//     from a7) immediately before the existing RTE lowering.
//
// Frame layout (16B base, +16B when needsFP56Save):
//
//   Offset  0..7 : __m68kto64_fpcr_save     (always)
//   Offset  8..15: __m68kto64_fp_scratch_q  (always)
//   Offset 16..23: __m68kto64_fp5_save      (only if needsFP56Save)
//   Offset 24..31: __m68kto64_fp6_save      (only if needsFP56Save)
//
// __m68kto64_fp_const_pool is read-only (one labelled `dc.d` per FP
// immediate) and NOT in the save set — a handler cannot mutate it.
//
// Walkback rules (line scanner):
//
//   * Block start = nearest preceding label (LineLabelOnly or l.Label != "").
//   * Block contents accept: any executable instruction, JSR/BSR (returns
//     preserve flow), conditional Bcc.
//   * Block terminators: RTE (mark and wrap), RTS/JMP/BRA (block ends but
//     not a handler — skip wrap).
//   * Fall-through into next label: outermost label gets the save stub.
//
// Default: fpIrqWrap off — single-thread guests skip the cost.

// fileTouchesFP56Scratch reports whether any op in the line stream will
// trigger Phase 1's FP5/FP6 spill wrapper (and thus require the FP5/FP6
// slots in any handler frame). Mnemonic-based — see fpu_arith.go /
// fpu_shadow.go / fpu_transcendental.go for the wrapped sites.
func fileTouchesFP56Scratch(lines []Line) bool {
	for _, l := range lines {
		if l.Kind != LineInstruction {
			continue
		}
		switch l.Mnemonic {
		case "fscale", "ftst", "fcmp",
			"fneg", "fabs", "fsqrt", "fint", "fintrz",
			"fgetexp", "fgetman",
			"fadd", "fsub", "fmul", "fdiv", "fmod", "frem", "fsglmul", "fsgldiv",
			"fsin", "fcos", "ftan", "fatan", "fetox", "flogn",
			"fasin", "facos", "fcosh", "fsinh", "ftanh", "fatanh",
			"fetoxm1", "flog10", "flog2", "flognp1", "ftentox", "ftwotox":
			return true
		}
	}
	return false
}

// scanRTEHandlerBlocks performs Pass 1: walks the lexed line stream and
// populates c.irqHandlerLabels / c.irqHandlerRTELine. Called once per
// ConvertLines invocation, only when c.fpIrqWrap is true.
func (c *Converter) scanRTEHandlerBlocks(lines []Line) {
	c.irqHandlerLabels = map[string]bool{}
	c.irqHandlerRTELine = map[int]string{}
	c.irqOrphanRTELine = map[int]bool{}
	if !c.fpIrqWrap {
		c.irqWrapInitialized = true
		return
	}
	// Pre-determine handler frame size: if any op in the line stream will
	// trigger needsFP56Save (FP5/FP6 scratch-clobbering op), the frame must
	// cover the FP5/FP6 slots globally. Set the flag eagerly so the entry
	// stub emitted at the first handler label sees the final value.
	if !c.needsFP56Save && fileTouchesFP56Scratch(lines) {
		c.needsFP56Save = true
	}
	curLabel := ""
	pendingRTEs := []int{}
	commit := func() {
		if curLabel != "" && len(pendingRTEs) > 0 {
			c.irqHandlerLabels[curLabel] = true
			for _, idx := range pendingRTEs {
				c.irqHandlerRTELine[idx] = curLabel
			}
		}
		curLabel = ""
		pendingRTEs = pendingRTEs[:0]
	}
	for i, l := range lines {
		// A new label opens a new block only if there is no active block.
		// Inside an active block (curLabel != ""), an inner label is a
		// fall-through marker and the outer label keeps ownership.
		newLabel := ""
		if l.Kind == LineLabelOnly {
			newLabel = l.Label
		} else if l.Label != "" {
			newLabel = l.Label
		}
		if newLabel != "" && curLabel == "" {
			curLabel = newLabel
		}
		if l.Kind != LineInstruction {
			continue
		}
		switch l.Mnemonic {
		case "rte":
			// RTE terminates the block AND marks it as a handler when
			// there is a preceding label in scope. Orphan RTE (no
			// active label) cannot be wrapped — record for emit-time
			// diag/error.
			if curLabel == "" {
				c.irqOrphanRTELine[i] = true
			} else {
				pendingRTEs = append(pendingRTEs, i)
				commit()
			}
		case "rts", "jmp", "bra":
			// Block ends here, not a handler — drop pending state.
			curLabel = ""
			pendingRTEs = pendingRTEs[:0]
		}
	}
	commit()
	c.irqWrapInitialized = true
}

// irqWrapFrameSize returns the handler frame size in bytes (16 base, 32 if
// FP5/FP6 slots are in the save set).
func (c *Converter) irqWrapFrameSize() int {
	if c.needsFP56Save {
		return 32
	}
	return 16
}

// emitIRQHandlerEntry emits the save stub at handler entry. Called from
// convertLexed when emitting a label that is in c.irqHandlerLabels. Uses
// integer scratch r17 (ScrV1) as the 32-bit shuttle — 2× load.l/store.l
// per 8B slot, 4 memops/slot. FP-register shuttle is disallowed because
// every even f-reg overlays a guest FP slot (per IE64 ISA §4.6.6,
// fpu_regmap.go:23).
func (c *Converter) emitIRQHandlerEntry(e *Emit, label string) {
	n := c.irqWrapFrameSize()
	e.Lf("; m68kto64: handler at %s wrapped with FP-slot save (%dB frame)", label, n)
	e.Lf("sub.l %s, %s, #%d", GuestSP, GuestSP, n)
	c.emitIRQSlotStore(e, FPSlotFPCRSave, 0)
	c.emitIRQSlotStore(e, FPSlotScratchQ, 8)
	if c.needsFP56Save {
		c.emitIRQSlotStore(e, FPSlotFP5Save, 16)
		c.emitIRQSlotStore(e, FPSlotFP6Save, 24)
	}
}

// emitIRQHandlerExit emits the restore stub immediately before the RTE
// lowering. Called from emitRte when the current line index is in
// c.irqHandlerRTELine. Reverses the entry stub.
func (c *Converter) emitIRQHandlerExit(e *Emit) {
	n := c.irqWrapFrameSize()
	e.L("; m68kto64: restore FP slots before RTE")
	if c.needsFP56Save {
		c.emitIRQSlotLoad(e, FPSlotFP6Save, 24)
		c.emitIRQSlotLoad(e, FPSlotFP5Save, 16)
	}
	c.emitIRQSlotLoad(e, FPSlotScratchQ, 8)
	c.emitIRQSlotLoad(e, FPSlotFPCRSave, 0)
	e.Lf("add.l %s, %s, #%d", GuestSP, GuestSP, n)
}

// emitIRQSlotStore copies an 8-byte global slot to the handler's stack
// frame at the given offset (relative to a7=r30 after sub.l).
func (c *Converter) emitIRQSlotStore(e *Emit, slot string, frameOff int) {
	e.Lf("la %s, %s", ScrEA, slot)
	e.Lf("load.l %s, (%s)", ScrV1, ScrEA)
	e.Lf("store.l %s, %d(%s)", ScrV1, frameOff, GuestSP)
	e.Lf("load.l %s, 4(%s)", ScrV1, ScrEA)
	e.Lf("store.l %s, %d(%s)", ScrV1, frameOff+4, GuestSP)
}

// emitIRQSlotLoad copies 8 bytes from the handler's stack frame back into
// the named global slot.
func (c *Converter) emitIRQSlotLoad(e *Emit, slot string, frameOff int) {
	e.Lf("load.l %s, %d(%s)", ScrV1, frameOff, GuestSP)
	e.Lf("la %s, %s", ScrEA, slot)
	e.Lf("store.l %s, (%s)", ScrV1, ScrEA)
	e.Lf("load.l %s, %d(%s)", ScrV1, frameOff+4, GuestSP)
	e.Lf("la %s, %s", ScrEA, slot)
	e.Lf("store.l %s, 4(%s)", ScrV1, ScrEA)
}
