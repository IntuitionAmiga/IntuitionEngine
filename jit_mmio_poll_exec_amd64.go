//go:build amd64 && (linux || windows || darwin)

package main

import "encoding/binary"

func ie64InstrFields(mem []byte, pc uint64) (op, rd, size, xbit, rs, rt byte, imm uint32, ok bool) {
	if pc+IE64_INSTR_SIZE > uint64(len(mem)) {
		return 0, 0, 0, 0, 0, 0, 0, false
	}
	instr := mem[pc : pc+IE64_INSTR_SIZE]
	byte1 := instr[1]
	return instr[0], byte1 >> 3, (byte1 >> 1) & 0x03, byte1 & 1, instr[2] >> 3, instr[3] >> 3, binary.LittleEndian.Uint32(instr[4:]), true
}

func (cpu *CPU64) tryFastIE64MMIOPollLoop() (bool, uint32) {
	if cpu == nil || cpu.bus == nil || cpu.mmuEnabled || cpu.PC > uint64(len(cpu.memory)) {
		return false, 0
	}
	pc := cpu.PC
	op0, rd0, size0, _, rs0, _, imm0, ok := ie64InstrFields(cpu.memory, pc)
	if !ok || op0 != OP_LOAD || rd0 == 0 {
		return false, 0
	}
	op1, rd1, size1, xbit1, rs1, _, imm1, ok := ie64InstrFields(cpu.memory, pc+IE64_INSTR_SIZE)
	if !ok || op1 != OP_AND64 || rd1 != rd0 || rs1 != rd0 || size1 != size0 || xbit1 != 1 {
		return false, 0
	}
	op2, _, _, _, rs2, rt2, imm2, ok := ie64InstrFields(cpu.memory, pc+2*IE64_INSTR_SIZE)
	if !ok || (op2 != OP_BEQ && op2 != OP_BNE) || rs2 != rd0 || rt2 != 0 {
		return false, 0
	}
	if uint64(int64(pc+2*IE64_INSTR_SIZE)+int64(int32(imm2))) != pc {
		return false, 0
	}
	addr := uint64(int64(cpu.regs[rs0]) + int64(int32(imm0)))
	if addr > 0xFFFFFFFF || !cpu.bus.IsIOAddress(uint32(addr)) {
		return false, 0
	}
	pattern := IE64PollPattern
	pattern.Test = PollTestBitTest
	pattern.AddressIsMMIOPredicate = func(a uint32) bool {
		return cpu.bus.IsIOAddress(a)
	}
	match := TryFastMMIOPoll([]PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: uint32(addr)},
		{Kind: PollInstrTest, TestShape: PollTestBitTest},
		{Kind: PollInstrBranchBackward},
	}, &pattern)
	if !match.Matched {
		return false, 0
	}
	iterCap := pattern.IterationCap
	if iterCap <= 0 {
		iterCap = DefaultPollIterationCap
	}
	iterations := 0
	// Exit the spin when a device records an external interrupt: it only sets
	// pendingIRQMask now (no longer flips inInterrupt), so without this check a
	// guest polling an MMIO status flag while expecting that interrupt would
	// never yield to the dispatcher to deliver it. Leaving cpu.PC at the loop
	// head lets the dispatcher's top-of-loop poll vector and resume here.
	for cpu.running.Load() && !cpu.inInterrupt.Load() && cpu.pendingIRQMask.Load() == 0 {
		value := maskToSize(cpu.loadMem(addr, size0), size0)
		if cpu.trapped {
			cpu.trapped = false
			return true, uint32(iterations * 3)
		}
		value = maskToSize(value&uint64(imm1), size0)
		cpu.regs[rd0] = value
		iterations++

		branchTaken := value == 0
		if op2 == OP_BNE {
			branchTaken = !branchTaken
		}
		if !branchTaken {
			cpu.PC = pc + 3*IE64_INSTR_SIZE
			return true, uint32(iterations * 3)
		}
		if iterations >= iterCap {
			cpu.PC = pc
			return true, uint32(iterations * 3)
		}
	}
	return true, uint32(iterations * 3)
}

func (cpu *M68KCPU) tryFastM68KMMIOPollLoop() (bool, uint32) {
	if cpu == nil {
		return false, 0
	}
	// Decline matching when an interrupt/exception is already pending. The poll
	// loops bail immediately on a pending flag, so matching here would execute
	// zero iterations, return retired=0, and leave PC at the loop head. The
	// dispatcher's checkPending early-returns unless the instruction count
	// crosses a 256-instruction boundary, so with retired=0 the count never
	// moves, the interrupt is never delivered, and the next dispatch matches
	// this same fast path again — a livelock. Falling through to the normal
	// block path runs real instructions that advance the count and deliver it.
	if cpu.pendingException.Load() != 0 || cpu.pendingInterrupt.Load() != 0 {
		return false, 0
	}
	mb, ok := cpu.bus.(*MachineBus)
	if !ok {
		return false, 0
	}
	mem := cpu.memory
	pc := cpu.PC
	if uint64(pc)+12 > uint64(len(mem)) || pc&1 != 0 {
		return false, 0
	}
	if matched, retired := cpu.tryFastM68KFullBTSTCountdownMMIOPollLoop(mb, mem, pc); matched {
		return true, retired
	}
	if matched, retired := cpu.tryFastM68KBTSTCountdownMMIOPollLoop(mb, mem, pc); matched {
		return true, retired
	}
	moveOp := binary.BigEndian.Uint16(mem[pc:])
	size := M68K_SIZE_BYTE
	base := uint16(0x1039)
	switch moveOp & 0xF1FF {
	case 0x1039:
		size, base = M68K_SIZE_BYTE, 0x1039
	case 0x3039:
		size, base = M68K_SIZE_WORD, 0x3039
	case 0x2039:
		size, base = M68K_SIZE_LONG, 0x2039
	default:
		return false, 0
	}
	reg := (moveOp >> 9) & 0x7
	if moveOp != base|(reg<<9) {
		return false, 0
	}
	addr := binary.BigEndian.Uint32(mem[pc+2:])
	if !mb.IsIOAddress(addr) {
		return false, 0
	}
	testLen := uint32(2)
	testIsBTST := false
	bitNum := uint16(0)
	tstOp := binary.BigEndian.Uint16(mem[pc+6:])
	wantTST := uint16(reg)
	switch size {
	case M68K_SIZE_BYTE:
		wantTST |= 0x4A00
	case M68K_SIZE_WORD:
		wantTST |= 0x4A40
	case M68K_SIZE_LONG:
		wantTST |= 0x4A80
	default:
		return false, 0
	}
	if tstOp == wantTST {
		testLen = 2
	} else if tstOp == 0x0800|uint16(reg) {
		testLen = 4
		testIsBTST = true
		bitNum = binary.BigEndian.Uint16(mem[pc+8:])
	} else {
		return false, 0
	}
	bccPC := pc + 6 + testLen
	bcc := binary.BigEndian.Uint16(mem[bccPC:])
	cond := byte(bcc >> 8)
	if cond != 0x66 && cond != 0x67 {
		return false, 0
	}
	afterBranchPC := bccPC + 2
	if uint32(int32(afterBranchPC)+int32(int8(bcc))) != pc {
		return false, 0
	}
	pattern := M68KPollPattern
	pattern.Load = m68kSizeToPollLoadShape(size)
	pattern.Test = PollTestBitTest
	pattern.AddressIsMMIOPredicate = func(a uint32) bool {
		return mb.IsIOAddress(a)
	}
	match := TryFastMMIOPoll([]PollInstr{
		{Kind: PollInstrLoad, LoadShape: m68kSizeToPollLoadShape(size), LoadAddr: addr},
		{Kind: PollInstrTest, TestShape: PollTestBitTest},
		{Kind: PollInstrBranchBackward},
	}, &pattern)
	if !match.Matched {
		return false, 0
	}
	iterCap := pattern.IterationCap
	if iterCap <= 0 {
		iterCap = DefaultPollIterationCap
	}
	iterations := 0
	for cpu.running.Load() && cpu.pendingException.Load() == 0 && cpu.pendingInterrupt.Load() == 0 {
		var value uint32
		switch size {
		case M68K_SIZE_BYTE:
			value = uint32(cpu.Read8(addr))
		case M68K_SIZE_WORD:
			value = uint32(cpu.Read16(addr))
		case M68K_SIZE_LONG:
			value = cpu.Read32(addr)
		}
		switch size {
		case M68K_SIZE_BYTE:
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFFFF00) | (value & 0xFF)
		case M68K_SIZE_WORD:
			cpu.DataRegs[reg] = (cpu.DataRegs[reg] & 0xFFFF0000) | (value & 0xFFFF)
		case M68K_SIZE_LONG:
			cpu.DataRegs[reg] = value
		}
		if testIsBTST {
			// BTST #imm,Dn is register-direct, so it always operates modulo 32 on
			// the FULL 32-bit data register — and tests the bit AFTER the MOVE has
			// written the low byte/word and preserved the upper bits. Masking the
			// bit number by the load size and testing only the freshly-loaded
			// `value` diverges from the interpreter for any bit number that indexes
			// the preserved upper bits (e.g. BTST #8..31 after MOVE.B/MOVE.W).
			bit := bitNum & 31
			bitSet := cpu.DataRegs[reg]&(uint32(1)<<bit) != 0
			cpu.m68kSetMoveThenBTSTFlags(value, size, !bitSet)
		} else {
			// MOVE then TST: both set N/Z from the value and clear V/C.
			cpu.SetFlagsNZ(value, size)
			cpu.SR &= ^uint16(M68K_SR_V | M68K_SR_C)
		}
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_REG + M68K_CYCLE_BRANCH
		iterations++

		zSet := (cpu.SR & M68K_SR_Z) != 0
		branchTaken := !zSet
		if cond == 0x67 {
			branchTaken = zSet
		}
		if !branchTaken {
			cpu.PC = afterBranchPC
			return true, uint32(iterations * 3)
		}
		if iterations >= iterCap {
			cpu.PC = pc
			return true, uint32(iterations * 3)
		}
	}
	return true, uint32(iterations * 3)
}

// m68kSetMoveThenBTSTFlags replicates the CCR produced by `MOVE <ea>,Dn`
// immediately followed by `BTST #n,Dn`: the MOVE sets N/Z from the moved value
// and clears V/C (X unchanged), then BTST overrides Z only. The fast poll loops
// previously updated only Z, leaving N/V/C stale relative to the interpreter.
func (cpu *M68KCPU) m68kSetMoveThenBTSTFlags(value uint32, size int, zSet bool) {
	cpu.SetFlagsNZ(value, size)
	cpu.SR &= ^uint16(M68K_SR_V | M68K_SR_C)
	if zSet {
		cpu.SR |= M68K_SR_Z
	} else {
		cpu.SR &= ^uint16(M68K_SR_Z)
	}
}

func (cpu *M68KCPU) tryFastM68KFullBTSTCountdownMMIOPollLoop(mb *MachineBus, mem []byte, pc uint32) (bool, uint32) {
	if uint64(pc)+40 > uint64(len(mem)) {
		return false, 0
	}
	move0 := binary.BigEndian.Uint16(mem[pc:])
	if move0&0xF1FF != 0x2039 {
		return false, 0
	}
	reg := (move0 >> 9) & 7
	addr0 := binary.BigEndian.Uint32(mem[pc+2:])
	if !mb.IsIOAddress(addr0) {
		return false, 0
	}
	btst0 := binary.BigEndian.Uint16(mem[pc+6:])
	if btst0 != 0x0800|uint16(reg) {
		return false, 0
	}
	bitNum := binary.BigEndian.Uint16(mem[pc+8:])
	branch0 := binary.BigEndian.Uint16(mem[pc+10:])
	if branch0 != 0x6704 && branch0 != 0x6604 {
		return false, 0
	}
	if binary.BigEndian.Uint16(mem[pc+12:]) != 0x5380 || binary.BigEndian.Uint16(mem[pc+14:]) != 0x66F0 {
		return false, 0
	}
	if binary.BigEndian.Uint16(mem[pc+16:]) != 0x203C {
		return false, 0
	}
	timeout := binary.BigEndian.Uint32(mem[pc+18:])
	move1 := binary.BigEndian.Uint16(mem[pc+22:])
	if move1 != move0 {
		return false, 0
	}
	addr1 := binary.BigEndian.Uint32(mem[pc+24:])
	if addr1 != addr0 {
		return false, 0
	}
	if binary.BigEndian.Uint16(mem[pc+28:]) != btst0 || binary.BigEndian.Uint16(mem[pc+30:]) != bitNum {
		return false, 0
	}
	branch1 := binary.BigEndian.Uint16(mem[pc+32:])
	if branch1 != 0x6604 && branch1 != 0x6704 {
		return false, 0
	}
	if binary.BigEndian.Uint16(mem[pc+34:]) != 0x5380 || binary.BigEndian.Uint16(mem[pc+36:]) != 0x66F0 ||
		binary.BigEndian.Uint16(mem[pc+38:]) != 0x4E75 {
		return false, 0
	}

	bit := bitNum & 31
	statusBitSet := func(value uint32) bool {
		return value&(uint32(1)<<bit) != 0
	}
	branchTaken := func(op uint16, zSet bool) bool {
		if op>>8 == 0x66 {
			return !zSet
		}
		return zSet
	}
	retired := uint32(0)
	iterCap := DefaultPollIterationCap

	phase1Done := false
	for cpu.running.Load() && cpu.pendingException.Load() == 0 && cpu.pendingInterrupt.Load() == 0 {
		value := cpu.Read32(addr0)
		cpu.DataRegs[reg] = value
		zSet := !statusBitSet(value)
		cpu.m68kSetMoveThenBTSTFlags(value, M68K_SIZE_LONG, zSet)
		retired += 2
		if branchTaken(branch0, zSet) {
			phase1Done = true
			break
		}
		old := cpu.DataRegs[0]
		cpu.DataRegs[0] = old - 1
		// SUBQ.L #1,D0 sets the full CCR each iteration; the following BNE leaves
		// it intact, so any loop-top yield (interrupt) or fall-through sees it.
		cpu.setSubqFlags(old, 1, cpu.DataRegs[0], M68K_SIZE_LONG)
		retired += 2
		if cpu.DataRegs[0] == 0 {
			phase1Done = true
			break
		}
		if retired >= uint32(iterCap*4) {
			cpu.PC = pc
			return true, retired
		}
	}
	if !phase1Done {
		// Loop exited because an interrupt/exception became pending (or we are
		// stopping), NOT because phase 1 finished. Leave PC at the phase-1 head
		// so the dispatcher delivers the interrupt and resumes here — do NOT
		// fall through into the phase-2 timeout setup, which would clobber D0
		// and advance PC past work that never ran.
		cpu.PC = pc
		return true, retired
	}

	cpu.DataRegs[0] = timeout
	// MOVE.L #imm,D0 sets N/Z from the moved value and clears V/C (X unchanged).
	// If an interrupt is pending before the phase-2 loop runs its first load, we
	// yield at pc+22 with this CCR, not the prior BTST/SUBQ's.
	cpu.SetFlagsNZ(timeout, M68K_SIZE_LONG)
	cpu.SR &= ^uint16(M68K_SR_V | M68K_SR_C)
	retired++
	for cpu.running.Load() && cpu.pendingException.Load() == 0 && cpu.pendingInterrupt.Load() == 0 {
		value := cpu.Read32(addr0)
		cpu.DataRegs[reg] = value
		zSet := !statusBitSet(value)
		cpu.m68kSetMoveThenBTSTFlags(value, M68K_SIZE_LONG, zSet)
		retired += 2
		if branchTaken(branch1, zSet) {
			cpu.PC = pc + 38
			return true, retired
		}
		old := cpu.DataRegs[0]
		cpu.DataRegs[0] = old - 1
		// SUBQ.L #1,D0 sets the full CCR each iteration; the BNE preserves it, so
		// the fall-through-to-RTS timeout and any loop-top yield see SUBQ flags.
		cpu.setSubqFlags(old, 1, cpu.DataRegs[0], M68K_SIZE_LONG)
		retired += 2
		if cpu.DataRegs[0] == 0 {
			cpu.PC = pc + 38
			return true, retired
		}
		if retired >= uint32(iterCap*8) {
			cpu.PC = pc + 22
			return true, retired
		}
	}
	cpu.PC = pc + 22
	return true, retired
}

func (cpu *M68KCPU) tryFastM68KBTSTCountdownMMIOPollLoop(mb *MachineBus, mem []byte, pc uint32) (bool, uint32) {
	if uint64(pc)+34 > uint64(len(mem)) {
		return false, 0
	}
	btst0 := binary.BigEndian.Uint16(mem[pc:])
	if btst0&0xFFF8 != 0x0800 {
		return false, 0
	}
	reg := btst0 & 7
	bitNum := binary.BigEndian.Uint16(mem[pc+2:])
	branch0 := binary.BigEndian.Uint16(mem[pc+4:])
	if branch0 != 0x6704 && branch0 != 0x6604 {
		return false, 0
	}
	if binary.BigEndian.Uint16(mem[pc+6:]) != 0x5380 || binary.BigEndian.Uint16(mem[pc+8:]) != 0x66F0 {
		return false, 0
	}
	if binary.BigEndian.Uint16(mem[pc+10:]) != 0x203C {
		return false, 0
	}
	timeout := binary.BigEndian.Uint32(mem[pc+12:])
	moveOp := binary.BigEndian.Uint16(mem[pc+16:])
	if moveOp != 0x2039|uint16(reg<<9) {
		return false, 0
	}
	addr := binary.BigEndian.Uint32(mem[pc+18:])
	if !mb.IsIOAddress(addr) {
		return false, 0
	}
	if binary.BigEndian.Uint16(mem[pc+22:]) != btst0 || binary.BigEndian.Uint16(mem[pc+24:]) != bitNum {
		return false, 0
	}
	branch1 := binary.BigEndian.Uint16(mem[pc+26:])
	if branch1 != 0x6604 && branch1 != 0x6704 {
		return false, 0
	}
	if binary.BigEndian.Uint16(mem[pc+28:]) != 0x5380 || binary.BigEndian.Uint16(mem[pc+30:]) != 0x66F0 ||
		binary.BigEndian.Uint16(mem[pc+32:]) != 0x4E75 {
		return false, 0
	}

	bit := bitNum & 31
	statusBitSet := func(value uint32) bool {
		return value&(uint32(1)<<bit) != 0
	}
	branchTaken := func(op uint16, zSet bool) bool {
		if op>>8 == 0x66 {
			return !zSet
		}
		return zSet
	}

	retired := uint32(0)
	firstZ := !statusBitSet(cpu.DataRegs[reg])
	// Phase-1 BTST tests an already-MOVE'd Dn (the MOVE lives at pc-6, outside
	// this match), so N/V/C in SR are already the MOVE's; BTST overrides only Z.
	if firstZ {
		cpu.SR |= M68K_SR_Z
	} else {
		cpu.SR &= ^uint16(M68K_SR_Z)
	}
	if branchTaken(branch0, firstZ) {
		retired += 2
	} else {
		old := cpu.DataRegs[0]
		cpu.DataRegs[0] = old - 1
		// SUBQ.L #1,D0 sets the full CCR; the following BNE leaves it intact.
		cpu.setSubqFlags(old, 1, cpu.DataRegs[0], M68K_SIZE_LONG)
		retired += 3
		if cpu.DataRegs[0] != 0 {
			if pc < 6 {
				cpu.PC = pc
				return true, retired
			}
			cpu.PC = pc - 6
			return true, retired
		}
	}

	cpu.DataRegs[0] = timeout
	// MOVE.L #imm,D0 sets N/Z from the moved value and clears V/C (X unchanged).
	// A pending interrupt before the phase-2 loop's first load yields at pc+16
	// with this CCR, not the prior BTST/SUBQ's.
	cpu.SetFlagsNZ(timeout, M68K_SIZE_LONG)
	cpu.SR &= ^uint16(M68K_SR_V | M68K_SR_C)
	retired++
	iterCap := DefaultPollIterationCap
	for cpu.running.Load() && cpu.pendingException.Load() == 0 && cpu.pendingInterrupt.Load() == 0 {
		value := cpu.Read32(addr)
		cpu.DataRegs[reg] = value
		zSet := !statusBitSet(value)
		cpu.m68kSetMoveThenBTSTFlags(value, M68K_SIZE_LONG, zSet)
		retired += 2
		if branchTaken(branch1, zSet) {
			cpu.PC = pc + 32
			return true, retired
		}
		old := cpu.DataRegs[0]
		cpu.DataRegs[0] = old - 1
		// SUBQ.L #1,D0 sets the full CCR (Z/N/V/C/X) each iteration; the BNE
		// preserves it. Setting it unconditionally keeps the fall-through-to-RTS
		// timeout and any loop-top interrupt yield in parity with the interpreter.
		cpu.setSubqFlags(old, 1, cpu.DataRegs[0], M68K_SIZE_LONG)
		retired += 2
		if cpu.DataRegs[0] == 0 {
			cpu.PC = pc + 32
			return true, retired
		}
		if retired >= uint32(iterCap*4) {
			cpu.PC = pc + 16
			return true, retired
		}
	}
	cpu.PC = pc + 16
	return true, retired
}

func m68kSizeToPollLoadShape(size int) PollLoadShape {
	switch size {
	case M68K_SIZE_BYTE:
		return PollLoad8
	case M68K_SIZE_WORD:
		return PollLoad16
	case M68K_SIZE_LONG:
		return PollLoad32
	default:
		return PollLoad16
	}
}

func poll6502PageIsMMIO(adapter *Bus6502Adapter, addr uint16) bool {
	if adapter == nil || adapter.ioPageBitmap == nil {
		return false
	}
	page := translateIO8Bit_6502(addr) >> 8
	return page < uint32(len(adapter.ioPageBitmap)) && adapter.ioPageBitmap[page]
}

func (cpu *CPU_6502) tryFast6502MMIOPollLoop(adapter *Bus6502Adapter) (bool, uint32) {
	if adapter == nil {
		return false, 0
	}
	pc := cpu.PC
	if int(pc)+7 > len(cpu.fastAdapter.memDirect) {
		return false, 0
	}
	mem := cpu.fastAdapter.memDirect
	if mem[pc] != 0xAD || mem[pc+3] != 0x29 {
		return false, 0
	}
	jcc := mem[pc+5]
	if jcc != 0xF0 && jcc != 0xD0 {
		return false, 0
	}
	if int32(uint32(pc)+7)+int32(int8(mem[pc+6])) != int32(pc) {
		return false, 0
	}
	addr := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
	mask := mem[pc+4]
	pattern := P65PollPattern
	pattern.Test = PollTestBitTest
	pattern.AddressIsMMIOPredicate = func(a uint32) bool {
		return poll6502PageIsMMIO(adapter, uint16(a))
	}
	match := TryFastMMIOPoll([]PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad8, LoadAddr: uint32(addr)},
		{Kind: PollInstrTest, TestShape: PollTestBitTest},
		{Kind: PollInstrBranchBackward},
	}, &pattern)
	if !match.Matched {
		return false, 0
	}
	iterations := 0
	iterCap := pattern.IterationCap
	if iterCap <= 0 {
		iterCap = DefaultPollIterationCap
	}
	for cpu.running.Load() && !cpu.nmiPending.Load() && !(cpu.irqPending.Load() && cpu.SR&INTERRUPT_FLAG == 0) {
		value := adapter.Read(addr)
		andValue := value & mask
		cpu.A = andValue
		cpu.SR = (cpu.SR &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[andValue]
		cpu.Cycles += 8
		iterations++

		branchTaken := andValue == 0
		if jcc == 0xD0 {
			branchTaken = !branchTaken
		}
		if !branchTaken {
			cpu.PC = pc + 7
			return true, uint32(iterations * 3)
		}
		if iterations >= iterCap {
			cpu.PC = pc
			return true, uint32(iterations * 3)
		}
	}
	return true, uint32(iterations * 3)
}

func (cpu *CPU_Z80) tryFastZ80MMIOPollLoop(adapter *Z80BusAdapter) (bool, uint32, uint32) {
	if adapter == nil || adapter.bus == nil {
		return false, 0, 0
	}
	mem := adapter.bus.GetMemory()
	pc := cpu.PC
	if int(pc)+7 > len(mem) {
		return false, 0, 0
	}
	if mem[pc] != 0x3A || mem[pc+3] != 0xE6 {
		return false, 0, 0
	}
	jcc := mem[pc+5]
	if jcc != 0x28 && jcc != 0x20 {
		return false, 0, 0
	}
	if int32(uint32(pc)+7)+int32(int8(mem[pc+6])) != int32(pc) {
		return false, 0, 0
	}
	addr := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
	mask := mem[pc+4]
	pattern := Z80PollPattern
	pattern.Test = PollTestBitTest
	pattern.AddressIsMMIOPredicate = func(a uint32) bool {
		host := translateIO8Bit(uint16(a))
		return adapter.bus.IsIOAddress(host)
	}
	match := TryFastMMIOPoll([]PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad8, LoadAddr: uint32(addr)},
		{Kind: PollInstrTest, TestShape: PollTestBitTest},
		{Kind: PollInstrBranchBackward},
	}, &pattern)
	if !match.Matched {
		return false, 0, 0
	}
	iterations := 0
	iterCap := pattern.IterationCap
	if iterCap <= 0 {
		iterCap = DefaultPollIterationCap
	}
	for cpu.running.Load() && !cpu.nmiPending.Load() && !(cpu.irqLine.Load() && cpu.IFF1) && !cpu.Halted && cpu.iffDelay == 0 {
		value := adapter.Read(addr)
		cpu.A = value & mask
		cpu.F = (cpu.F & z80FlagC) | z80FlagH
		cpu.setSZPFlags(cpu.A)
		iterations++

		branchTaken := cpu.A == 0
		if jcc == 0x20 {
			branchTaken = !branchTaken
		}
		if !branchTaken {
			cpu.PC = pc + 7
			cpu.tick(iterations * 20)
			return true, uint32(iterations * 3), uint32(iterations)
		}
		if iterations >= iterCap {
			cpu.PC = pc
			cpu.tick(iterations * 20)
			return true, uint32(iterations * 3), uint32(iterations)
		}
	}
	cpu.tick(iterations * 20)
	return true, uint32(iterations * 3), uint32(iterations)
}
