// jit_x86_coverage_manifest.go - checked x86 JIT opcode-family inventory
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

// x86JITCoveragePath describes how a backend executes a decoder-supported
// opcode family.  It is deliberately declarative: the amd64 test consumes the
// samples below and checks that the production admission gate can reach the
// documented direct lowerer or x87 helper exit.
type x86JITCoveragePath string

const (
	x86JITCoverageDirect      x86JITCoveragePath = "direct"
	x86JITCoverageFPUHelper   x86JITCoveragePath = "x87-helper"
	x86JITCoverageFallback    x86JITCoveragePath = "interpreter-fallback"
	x86JITCoverageUnavailable x86JITCoveragePath = "unavailable"
)

type x86JITCoverageRow struct {
	form   string
	sample []byte
	amd64  x86JITCoveragePath
	arm64  x86JITCoveragePath
	wasm   x86JITCoveragePath
	test   string
}

// x86JITCoverageManifest is the backend-neutral form inventory maintained
// beside the decoder. It covers the implemented i386 interpreter families,
// with one representative decoder byte sequence per family. Address-size and
// segment-prefix variants are intentionally listed separately where their
// amd64 path is the immutable x87 helper rather than flat direct lowering.
//
// ARM64 and wasm remain unavailable as whole x86 JIT backends. They must not
// be represented as scalar substitutes or partial helpers.
var x86JITCoverageManifest = []x86JITCoverageRow{
	{"MOV r32,imm32", []byte{0xB8, 1, 0, 0, 0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r8,imm8", []byte{0xB0, 1}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r/m8,r8", []byte{0x88, 0xC3}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r/m32,r32", []byte{0x89, 0xD8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r32,r/m32", []byte{0x8B, 0xD8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"LEA SIB/disp32", []byte{0x8D, 0x84, 0xB3, 0, 1, 0, 0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_GeneralLEAFormsMatchInterpreter"},
	{"byte ALU r/m,r", []byte{0x00, 0xD8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"dword ALU r/m,r", []byte{0x01, 0xD8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"ALU accumulator,imm", []byte{0x05, 1, 0, 0, 0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"Grp1 r/m32,imm8", []byte{0x83, 0xC0, 1}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"INC/DEC register", []byte{0x40}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"XCHG", []byte{0x87, 0xD8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"TEST", []byte{0x85, 0xD8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"Grp2 byte,count one", []byte{0xD0, 0xC0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_Grp2ByteCLMatchesInterpreter"},
	{"Grp2 dword,imm8", []byte{0xC1, 0xE0, 2}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"Grp2 byte,CL", []byte{0xD2, 0xE0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_Grp2ByteCLMatchesInterpreter"},
	{"Grp3", []byte{0xF7, 0xD8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"IMUL immediate", []byte{0x6B, 0xC0, 2}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"PUSH/POP register", []byte{0x50}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"PUSHA/POPA", []byte{0x60}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"PUSHF/POPF", []byte{0x9C}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_PUSHFMatchesInterpreter"},
	{"CLI/STI", []byte{0xFA}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_CLI_STIMatchesInterpreter"},
	{"ignored operand-size prefix", []byte{0x66, 0x90}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_IgnoredOperandSizePrefixesMatchInterpreter"},
	{"segment MOV", []byte{0x8C, 0xD8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"LES/LDS", []byte{0xC4, 0x00}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"ENTER/LEAVE", []byte{0xC8, 0, 0, 0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_ENTERLevelZeroMatchesInterpreter"},
	{"CBW/CWDE and CWD/CDQ", []byte{0x66, 0x98}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_OperandSizeSignExtendMatchesInterpreter"},
	{"near CALL", []byte{0xE8, 0, 0, 0, 0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_Chain_CALL"},
	{"near RET", []byte{0xC3}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_RETImm16MatchesInterpreter"},
	{"near JMP", []byte{0xEB, 0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"Jcc", []byte{0x74, 0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"LOOP", []byte{0xE2, 0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"bit test", []byte{0x0F, 0xA3, 0xC8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"double shift", []byte{0x0F, 0xA4, 0xC8, 1}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOVZX/MOVSX", []byte{0x0F, 0xB6, 0xC0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"SETcc", []byte{0x0F, 0x94, 0xC0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"CMOVcc", []byte{0x0F, 0x44, 0xC0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"BSF/BSR", []byte{0x0F, 0xBC, 0xC0}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"BSWAP", []byte{0x0F, 0xC8}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"REP MOVS", []byte{0xF3, 0xA4}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_REPWordStringsMatchInterpreter"},
	{"REP CMPS/SCAS", []byte{0xF3, 0xA7}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_REPWordComparisonsMatchInterpreter"},
	{"REP LODS", []byte{0xF3, 0xAC}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_REPLODSMatchesInterpreter"},
	{"x87 direct arithmetic", []byte{0xD8, 0xC1}, x86JITCoverageDirect, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_FPU_FADDS_mem32"},
	{"x87 canonical helper", []byte{0xD9, 0xF0}, x86JITCoverageFPUHelper, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_FPUHelperFormEmitsCanonicalExit"},
	{"x87 segment helper", []byte{0x64, 0xD9, 0xF0}, x86JITCoverageFPUHelper, x86JITCoverageUnavailable, x86JITCoverageUnavailable, "TestX86JIT_FPUHelperPrefixFormsAreAdmittedByBlockCompiler"},
}

// x86JITBaseOpcodeInventory and x86JITExtendedOpcodeInventory are the
// source-backed completeness guards for the interpreter dispatch tables.  The
// representative form manifest above proves bytes reach production lowering;
// these maps ensure no implemented interpreter dispatch entry is silently
// omitted from the backend contract.  The 0F base entry is structural and
// delegates to the extended table, so it is deliberately absent from base.
var x86JITBaseOpcodeInventory = func() map[byte]x86JITCoveragePath {
	m := make(map[byte]x86JITCoveragePath)
	set := func(path x86JITCoveragePath, first, last byte) {
		for op := first; op <= last; op++ {
			m[op] = path
		}
	}
	set(x86JITCoverageDirect, 0x00, 0x0E)
	set(x86JITCoverageDirect, 0x10, 0x25)
	set(x86JITCoverageDirect, 0x27, 0x3D)
	set(x86JITCoverageDirect, 0x3F, 0x6B)
	set(x86JITCoverageFallback, 0x6C, 0x6F)
	set(x86JITCoverageDirect, 0x70, 0x99)
	m[0x9A] = x86JITCoverageFallback
	set(x86JITCoverageDirect, 0x9B, 0xC9)
	set(x86JITCoverageFallback, 0xCA, 0xCF)
	set(x86JITCoverageDirect, 0xD0, 0xD7)
	set(x86JITCoverageFPUHelper, 0xD8, 0xDF)
	set(x86JITCoverageDirect, 0xE0, 0xE3)
	set(x86JITCoverageFallback, 0xE4, 0xE7)
	set(x86JITCoverageDirect, 0xE8, 0xE9)
	m[0xEA] = x86JITCoverageFallback
	m[0xEB] = x86JITCoverageDirect
	set(x86JITCoverageFallback, 0xEC, 0xEF)
	m[0xF4] = x86JITCoverageFallback
	set(x86JITCoverageDirect, 0xF5, 0xFD)
	m[0xFE] = x86JITCoverageDirect
	m[0xFF] = x86JITCoverageDirect
	return m
}()

var x86JITExtendedOpcodeInventory = func() map[byte]x86JITCoveragePath {
	m := make(map[byte]x86JITCoveragePath)
	set := func(path x86JITCoveragePath, first, last byte) {
		for op := first; op <= last; op++ {
			m[op] = path
		}
	}
	set(x86JITCoverageDirect, 0x40, 0x4F)
	set(x86JITCoverageDirect, 0x80, 0x9F)
	set(x86JITCoverageDirect, 0xA0, 0xA1)
	m[0xA3] = x86JITCoverageDirect
	set(x86JITCoverageDirect, 0xA4, 0xA5)
	set(x86JITCoverageDirect, 0xA8, 0xA9)
	m[0xAB] = x86JITCoverageDirect
	set(x86JITCoverageDirect, 0xAC, 0xAD)
	m[0xAF] = x86JITCoverageDirect
	m[0xB3] = x86JITCoverageDirect
	set(x86JITCoverageDirect, 0xB6, 0xB7)
	m[0xBA] = x86JITCoverageDirect
	m[0xBB] = x86JITCoverageDirect
	set(x86JITCoverageDirect, 0xBC, 0xBF)
	set(x86JITCoverageDirect, 0xC8, 0xCF)
	return m
}()
