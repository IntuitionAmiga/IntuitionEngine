// jit_z80_emit_arm64.go - Z80 JIT compiler: ARM64 native code emitter

//go:build arm64 && linux

package main

import (
	"fmt"
)

// ===========================================================================
// Z80 → ARM64 Register Mapping
// ===========================================================================
//
// Host (ARM64)   Z80 Register     Notes
// ────────────   ────────────     ─────
// W19            A                Callee-saved.
// W20            F                Callee-saved.
// W21            BC (B=hi, C=lo)  Callee-saved, packed 16-bit.
// W22            DE (D=hi, E=lo)  Callee-saved, packed 16-bit.
// W23            HL (H=hi, L=lo)  Callee-saved, packed 16-bit.
// W24            SP (Z80)         Callee-saved. 16-bit stack pointer.
// X25            MemBase          &MachineBus.memory[0].
// X26            Context          &Z80JITContext.
// X27            DirectPageBM     &directPageBitmap[0].
// X28            CpuPtr           &CPU_Z80.
// X0             Entry arg        Context on entry.
// W1-W9          Scratch          Caller-saved.
// X10-X17        Scratch          More scratch.
// X29/X30        FP/LR            Saved/restored.

// compileBlockZ80Stub emits every non-observation Z80 form admitted by the
// shared frontend. Port, mapped-memory and code-page writes return to the
// dispatcher before host-visible work so the frozen canonical helper owns the
// observation boundary.
func compileBlockZ80Stub(instrs []JITZ80Instr, startPC, endPC uint16, execMem *ExecMem, totalR int) (*JITBlock, error) {
	if len(instrs) == 0 {
		return nil, fmt.Errorf("Z80 JIT ARM64: canonical helper required")
	}
	var buf CodeBuffer

	totalCycles := uint32(0)
	for _, instr := range instrs {
		totalCycles += uint32(instr.cycles)
	}

	// X0 = context pointer on entry. NOP has no register or memory effects.
	// Direct register forms update CPU_Z80 through the tested context ABI.
	arm64LdrImm64(&buf, 2, 0, jzCtxOffCpuPtr) // X2 = CPU_Z80*
	nextPC := endPC
	dynamicPC := false
	dynamicCycles := false
	retiredCycles, retiredR := uint32(0), uint32(0)
	for i := range instrs {
		instr := instrs[i]
		prefixCycleSurcharge := uint32(0)
		if (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && !z80DDFDExplicitOpcode(instr.opcode) {
			prefixCycleSurcharge = 4
		}
		if !z80ARM64DirectInstruction(instr) {
			return nil, fmt.Errorf("Z80 JIT ARM64: canonical helper required")
		}
		switch {
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0xCB:
			indexOffset := cpuZ80OffIX
			if instr.prefix == z80JITPrefixFD {
				indexOffset = cpuZ80OffIY
			}
			arm64LdrhImm(&buf, 3, 2, int(indexOffset))
			if instr.displacement < 0 {
				arm64AddSubImm(&buf, 3, 3, uint16(-int16(instr.displacement)), true)
			} else {
				arm64AddSubImm(&buf, 3, 3, uint16(instr.displacement), false)
			}
			arm64MovImm32(&buf, 4, 0xFFFF)
			arm64AndReg(&buf, 3, 3, 4)
			arm64MovRegW(&buf, 7, 3) // preserve indexed address across the guarded load
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 1, 3) // old memory byte
			subOp := instr.cbSubOp
			switch subOp >> 6 {
			case 0: // rotate or shift
				arm64EmitCBValueOperation(&buf, 2, (subOp>>3)&7)
				if subOp&7 != 6 {
					arm64StrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(subOp&7)))
				}
				arm64MovRegW(&buf, 4, 7)
				arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), true)
			case 1: // BIT b,(IX/IY+d)
				arm64EmitBITFlags(&buf, 2, 1, (subOp>>3)&7)
			case 2, 3: // RES/SET b,(IX/IY+d),r
				arm64MovImm32(&buf, 4, uint32(1<<((subOp>>3)&7)))
				if subOp>>6 == 2 {
					arm64MovImm32(&buf, 5, ^uint32(1<<((subOp>>3)&7)))
					arm64AndReg(&buf, 3, 3, 5)
				} else {
					arm64OrrReg(&buf, 3, 3, 4)
				}
				if subOp&7 != 6 {
					arm64StrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(subOp&7)))
				}
				arm64MovRegW(&buf, 4, 7)
				arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), true)
			}
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && z80DDFDExplicitOpcode(instr.opcode) && instr.opcode != 0xE3:
			indexOffset := cpuZ80OffIX
			if instr.prefix == z80JITPrefixFD {
				indexOffset = cpuZ80OffIY
			}
			op := instr.opcode
			switch {
			case op == 0x21: // LD IX/IY,nn
				arm64MovImm32(&buf, 1, uint32(instr.operand))
				arm64StrhImm(&buf, 1, 2, int(indexOffset))
			case op == 0x23 || op == 0x2B: // INC/DEC IX/IY
				arm64LdrhImm(&buf, 1, 2, int(indexOffset))
				arm64AddSubImm(&buf, 1, 1, 1, op == 0x2B)
				arm64StrhImm(&buf, 1, 2, int(indexOffset))
			case op == 0xF9: // LD SP,IX/IY
				arm64LdrhImm(&buf, 1, 2, int(indexOffset))
				arm64StrhImm(&buf, 1, 2, int(cpuZ80OffSP))
			case op == 0xE9: // JP (IX/IY)
				if i != len(instrs)-1 {
					return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal JP (IX/IY)")
				}
				arm64LdrhImm(&buf, 1, 2, int(indexOffset))
				arm64StrhImm(&buf, 1, 2, int(cpuZ80OffWZ))
				dynamicPC = true
			case op == 0x09 || op == 0x19 || op == 0x29 || op == 0x39:
				arm64EmitAddIndexPair(&buf, 2, indexOffset, (op>>4)&3)
			case op == 0x22: // LD (nn),IX/IY
				arm64LdrhImm(&buf, 6, 2, int(indexOffset))
				arm64EmitStorePairAbsoluteReg(&buf, 2, uint16(instr.operand), 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			case op == 0x2A: // LD IX/IY,(nn)
				arm64EmitLoadPairAbsolute(&buf, 2, uint16(instr.operand), 1, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
				arm64StrhImm(&buf, 1, 2, int(indexOffset))
			case op&0xC7 == 0x46 && op != 0x76: // LD r,(IX/IY+d)
				arm64EmitIndexAddress(&buf, 2, indexOffset, instr.displacement, 3)
				arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
				arm64StrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset((op>>3)&7)))
			case op >= 0x70 && op <= 0x77 && op != 0x76: // LD (IX/IY+d),r
				arm64EmitIndexAddress(&buf, 2, indexOffset, instr.displacement, 4)
				arm64LdrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(op&7)))
				arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), true)
			case op == 0x36: // LD (IX/IY+d),n
				arm64EmitIndexAddress(&buf, 2, indexOffset, instr.displacement, 4)
				arm64MovImm32(&buf, 3, uint32(instr.operand))
				arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), true)
			case op&0xC7 == 0x86: // ALU A,(IX/IY+d)
				arm64EmitIndexAddress(&buf, 2, indexOffset, instr.displacement, 3)
				arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
				switch op >> 3 {
				case 0x10:
					arm64EmitAddReg(&buf, 2, false)
				case 0x11:
					arm64EmitAddReg(&buf, 2, true)
				case 0x12:
					arm64EmitSubReg(&buf, 2, false)
				case 0x13:
					arm64EmitSubReg(&buf, 2, true)
				case 0x17:
					arm64EmitSubReg(&buf, 2, false)
					arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
				default:
					arm64EmitLogicALU(&buf, 2, op, false, 0)
				}
			case op == 0x34 || op == 0x35: // INC/DEC (IX/IY+d)
				arm64EmitIndexAddress(&buf, 2, indexOffset, instr.displacement, 3)
				arm64MovRegW(&buf, 7, 3)
				arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
				arm64MovRegW(&buf, 1, 3)
				arm64EmitIncDecValue(&buf, 2, op == 0x35)
				arm64MovRegW(&buf, 4, 7)
				arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), true)
			case op == 0xE5 || op == 0xE1:
				if op == 0xE5 {
					arm64LdrhImm(&buf, 6, 2, int(indexOffset))
					arm64EmitPushValue(&buf, 2, 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
				} else {
					arm64EmitPopValue(&buf, 2, 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
					arm64StrhImm(&buf, 6, 2, int(indexOffset))
				}
			}
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && !z80DDFDExplicitOpcode(instr.opcode) && z80DDFDUsesIndexBytes(instr.opcode):
			op := instr.opcode
			switch {
			case op&0xC7 == 0x06:
				arm64MovImm32(&buf, 1, uint32(instr.operand))
				arm64StrbImm(&buf, 1, 2, int(z80ARM64IndexReg8Offset(instr.prefix, (op>>3)&7)))
			case op&0xC7 == 0x04 || op&0xC7 == 0x05:
				arm64EmitIncDecReg(&buf, 2, z80ARM64IndexReg8Offset(instr.prefix, (op>>3)&7), op&0xC7 == 0x05)
			case op >= 0x40 && op <= 0x7F:
				arm64LdrbImm(&buf, 1, 2, int(z80ARM64IndexReg8Offset(instr.prefix, op&7)))
				arm64StrbImm(&buf, 1, 2, int(z80ARM64IndexReg8Offset(instr.prefix, (op>>3)&7)))
			case op >= 0x80 && op <= 0xBF:
				arm64LdrbImm(&buf, 3, 2, int(z80ARM64IndexReg8Offset(instr.prefix, op&7)))
				switch op >> 3 {
				case 0x10:
					arm64EmitAddReg(&buf, 2, false)
				case 0x11:
					arm64EmitAddReg(&buf, 2, true)
				case 0x12:
					arm64EmitSubReg(&buf, 2, false)
				case 0x13:
					arm64EmitSubReg(&buf, 2, true)
				case 0x17: // CP
					arm64EmitSubReg(&buf, 2, false)
					arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
				default:
					arm64EmitLogicALU(&buf, 2, op, false, 0)
				}
			}
		case instr.prefix == z80JITPrefixCB && z80CBSetResHLOpcode(instr.opcode):
			arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovImm32(&buf, 4, uint32(1<<((instr.opcode>>3)&7)))
			if instr.opcode>>6 == 2 {
				arm64MovImm32(&buf, 5, ^uint32(1<<((instr.opcode>>3)&7)))
				arm64AndReg(&buf, 3, 3, 5)
			} else {
				arm64OrrReg(&buf, 3, 3, 4)
			}
			arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), false)
		case instr.prefix == z80JITPrefixCB && z80CBRLCRegisterOpcode(instr.opcode):
			arm64EmitCBRLCReg(&buf, 2, z80ARM64Reg8Offset(instr.opcode&7))
		case instr.prefix == z80JITPrefixCB && z80CBRRCRegisterOpcode(instr.opcode):
			arm64EmitCBRRCReg(&buf, 2, z80ARM64Reg8Offset(instr.opcode&7))
		case instr.prefix == z80JITPrefixCB && z80CBRLRegisterOpcode(instr.opcode):
			arm64EmitCBRLReg(&buf, 2, z80ARM64Reg8Offset(instr.opcode&7))
		case instr.prefix == z80JITPrefixCB && z80CBRRRegisterOpcode(instr.opcode):
			arm64EmitCBRRReg(&buf, 2, z80ARM64Reg8Offset(instr.opcode&7))
		case instr.prefix == z80JITPrefixCB && z80CBSLARegisterOpcode(instr.opcode):
			arm64EmitCBSLAReg(&buf, 2, z80ARM64Reg8Offset(instr.opcode&7))
		case instr.prefix == z80JITPrefixCB && z80CBSRLRegisterOpcode(instr.opcode):
			arm64EmitCBSRLReg(&buf, 2, z80ARM64Reg8Offset(instr.opcode&7))
		case instr.prefix == z80JITPrefixCB && z80CBSRARegisterOpcode(instr.opcode):
			arm64EmitCBSRAReg(&buf, 2, z80ARM64Reg8Offset(instr.opcode&7))
		case instr.prefix == z80JITPrefixCB && z80CBSLLRegisterOpcode(instr.opcode):
			arm64EmitCBSLLReg(&buf, 2, z80ARM64Reg8Offset(instr.opcode&7))
		case instr.prefix == z80JITPrefixCB && instr.opcode>>6 == 0 && instr.opcode&7 == 6:
			arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 1, 3) // old value
			group := (instr.opcode >> 3) & 7
			switch group {
			case 0: // RLC
				buf.Emit32(arm64LSL_W_imm(3, 1, 1))
				arm64LsrImm(&buf, 4, 1, 7)
				arm64OrrReg(&buf, 3, 3, 4)
			case 1: // RRC
				arm64LsrImm(&buf, 3, 1, 1)
				buf.Emit32(arm64LSL_W_imm(4, 1, 7))
				arm64OrrReg(&buf, 3, 3, 4)
			case 2: // RL
				buf.Emit32(arm64LSL_W_imm(3, 1, 1))
				arm64LdrbImm(&buf, 4, 2, int(cpuZ80OffF))
				arm64MovImm32(&buf, 5, z80FlagC)
				arm64AndReg(&buf, 4, 4, 5)
				arm64OrrReg(&buf, 3, 3, 4)
			case 3: // RR
				arm64LsrImm(&buf, 3, 1, 1)
				arm64LdrbImm(&buf, 4, 2, int(cpuZ80OffF))
				arm64MovImm32(&buf, 5, z80FlagC)
				arm64AndReg(&buf, 4, 4, 5)
				buf.Emit32(arm64LSL_W_imm(4, 4, 7))
				arm64OrrReg(&buf, 3, 3, 4)
			case 4: // SLA
				buf.Emit32(arm64LSL_W_imm(3, 1, 1))
			case 5: // SRA
				arm64LsrImm(&buf, 3, 1, 1)
				arm64MovImm32(&buf, 4, 0x80)
				arm64AndReg(&buf, 4, 1, 4)
				arm64OrrReg(&buf, 3, 3, 4)
			case 6: // SLL
				buf.Emit32(arm64LSL_W_imm(3, 1, 1))
				arm64MovImm32(&buf, 4, 1)
				arm64OrrReg(&buf, 3, 3, 4)
			case 7: // SRL
				arm64LsrImm(&buf, 3, 1, 1)
			}
			arm64MovImm32(&buf, 4, 0xFF)
			arm64AndReg(&buf, 3, 3, 4)
			arm64EmitCBRotateFlags(&buf, 2, 1, 3, group == 0 || group == 2 || group == 4 || group == 6)
			arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), false)
		case instr.prefix == z80JITPrefixCB && instr.opcode>>6 == 1: // BIT b,r
			bit := (instr.opcode >> 3) & 7
			if instr.opcode&7 == 6 {
				arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
				arm64MovRegW(&buf, 1, 3)
			} else {
				arm64LdrbImm(&buf, 1, 2, int(z80ARM64Reg8Offset(instr.opcode&7)))
			}
			arm64EmitBITFlags(&buf, 2, 1, bit)
		case instr.prefix == z80JITPrefixCB && z80CBSetResRegisterOpcode(instr.opcode):
			offset := z80ARM64Reg8Offset(instr.opcode & 7)
			arm64LdrbImm(&buf, 1, 2, int(offset))
			arm64MovImm32(&buf, 3, uint32(1<<((instr.opcode>>3)&7)))
			if instr.opcode>>6 == 2 { // RES
				arm64MovImm32(&buf, 3, ^uint32(1<<((instr.opcode>>3)&7)))
				arm64AndReg(&buf, 1, 1, 3)
			} else { // SET
				arm64OrrReg(&buf, 1, 1, 3)
			}
			arm64StrbImm(&buf, 1, 2, int(offset))
		case instr.prefix == z80JITPrefixED && instr.opcode == 0x57: // LD A,I
			arm64LdrbImm(&buf, 1, 2, int(cpuZ80OffI))
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffF))
			arm64MovImm32(&buf, 4, z80FlagC)
			arm64AndReg(&buf, 3, 3, 4)
			arm64MovImm32(&buf, 4, z80FlagS|z80FlagX|z80FlagY)
			arm64AndReg(&buf, 4, 1, 4)
			arm64OrrReg(&buf, 3, 3, 4)
			buf.Emit32(arm64CMP_W_imm(1, 0))
			buf.Emit32(arm64CSET_W(4, arm64CondEQ))
			buf.Emit32(arm64LSL_W_imm(4, 4, 6))
			arm64OrrReg(&buf, 3, 3, 4)
			arm64LdrbImm(&buf, 4, 2, int(cpuZ80OffIFF2))
			buf.Emit32(arm64CMP_W_imm(4, 0))
			buf.Emit32(arm64CSET_W(4, arm64CondNE))
			buf.Emit32(arm64LSL_W_imm(4, 4, 2))
			arm64OrrReg(&buf, 3, 3, 4)
			arm64StrbImm(&buf, 3, 2, int(cpuZ80OffF))
		case instr.prefix == z80JITPrefixED && z80EDNEGOpcode(instr.opcode): // NEG = 0-A
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffA))
			arm64MovImm32(&buf, 1, 0)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
			arm64EmitSubReg(&buf, 2, false)
		case instr.prefix == z80JITPrefixED && z80EDInterruptMode(instr.opcode) >= 0:
			arm64MovImm32(&buf, 1, uint32(z80EDInterruptMode(instr.opcode)))
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffIM))
		case instr.prefix == z80JITPrefixED && instr.opcode == 0x47: // LD I,A
			arm64LdrbImm(&buf, 1, 2, int(cpuZ80OffA))
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffI))
		case instr.prefix == z80JITPrefixED && instr.opcode == 0x4F: // LD R,A
			arm64LdrbImm(&buf, 1, 2, int(cpuZ80OffA))
			arm64MovImm32(&buf, 3, 0x80)
			arm64AndReg(&buf, 3, 1, 3)
			arm64MovImm32(&buf, 4, 0x7F)
			arm64AndReg(&buf, 1, 1, 4)
			arm64AddSubImm(&buf, 1, 1, uint16(retiredR)+uint16(instr.rIncrements), true)
			arm64AndReg(&buf, 1, 1, 4)
			arm64OrrReg(&buf, 1, 1, 3)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffR))
		case instr.prefix == z80JITPrefixED && instr.opcode == 0x5F: // LD A,R
			arm64LdrbImm(&buf, 1, 2, int(cpuZ80OffR))
			arm64MovImm32(&buf, 3, 0x80)
			arm64AndReg(&buf, 3, 1, 3)
			arm64MovImm32(&buf, 4, 0x7F)
			arm64AndReg(&buf, 1, 1, 4)
			arm64AddSubImm(&buf, 1, 1, uint16(retiredR)+uint16(instr.rIncrements), false)
			arm64AndReg(&buf, 1, 1, 4)
			arm64OrrReg(&buf, 1, 1, 3)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
			arm64EmitLDAIRFlags(&buf, 2, 1)
		case instr.prefix == z80JITPrefixED && (instr.opcode&0xCF == 0x42 || instr.opcode&0xCF == 0x4A):
			arm64EmitADCSubHLPair(&buf, 2, (instr.opcode>>4)&3, instr.opcode&0xCF == 0x42)
		case instr.prefix == z80JITPrefixED && instr.opcode&0xCF == 0x43: // LD (nn),rp
			arm64LoadZ80Pair16(&buf, 2, (instr.opcode>>4)&3, 6, 8)
			arm64EmitStorePairAbsoluteReg(&buf, 2, uint16(instr.operand), 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
		case instr.prefix == z80JITPrefixED && instr.opcode&0xCF == 0x4B: // LD rp,(nn)
			address := uint16(instr.operand)
			arm64MovImm32(&buf, 3, uint32(address))
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 7, 3)
			arm64MovImm32(&buf, 3, uint32(uint16(address+1)))
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			buf.Emit32(arm64LSL_W_imm(3, 3, 8))
			arm64OrrReg(&buf, 1, 3, 7)
			arm64StoreZ80Pair16(&buf, 2, (instr.opcode>>4)&3, 1, 7)
			arm64MovImm32(&buf, 1, uint32(uint16(address+1)))
			arm64StrhImm(&buf, 1, 2, int(cpuZ80OffWZ))
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0x45 || instr.opcode == 0x4D || instr.opcode == 0x55 || instr.opcode == 0x5D || instr.opcode == 0x65 || instr.opcode == 0x6D || instr.opcode == 0x75 || instr.opcode == 0x7D):
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal RETN/RETI")
			}
			arm64LdrhImm(&buf, 9, 2, int(cpuZ80OffSP))
			arm64MovRegW(&buf, 3, 9)
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 7, 3)
			arm64AddSubImm(&buf, 3, 9, 1, false)
			arm64MovImm32(&buf, 8, 0xFFFF)
			arm64AndReg(&buf, 3, 3, 8)
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			buf.Emit32(arm64LSL_W_imm(3, 3, 8))
			arm64OrrReg(&buf, 1, 3, 7)
			arm64AddSubImm(&buf, 9, 9, 2, false)
			arm64StrhImm(&buf, 9, 2, int(cpuZ80OffSP))
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffIFF2))
			arm64StrbImm(&buf, 3, 2, int(cpuZ80OffIFF1))
			dynamicPC = true
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0x67 || instr.opcode == 0x6F): // RRD/RLD
			arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 1, 3) // old memory byte
			arm64LdrbImm(&buf, 4, 2, int(cpuZ80OffA))
			arm64MovImm32(&buf, 6, 0xF0)
			arm64AndReg(&buf, 5, 4, 6)
			if instr.opcode == 0x67 { // RRD
				arm64MovImm32(&buf, 6, 0x0F)
				arm64AndReg(&buf, 6, 1, 6)
				arm64OrrReg(&buf, 5, 5, 6)
				arm64LsrImm(&buf, 3, 1, 4)
				buf.Emit32(arm64LSL_W_imm(6, 4, 4))
				arm64OrrReg(&buf, 3, 3, 6)
			} else { // RLD
				arm64LsrImm(&buf, 6, 1, 4)
				arm64OrrReg(&buf, 5, 5, 6)
				buf.Emit32(arm64LSL_W_imm(3, 1, 4))
				arm64MovImm32(&buf, 6, 0x0F)
				arm64AndReg(&buf, 6, 4, 6)
				arm64OrrReg(&buf, 3, 3, 6)
			}
			arm64MovImm32(&buf, 6, 0xFF)
			arm64AndReg(&buf, 3, 3, 6)
			arm64MovRegW(&buf, 7, 3)
			arm64StrbImm(&buf, 5, 2, int(cpuZ80OffA))
			arm64EmitSZPFlagsPreserveCarry(&buf, 2, 5)
			arm64MovRegW(&buf, 3, 7)
			arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), false)
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0xA0 || instr.opcode == 0xA8 || instr.opcode == 0xB0 || instr.opcode == 0xB8): // LDI/LDD/LDIR/LDDR
			repeat := instr.opcode&0x10 != 0
			decrement := instr.opcode&0x08 != 0
			if repeat && i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal block transfer")
			}
			arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 7, 3) // transferred byte
			arm64LoadZ80Pair16(&buf, 2, 1, 3, 8)
			arm64MovRegW(&buf, 9, 3) // destination address
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64LoadZ80Pair16(&buf, 2, 0, 1, 8)
			arm64AddSubImm(&buf, 1, 1, 1, true)
			arm64StoreZ80Pair16(&buf, 2, 0, 1, 8)
			arm64LoadZ80Pair16(&buf, 2, 2, 3, 8)
			arm64AddSubImm(&buf, 3, 3, 1, decrement)
			arm64StoreZ80Pair16(&buf, 2, 2, 3, 8)
			arm64LoadZ80Pair16(&buf, 2, 1, 4, 8)
			arm64AddSubImm(&buf, 4, 4, 1, decrement)
			arm64StoreZ80Pair16(&buf, 2, 1, 4, 8)
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffF))
			arm64MovImm32(&buf, 8, z80FlagS|z80FlagZ|z80FlagC)
			arm64AndReg(&buf, 3, 3, 8)
			buf.Emit32(arm64CMP_W_imm(1, 0))
			buf.Emit32(arm64CSET_W(8, arm64CondNE))
			buf.Emit32(arm64LSL_W_imm(8, 8, 2))
			arm64OrrReg(&buf, 3, 3, 8)
			arm64LdrbImm(&buf, 4, 2, int(cpuZ80OffA))
			arm64AddReg(&buf, 4, 4, 7)
			arm64MovImm32(&buf, 8, z80FlagX|z80FlagY)
			arm64AndReg(&buf, 4, 4, 8)
			arm64OrrReg(&buf, 3, 3, 4)
			arm64StrbImm(&buf, 3, 2, int(cpuZ80OffF))
			arm64MovRegW(&buf, 3, 7)
			arm64MovRegW(&buf, 4, 9)
			arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), true)
			if repeat {
				buf.Emit32(arm64CMP_W_imm(1, 0))
				arm64MovImm32(&buf, 3, uint32(startPC+instr.pcOffset))
				arm64MovImm32(&buf, 4, uint32(startPC+instr.pcOffset+uint16(instr.length)))
				buf.Emit32(arm64CSEL_W(1, 3, 4, arm64CondNE))
				arm64MovImm32(&buf, 3, retiredCycles+21)
				arm64MovImm32(&buf, 4, retiredCycles+16)
				buf.Emit32(arm64CSEL_W(9, 3, 4, arm64CondNE))
				dynamicPC, dynamicCycles = true, true
			}
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0xA1 || instr.opcode == 0xA9 || instr.opcode == 0xB1 || instr.opcode == 0xB9): // CPI/CPD/CPIR/CPDR
			repeat := instr.opcode&0x10 != 0
			decrement := instr.opcode&0x08 != 0
			if repeat && i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal block compare")
			}
			arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 7, 3)
			arm64LoadZ80Pair16(&buf, 2, 2, 3, 8)
			arm64AddSubImm(&buf, 3, 3, 1, decrement)
			arm64StoreZ80Pair16(&buf, 2, 2, 3, 8)
			arm64LoadZ80Pair16(&buf, 2, 0, 8, 6)
			arm64AddSubImm(&buf, 8, 8, 1, true)
			arm64StoreZ80Pair16(&buf, 2, 0, 8, 6)
			arm64MovRegW(&buf, 3, 7)
			arm64EmitSubReg(&buf, 2, false)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffF))
			arm64MovImm32(&buf, 4, ^uint32(z80FlagPV))
			arm64AndReg(&buf, 3, 3, 4)
			buf.Emit32(arm64CMP_W_imm(8, 0))
			buf.Emit32(arm64CSET_W(4, arm64CondNE))
			buf.Emit32(arm64LSL_W_imm(4, 4, 2))
			arm64OrrReg(&buf, 3, 3, 4)
			arm64StrbImm(&buf, 3, 2, int(cpuZ80OffF))
			if repeat {
				buf.Emit32(arm64CMP_W_imm(8, 0))
				buf.Emit32(arm64CSET_W(6, arm64CondNE))
				arm64MovImm32(&buf, 4, z80FlagZ)
				arm64AndReg(&buf, 4, 3, 4)
				buf.Emit32(arm64CMP_W_imm(4, 0))
				buf.Emit32(arm64CSET_W(4, arm64CondEQ))
				arm64AndReg(&buf, 6, 6, 4)
				buf.Emit32(arm64CMP_W_imm(6, 0))
				arm64MovImm32(&buf, 3, uint32(startPC+instr.pcOffset))
				arm64MovImm32(&buf, 4, uint32(startPC+instr.pcOffset+uint16(instr.length)))
				buf.Emit32(arm64CSEL_W(1, 3, 4, arm64CondNE))
				arm64MovImm32(&buf, 3, retiredCycles+21)
				arm64MovImm32(&buf, 4, retiredCycles+16)
				buf.Emit32(arm64CSEL_W(9, 3, 4, arm64CondNE))
				dynamicPC, dynamicCycles = true, true
			}
		case instr.prefix == z80JITPrefixED:
			// Remaining non-observation ED encodings are either lowered above
			// or architecturally execute as an 8-cycle NOP. The exhaustive
			// manifest differential prevents a semantic opcode landing here.
		case instr.opcode&0xC7 == 0x06:
			if instr.opcode == 0x36 {
				arm64MovImm32(&buf, 3, uint32(instr.operand))
				arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), false)
				break
			}
			arm64MovImm32(&buf, 1, uint32(instr.operand))
			arm64StrbImm(&buf, 1, 2, int(z80ARM64Reg8Offset((instr.opcode>>3)&0x07)))
		case instr.opcode == 0x02 || instr.opcode == 0x12:
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffA))
			high, low := z80ARM64Pair8Offsets((instr.opcode >> 4) & 1)
			arm64LdrbImm(&buf, 4, 2, int(high))
			buf.Emit32(arm64LSL_W_imm(4, 4, 8))
			arm64LdrbImm(&buf, 5, 2, int(low))
			arm64OrrReg(&buf, 4, 4, 5)
			arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), true)
		case instr.opcode == 0x3A:
			arm64MovImm32(&buf, 3, uint32(instr.operand))
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 1, 3)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
			arm64MovImm32(&buf, 1, uint32(instr.operand))
			arm64StrhImm(&buf, 1, 2, int(cpuZ80OffWZ))
		case instr.opcode == 0x2A:
			address := uint16(instr.operand)
			arm64MovImm32(&buf, 3, uint32(address))
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 7, 3) // low byte, no architectural write yet
			arm64MovImm32(&buf, 3, uint32(uint16(address+1)))
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64StrbImm(&buf, 3, 2, int(cpuZ80OffH))
			arm64StrbImm(&buf, 7, 2, int(cpuZ80OffL))
			arm64MovImm32(&buf, 1, uint32(uint16(address+1)))
			arm64StrhImm(&buf, 1, 2, int(cpuZ80OffWZ))
		case instr.opcode == 0x22: // LD (nn),HL
			arm64LoadZ80Pair16(&buf, 2, 2, 6, 8)
			arm64EmitStorePairAbsoluteReg(&buf, 2, uint16(instr.operand), 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
		case instr.opcode >= 0x70 && instr.opcode <= 0x77 && instr.opcode != 0x76:
			arm64LdrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(instr.opcode&7)))
			arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), false)
		case instr.opcode == 0x32:
			arm64MovImm32(&buf, 3, uint32(instr.operand))
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffA))
			arm64MovImm32(&buf, 4, uint32(instr.operand))
			arm64MovImm32(&buf, 1, uint32(instr.operand))
			arm64StrhImm(&buf, 1, 2, int(cpuZ80OffWZ))
			arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), true)
		case instr.opcode == 0x0A || instr.opcode == 0x1A:
			high, low := z80ARM64Pair8Offsets((instr.opcode >> 4) & 1)
			arm64EmitGuardedLoadPair(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, high, low)
			arm64MovRegW(&buf, 1, 3)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
		case instr.opcode&0xCF == 0x01:
			pair := (instr.opcode >> 4) & 0x03
			if pair == 3 { // LD SP,nn
				arm64MovImm32(&buf, 1, uint32(instr.operand))
				arm64StrhImm(&buf, 1, 2, int(cpuZ80OffSP))
			} else {
				high, low := z80ARM64Pair8Offsets(pair)
				arm64MovImm32(&buf, 1, uint32(instr.operand>>8))
				arm64StrbImm(&buf, 1, 2, int(high))
				arm64MovImm32(&buf, 1, uint32(instr.operand))
				arm64StrbImm(&buf, 1, 2, int(low))
			}
		case instr.opcode&0xCF == 0x03 || instr.opcode&0xCF == 0x0B:
			pair := (instr.opcode >> 4) & 0x03
			offset := cpuZ80OffSP
			needsSwap := false
			if pair != 3 {
				offset, _ = z80ARM64Pair8Offsets(pair)
				needsSwap = true
			}
			arm64LdrhImm(&buf, 1, 2, int(offset))
			if needsSwap {
				arm64Rev16W(&buf, 1, 1)
			}
			arm64AddSubImm(&buf, 1, 1, 1, instr.opcode&0xCF == 0x0B)
			if needsSwap {
				arm64Rev16W(&buf, 1, 1)
			}
			arm64StrhImm(&buf, 1, 2, int(offset))
		case instr.opcode >= 0x40 && instr.opcode <= 0x7F:
			dest := (instr.opcode >> 3) & 0x07
			src := instr.opcode & 0x07
			if src == 6 {
				arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
				arm64MovRegW(&buf, 1, 3)
			} else {
				arm64LdrbImm(&buf, 1, 2, int(z80ARM64Reg8Offset(src)))
			}
			arm64StrbImm(&buf, 1, 2, int(z80ARM64Reg8Offset(dest)))
		case instr.opcode == 0xF9: // LD SP,HL
			arm64LdrbImm(&buf, 1, 2, int(cpuZ80OffL))
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffSP))
			arm64LdrbImm(&buf, 1, 2, int(cpuZ80OffH))
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffSP+1))
		case instr.opcode == 0xEB: // EX DE,HL
			arm64SwapByte(&buf, 2, cpuZ80OffD, cpuZ80OffH)
			arm64SwapByte(&buf, 2, cpuZ80OffE, cpuZ80OffL)
		case instr.opcode == 0x08: // EX AF,AF'
			arm64SwapByte(&buf, 2, cpuZ80OffA, cpuZ80OffA2)
			arm64SwapByte(&buf, 2, cpuZ80OffF, cpuZ80OffF2)
		case instr.opcode == 0xD9: // EXX
			arm64SwapByte(&buf, 2, cpuZ80OffB, cpuZ80OffB2)
			arm64SwapByte(&buf, 2, cpuZ80OffC, cpuZ80OffC2)
			arm64SwapByte(&buf, 2, cpuZ80OffD, cpuZ80OffD2)
			arm64SwapByte(&buf, 2, cpuZ80OffE, cpuZ80OffE2)
			arm64SwapByte(&buf, 2, cpuZ80OffH, cpuZ80OffH2)
			arm64SwapByte(&buf, 2, cpuZ80OffL, cpuZ80OffL2)
		case instr.opcode == 0x37: // SCF
			arm64EmitSCF(&buf, 2)
		case instr.opcode == 0x3F: // CCF
			arm64EmitCCF(&buf, 2)
		case instr.opcode == 0x2F: // CPL
			arm64EmitCPL(&buf, 2)
		case instr.opcode == 0x27: // DAA
			arm64LdrbImm(&buf, 1, 2, int(cpuZ80OffA))
			buf.Emit32(arm64LSL_W_imm(1, 1, 3))
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffF))
			arm64MovImm32(&buf, 4, z80FlagC)
			arm64AndReg(&buf, 4, 3, 4)
			buf.Emit32(arm64LSL_W_imm(4, 4, 2))
			arm64OrrReg(&buf, 1, 1, 4)
			arm64MovImm32(&buf, 4, z80FlagH)
			arm64AndReg(&buf, 4, 3, 4)
			arm64LsrImm(&buf, 4, 4, 3)
			arm64OrrReg(&buf, 1, 1, 4)
			arm64MovImm32(&buf, 4, z80FlagN)
			arm64AndReg(&buf, 4, 3, 4)
			arm64LsrImm(&buf, 4, 4, 1)
			arm64OrrReg(&buf, 1, 1, 4)
			buf.Emit32(arm64LSL_W_imm(1, 1, 1)) // uint16 table byte offset
			arm64LdrImm64(&buf, 4, 0, jzCtxOffDAATablePtr)
			buf.Emit32(arm64LDRH_reg(1, 4, 1))
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffF))
			arm64LsrImm(&buf, 1, 1, 8)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
		case instr.opcode == 0x07 || instr.opcode == 0x0F || instr.opcode == 0x17 || instr.opcode == 0x1F:
			arm64EmitRotateA(&buf, 2, instr.opcode)
		case (instr.opcode == 0x34 || instr.opcode == 0x35) && instr.prefix == z80JITPrefixNone:
			arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 1, 3)
			arm64EmitIncDecValue(&buf, 2, instr.opcode == 0x35)
			arm64EmitGuardedStore(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR, uint32(instr.length), uint32(instr.cycles), uint32(instr.rIncrements), false)
		case instr.opcode&0xC7 == 0x04: // INC r
			arm64EmitIncDecReg(&buf, 2, z80ARM64Reg8Offset((instr.opcode>>3)&7), false)
		case instr.opcode&0xC7 == 0x05: // DEC r
			arm64EmitIncDecReg(&buf, 2, z80ARM64Reg8Offset((instr.opcode>>3)&7), true)
		case instr.opcode&0xCF == 0x09: // ADD HL,rp
			arm64EmitAddHLPair(&buf, 2, (instr.opcode>>4)&3)
		case instr.opcode >= 0xA0 && instr.opcode <= 0xB7: // AND/XOR/OR A,r
			if instr.opcode&7 == 6 {
				arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			} else {
				arm64LdrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(instr.opcode&7)))
			}
			arm64EmitLogicALU(&buf, 2, instr.opcode, false, 0)
		case instr.opcode >= 0x80 && instr.opcode <= 0x87: // ADD A,r
			if instr.opcode == 0x86 {
				arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			} else {
				arm64LdrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(instr.opcode&7)))
			}
			arm64EmitAddReg(&buf, 2, false)
		case instr.opcode == 0xC6: // ADD A,n
			arm64MovImm32(&buf, 3, uint32(instr.operand))
			arm64EmitAddReg(&buf, 2, false)
		case instr.opcode >= 0x88 && instr.opcode <= 0x8F: // ADC A,r
			if instr.opcode&7 == 6 {
				arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			} else {
				arm64LdrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(instr.opcode&7)))
			}
			arm64EmitAddReg(&buf, 2, true)
		case instr.opcode == 0xCE: // ADC A,n
			arm64MovImm32(&buf, 3, uint32(instr.operand))
			arm64EmitAddReg(&buf, 2, true)
		case instr.opcode >= 0x90 && instr.opcode <= 0x97: // SUB A,r
			if instr.opcode&7 == 6 {
				arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			} else {
				arm64LdrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(instr.opcode&7)))
			}
			arm64EmitSubReg(&buf, 2, false)
		case instr.opcode == 0xD6: // SUB A,n
			arm64MovImm32(&buf, 3, uint32(instr.operand))
			arm64EmitSubReg(&buf, 2, false)
		case instr.opcode >= 0x98 && instr.opcode <= 0x9F: // SBC A,r
			if instr.opcode&7 == 6 {
				arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			} else {
				arm64LdrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(instr.opcode&7)))
			}
			arm64EmitSubReg(&buf, 2, true)
		case instr.opcode == 0xDE: // SBC A,n
			arm64MovImm32(&buf, 3, uint32(instr.operand))
			arm64EmitSubReg(&buf, 2, true)
		case instr.opcode >= 0xB8 && instr.opcode <= 0xBF: // CP A,r
			if instr.opcode&7 == 6 {
				arm64EmitGuardedLoadHL(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			} else {
				arm64LdrbImm(&buf, 3, 2, int(z80ARM64Reg8Offset(instr.opcode&7)))
			}
			arm64EmitSubReg(&buf, 2, false)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
		case instr.opcode == 0xFE: // CP A,n
			arm64MovImm32(&buf, 3, uint32(instr.operand))
			arm64EmitSubReg(&buf, 2, false)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffA))
		case instr.opcode == 0xE6 || instr.opcode == 0xEE || instr.opcode == 0xF6: // AND/XOR/OR A,n
			arm64EmitLogicALU(&buf, 2, instr.opcode, true, byte(instr.operand))
		case instr.opcode == 0xE3: // EX (SP),HL/IX/IY
			arm64LdrhImm(&buf, 9, 2, int(cpuZ80OffSP))
			arm64MovRegW(&buf, 3, 9)
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 7, 3)
			arm64AddSubImm(&buf, 3, 9, 1, false)
			arm64MovImm32(&buf, 8, 0xFFFF)
			arm64AndReg(&buf, 3, 3, 8)
			arm64EmitGuardedLoadAddress(&buf, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovRegW(&buf, 8, 3)
			arm64AddSubImm(&buf, 6, 9, 1, false)
			arm64MovImm32(&buf, 4, 0xFFFF)
			arm64AndReg(&buf, 6, 6, 4)
			arm64EmitBailIfCodeAddress(&buf, 9, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64EmitBailIfCodeAddress(&buf, 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			if instr.prefix == z80JITPrefixDD {
				arm64LdrhImm(&buf, 6, 2, int(cpuZ80OffIX))
			} else if instr.prefix == z80JITPrefixFD {
				arm64LdrhImm(&buf, 6, 2, int(cpuZ80OffIY))
			} else {
				arm64LoadZ80Pair16(&buf, 2, 2, 6, 4)
				arm64StrbImm(&buf, 7, 2, int(cpuZ80OffL))
				arm64StrbImm(&buf, 8, 2, int(cpuZ80OffH))
			}
			buf.Emit32(arm64LSL_W_imm(8, 8, 8))
			arm64OrrReg(&buf, 8, 8, 7)
			if instr.prefix == z80JITPrefixDD {
				arm64StrhImm(&buf, 8, 2, int(cpuZ80OffIX))
			} else if instr.prefix == z80JITPrefixFD {
				arm64StrhImm(&buf, 8, 2, int(cpuZ80OffIY))
			}
			arm64StrhImm(&buf, 8, 2, int(cpuZ80OffWZ))
			arm64LdrImm64(&buf, 5, 0, jzCtxOffMemPtr)
			buf.Emit32(arm64STRB_reg(6, 5, 9))
			arm64LsrImm(&buf, 6, 6, 8)
			arm64AddSubImm(&buf, 9, 9, 1, false)
			arm64MovImm32(&buf, 4, 0xFFFF)
			arm64AndReg(&buf, 9, 9, 4)
			buf.Emit32(arm64STRB_reg(6, 5, 9))
		case instr.opcode&0xCF == 0xC5: // PUSH rp
			pair := (instr.opcode >> 4) & 3
			if pair == 3 {
				arm64LdrbImm(&buf, 6, 2, int(cpuZ80OffA))
				buf.Emit32(arm64LSL_W_imm(6, 6, 8))
				arm64LdrbImm(&buf, 8, 2, int(cpuZ80OffF))
				arm64OrrReg(&buf, 6, 6, 8)
			} else {
				arm64LoadZ80Pair16(&buf, 2, pair, 6, 8)
			}
			arm64EmitPushValue(&buf, 2, 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
		case instr.opcode&0xCF == 0xC1: // POP rp
			pair := (instr.opcode >> 4) & 3
			arm64EmitPopValue(&buf, 2, 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			if pair == 3 {
				arm64StrbImm(&buf, 6, 2, int(cpuZ80OffF))
				arm64LsrImm(&buf, 8, 6, 8)
				arm64StrbImm(&buf, 8, 2, int(cpuZ80OffA))
			} else {
				arm64StoreZ80Pair16(&buf, 2, pair, 6, 8)
			}
		case instr.opcode == 0xF3: // DI
			arm64MovImm32(&buf, 1, 0)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffIFF1))
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffIFF2))
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffIffDelay))
		case instr.opcode == 0xFB: // EI
			arm64MovImm32(&buf, 1, 1)
			arm64StrbImm(&buf, 1, 2, int(cpuZ80OffIffDelay))
		case instr.opcode == 0x10: // DJNZ e
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal DJNZ")
			}
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffB))
			arm64AddSubImm(&buf, 3, 3, 1, true)
			arm64StrbImm(&buf, 3, 2, int(cpuZ80OffB))
			buf.Emit32(arm64CMP_W_imm(3, 0))
			instrPC := startPC + instr.pcOffset
			target := instrPC + uint16(instr.length) + uint16(int16(int8(instr.operand)))
			arm64MovImm32(&buf, 4, uint32(target))
			arm64MovImm32(&buf, 5, uint32(instrPC+uint16(instr.length)))
			buf.Emit32(arm64CSEL_W(1, 4, 5, arm64CondNE))
			arm64MovImm32(&buf, 4, retiredCycles+prefixCycleSurcharge+13)
			arm64MovImm32(&buf, 5, retiredCycles+prefixCycleSurcharge+8)
			buf.Emit32(arm64CSEL_W(9, 4, 5, arm64CondNE))
			dynamicPC, dynamicCycles = true, true
		case instr.opcode == 0x20 || instr.opcode == 0x28 || instr.opcode == 0x30 || instr.opcode == 0x38: // JR cc,e
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal JR cc")
			}
			arm64EmitCondition(&buf, 2, (instr.opcode>>3)&3, 6)
			buf.Emit32(arm64CMP_W_imm(6, 0))
			instrPC := startPC + instr.pcOffset
			target := instrPC + uint16(instr.length) + uint16(int16(int8(instr.operand)))
			arm64MovImm32(&buf, 4, uint32(target))
			arm64MovImm32(&buf, 5, uint32(instrPC+uint16(instr.length)))
			buf.Emit32(arm64CSEL_W(1, 4, 5, arm64CondNE))
			arm64MovImm32(&buf, 4, retiredCycles+prefixCycleSurcharge+12)
			arm64MovImm32(&buf, 5, retiredCycles+prefixCycleSurcharge+7)
			buf.Emit32(arm64CSEL_W(9, 4, 5, arm64CondNE))
			dynamicPC, dynamicCycles = true, true
		case instr.opcode&0xC7 == 0xC2: // JP cc,nn
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal JP cc")
			}
			arm64EmitCondition(&buf, 2, (instr.opcode>>3)&7, 6)
			buf.Emit32(arm64CMP_W_imm(6, 0))
			arm64MovImm32(&buf, 4, uint32(instr.operand))
			arm64MovImm32(&buf, 5, uint32(startPC+instr.pcOffset+uint16(instr.length)))
			buf.Emit32(arm64CSEL_W(1, 4, 5, arm64CondNE))
			dynamicPC = true
		case instr.opcode&0xC7 == 0xC4: // CALL cc,nn
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal CALL cc")
			}
			arm64EmitCondition(&buf, 2, (instr.opcode>>3)&7, 6)
			notTakenBranch := buf.Len()
			buf.Emit32(arm64CBZ(6, 0))
			arm64MovImm32(&buf, 6, uint32(startPC+instr.pcOffset+uint16(instr.length)))
			arm64EmitPushValue(&buf, 2, 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovImm32(&buf, 1, uint32(instr.operand))
			arm64MovImm32(&buf, 9, retiredCycles+prefixCycleSurcharge+17)
			doneBranch := buf.Len()
			buf.Emit32(arm64B(0))
			notTaken := buf.Len()
			arm64MovImm32(&buf, 1, uint32(startPC+instr.pcOffset+uint16(instr.length)))
			arm64MovImm32(&buf, 9, retiredCycles+prefixCycleSurcharge+10)
			done := buf.Len()
			buf.PatchUint32(notTakenBranch, arm64CBZ(6, int32(notTaken-notTakenBranch)))
			buf.PatchUint32(doneBranch, arm64B(int32(done-doneBranch)))
			dynamicPC, dynamicCycles = true, true
		case instr.opcode&0xC7 == 0xC0: // RET cc
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal RET cc")
			}
			arm64EmitCondition(&buf, 2, (instr.opcode>>3)&7, 6)
			notTakenBranch := buf.Len()
			buf.Emit32(arm64CBZ(6, 0))
			arm64EmitPopValue(&buf, 2, 1, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovImm32(&buf, 9, retiredCycles+prefixCycleSurcharge+11)
			doneBranch := buf.Len()
			buf.Emit32(arm64B(0))
			notTaken := buf.Len()
			arm64MovImm32(&buf, 1, uint32(startPC+instr.pcOffset+uint16(instr.length)))
			arm64MovImm32(&buf, 9, retiredCycles+prefixCycleSurcharge+5)
			done := buf.Len()
			buf.PatchUint32(notTakenBranch, arm64CBZ(6, int32(notTaken-notTakenBranch)))
			buf.PatchUint32(doneBranch, arm64B(int32(done-doneBranch)))
			dynamicPC, dynamicCycles = true, true
		case instr.opcode == 0xCD: // CALL nn
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal CALL")
			}
			arm64MovImm32(&buf, 6, uint32(startPC+instr.pcOffset+uint16(instr.length)))
			arm64EmitPushValue(&buf, 2, 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovImm32(&buf, 1, uint32(instr.operand))
			dynamicPC = true
		case instr.opcode == 0xC9: // RET
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal RET")
			}
			arm64EmitPopValue(&buf, 2, 1, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			dynamicPC = true
		case instr.opcode&0xC7 == 0xC7: // RST p
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal RST")
			}
			arm64MovImm32(&buf, 6, uint32(startPC+instr.pcOffset+uint16(instr.length)))
			arm64EmitPushValue(&buf, 2, 6, uint16(startPC+instr.pcOffset), uint32(i), retiredCycles, retiredR)
			arm64MovImm32(&buf, 1, uint32(instr.opcode&0x38))
			dynamicPC = true
		case instr.opcode == 0xC3:
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal JP")
			}
			nextPC = instr.operand
		case instr.opcode == 0x18:
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal JR")
			}
			instrPC := startPC + instr.pcOffset
			nextPC = instrPC + uint16(instr.length) + uint16(int16(int8(instr.operand)))
		case instr.opcode == 0xE9: // JP (HL)
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("Z80 JIT ARM64: non-terminal JP (HL)")
			}
			arm64LdrbImm(&buf, 1, 2, int(cpuZ80OffH))
			buf.Emit32(arm64LSL_W_imm(1, 1, 8))
			arm64LdrbImm(&buf, 3, 2, int(cpuZ80OffL))
			arm64OrrReg(&buf, 1, 1, 3)
			arm64StrhImm(&buf, 1, 2, int(cpuZ80OffWZ))
			dynamicPC = true
		}
		retiredCycles += uint32(instr.cycles)
		retiredR += uint32(instr.rIncrements)
	}
	//
	// Publish a static or dynamic return PC.
	if !dynamicPC {
		arm64MovImm32(&buf, 1, uint32(nextPC))
	}
	// STR W1, [X0, #jzCtxOffRetPC]
	arm64StrImm(&buf, 1, 0, jzCtxOffRetPC, false)

	// MOV W1, #instrCount
	arm64MovImm32(&buf, 1, uint32(len(instrs)))
	// STR W1, [X0, #jzCtxOffRetCount]
	arm64StrImm(&buf, 1, 0, jzCtxOffRetCount, false)

	// MOV X1, #totalCycles
	if !dynamicCycles {
		arm64MovImm32(&buf, 9, totalCycles)
	}
	// STR X1, [X0, #jzCtxOffRetCycles]
	arm64StrImm(&buf, 9, 0, jzCtxOffRetCycles, true)

	// Publish the architectural refresh-register increments. The common
	// dispatcher owns R and consumes this field for every native backend.
	arm64MovImm32(&buf, 1, uint32(totalR))
	arm64StrImm(&buf, 1, 0, jzCtxOffChainRIncrements, false)

	// RET
	buf.Emit32(0xD65F03C0)

	code := buf.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, fmt.Errorf("Z80 JIT ARM64 emit: %w", err)
	}
	flushICache(addr, uintptr(len(code)))

	block := &JITBlock{
		startPC:     uint64(startPC),
		endPC:       uint64(endPC),
		instrCount:  len(instrs),
		execAddr:    addr,
		execSize:    len(code),
		rIncrements: totalR,
	}

	return block, nil
}

func z80ARM64CBDirect(opcode byte) bool {
	return opcode>>6 <= 1 || z80CBSetResHLOpcode(opcode) ||
		z80CBRegisterOpcode(opcode) || z80CBRLCRegisterOpcode(opcode) ||
		z80CBRRCRegisterOpcode(opcode) || z80CBRLRegisterOpcode(opcode) ||
		z80CBRRRegisterOpcode(opcode) || z80CBSLARegisterOpcode(opcode) ||
		z80CBSRLRegisterOpcode(opcode) || z80CBSRARegisterOpcode(opcode) ||
		z80CBSLLRegisterOpcode(opcode)
}

func z80ARM64DirectInstruction(instr JITZ80Instr) bool {
	switch instr.prefix {
	case z80JITPrefixNone:
		return z80ARM64CanEmit(instr.opcode)
	case z80JITPrefixCB:
		return z80ARM64CBDirect(instr.opcode)
	case z80JITPrefixDD, z80JITPrefixFD:
		if instr.opcode == 0xCB {
			return true
		}
		if !z80DDFDExplicitOpcode(instr.opcode) {
			base := instr
			base.prefix = z80JITPrefixNone
			return z80ARM64CanEmit(base.opcode)
		}
		return true
	case z80JITPrefixED:
		return !z80JITNeedsFallback(&instr)
	default:
		return false
	}
}

func z80ARM64CanEmit(opcode byte) bool {
	if opcode == 0x00 {
		return true
	}
	if opcode == 0x47 { // ED LD I,A (prefix admission is checked by caller)
		return true
	}
	if opcode == 0x57 { // ED LD A,I
		return true
	}
	if z80EDNEGOpcode(opcode) { // ED NEG aliases
		return true
	}
	if z80EDInterruptMode(opcode) >= 0 { // ED IM aliases
		return true
	}
	if opcode == 0x0A || opcode == 0x1A {
		return true
	}
	if opcode == 0x02 || opcode == 0x12 {
		return true
	}
	if opcode == 0x3A {
		return true
	}
	if opcode == 0x2A || opcode == 0x22 {
		return true
	}
	if opcode == 0x32 {
		return true
	}
	if opcode&0xC7 == 0x06 {
		return true
	}
	if opcode == 0xE9 {
		return true
	}
	if opcode&0xC7 == 0x04 || opcode&0xC7 == 0x05 {
		return true
	}
	if opcode&0xCF == 0x09 {
		return true
	}
	if opcode == 0x07 || opcode == 0x0F || opcode == 0x17 || opcode == 0x1F {
		return true
	}
	if opcode >= 0xA0 && opcode <= 0xB7 {
		return true
	}
	if opcode >= 0x80 && opcode <= 0x87 {
		return true // ADD A,(HL) uses the guarded direct-memory emitter.
	}
	if opcode == 0xC6 {
		return true
	}
	if opcode >= 0x88 && opcode <= 0x8F {
		return true
	}
	if opcode == 0xCE {
		return true
	}
	if opcode >= 0x90 && opcode <= 0x97 {
		return true
	}
	if opcode == 0xD6 {
		return true
	}
	if opcode >= 0x98 && opcode <= 0x9F {
		return true
	}
	if opcode == 0xDE {
		return true
	}
	if opcode >= 0xB8 && opcode <= 0xBF {
		return true
	}
	if opcode == 0xFE {
		return true
	}
	if opcode == 0xE6 || opcode == 0xEE || opcode == 0xF6 {
		return true
	}
	if opcode&0xCF == 0x01 || opcode&0xCF == 0x03 || opcode&0xCF == 0x0B || opcode&0xCF == 0xC5 || opcode&0xCF == 0xC1 || opcode == 0xF9 || opcode == 0x08 || opcode == 0xD9 || opcode == 0xEB || opcode == 0xE3 || opcode == 0x37 || opcode == 0x3F || opcode == 0x2F || opcode == 0x27 || opcode == 0xF3 || opcode == 0xFB {
		return true
	}
	if opcode >= 0x40 && opcode <= 0x7F && opcode != 0x76 {
		return true
	}
	return opcode == 0xC3 || opcode == 0x18 || opcode == 0x10 || opcode == 0xCD || opcode == 0xC9 ||
		opcode == 0x20 || opcode == 0x28 || opcode == 0x30 || opcode == 0x38 ||
		opcode&0xC7 == 0xC2 || opcode&0xC7 == 0xC4 || opcode&0xC7 == 0xC0 || opcode&0xC7 == 0xC7
}

// arm64EmitGuardedLoadHL loads (HL) only when its page is direct. Its bail return
// retires the completed prefix and leaves this instruction for the frozen
// canonical helper, before any non-direct memory is observed.
func arm64EmitGuardedLoadHL(buf *CodeBuffer, instrPC uint16, retired, priorCycles, priorR uint32) {
	arm64EmitGuardedLoadPair(buf, instrPC, retired, priorCycles, priorR, cpuZ80OffH, cpuZ80OffL)
}

// arm64EmitGuardedLoadPair leaves the admitted byte in W3.
func arm64EmitGuardedLoadPair(buf *CodeBuffer, instrPC uint16, retired, priorCycles, priorR uint32, high, low uintptr) {
	// W3 = selected pair.
	arm64LdrbImm(buf, 3, 2, int(high))
	buf.Emit32(arm64LSL_W_imm(3, 3, 8))
	arm64LdrbImm(buf, 4, 2, int(low))
	arm64OrrReg(buf, 3, 3, 4)
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
}

// arm64EmitGuardedLoadAddress admits the address in W3 and overwrites W3
// with the byte only for a direct page.
func arm64EmitGuardedLoadAddress(buf *CodeBuffer, instrPC uint16, retired, priorCycles, priorR uint32) {
	// W5 = directPageBitmap[HL>>8].
	arm64LdrImm64(buf, 4, 0, jzCtxOffDirectPageBitmapPtr)
	arm64LsrImm(buf, 5, 3, 8)
	buf.Emit32(arm64LDRB_reg(5, 4, 5))
	guardOff := buf.Len()
	buf.Emit32(arm64CBNZ(5, 0))
	// Direct page: W3 becomes the source operand for arm64EmitAddReg.
	arm64LdrImm64(buf, 4, 0, jzCtxOffMemPtr)
	buf.Emit32(arm64LDRB_reg(3, 4, 3))
	skipBailOff := buf.Len()
	buf.Emit32(arm64B(0))

	bailOff := buf.Len()
	arm64MovImm32(buf, 1, 1)
	arm64StrImm(buf, 1, 0, jzCtxOffNeedBail, false)
	arm64MovImm32(buf, 1, uint32(instrPC))
	arm64StrImm(buf, 1, 0, jzCtxOffRetPC, false)
	arm64MovImm32(buf, 1, retired)
	arm64StrImm(buf, 1, 0, jzCtxOffRetCount, false)
	arm64MovImm32(buf, 1, priorCycles)
	arm64StrImm(buf, 1, 0, jzCtxOffRetCycles, true)
	arm64MovImm32(buf, 1, priorR)
	arm64StrImm(buf, 1, 0, jzCtxOffChainRIncrements, false)
	buf.Emit32(arm64RET())

	afterBail := buf.Len()
	buf.PatchUint32(guardOff, arm64CBNZ(5, int32(bailOff-guardOff)))
	buf.PatchUint32(skipBailOff, arm64B(int32(afterBail-skipBailOff)))
}

// arm64EmitGuardedStoreHL commits W3 to direct RAM. A write to a code page
// returns after the write with NeedInval set, so stale blocks cannot execute.
func arm64EmitGuardedStore(buf *CodeBuffer, instrPC uint16, retired, priorCycles, priorR, length, cycles, rIncrements uint32, addressSet bool) {
	if !addressSet {
		arm64LdrbImm(buf, 4, 2, int(cpuZ80OffH))
		buf.Emit32(arm64LSL_W_imm(4, 4, 8))
		arm64LdrbImm(buf, 5, 2, int(cpuZ80OffL))
		arm64OrrReg(buf, 4, 4, 5) // W4 = HL
	}
	arm64LdrImm64(buf, 5, 0, jzCtxOffDirectPageBitmapPtr)
	arm64LsrImm(buf, 6, 4, 8)
	buf.Emit32(arm64LDRB_reg(6, 5, 6))
	guardOff := buf.Len()
	buf.Emit32(arm64CBNZ(6, 0))
	arm64LdrImm64(buf, 5, 0, jzCtxOffMemPtr)
	buf.Emit32(arm64STRB_reg(3, 5, 4))
	arm64LsrImm(buf, 6, 4, 8)
	arm64LdrImm64(buf, 5, 0, jzCtxOffCodePageBitmapPtr)
	buf.Emit32(arm64LDRB_reg(5, 5, 6))
	selfModOff := buf.Len()
	buf.Emit32(arm64CBNZ(5, 0))
	doneOff := buf.Len()
	buf.Emit32(arm64B(0))

	bailOff := buf.Len()
	arm64EmitBailReturn(buf, instrPC, retired, priorCycles, priorR)
	selfModLabel := buf.Len()
	arm64MovImm32(buf, 1, 1)
	arm64StrImm(buf, 1, 0, jzCtxOffNeedInval, false)
	arm64StrImm(buf, 6, 0, jzCtxOffInvalPage, false)
	arm64MovImm32(buf, 1, uint32(instrPC)+length)
	arm64StrImm(buf, 1, 0, jzCtxOffRetPC, false)
	arm64MovImm32(buf, 1, retired+1)
	arm64StrImm(buf, 1, 0, jzCtxOffRetCount, false)
	arm64MovImm32(buf, 1, priorCycles+cycles)
	arm64StrImm(buf, 1, 0, jzCtxOffRetCycles, true)
	arm64MovImm32(buf, 1, priorR+rIncrements)
	arm64StrImm(buf, 1, 0, jzCtxOffChainRIncrements, false)
	buf.Emit32(arm64RET())
	afterExits := buf.Len()
	buf.PatchUint32(guardOff, arm64CBNZ(6, int32(bailOff-guardOff)))
	buf.PatchUint32(selfModOff, arm64CBNZ(5, int32(selfModLabel-selfModOff)))
	buf.PatchUint32(doneOff, arm64B(int32(afterExits-doneOff)))
}

func arm64EmitBailReturn(buf *CodeBuffer, instrPC uint16, retired, priorCycles, priorR uint32) {
	arm64MovImm32(buf, 1, 1)
	arm64StrImm(buf, 1, 0, jzCtxOffNeedBail, false)
	arm64MovImm32(buf, 1, uint32(instrPC))
	arm64StrImm(buf, 1, 0, jzCtxOffRetPC, false)
	arm64MovImm32(buf, 1, retired)
	arm64StrImm(buf, 1, 0, jzCtxOffRetCount, false)
	arm64MovImm32(buf, 1, priorCycles)
	arm64StrImm(buf, 1, 0, jzCtxOffRetCycles, true)
	arm64MovImm32(buf, 1, priorR)
	arm64StrImm(buf, 1, 0, jzCtxOffChainRIncrements, false)
	buf.Emit32(arm64RET())
}

func arm64EmitBailIfCodeAddress(buf *CodeBuffer, address int, instrPC uint16, retired, priorCycles, priorR uint32) {
	arm64LdrImm64(buf, 4, 0, jzCtxOffCodePageBitmapPtr)
	arm64LsrImm(buf, 5, address, 8)
	buf.Emit32(arm64LDRB_reg(5, 4, 5))
	directOff := buf.Len()
	buf.Emit32(arm64CBZ(5, 0))
	arm64EmitBailReturn(buf, instrPC, retired, priorCycles, priorR)
	afterBail := buf.Len()
	buf.PatchUint32(directOff, arm64CBZ(5, int32(afterBail-directOff)))
}

func z80ARM64Pair8Offsets(pair byte) (high, low uintptr) {
	switch pair {
	case 0:
		return cpuZ80OffB, cpuZ80OffC
	case 1:
		return cpuZ80OffD, cpuZ80OffE
	case 2:
		return cpuZ80OffH, cpuZ80OffL
	default:
		panic("ARM64 Z80 invalid register pair")
	}
}

func z80ARM64Reg8Offset(reg byte) uintptr {
	switch reg {
	case 0:
		return cpuZ80OffB
	case 1:
		return cpuZ80OffC
	case 2:
		return cpuZ80OffD
	case 3:
		return cpuZ80OffE
	case 4:
		return cpuZ80OffH
	case 5:
		return cpuZ80OffL
	case 7:
		return cpuZ80OffA
	default:
		panic("ARM64 Z80 invalid 8-bit register")
	}
}

func z80ARM64IndexReg8Offset(prefix, reg byte) uintptr {
	if reg != 4 && reg != 5 {
		return z80ARM64Reg8Offset(reg)
	}
	indexOffset := cpuZ80OffIX
	if prefix == z80JITPrefixFD {
		indexOffset = cpuZ80OffIY
	}
	if reg == 4 {
		return indexOffset + 1
	}
	return indexOffset
}

// arm64MovImm32 emits MOV Wd, #imm (using MOVZ + optional MOVK for values > 16 bits)
func arm64MovImm32(buf *CodeBuffer, rd int, imm uint32) {
	// MOVZ Wd, #(imm & 0xFFFF)
	buf.Emit32(0x52800000 | uint32(rd) | ((imm & 0xFFFF) << 5))
	if imm > 0xFFFF {
		// MOVK Wd, #(imm >> 16), LSL #16
		buf.Emit32(0x72A00000 | uint32(rd) | (((imm >> 16) & 0xFFFF) << 5))
	}
}

// arm64StrImm emits STR Wt/Xt, [Xn, #imm] (unsigned offset)
func arm64StrImm(buf *CodeBuffer, rt, rn, offset int, is64 bool) {
	if is64 {
		// STR Xt, [Xn, #offset] — 64-bit store, offset must be 8-byte aligned
		scaledOff := uint32(offset / 8)
		buf.Emit32(0xF9000000 | (scaledOff << 10) | (uint32(rn) << 5) | uint32(rt))
	} else {
		// STR Wt, [Xn, #offset] — 32-bit store, offset must be 4-byte aligned
		scaledOff := uint32(offset / 4)
		buf.Emit32(0xB9000000 | (scaledOff << 10) | (uint32(rn) << 5) | uint32(rt))
	}
}

func arm64LdrImm64(buf *CodeBuffer, rt, rn, offset int) {
	scaledOff := uint32(offset / 8)
	buf.Emit32(0xF9400000 | (scaledOff << 10) | (uint32(rn) << 5) | uint32(rt))
}

func arm64StrbImm(buf *CodeBuffer, rt, rn, offset int) {
	buf.Emit32(0x39000000 | (uint32(offset) << 10) | (uint32(rn) << 5) | uint32(rt))
}

func arm64LdrbImm(buf *CodeBuffer, rt, rn, offset int) {
	buf.Emit32(0x39400000 | (uint32(offset) << 10) | (uint32(rn) << 5) | uint32(rt))
}

func arm64StrhImm(buf *CodeBuffer, rt, rn, offset int) {
	buf.Emit32(0x79000000 | (uint32(offset/2) << 10) | (uint32(rn) << 5) | uint32(rt))
}

func arm64LdrhImm(buf *CodeBuffer, rt, rn, offset int) {
	buf.Emit32(0x79400000 | (uint32(offset/2) << 10) | (uint32(rn) << 5) | uint32(rt))
}

func arm64Rev16W(buf *CodeBuffer, rd, rn int) {
	buf.Emit32(0x5AC00400 | uint32(rn)<<5 | uint32(rd))
}

func arm64AddSubImm(buf *CodeBuffer, rd, rn int, imm uint16, sub bool) {
	op := uint32(0x11000000)
	if sub {
		op = 0x51000000
	}
	buf.Emit32(op | uint32(imm)<<10 | uint32(rn)<<5 | uint32(rd))
}

func arm64SwapByte(buf *CodeBuffer, base int, first, second uintptr) {
	arm64LdrbImm(buf, 1, base, int(first))
	arm64LdrbImm(buf, 3, base, int(second))
	arm64StrbImm(buf, 3, base, int(first))
	arm64StrbImm(buf, 1, base, int(second))
}

func arm64AndReg(buf *CodeBuffer, rd, rn, rm int) {
	buf.Emit32(0x0A000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd))
}

func arm64OrrReg(buf *CodeBuffer, rd, rn, rm int) {
	buf.Emit32(0x2A000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd))
}

func arm64EorReg(buf *CodeBuffer, rd, rn, rm int) {
	buf.Emit32(0x4A000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd))
}

func arm64AddReg(buf *CodeBuffer, rd, rn, rm int) {
	buf.Emit32(0x0B000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd))
}

func arm64SubReg(buf *CodeBuffer, rd, rn, rm int) {
	buf.Emit32(0x4B000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd))
}

func arm64CMP_W_reg(buf *CodeBuffer, rn, rm int) {
	buf.Emit32(0x6B00001F | uint32(rm)<<16 | uint32(rn)<<5)
}

func arm64LsrImm(buf *CodeBuffer, rd, rn, shift int) {
	buf.Emit32(0x53007C00 | uint32(shift&31)<<16 | uint32(rn)<<5 | uint32(rd))
}

func arm64MovRegW(buf *CodeBuffer, rd, rn int) {
	buf.Emit32(0x2A0003E0 | uint32(rn)<<16 | uint32(rd)) // ORR Wd, WZR, Wn
}

func arm64Madd(buf *CodeBuffer, rd, rn, rm, ra int) {
	buf.Emit32(0x1B000000 | uint32(rm)<<16 | uint32(ra)<<10 | uint32(rn)<<5 | uint32(rd))
}

func arm64EmitSCF(buf *CodeBuffer, base int) {
	arm64LdrbImm(buf, 1, base, int(cpuZ80OffF))
	arm64LdrbImm(buf, 3, base, int(cpuZ80OffA))
	arm64MovImm32(buf, 4, z80FlagS|z80FlagZ|z80FlagPV)
	arm64AndReg(buf, 1, 1, 4)
	arm64MovImm32(buf, 4, z80FlagX|z80FlagY)
	arm64AndReg(buf, 3, 3, 4)
	arm64OrrReg(buf, 1, 1, 3)
	arm64MovImm32(buf, 4, z80FlagC)
	arm64OrrReg(buf, 1, 1, 4)
	arm64StrbImm(buf, 1, base, int(cpuZ80OffF))
}

func arm64EmitCondition(buf *CodeBuffer, base int, condition, dst byte) {
	flag := byte(z80FlagZ)
	wantSet := condition&1 != 0
	switch condition >> 1 {
	case 1:
		flag = z80FlagC
	case 2:
		flag = z80FlagPV
	case 3:
		flag = z80FlagS
	}
	arm64LdrbImm(buf, int(dst), base, int(cpuZ80OffF))
	arm64MovImm32(buf, 8, uint32(flag))
	arm64AndReg(buf, int(dst), int(dst), 8)
	buf.Emit32(arm64CMP_W_imm(dst, 0))
	cond := byte(arm64CondEQ)
	if wantSet {
		cond = arm64CondNE
	}
	buf.Emit32(arm64CSET_W(dst, cond))
}

func arm64EmitCCF(buf *CodeBuffer, base int) {
	arm64LdrbImm(buf, 1, base, int(cpuZ80OffF))
	arm64LdrbImm(buf, 3, base, int(cpuZ80OffA))
	arm64MovImm32(buf, 4, z80FlagC)
	arm64AndReg(buf, 4, 1, 4) // old carry
	arm64MovImm32(buf, 5, z80FlagS|z80FlagZ|z80FlagPV)
	arm64AndReg(buf, 1, 1, 5)
	arm64MovImm32(buf, 5, z80FlagX|z80FlagY)
	arm64AndReg(buf, 3, 3, 5)
	arm64OrrReg(buf, 1, 1, 3)
	arm64MovImm32(buf, 5, 15)
	arm64MovImm32(buf, 6, 1)
	arm64Madd(buf, 4, 4, 5, 6) // oldC*15+1: C when clear, H when set
	arm64OrrReg(buf, 1, 1, 4)
	arm64StrbImm(buf, 1, base, int(cpuZ80OffF))
}

func arm64EmitCPL(buf *CodeBuffer, base int) {
	arm64LdrbImm(buf, 1, base, int(cpuZ80OffA))
	arm64MovImm32(buf, 3, 0xFF)
	arm64EorReg(buf, 1, 1, 3)
	arm64StrbImm(buf, 1, base, int(cpuZ80OffA))
	arm64LdrbImm(buf, 3, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 4, z80FlagS|z80FlagZ|z80FlagPV|z80FlagC)
	arm64AndReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, z80FlagH|z80FlagN)
	arm64OrrReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, z80FlagX|z80FlagY)
	arm64AndReg(buf, 1, 1, 4)
	arm64OrrReg(buf, 3, 3, 1)
	arm64StrbImm(buf, 3, base, int(cpuZ80OffF))
}

func arm64LoadZ80Pair16(buf *CodeBuffer, base int, pair byte, dst, scratch int) {
	if pair == 3 {
		arm64LdrhImm(buf, dst, base, int(cpuZ80OffSP))
		return
	}
	high, low := z80ARM64Pair8Offsets(pair)
	arm64LdrbImm(buf, dst, base, int(high))
	buf.Emit32(arm64LSL_W_imm(byte(dst), byte(dst), 8))
	arm64LdrbImm(buf, scratch, base, int(low))
	arm64OrrReg(buf, dst, dst, scratch)
}

func arm64StoreZ80Pair16(buf *CodeBuffer, base int, pair byte, value, scratch int) {
	if pair == 3 {
		arm64StrhImm(buf, value, base, int(cpuZ80OffSP))
		return
	}
	high, low := z80ARM64Pair8Offsets(pair)
	arm64StrbImm(buf, value, base, int(low))
	arm64LsrImm(buf, scratch, value, 8)
	arm64StrbImm(buf, scratch, base, int(high))
}

func arm64EmitIndexAddress(buf *CodeBuffer, base int, indexOffset uintptr, displacement int8, dst int) {
	arm64LdrhImm(buf, dst, base, int(indexOffset))
	if displacement < 0 {
		arm64AddSubImm(buf, dst, dst, uint16(-int16(displacement)), true)
	} else {
		arm64AddSubImm(buf, dst, dst, uint16(displacement), false)
	}
	arm64MovImm32(buf, 8, 0xFFFF)
	arm64AndReg(buf, dst, dst, 8)
}

func arm64EmitLoadPairAbsolute(buf *CodeBuffer, base int, address uint16, dst int, instrPC uint16, retired, priorCycles, priorR uint32) {
	arm64MovImm32(buf, 3, uint32(address))
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
	arm64MovRegW(buf, 7, 3)
	arm64MovImm32(buf, 3, uint32(uint16(address+1)))
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
	buf.Emit32(arm64LSL_W_imm(3, 3, 8))
	arm64OrrReg(buf, dst, 3, 7)
	arm64MovImm32(buf, 8, uint32(uint16(address+1)))
	arm64StrhImm(buf, 8, base, int(cpuZ80OffWZ))
}

func arm64EmitStorePairAbsolute(buf *CodeBuffer, base int, address, value uint16, instrPC uint16, retired, priorCycles, priorR uint32) {
	arm64MovImm32(buf, 6, uint32(value))
	arm64EmitStorePairAbsoluteReg(buf, base, address, 6, instrPC, retired, priorCycles, priorR)
}

func arm64EmitStorePairAbsoluteReg(buf *CodeBuffer, base int, address uint16, value int, instrPC uint16, retired, priorCycles, priorR uint32) {
	arm64MovImm32(buf, 9, uint32(address))
	arm64MovRegW(buf, 3, 9)
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
	arm64AddSubImm(buf, 3, 9, 1, false)
	arm64MovImm32(buf, 8, 0xFFFF)
	arm64AndReg(buf, 3, 3, 8)
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
	arm64AddSubImm(buf, 8, 9, 1, false)
	arm64MovImm32(buf, 4, 0xFFFF)
	arm64AndReg(buf, 8, 8, 4)
	arm64EmitBailIfCodeAddress(buf, 9, instrPC, retired, priorCycles, priorR)
	arm64EmitBailIfCodeAddress(buf, 8, instrPC, retired, priorCycles, priorR)
	arm64LdrImm64(buf, 5, 0, jzCtxOffMemPtr)
	arm64MovImm32(buf, 4, 0xFF)
	arm64AndReg(buf, 7, value, 4)
	buf.Emit32(arm64STRB_reg(7, 5, 9))
	arm64LsrImm(buf, 7, value, 8)
	buf.Emit32(arm64STRB_reg(7, 5, 8))
	arm64StrhImm(buf, 8, base, int(cpuZ80OffWZ))
}

func arm64EmitPushValue(buf *CodeBuffer, base, value int, instrPC uint16, retired, priorCycles, priorR uint32) {
	arm64LdrhImm(buf, 9, base, int(cpuZ80OffSP))
	arm64AddSubImm(buf, 8, 9, 1, true)
	arm64MovImm32(buf, 7, 0xFFFF)
	arm64AndReg(buf, 8, 8, 7)
	arm64MovRegW(buf, 3, 8)
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
	arm64AddSubImm(buf, 7, 9, 2, true)
	arm64MovImm32(buf, 4, 0xFFFF)
	arm64AndReg(buf, 7, 7, 4)
	arm64MovRegW(buf, 3, 7)
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
	arm64EmitBailIfCodeAddress(buf, 8, instrPC, retired, priorCycles, priorR)
	arm64EmitBailIfCodeAddress(buf, 7, instrPC, retired, priorCycles, priorR)
	arm64LdrImm64(buf, 5, 0, jzCtxOffMemPtr)
	arm64LsrImm(buf, 4, value, 8)
	buf.Emit32(arm64STRB_reg(4, 5, 8))
	buf.Emit32(arm64STRB_reg(byte(value), 5, 7))
	arm64StrhImm(buf, 7, base, int(cpuZ80OffSP))
}

func arm64EmitPopValue(buf *CodeBuffer, base, dst int, instrPC uint16, retired, priorCycles, priorR uint32) {
	arm64LdrhImm(buf, 9, base, int(cpuZ80OffSP))
	arm64MovRegW(buf, 3, 9)
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
	arm64MovRegW(buf, 7, 3)
	arm64AddSubImm(buf, 3, 9, 1, false)
	arm64MovImm32(buf, 8, 0xFFFF)
	arm64AndReg(buf, 3, 3, 8)
	arm64EmitGuardedLoadAddress(buf, instrPC, retired, priorCycles, priorR)
	buf.Emit32(arm64LSL_W_imm(3, 3, 8))
	arm64OrrReg(buf, dst, 3, 7)
	arm64AddSubImm(buf, 9, 9, 2, false)
	arm64StrhImm(buf, 9, base, int(cpuZ80OffSP))
}

func arm64EmitAddIndexPair(buf *CodeBuffer, base int, indexOffset uintptr, pair byte) {
	arm64LdrhImm(buf, 1, base, int(indexOffset))
	if pair == 2 {
		arm64MovRegW(buf, 3, 1)
	} else {
		arm64LoadZ80Pair16(buf, base, pair, 3, 8)
	}
	arm64AddReg(buf, 4, 1, 3)
	arm64StrhImm(buf, 4, base, int(indexOffset))
	arm64MovImm32(buf, 6, 0x0FFF)
	arm64AndReg(buf, 1, 1, 6)
	arm64AndReg(buf, 3, 3, 6)
	arm64AddReg(buf, 1, 1, 3)
	arm64LsrImm(buf, 1, 1, 12)
	arm64MovImm32(buf, 6, 1)
	arm64AndReg(buf, 1, 1, 6)
	buf.Emit32(arm64LSL_W_imm(1, 1, 4))
	arm64LsrImm(buf, 6, 4, 16)
	arm64OrrReg(buf, 1, 1, 6)
	arm64LdrbImm(buf, 5, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 6, z80FlagS|z80FlagZ|z80FlagPV)
	arm64AndReg(buf, 5, 5, 6)
	arm64LsrImm(buf, 6, 4, 8)
	arm64MovImm32(buf, 3, z80FlagX|z80FlagY)
	arm64AndReg(buf, 6, 6, 3)
	arm64OrrReg(buf, 5, 5, 6)
	arm64OrrReg(buf, 5, 5, 1)
	arm64StrbImm(buf, 5, base, int(cpuZ80OffF))
}

func arm64EmitAddHLPair(buf *CodeBuffer, base int, pair byte) {
	// W1 old HL, W3 source pair, W4 17-bit sum, W5 flags, W6 scratch.
	arm64LoadZ80Pair16(buf, base, 2, 1, 4)
	arm64LoadZ80Pair16(buf, base, pair, 3, 4)
	arm64AddReg(buf, 4, 1, 3)
	// Result is low 16 bits and lives in H,L as big endian bytes.
	arm64LsrImm(buf, 6, 4, 8)
	arm64StrbImm(buf, 6, base, int(cpuZ80OffH))
	arm64StrbImm(buf, 4, base, int(cpuZ80OffL))
	// H is carry from bit 11; C is carry from bit 15.
	arm64MovImm32(buf, 6, 0x0FFF)
	arm64AndReg(buf, 1, 1, 6)
	arm64AndReg(buf, 3, 3, 6)
	arm64AddReg(buf, 1, 1, 3)
	arm64LsrImm(buf, 1, 1, 12)
	arm64MovImm32(buf, 6, 1)
	arm64AndReg(buf, 1, 1, 6)
	buf.Emit32(arm64LSL_W_imm(1, 1, 4))
	arm64LsrImm(buf, 6, 4, 16)
	arm64OrrReg(buf, 1, 1, 6) // H|C
	// Preserve S, Z and P/V; X/Y follow the high result byte.
	arm64LdrbImm(buf, 5, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 6, z80FlagS|z80FlagZ|z80FlagPV)
	arm64AndReg(buf, 5, 5, 6)
	arm64LsrImm(buf, 6, 4, 8)
	arm64MovImm32(buf, 3, z80FlagX|z80FlagY)
	arm64AndReg(buf, 6, 6, 3)
	arm64OrrReg(buf, 5, 5, 6)
	arm64OrrReg(buf, 5, 5, 1)
	arm64StrbImm(buf, 5, base, int(cpuZ80OffF))
}

func arm64EmitADCSubHLPair(buf *CodeBuffer, base int, pair byte, subtract bool) {
	// W1=old HL, W3=source, W4=carry-in, W5=full result, W6=result16,
	// W7=flags, W8/W9=scratch.
	arm64LoadZ80Pair16(buf, base, 2, 1, 8)
	arm64LoadZ80Pair16(buf, base, pair, 3, 8)
	arm64LdrbImm(buf, 4, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 8, z80FlagC)
	arm64AndReg(buf, 4, 4, 8)
	if subtract {
		arm64SubReg(buf, 5, 1, 3)
		arm64SubReg(buf, 5, 5, 4)
	} else {
		arm64AddReg(buf, 5, 1, 3)
		arm64AddReg(buf, 5, 5, 4)
	}
	arm64MovImm32(buf, 8, 0xFFFF)
	arm64AndReg(buf, 6, 5, 8)
	arm64LsrImm(buf, 8, 6, 8)
	arm64StrbImm(buf, 8, base, int(cpuZ80OffH))
	arm64StrbImm(buf, 6, base, int(cpuZ80OffL))

	arm64MovImm32(buf, 9, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 7, 8, 9)
	buf.Emit32(arm64CMP_W_imm(6, 0))
	buf.Emit32(arm64CSET_W(9, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(9, 9, 6))
	arm64OrrReg(buf, 7, 7, 9)

	if subtract {
		arm64EorReg(buf, 8, 1, 3)
		arm64EorReg(buf, 8, 8, 6)
		arm64MovImm32(buf, 9, 0x1000)
		arm64AndReg(buf, 8, 8, 9)
		arm64LsrImm(buf, 8, 8, 8)
		arm64OrrReg(buf, 7, 7, 8)
		arm64MovImm32(buf, 8, z80FlagN)
		arm64OrrReg(buf, 7, 7, 8)
		arm64AddReg(buf, 8, 3, 4)
		arm64CMP_W_reg(buf, 1, 8)
		buf.Emit32(arm64CSET_W(8, arm64CondLO))
		arm64OrrReg(buf, 7, 7, 8)
		// Overflow: (old xor source) & (old xor result) & $8000.
		arm64EorReg(buf, 8, 1, 3)
		arm64EorReg(buf, 9, 1, 6)
	} else {
		arm64MovImm32(buf, 8, 0x0FFF)
		arm64AndReg(buf, 9, 1, 8)
		arm64AndReg(buf, 8, 3, 8)
		arm64AddReg(buf, 9, 9, 8)
		arm64AddReg(buf, 9, 9, 4)
		arm64LsrImm(buf, 9, 9, 12)
		arm64MovImm32(buf, 8, 1)
		arm64AndReg(buf, 9, 9, 8)
		buf.Emit32(arm64LSL_W_imm(9, 9, 4))
		arm64OrrReg(buf, 7, 7, 9)
		arm64LsrImm(buf, 8, 5, 16)
		arm64MovImm32(buf, 9, 1)
		arm64AndReg(buf, 8, 8, 9)
		arm64OrrReg(buf, 7, 7, 8)
		// Overflow: ~(old xor source) & (old xor result) & $8000.
		arm64EorReg(buf, 8, 1, 3)
		arm64MovImm32(buf, 9, 0xFFFF)
		arm64EorReg(buf, 8, 8, 9)
		arm64EorReg(buf, 9, 1, 6)
	}
	arm64AndReg(buf, 8, 8, 9)
	arm64MovImm32(buf, 9, 0x8000)
	arm64AndReg(buf, 8, 8, 9)
	buf.Emit32(arm64CMP_W_imm(8, 0))
	buf.Emit32(arm64CSET_W(8, arm64CondNE))
	buf.Emit32(arm64LSL_W_imm(8, 8, 2))
	arm64OrrReg(buf, 7, 7, 8)
	arm64StrbImm(buf, 7, base, int(cpuZ80OffF))
}

func arm64EmitRotateA(buf *CodeBuffer, base int, opcode byte) {
	arm64LdrbImm(buf, 1, base, int(cpuZ80OffA))
	arm64LdrbImm(buf, 3, base, int(cpuZ80OffF))
	if opcode == 0x07 || opcode == 0x17 { // left
		arm64LsrImm(buf, 4, 1, 7) // carry
		buf.Emit32(arm64LSL_W_imm(1, 1, 1))
		if opcode == 0x07 {
			arm64OrrReg(buf, 1, 1, 4)
		} else {
			arm64MovImm32(buf, 5, z80FlagC)
			arm64AndReg(buf, 5, 3, 5)
			arm64OrrReg(buf, 1, 1, 5)
		}
	} else { // right
		arm64MovImm32(buf, 4, z80FlagC)
		arm64AndReg(buf, 4, 1, 4) // carry
		arm64LsrImm(buf, 1, 1, 1)
		if opcode == 0x0F {
			buf.Emit32(arm64LSL_W_imm(4, 4, 7))
			arm64OrrReg(buf, 1, 1, 4)
			arm64LsrImm(buf, 4, 4, 7)
		} else {
			arm64MovImm32(buf, 5, z80FlagC)
			arm64AndReg(buf, 5, 3, 5)
			buf.Emit32(arm64LSL_W_imm(5, 5, 7))
			arm64OrrReg(buf, 1, 1, 5)
		}
	}
	arm64StrbImm(buf, 1, base, int(cpuZ80OffA))
	arm64MovImm32(buf, 5, z80FlagS|z80FlagZ|z80FlagPV)
	arm64AndReg(buf, 3, 3, 5)
	arm64MovImm32(buf, 5, z80FlagX|z80FlagY)
	arm64AndReg(buf, 1, 1, 5)
	arm64OrrReg(buf, 3, 3, 1)
	arm64OrrReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(cpuZ80OffF))
}

// arm64EmitAddReg emits ADD A,r. W3 contains the zero-extended operand.
func arm64EmitAddReg(buf *CodeBuffer, base int, carry bool) {
	// W1=A, W3=operand, W4=full sum, W5=flags, W6/W7=scratch.
	arm64LdrbImm(buf, 1, base, int(cpuZ80OffA))
	arm64AddReg(buf, 4, 1, 3)
	if carry {
		arm64LdrbImm(buf, 6, base, int(cpuZ80OffF))
		arm64MovImm32(buf, 7, z80FlagC)
		arm64AndReg(buf, 6, 6, 7)
		arm64AddReg(buf, 4, 4, 6)
	}
	arm64MovImm32(buf, 7, 0xFF)
	arm64AndReg(buf, 7, 4, 7)
	arm64StrbImm(buf, 7, base, int(cpuZ80OffA))
	arm64MovImm32(buf, 6, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 5, 7, 6)
	buf.Emit32(arm64CMP_W_imm(7, 0))
	buf.Emit32(arm64CSET_W(6, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(6, 6, 6))
	arm64OrrReg(buf, 5, 5, 6)
	// H is carry from bit three.
	arm64MovImm32(buf, 6, 0x0F)
	arm64AndReg(buf, 7, 1, 6)
	arm64AndReg(buf, 6, 3, 6)
	arm64AddReg(buf, 6, 7, 6)
	if carry {
		arm64LdrbImm(buf, 7, base, int(cpuZ80OffF))
		arm64MovImm32(buf, 8, z80FlagC)
		arm64AndReg(buf, 7, 7, 8)
		arm64AddReg(buf, 6, 6, 7)
	}
	arm64LsrImm(buf, 6, 6, 4)
	arm64MovImm32(buf, 7, 1)
	arm64AndReg(buf, 6, 6, 7)
	buf.Emit32(arm64LSL_W_imm(6, 6, 4))
	arm64OrrReg(buf, 5, 5, 6)
	// Signed overflow: ~(A xor r) & (A xor result) & $80.
	arm64EorReg(buf, 6, 1, 3)
	arm64MovImm32(buf, 7, 0xFF)
	arm64EorReg(buf, 6, 6, 7)
	arm64LdrbImm(buf, 7, base, int(cpuZ80OffA))
	arm64EorReg(buf, 7, 1, 7)
	arm64AndReg(buf, 6, 6, 7)
	arm64MovImm32(buf, 7, 0x80)
	arm64AndReg(buf, 6, 6, 7)
	buf.Emit32(arm64CMP_W_imm(6, 0))
	buf.Emit32(arm64CSET_W(6, arm64CondNE))
	buf.Emit32(arm64LSL_W_imm(6, 6, 2))
	arm64OrrReg(buf, 5, 5, 6)
	// C is bit eight of the full sum.
	arm64LsrImm(buf, 6, 4, 8)
	arm64MovImm32(buf, 7, 1)
	arm64AndReg(buf, 6, 6, 7)
	arm64OrrReg(buf, 5, 5, 6)
	arm64StrbImm(buf, 5, base, int(cpuZ80OffF))
}

// arm64EmitSubReg emits SUB A,r. W3 contains the zero-extended operand.
func arm64EmitSubReg(buf *CodeBuffer, base int, carry bool) {
	// W1=A, W3=operand, W4=result, W5=flags, W6/W7=scratch.
	arm64LdrbImm(buf, 1, base, int(cpuZ80OffA))
	arm64SubReg(buf, 4, 1, 3)
	if carry {
		arm64LdrbImm(buf, 6, base, int(cpuZ80OffF))
		arm64MovImm32(buf, 7, z80FlagC)
		arm64AndReg(buf, 6, 6, 7)
		arm64SubReg(buf, 4, 4, 6)
	}
	arm64MovImm32(buf, 7, 0xFF)
	arm64AndReg(buf, 4, 4, 7)
	arm64StrbImm(buf, 4, base, int(cpuZ80OffA))
	arm64MovImm32(buf, 6, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 5, 4, 6)
	arm64MovImm32(buf, 6, z80FlagN)
	arm64OrrReg(buf, 5, 5, 6)
	buf.Emit32(arm64CMP_W_imm(4, 0))
	buf.Emit32(arm64CSET_W(6, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(6, 6, 6))
	arm64OrrReg(buf, 5, 5, 6)
	// H is borrow from bit three.
	arm64EorReg(buf, 6, 1, 3)
	arm64EorReg(buf, 6, 6, 4)
	arm64MovImm32(buf, 7, z80FlagH)
	arm64AndReg(buf, 6, 6, 7)
	arm64OrrReg(buf, 5, 5, 6)
	// Signed overflow: (A xor r) & (A xor result) & $80.
	arm64EorReg(buf, 6, 1, 3)
	arm64EorReg(buf, 7, 1, 4)
	arm64AndReg(buf, 6, 6, 7)
	arm64MovImm32(buf, 7, 0x80)
	arm64AndReg(buf, 6, 6, 7)
	buf.Emit32(arm64CMP_W_imm(6, 0))
	buf.Emit32(arm64CSET_W(6, arm64CondNE))
	buf.Emit32(arm64LSL_W_imm(6, 6, 2))
	arm64OrrReg(buf, 5, 5, 6)
	// C is an unsigned borrow.
	if carry {
		arm64LdrbImm(buf, 6, base, int(cpuZ80OffF))
		arm64MovImm32(buf, 7, z80FlagC)
		arm64AndReg(buf, 6, 6, 7)
		arm64AddReg(buf, 3, 3, 6)
	}
	arm64CMP_W_reg(buf, 1, 3)
	buf.Emit32(arm64CSET_W(6, arm64CondLO))
	arm64OrrReg(buf, 5, 5, 6)
	arm64StrbImm(buf, 5, base, int(cpuZ80OffF))
}

// arm64EmitLogicALU emits AND/XOR/OR A,operand. W3 contains a register
// operand unless immediate is true. Logical Z80 flags are S,Z,P/V and X/Y
// from the result; AND additionally sets H.
func arm64EmitLogicALU(buf *CodeBuffer, base int, opcode byte, immediate bool, imm byte) {
	arm64LdrbImm(buf, 1, base, int(cpuZ80OffA))
	if immediate {
		arm64MovImm32(buf, 3, uint32(imm))
	}
	switch {
	case opcode == 0xE6 || opcode >= 0xA0 && opcode <= 0xA7:
		arm64AndReg(buf, 1, 1, 3)
	case opcode == 0xEE || opcode >= 0xA8 && opcode <= 0xAF:
		arm64EorReg(buf, 1, 1, 3)
	default:
		arm64OrrReg(buf, 1, 1, 3)
	}
	arm64StrbImm(buf, 1, base, int(cpuZ80OffA))
	arm64MovImm32(buf, 4, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 4, 1, 4)
	buf.Emit32(arm64CMP_W_imm(1, 0))
	buf.Emit32(arm64CSET_W(5, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(5, 5, 6))
	arm64OrrReg(buf, 4, 4, 5)
	// Fold to an odd-parity bit, then set P/V for even parity.
	arm64MovRegW(buf, 5, 1)
	arm64LsrImm(buf, 6, 5, 4)
	arm64EorReg(buf, 5, 5, 6)
	arm64LsrImm(buf, 6, 5, 2)
	arm64EorReg(buf, 5, 5, 6)
	arm64LsrImm(buf, 6, 5, 1)
	arm64EorReg(buf, 5, 5, 6)
	arm64MovImm32(buf, 6, 1)
	arm64AndReg(buf, 5, 5, 6)
	buf.Emit32(arm64CMP_W_imm(5, 0))
	buf.Emit32(arm64CSET_W(5, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(5, 5, 2))
	arm64OrrReg(buf, 4, 4, 5)
	if opcode == 0xE6 || opcode >= 0xA0 && opcode <= 0xA7 {
		arm64MovImm32(buf, 5, z80FlagH)
		arm64OrrReg(buf, 4, 4, 5)
	}
	arm64StrbImm(buf, 4, base, int(cpuZ80OffF))
}

func arm64EmitIncDecReg(buf *CodeBuffer, base int, offset uintptr, decrement bool) {
	arm64LdrbImm(buf, 1, base, int(offset)) // old value
	arm64EmitIncDecValue(buf, base, decrement)
	arm64StrbImm(buf, 3, base, int(offset))
}

// arm64EmitIncDecValue consumes the old byte in W1 and leaves the result in
// W3 while publishing INC/DEC flags.
func arm64EmitIncDecValue(buf *CodeBuffer, base int, decrement bool) {
	arm64AddSubImm(buf, 3, 1, 1, decrement) // result
	arm64MovImm32(buf, 6, 0xFF)
	arm64AndReg(buf, 3, 3, 6)
	arm64LdrbImm(buf, 4, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 5, z80FlagC)
	arm64AndReg(buf, 4, 4, 5)
	if decrement {
		arm64MovImm32(buf, 5, z80FlagN)
		arm64OrrReg(buf, 4, 4, 5)
	}
	arm64MovImm32(buf, 5, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 5, 3, 5)
	arm64OrrReg(buf, 4, 4, 5)
	buf.Emit32(arm64CMP_W_imm(3, 0))
	buf.Emit32(arm64CSET_W(5, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(5, 5, 6))
	arm64OrrReg(buf, 4, 4, 5)
	arm64MovImm32(buf, 6, 0x0F)
	arm64AndReg(buf, 6, 1, 6)
	if decrement {
		buf.Emit32(arm64CMP_W_imm(6, 0))
		buf.Emit32(arm64CSET_W(5, arm64CondEQ))
	} else {
		buf.Emit32(arm64CMP_W_imm(6, 0x0F))
		buf.Emit32(arm64CSET_W(5, arm64CondEQ))
	}
	buf.Emit32(arm64LSL_W_imm(5, 5, 4))
	arm64OrrReg(buf, 4, 4, 5)
	if decrement {
		buf.Emit32(arm64CMP_W_imm(1, 0x80))
	} else {
		buf.Emit32(arm64CMP_W_imm(1, 0x7F))
	}
	buf.Emit32(arm64CSET_W(5, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(5, 5, 2))
	arm64OrrReg(buf, 4, 4, 5)
	arm64StrbImm(buf, 4, base, int(cpuZ80OffF))
}

// arm64EmitBITFlags publishes BIT flags for the byte in value. It preserves
// carry, sets H, mirrors the zero result into Z and P/V, and takes X/Y from
// the tested byte, matching CPU_Z80 for register and memory forms.
func arm64EmitBITFlags(buf *CodeBuffer, base, value int, bit byte) {
	arm64LdrbImm(buf, 3, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 4, z80FlagC)
	arm64AndReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, z80FlagH)
	arm64OrrReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, z80FlagX|z80FlagY)
	arm64AndReg(buf, 4, value, 4)
	arm64OrrReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, uint32(1<<bit))
	arm64AndReg(buf, 4, value, 4)
	buf.Emit32(arm64CMP_W_imm(4, 0))
	buf.Emit32(arm64CSET_W(5, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(4, 5, 6))
	arm64OrrReg(buf, 3, 3, 4)
	buf.Emit32(arm64LSL_W_imm(4, 5, 2))
	arm64OrrReg(buf, 3, 3, 4)
	if bit == 7 {
		arm64LsrImm(buf, 4, value, 7)
		buf.Emit32(arm64LSL_W_imm(4, 4, 7))
		arm64OrrReg(buf, 3, 3, 4)
	}
	arm64StrbImm(buf, 3, base, int(cpuZ80OffF))
}

func arm64EmitLDAIRFlags(buf *CodeBuffer, base, value int) {
	arm64LdrbImm(buf, 3, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 4, z80FlagC)
	arm64AndReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 4, value, 4)
	arm64OrrReg(buf, 3, 3, 4)
	buf.Emit32(arm64CMP_W_imm(byte(value), 0))
	buf.Emit32(arm64CSET_W(4, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(4, 4, 6))
	arm64OrrReg(buf, 3, 3, 4)
	arm64LdrbImm(buf, 4, base, int(cpuZ80OffIFF2))
	buf.Emit32(arm64CMP_W_imm(4, 0))
	buf.Emit32(arm64CSET_W(4, arm64CondNE))
	buf.Emit32(arm64LSL_W_imm(4, 4, 2))
	arm64OrrReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(cpuZ80OffF))
}

func arm64EmitSZPFlagsPreserveCarry(buf *CodeBuffer, base, value int) {
	arm64LdrbImm(buf, 3, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 4, z80FlagC)
	arm64AndReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 4, value, 4)
	arm64OrrReg(buf, 3, 3, 4)
	buf.Emit32(arm64CMP_W_imm(byte(value), 0))
	buf.Emit32(arm64CSET_W(4, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(4, 4, 6))
	arm64OrrReg(buf, 3, 3, 4)
	arm64MovRegW(buf, 4, value)
	for _, shift := range []int{4, 2, 1} {
		arm64LsrImm(buf, 6, 4, shift)
		arm64EorReg(buf, 4, 4, 6)
	}
	arm64MovImm32(buf, 6, 1)
	arm64AndReg(buf, 4, 4, 6)
	buf.Emit32(arm64CMP_W_imm(4, 0))
	buf.Emit32(arm64CSET_W(4, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(4, 4, 2))
	arm64OrrReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(cpuZ80OffF))
}

// arm64EmitCBValueOperation transforms the old byte in W1 into W3 and emits
// the common rotate/shift flags. group is the CB operation selector 0..7.
func arm64EmitCBValueOperation(buf *CodeBuffer, base int, group byte) {
	switch group {
	case 0: // RLC
		buf.Emit32(arm64LSL_W_imm(3, 1, 1))
		arm64LsrImm(buf, 4, 1, 7)
		arm64OrrReg(buf, 3, 3, 4)
	case 1: // RRC
		arm64LsrImm(buf, 3, 1, 1)
		buf.Emit32(arm64LSL_W_imm(4, 1, 7))
		arm64OrrReg(buf, 3, 3, 4)
	case 2: // RL
		buf.Emit32(arm64LSL_W_imm(3, 1, 1))
		arm64LdrbImm(buf, 4, base, int(cpuZ80OffF))
		arm64MovImm32(buf, 5, z80FlagC)
		arm64AndReg(buf, 4, 4, 5)
		arm64OrrReg(buf, 3, 3, 4)
	case 3: // RR
		arm64LsrImm(buf, 3, 1, 1)
		arm64LdrbImm(buf, 4, base, int(cpuZ80OffF))
		arm64MovImm32(buf, 5, z80FlagC)
		arm64AndReg(buf, 4, 4, 5)
		buf.Emit32(arm64LSL_W_imm(4, 4, 7))
		arm64OrrReg(buf, 3, 3, 4)
	case 4: // SLA
		buf.Emit32(arm64LSL_W_imm(3, 1, 1))
	case 5: // SRA
		arm64LsrImm(buf, 3, 1, 1)
		arm64MovImm32(buf, 4, 0x80)
		arm64AndReg(buf, 4, 1, 4)
		arm64OrrReg(buf, 3, 3, 4)
	case 6: // SLL
		buf.Emit32(arm64LSL_W_imm(3, 1, 1))
		arm64MovImm32(buf, 4, 1)
		arm64OrrReg(buf, 3, 3, 4)
	case 7: // SRL
		arm64LsrImm(buf, 3, 1, 1)
	}
	arm64MovImm32(buf, 4, 0xFF)
	arm64AndReg(buf, 3, 3, 4)
	arm64EmitCBRotateFlags(buf, base, 1, 3, group == 0 || group == 2 || group == 4 || group == 6)
}

func arm64EmitCBRLCReg(buf *CodeBuffer, base int, offset uintptr) {
	arm64LdrbImm(buf, 1, base, int(offset)) // old
	buf.Emit32(arm64LSL_W_imm(3, 1, 1))
	arm64LsrImm(buf, 4, 1, 7)
	arm64OrrReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, 0xFF)
	arm64AndReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(offset))
	arm64MovImm32(buf, 4, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 5, 3, 4)
	buf.Emit32(arm64CMP_W_imm(3, 0))
	buf.Emit32(arm64CSET_W(4, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(4, 4, 6))
	arm64OrrReg(buf, 5, 5, 4)
	arm64MovRegW(buf, 4, 3)
	for _, shift := range []int{4, 2, 1} {
		arm64LsrImm(buf, 6, 4, shift)
		arm64EorReg(buf, 4, 4, 6)
	}
	arm64MovImm32(buf, 6, 1)
	arm64AndReg(buf, 4, 4, 6)
	buf.Emit32(arm64CMP_W_imm(4, 0))
	buf.Emit32(arm64CSET_W(4, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(4, 4, 2))
	arm64OrrReg(buf, 5, 5, 4)
	arm64LsrImm(buf, 4, 1, 7)
	arm64MovImm32(buf, 6, z80FlagC)
	arm64AndReg(buf, 4, 4, 6)
	arm64OrrReg(buf, 5, 5, 4)
	arm64StrbImm(buf, 5, base, int(cpuZ80OffF))
}

func arm64EmitCBRRCReg(buf *CodeBuffer, base int, offset uintptr) {
	arm64LdrbImm(buf, 1, base, int(offset))
	arm64LsrImm(buf, 3, 1, 1)
	buf.Emit32(arm64LSL_W_imm(4, 1, 7))
	arm64OrrReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, 0xFF)
	arm64AndReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(offset))
	arm64EmitCBRotateFlags(buf, base, 1, 3, false)
}

func arm64EmitCBRLReg(buf *CodeBuffer, base int, offset uintptr) {
	arm64LdrbImm(buf, 1, base, int(offset))
	buf.Emit32(arm64LSL_W_imm(3, 1, 1))
	arm64LdrbImm(buf, 4, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 5, z80FlagC)
	arm64AndReg(buf, 4, 4, 5)
	arm64OrrReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, 0xFF)
	arm64AndReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(offset))
	arm64EmitCBRotateFlags(buf, base, 1, 3, true)
}

func arm64EmitCBRRReg(buf *CodeBuffer, base int, offset uintptr) {
	arm64LdrbImm(buf, 1, base, int(offset))
	arm64LsrImm(buf, 3, 1, 1)
	arm64LdrbImm(buf, 4, base, int(cpuZ80OffF))
	arm64MovImm32(buf, 5, z80FlagC)
	arm64AndReg(buf, 4, 4, 5)
	buf.Emit32(arm64LSL_W_imm(4, 4, 7))
	arm64OrrReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(offset))
	arm64EmitCBRotateFlags(buf, base, 1, 3, false)
}

func arm64EmitCBSLAReg(buf *CodeBuffer, base int, offset uintptr) {
	arm64LdrbImm(buf, 1, base, int(offset))
	buf.Emit32(arm64LSL_W_imm(3, 1, 1))
	arm64MovImm32(buf, 4, 0xFF)
	arm64AndReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(offset))
	arm64EmitCBRotateFlags(buf, base, 1, 3, true)
}

func arm64EmitCBSRLReg(buf *CodeBuffer, base int, offset uintptr) {
	arm64LdrbImm(buf, 1, base, int(offset))
	arm64LsrImm(buf, 3, 1, 1)
	arm64StrbImm(buf, 3, base, int(offset))
	arm64EmitCBRotateFlags(buf, base, 1, 3, false)
}

func arm64EmitCBSRAReg(buf *CodeBuffer, base int, offset uintptr) {
	arm64LdrbImm(buf, 1, base, int(offset))
	arm64LsrImm(buf, 3, 1, 1)
	arm64MovImm32(buf, 4, 0x80)
	arm64AndReg(buf, 4, 1, 4)
	arm64OrrReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(offset))
	arm64EmitCBRotateFlags(buf, base, 1, 3, false)
}

func arm64EmitCBSLLReg(buf *CodeBuffer, base int, offset uintptr) {
	arm64LdrbImm(buf, 1, base, int(offset))
	buf.Emit32(arm64LSL_W_imm(3, 1, 1))
	arm64MovImm32(buf, 4, 1)
	arm64OrrReg(buf, 3, 3, 4)
	arm64MovImm32(buf, 4, 0xFF)
	arm64AndReg(buf, 3, 3, 4)
	arm64StrbImm(buf, 3, base, int(offset))
	arm64EmitCBRotateFlags(buf, base, 1, 3, true)
}

func arm64EmitCBRotateFlags(buf *CodeBuffer, base, old, result int, carryHigh bool) {
	arm64MovImm32(buf, 4, z80FlagS|z80FlagX|z80FlagY)
	arm64AndReg(buf, 5, result, 4)
	buf.Emit32(arm64CMP_W_imm(byte(result), 0))
	buf.Emit32(arm64CSET_W(4, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(4, 4, 6))
	arm64OrrReg(buf, 5, 5, 4)
	arm64MovRegW(buf, 4, result)
	for _, shift := range []int{4, 2, 1} {
		arm64LsrImm(buf, 6, 4, shift)
		arm64EorReg(buf, 4, 4, 6)
	}
	arm64MovImm32(buf, 6, 1)
	arm64AndReg(buf, 4, 4, 6)
	buf.Emit32(arm64CMP_W_imm(4, 0))
	buf.Emit32(arm64CSET_W(4, arm64CondEQ))
	buf.Emit32(arm64LSL_W_imm(4, 4, 2))
	arm64OrrReg(buf, 5, 5, 4)
	if carryHigh {
		arm64LsrImm(buf, 4, old, 7)
	} else {
		arm64MovImm32(buf, 6, 1)
		arm64AndReg(buf, 4, old, 6)
	}
	arm64OrrReg(buf, 5, 5, 4)
	arm64StrbImm(buf, 5, base, int(cpuZ80OffF))
}
