package main

// Prefix values are shared by native and js/wasm frontend and helper code.
const (
	z80JITPrefixNone byte = 0x00
	z80JITPrefixCB   byte = 0xCB
	z80JITPrefixDD   byte = 0xDD
	z80JITPrefixFD   byte = 0xFD
	z80JITPrefixED   byte = 0xED
)

// JITZ80Instr is the backend-neutral decoded instruction contract. Native
// emitters add no host-specific state to it, so keeping the type untagged lets
// the canonical opcode manifest compile into both Linux and js/wasm tests.
type JITZ80Instr struct {
	opcode       byte
	prefix       byte
	displacement int8
	operand      uint16
	hasOperand   bool
	length       byte
	pcOffset     uint16
	cycles       byte
	cbSubOp      byte
	rIncrements  byte
}
