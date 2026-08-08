package main

import (
	"fmt"
	"sort"
	"testing"
)

func TestIE32WasmEmitterBuildsImmediateLoadModule(t *testing.T) {
	mod, err := compileIE32WasmBlock([]ie32DecodedInstruction{{PC: PROG_START, Opcode: LDA, AddrMode: ADDR_IMMEDIATE, Operand: 0x1234}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(mod) < 8 || string(mod[:4]) != "\x00asm" {
		t.Fatalf("invalid wasm header: %x", mod)
	}
}

func TestIE32WasmEmitterBuildsImmediateALUModule(t *testing.T) {
	mod, err := compileIE32WasmBlock([]ie32DecodedInstruction{
		{PC: PROG_START, Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 9},
		{PC: PROG_START + INSTRUCTION_SIZE, Opcode: ADD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 3},
		{PC: PROG_START + 2*INSTRUCTION_SIZE, Opcode: NOT, Reg: REG_A},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(mod) < 8 || string(mod[:4]) != "\x00asm" {
		t.Fatalf("invalid wasm header: %x", mod)
	}
}

func TestIE32WasmEmitterBuildsDirectStoreModule(t *testing.T) {
	mod, err := compileIE32WasmBlock([]ie32DecodedInstruction{
		{PC: PROG_START, Opcode: STORE, Reg: REG_A, AddrMode: ADDR_DIRECT, Operand: 0x400},
		{PC: PROG_START + INSTRUCTION_SIZE, Opcode: STB, AddrMode: ADDR_DIRECT, Operand: 0x404},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(mod) < 8 || string(mod[:4]) != "\x00asm" {
		t.Fatalf("invalid wasm header: %x", mod)
	}
}

func TestIE32WasmEmitterBuildsRangeProvenRegisterIndirectReadModule(t *testing.T) {
	mod, err := compileIE32WasmBlock([]ie32DecodedInstruction{{
		PC:                          PROG_START,
		Opcode:                      LDA,
		AddrMode:                    ADDR_REG_IND,
		Operand:                     REG_X,
		rangeProvenRegisterIndirect: true,
		rangeBaseRegister:           REG_X,
	}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(mod) < 8 || string(mod[:4]) != "\x00asm" {
		t.Fatalf("invalid wasm header: %x", mod)
	}
}

func TestIE32WasmEmitterBuildsEveryDirectManifestForm(t *testing.T) {
	forms := make([]ie32OpcodeForm, 0)
	for form, kind := range ie32FormLowering {
		if kind == ie32LoweringDirect && form.AddrMode <= ADDR_DIRECT {
			forms = append(forms, form)
		}
	}
	sort.Slice(forms, func(i, j int) bool {
		if forms[i].Opcode != forms[j].Opcode {
			return forms[i].Opcode < forms[j].Opcode
		}
		return forms[i].AddrMode < forms[j].AddrMode
	})
	for _, form := range forms {
		t.Run(fmt.Sprintf("%s/mode-%d", ie32OpcodeName(form.Opcode), form.AddrMode), func(t *testing.T) {
			cpu := newIE32DirectManifestCPU(form)
			in, ok := decodeIE32Instruction(cpu.memory, PROG_START)
			if !ok {
				t.Fatal("decode test instruction")
			}
			// The emitter receives the safe, first-instruction specialisation
			// produced by runtime admission, rather than a dynamic host callback.
			if in.AddrMode == ADDR_MEM_IND {
				in.AddrMode, in.Operand = ADDR_DIRECT, 0x404
			}
			if in.AddrMode == ADDR_REG_IND {
				in.AddrMode, in.Operand = ADDR_DIRECT, 0x400
			}
			if _, err := compileIE32WasmBlockAtStack([]ie32DecodedInstruction{in}, cpu.SP); err != nil {
				t.Fatalf("direct manifest form rejected by wasm emitter: %v", err)
			}
		})
	}
}
