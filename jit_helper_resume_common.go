// jit_helper_resume_common.go - Shared IE64 helper-resume gate.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"os"
	"strings"
)

func ie64JITResumeEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("IE64_JIT_RESUME"))) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func (ctx *JITContext) clearResume() {
	if ctx == nil {
		return
	}
	ctx.ResumeValid = 0
	ctx.ResumeAddr = 0
	ctx.ResumePC = 0
	ctx.ResumePTBR = 0
	ctx.ResumeCountBase = 0
	ctx.ResumeMMUEnabled = 0
}

func (cpu *CPU64) canResumeJITHelper(helperRetired uint64) bool {
	if cpu == nil || cpu.jitCtx == nil || !ie64JITResumeEnabled() {
		return false
	}
	if !cpu.running.Load() {
		return false
	}
	ctx := cpu.jitCtx
	if helperRetired != 1 || ctx.ResumeValid == 0 || ctx.ResumeAddr == 0 {
		return false
	}
	if cpu.PC != ctx.ResumePC {
		return false
	}
	if ctx.NeedInval != 0 || cpu.jitNeedInval {
		return false
	}
	if cpu.timerEnabled.Load() {
		return false
	}
	if cpu.debugBreakpointsActive != nil && cpu.debugBreakpointsActive() {
		return false
	}
	currentMMU := uint32(0)
	if cpu.mmuEnabled {
		currentMMU = 1
	}
	if ctx.ResumeMMUEnabled != currentMMU || ctx.ResumePTBR != cpu.ptbr {
		return false
	}
	return true
}
