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
// ARM64 records paths implemented by the Linux backend. Rows marked
// unavailable resume through its interpreter boundary and remain direct-
// lowering gaps rather than claims of native coverage.
var x86JITCoverageManifest = []x86JITCoverageRow{
	{"MOV r32,imm32", []byte{0xB8, 1, 0, 0, 0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r8,imm8", []byte{0xB0, 1}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r/m8,r8", []byte{0x88, 0xC3}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r/m32,r32", []byte{0x89, 0xD8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r32,r/m32", []byte{0x8B, 0xD8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"MOV r32,m32 guarded", []byte{0x8B, 0x43, 0x04}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMemoryMOVReadsAndGuardBails"},
	{"MOV moffs guarded", []byte{0xA1, 0x00, 0x04, 0x00, 0x00}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMoffsMOV"},
	{"MOV r8,m8 guarded", []byte{0x8A, 0x6B, 0x08}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMemoryMOVReadsAndGuardBails"},
	{"MOV m32,r32 guarded", []byte{0x89, 0x03}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_ProductionMOVStoreInvalidatesCachedTarget"},
	{"MOV m,imm guarded", []byte{0xC7, 0x43, 0x04, 0x44, 0x33, 0x22, 0x11}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMemoryMOVImmediateStores"},
	{"LEA SIB/disp32", []byte{0x8D, 0x84, 0xB3, 0, 1, 0, 0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectLEASIBAndAbsolute"},
	{"byte ALU r/m,r", []byte{0x00, 0xD8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectArithmeticRegisterParity"},
	{"dword ALU r/m,r", []byte{0x01, 0xD8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectArithmeticRegisterParity"},
	{"ALU accumulator,imm", []byte{0x05, 1, 0, 0, 0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectArithmeticRegisterParity"},
	{"Grp1 r/m32,imm8", []byte{0x83, 0xC0, 1}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectArithmeticRegisterParity"},
	{"INC/DEC register", []byte{0x40}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectINCDECParity"},
	{"XCHG", []byte{0x87, 0xD8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"XCHG guarded memory", []byte{0x87, 0x03}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMemoryXCHG"},
	{"TEST", []byte{0x85, 0xD8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectTESTRegisterParity"},
	{"Grp2 byte,count one", []byte{0xD0, 0xE0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectShiftOneParity"},
	{"Grp2 word/dword,count one", []byte{0xD1, 0xE0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectShiftOneParity"},
	{"Grp2 memory,count one", []byte{0xD0, 0x23}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMemoryShiftOneParity"},
	{"Grp2 dword,imm8", []byte{0xC1, 0xE0, 2}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectShiftImmediate32Parity"},
	{"Grp2 byte shift,CL", []byte{0xD2, 0xE0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectGroup2ByteShiftCLParity"},
	{"Grp2 dword shift,CL", []byte{0xD3, 0xE0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectGroup2DwordShiftCLParity"},
	{"Grp3", []byte{0xF7, 0xD8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectGroup3NotNegParity"},
	{"IMUL immediate", []byte{0x6B, 0xC0, 2}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectIMUL16Parity"},
	{"PUSH/POP register", []byte{0x50}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectRegisterPushPop"},
	{"POP guarded memory", []byte{0x8F, 0x03}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectPopMemory"},
	{"PUSH immediate", []byte{0x6A, 0x80}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectImmediatePush"},
	{"PUSH/POP segment", []byte{0x06}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectSegmentPushPop"},
	{"PUSHA/POPA", []byte{0x60}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectPushaPopa"},
	{"PUSHF/POPF", []byte{0x9C}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectPushfPopf"},
	{"CLI/STI", []byte{0xFA}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_CLI_STIMatchesInterpreter"},
	{"ignored operand-size prefix", []byte{0x66, 0x90}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_IgnoredOperandSizePrefixesMatchInterpreter"},
	{"WAIT", []byte{0x9B}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectWAIT"},
	{"segment MOV", []byte{0x8C, 0xD8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectSegmentMOVRegisters"},
	{"segment MOV guarded memory", []byte{0x8C, 0x5B, 0x00}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMemorySegmentMOV"},
	{"LES/LDS", []byte{0xC4, 0x03}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectLESLDSParity"},
	{"XLAT", []byte{0xD7}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectXLAT"},
	{"ENTER/LEAVE", []byte{0xC8, 0, 0, 0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectENTERLevelZero"},
	{"LEAVE", []byte{0xC9}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectLEAVE"},
	{"CBW/CWDE and CWD/CDQ", []byte{0x66, 0x98}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectSignExtendInstructions"},
	{"near CALL", []byte{0xE8, 0, 0, 0, 0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectCALLRETParity"},
	{"near RET", []byte{0xC3}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectCALLRETParity"},
	{"near JMP", []byte{0xEB, 0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"Jcc", []byte{0x74, 0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectJccParity"},
	{"LOOP", []byte{0xE2, 0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectLoopParity"},
	{"bit test", []byte{0x0F, 0xA3, 0xC8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectBTRegisterParity"},
	{"double shift", []byte{0x0F, 0xA4, 0xC8, 1}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectDoubleShiftImmediateParity"},
	{"double shift 16-bit immediate", []byte{0x66, 0x0F, 0xA4, 0xC8, 1}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectDoubleShift16ImmediateParity"},
	{"double shift CL", []byte{0x0F, 0xA5, 0xC8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectDoubleShiftCLParity"},
	{"MOVS", []byte{0xA4}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMOVSBParity"},
	{"STOS", []byte{0xAA}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectSTOSParity"},
	{"LODS", []byte{0xAC}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectLODSParity"},
	{"CMPS", []byte{0xA6}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectCMPSParity"},
	{"SCAS", []byte{0xAE}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectSCASParity"},
	{"MOVZX/MOVSX", []byte{0x0F, 0xB6, 0xC0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMOVZXRegisters"},
	{"MOVZX/MOVSX guarded memory", []byte{0x0F, 0xB6, 0x43, 0x01}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectMemoryMOVExtensions"},
	{"SETcc", []byte{0x0F, 0x94, 0xC0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectSETccParity"},
	{"CMOVcc", []byte{0x0F, 0x44, 0xC0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectCMOVccParity"},
	{"BSF/BSR", []byte{0x0F, 0xBC, 0xC0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectBSFBSRParity"},
	{"BSWAP", []byte{0x0F, 0xC8}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86JIT_AMD64CoverageManifest"},
	{"SALC", []byte{0xD6}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectSALC"},
	{"REP MOVS", []byte{0xF3, 0xA4}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectREPMOVSParity"},
	{"REP STOS", []byte{0xF3, 0xAA}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectREPSTOSParity"},
	{"REP CMPS", []byte{0xF3, 0xA7}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectREPCMPSParity"},
	{"REP SCAS", []byte{0xF3, 0xAF}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectREPSCASParity"},
	{"REP LODS", []byte{0xF3, 0xAC}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectREPLODSParity"},
	{"x87 direct arithmetic", []byte{0xD8, 0xC1}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFADDRegisterParity"},
	{"x87 direct finite FMUL", []byte{0xD8, 0xC9}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFMULRegisterParity"},
	{"x87 direct finite FSUB", []byte{0xD8, 0xE1}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFSUBRegisterParity"},
	{"x87 direct finite FDIV", []byte{0xD8, 0xF1}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFDIVRegisterParity"},
	{"x87 FNOP", []byte{0xD9, 0xD0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFNOPProvenanceParity"},
	{"x87 FNCLEX", []byte{0xDB, 0xE2}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFNCLEXParity"},
	{"x87 FNINIT", []byte{0xDB, 0xE3}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFNINITParity"},
	{"x87 FNSTSW AX", []byte{0xDF, 0xE0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFNSTSWAXParity"},
	{"x87 FCHS/FABS", []byte{0xD9, 0xE0}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFCHSFABSParity"},
	{"x87 FFREE", []byte{0xDD, 0xC1}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFFREEProvenanceParity"},
	{"x87 FXCH", []byte{0xD9, 0xC9}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFXCHParity"},
	{"x87 FSTP ST(i)", []byte{0xDD, 0xD9}, x86JITCoverageDirect, x86JITCoverageDirect, x86JITCoverageUnavailable, "TestX86ARM64_DirectFSTPSTiParity"},
	{"x87 canonical helper", []byte{0xD9, 0xF0}, x86JITCoverageFPUHelper, x86JITCoverageFPUHelper, x86JITCoverageUnavailable, "TestX86JIT_FPUHelperFormEmitsCanonicalExit"},
	{"x87 segment helper", []byte{0x64, 0xD9, 0xF0}, x86JITCoverageFPUHelper, x86JITCoverageFPUHelper, x86JITCoverageUnavailable, "TestX86ARM64_FPUHelperPrefixParity"},
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
