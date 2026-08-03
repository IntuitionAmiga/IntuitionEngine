package main

// x86VisibleFlagsMask is the EFLAGS subset that maps safely to host RFLAGS:
// CF, PF, AF, ZF, SF and OF. DF and IF are guest-only control state and stay
// out of this mask and published separately when a backend needs them.
const x86VisibleFlagsMask = uint32(0x0000_08D5)

// x86JITPublishedFlagsMask extends the visible arithmetic subset with the
// guest-only control bits that native helpers may need to observe directly.
const x86JITPublishedFlagsMask = x86VisibleFlagsMask | x86FlagDF | x86FlagIF

// x86FindModRMPC returns the absolute memory address of the ModR/M byte.
func x86FindModRMPC(ji *X86JITInstr, memory []byte) uint32 {
	pc := ji.opcodePC
	memSize := uint32(len(memory))

	for pc < memSize {
		switch memory[pc] {
		case 0x26, 0x2E, 0x36, 0x3E, 0x64, 0x65, 0x66, 0x67, 0xF0, 0xF2, 0xF3:
			pc++
			continue
		}
		break
	}

	if pc >= memSize {
		return ji.opcodePC
	}

	if memory[pc] == 0x0F {
		return pc + 2
	}
	return pc + 1
}

// readLE32 reads a little-endian uint32 from memory at pc.
func readLE32(memory []byte, pc uint32) uint32 {
	return uint32(memory[pc]) | uint32(memory[pc+1])<<8 | uint32(memory[pc+2])<<16 | uint32(memory[pc+3])<<24
}

func x86JITReg8Index(idx byte) (int, uint) {
	idx &= 7
	if idx < 4 {
		return int(idx), 0
	}
	return int(idx - 4), 8
}
