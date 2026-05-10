package main

import (
	"fmt"
	"strings"
)

// Register classes recognised in m68k source.
type RegClass int

const (
	RegUnknown RegClass = iota
	RegData             // d0..d7
	RegAddr             // a0..a6, fp (a6 alias)
	RegSP               // a7 / sp -- emulated 32-bit guest stack
	RegPC               // pc
	RegCCR              // ccr
	RegSR               // sr
	RegUSP              // usp
)

// MappedReg is the result of resolving a m68k register token.
type MappedReg struct {
	Class    RegClass
	Name     string // canonical m68k name, lower-case ("d0", "a7", "pc", ...)
	IE64     string // IE64 register name ("r1".."r31"), empty for PC/CCR/SR/USP
	IsStack  bool   // true for a7/sp (so callers know to emit guest-stack lowering)
}

// regAliases is the canonical m68k → IE64 register-file map.
//
// d0..d7 → r1..r8
// a0..a6 → r9..r15
// a7 / sp → r30 (emulated 32-bit guest stack)
// fp aliases a6 (devpac convention).
var regAliases = func() map[string]MappedReg {
	m := map[string]MappedReg{}
	for i := 0; i <= 7; i++ {
		m[fmt.Sprintf("d%d", i)] = MappedReg{Class: RegData, Name: fmt.Sprintf("d%d", i), IE64: fmt.Sprintf("r%d", i+1)}
	}
	for i := 0; i <= 6; i++ {
		m[fmt.Sprintf("a%d", i)] = MappedReg{Class: RegAddr, Name: fmt.Sprintf("a%d", i), IE64: fmt.Sprintf("r%d", i+9)}
	}
	// fp aliases a6.
	m["fp"] = MappedReg{Class: RegAddr, Name: "a6", IE64: "r15"}
	// a7 / sp → r30 (emulated guest stack).
	m["a7"] = MappedReg{Class: RegSP, Name: "a7", IE64: "r30", IsStack: true}
	m["sp"] = MappedReg{Class: RegSP, Name: "a7", IE64: "r30", IsStack: true}
	// PC, CCR, SR, USP — class only, no direct IE64 mapping.
	m["pc"] = MappedReg{Class: RegPC, Name: "pc"}
	m["ccr"] = MappedReg{Class: RegCCR, Name: "ccr"}
	m["sr"] = MappedReg{Class: RegSR, Name: "sr"}
	m["usp"] = MappedReg{Class: RegUSP, Name: "usp"}
	return m
}()

// LookupRegister resolves a m68k register token (case-insensitive) to a MappedReg.
// Returns (zero, false) if the token is not a recognised m68k register name.
func LookupRegister(tok string) (MappedReg, bool) {
	r, ok := regAliases[strings.ToLower(strings.TrimSpace(tok))]
	return r, ok
}

// IsRegisterName reports whether tok names a m68k register.
func IsRegisterName(tok string) bool {
	_, ok := LookupRegister(tok)
	return ok
}
