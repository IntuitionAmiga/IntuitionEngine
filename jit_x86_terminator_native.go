//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

// x86ResolveTerminatorTarget computes the target PC for a block-terminating
// instruction, if it has a statically known target. Both native x86 JIT
// backends use the same guest relative-branch encoding.
func x86ResolveTerminatorTarget(ji *X86JITInstr, memory []byte, startPC uint32) (uint32, bool) {
	op := byte(ji.opcode)
	nextPC := ji.opcodePC + uint32(ji.length)

	switch op {
	case 0xE8: // CALL rel32
		immPC := ji.opcodePC + uint32(ji.length) - 4
		rel := int32(memory[immPC]) | int32(memory[immPC+1])<<8 | int32(memory[immPC+2])<<16 | int32(memory[immPC+3])<<24
		return uint32(int32(nextPC) + rel), true

	case 0xE9: // JMP rel32
		immPC := ji.opcodePC + uint32(ji.length) - 4
		rel := int32(memory[immPC]) | int32(memory[immPC+1])<<8 | int32(memory[immPC+2])<<16 | int32(memory[immPC+3])<<24
		return uint32(int32(nextPC) + rel), true

	case 0xEB: // JMP rel8
		immPC := ji.opcodePC + uint32(ji.length) - 1
		rel := int32(int8(memory[immPC]))
		return uint32(int32(nextPC) + rel), true

	case 0xC3, 0xC2: // RET target depends on the guest stack
		return 0, false
	}

	return 0, false
}
