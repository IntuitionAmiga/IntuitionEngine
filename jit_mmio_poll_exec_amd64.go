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
	for cpu.running.Load() && !cpu.inInterrupt.Load() {
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
	mb, ok := cpu.bus.(*MachineBus)
	if !ok {
		return false, 0
	}
	mem := cpu.memory
	pc := cpu.PC
	if uint64(pc)+10 > uint64(len(mem)) || pc&1 != 0 {
		return false, 0
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
	if tstOp != wantTST {
		return false, 0
	}
	bcc := binary.BigEndian.Uint16(mem[pc+8:])
	cond := byte(bcc >> 8)
	if cond != 0x66 && cond != 0x67 {
		return false, 0
	}
	if uint32(int32(pc+10)+int32(int8(bcc))) != pc {
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
	for cpu.running.Load() && cpu.pendingException.Load() == 0 {
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
		cpu.SetFlagsNZ(value, size)
		cpu.cycleCounter += M68K_CYCLE_MEM_READ + M68K_CYCLE_REG + M68K_CYCLE_BRANCH
		iterations++

		branchTaken := value != 0
		if cond == 0x67 {
			branchTaken = value == 0
		}
		if !branchTaken {
			cpu.PC = pc + 10
			return true, uint32(iterations * 3)
		}
		if iterations >= iterCap {
			cpu.PC = pc
			return true, uint32(iterations * 3)
		}
	}
	return true, uint32(iterations * 3)
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
