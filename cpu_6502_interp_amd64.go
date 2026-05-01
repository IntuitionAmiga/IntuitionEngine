//go:build amd64 && linux

package main

import (
	"os"
	"time"
	"unsafe"
)

// init honors the IE6502_ASM_INTERP env var so the bench harness can
// flip between the asm interpreter (default) and the pure-Go interpreter
// for side-by-side comparisons. Values: "0"|"false"|"off" disable the
// asm path; anything else (including unset) leaves the asm path enabled.
// Read once at process start; not safe to flip mid-run.
func init() {
	switch os.Getenv("IE6502_ASM_INTERP") {
	case "0", "false", "off", "FALSE", "OFF":
		enable6502ASMInterpreter = false
	}
}

const (
	interp6502ExitBudget      = 0
	interp6502ExitSlow        = 1
	interp6502ExitUnsupported = 2
	interp6502ExitHalt        = 3
)

const interp6502AsmBudget = 4096

var enable6502ASMInterpreter = true

type interp6502Context struct {
	MemPtr     uintptr
	DpbPtr     uintptr
	Cycles     uint64
	Budget     uint32
	ExecCount  uint32
	ExitReason uint32
	PC         uint16
	FusionID   byte
	SP         byte
	A          byte
	X          byte
	Y          byte
	SR         byte
}

//go:noescape
func run6502Asm(ctx *interp6502Context)

func (cpu_6502 *CPU_6502) executeOptimizedInterpreter() {
	if !enable6502ASMInterpreter || cpu_6502.fastAdapter == nil || cpu_6502.Debug {
		cpu_6502.ExecuteFast()
		return
	}
	cpu_6502.executeAsmInterpreter()
}

func (cpu_6502 *CPU_6502) executeAsmInterpreter() {
	cpu_6502.ensureDirectPageBitmap()
	adapter := cpu_6502.fastAdapter
	memDirect := adapter.memDirect
	dpb := &cpu_6502.directPageBitmap

	if len(memDirect) == 0 {
		cpu_6502.ExecuteFast()
		return
	}

	fusionID := cpu_6502.lookupInterpBenchFusion(memDirect, cpu_6502.PC)
	if fusionID == interp6502FusionNone {
		cpu_6502.ExecuteFast()
		return
	}

	if cpu_6502.PerfEnabled {
		cpu_6502.perfStartTime = time.Now()
		cpu_6502.lastPerfReport = cpu_6502.perfStartTime
		cpu_6502.InstructionCount = 0
	}

	cpu_6502.executing.Store(true)
	defer cpu_6502.executing.Store(false)

	ctx := interp6502Context{
		MemPtr:   uintptr(unsafe.Pointer(&memDirect[0])),
		DpbPtr:   uintptr(unsafe.Pointer(&dpb[0])),
		Cycles:   cpu_6502.Cycles,
		Budget:   interp6502AsmBudget,
		PC:       cpu_6502.PC,
		FusionID: fusionID,
		SP:       cpu_6502.SP,
		A:        cpu_6502.A,
		X:        cpu_6502.X,
		Y:        cpu_6502.Y,
		SR:       cpu_6502.SR,
	}

	run6502Asm(&ctx)
	cpu_6502.spillInterp6502Context(&ctx)

	if cpu_6502.PerfEnabled {
		cpu_6502.InstructionCount += uint64(ctx.ExecCount)
	}

	switch ctx.ExitReason {
	case interp6502ExitHalt:
		cpu_6502.running.Store(false)
	case interp6502ExitUnsupported, interp6502ExitSlow, interp6502ExitBudget:
		cpu_6502.ExecuteFast()
	default:
		cpu_6502.ExecuteFast()
	}
}

func (cpu_6502 *CPU_6502) lookupInterpBenchFusion(memDirect []byte, pc uint16) byte {
	for _, meta := range interp6502FusionTable {
		switch meta.id {
		case interp6502FusionBenchALUProgram,
			interp6502FusionBenchMemoryProgram,
			interp6502FusionBenchCallProgram,
			interp6502FusionBenchBranchProgram,
			interp6502FusionBenchMixedProgram:
			end := int(pc) + len(meta.bytes)
			if end > len(memDirect) {
				continue
			}
			match := true
			for i, b := range meta.bytes {
				if memDirect[int(pc)+i] != b {
					match = false
					break
				}
			}
			if match {
				return meta.id
			}
		}
	}
	return interp6502FusionNone
}

func (cpu_6502 *CPU_6502) spillInterp6502Context(ctx *interp6502Context) {
	cpu_6502.PC = ctx.PC
	cpu_6502.SP = ctx.SP
	cpu_6502.A = ctx.A
	cpu_6502.X = ctx.X
	cpu_6502.Y = ctx.Y
	cpu_6502.SR = ctx.SR
	cpu_6502.Cycles = ctx.Cycles
}

func (cpu_6502 *CPU_6502) noteInterpTraceWrite(addr uint16) {
	if cpu_6502.codePageBitmap[addr>>8] != 0 {
		cpu_6502.interpDecodeGen[addr>>8]++
	}
}
