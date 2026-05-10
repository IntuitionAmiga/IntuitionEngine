package main

import (
	"fmt"
	"strings"
)

// FPRegClass classifies floating-point and FPU control tokens.
type FPRegClass int

const (
	FPRegUnknown FPRegClass = iota
	FPRegData              // fp0..fp7
	FPRegFPCR
	FPRegFPSR
	FPRegFPIAR
)

// MappedFPReg is the result of resolving an m68k FP token.
//
// FP0–FP7 map to the *even-numbered* IE64 FP registers (f0/f2/.../f14); the
// adjacent odd register provides the implicit double-precision high half per
// IE64 ISA §4.6.6 and must not be referenced separately.
type MappedFPReg struct {
	Class FPRegClass
	Name  string // canonical m68k name, lower-case ("fp0", "fpcr", ...)
	IE64  string // IE64 register name ("f0", "f2", ..., "f14"), empty for control regs
	Index int    // 0..7 for FP0..FP7, -1 otherwise
}

var fpRegAliases = func() map[string]MappedFPReg {
	m := map[string]MappedFPReg{}
	for i := 0; i <= 7; i++ {
		m[fmt.Sprintf("fp%d", i)] = MappedFPReg{
			Class: FPRegData,
			Name:  fmt.Sprintf("fp%d", i),
			IE64:  fmt.Sprintf("f%d", 2*i),
			Index: i,
		}
	}
	m["fpcr"] = MappedFPReg{Class: FPRegFPCR, Name: "fpcr", Index: -1}
	m["fpsr"] = MappedFPReg{Class: FPRegFPSR, Name: "fpsr", Index: -1}
	m["fpiar"] = MappedFPReg{Class: FPRegFPIAR, Name: "fpiar", Index: -1}
	return m
}()

// LookupFPRegister resolves an m68k FP register/control token. Returns
// (zero, false) if the token is not a recognised FP name.
func LookupFPRegister(tok string) (MappedFPReg, bool) {
	r, ok := fpRegAliases[strings.ToLower(strings.TrimSpace(tok))]
	return r, ok
}

// IsFPRegisterName reports whether tok names an FPU register (FPn) or one of
// the FPU control regs (FPCR/FPSR/FPIAR).
func IsFPRegisterName(tok string) bool {
	_, ok := LookupFPRegister(tok)
	return ok
}

// FPSizeSuffixes is the set of m68k FPU size suffixes the lexer recognises.
//
//	.B / .W / .L  → integer source/dest, sign-extended into FPn
//	.S            → 32-bit IEEE single
//	.D            → 64-bit IEEE double (single 8-byte transfer; *not* 2× fload)
//	.X            → 80-bit extended; degraded to .D
//	.P            → 96-bit packed BCD; unsupported
//
// The integer suffixes overlap with the integer-side lexer; the FP-only
// additions are .D / .X / .P. Single-letter suffix recognition lives in
// lexer.go::SplitMnemonicSize.
var FPSizeSuffixes = map[string]struct{}{
	".b": {}, ".w": {}, ".l": {},
	".s": {}, ".d": {}, ".x": {}, ".p": {},
}

// IsFPSize reports whether `s` is a size suffix the FPU layer accepts. Does
// *not* require an FP-only suffix (.D/.X/.P) — accepts .B/.W/.L/.S as well.
func IsFPSize(s string) bool {
	_, ok := FPSizeSuffixes[strings.ToLower(s)]
	return ok
}

// IsFPOnlySize reports whether `s` is an FP-only size suffix (.D/.X/.P) — i.e.
// one that does not exist in the integer-side ISA.
func IsFPOnlySize(s string) bool {
	switch strings.ToLower(s) {
	case ".d", ".x", ".p":
		return true
	}
	return false
}
