package main

import (
	"context"
	"errors"
	"os/exec"
	"sync/atomic"
)

const (
	HostMMIOBase = HOST_MMIO_REGION_BASE
	HostMMIOEnd  = HOST_MMIO_REGION_END
)

const DefaultHostHelperPath = "/usr/libexec/intuitionengine-host-helper"

const (
	HostMMIOCommand = 0x00
	HostMMIOTrigger = 0x04
	HostMMIOStatus  = 0x08
	HostMMIOExit    = 0x0C
)

type HostCommand uint32

const (
	HostCommandNone HostCommand = iota
	HostCommandNet
	HostCommandUpdate
	HostCommandReboot
	HostCommandPoweroff
)

const (
	HostStatusRunning uint32 = iota
	HostStatusOK
	HostStatusErr
	HostStatusUserCancel
	HostStatusDisabled
	HostStatusIdle
)

type HostCommandResult struct {
	Status   uint32
	ExitCode uint32
	Err      error
}

type HostCommandRunner interface {
	RunHostCommand(ctx context.Context, cmd HostCommand) HostCommandResult
}

type HostHelperConfig struct {
	Enabled    bool
	Appliance  bool
	HelperPath string
}

type HostHelper struct {
	enabled   bool
	appliance bool
	runner    HostCommandRunner

	cmd    atomic.Uint32
	status atomic.Uint32
	exit   atomic.Uint32
}

func NewHostHelper(config HostHelperConfig) *HostHelper {
	var runner HostCommandRunner
	if config.Enabled {
		runner = ExternalHostCommandRunner{
			Path:      config.HelperPath,
			Appliance: config.Appliance,
		}
	}
	return NewHostHelperWithRunner(config.Enabled, config.Appliance, runner)
}

func NewHostHelperWithRunner(enabled bool, appliance bool, runner HostCommandRunner) *HostHelper {
	h := &HostHelper{
		enabled:   enabled,
		appliance: appliance,
		runner:    runner,
	}
	h.status.Store(HostStatusIdle)
	return h
}

func RegisterHostHelperMMIO(bus *MachineBus, helper *HostHelper) {
	if bus == nil || helper == nil {
		return
	}
	bus.MapIO(HostMMIOBase, HostMMIOEnd, helper.HandleRead, helper.HandleWrite)
}

func (h *HostHelper) SetCommand(cmd HostCommand) {
	if cmd < HostCommandNet || cmd > HostCommandPoweroff {
		h.cmd.Store(uint32(HostCommandNone))
		return
	}
	h.cmd.Store(uint32(cmd))
}

func (h *HostHelper) Trigger() {
	if !h.enabled {
		h.exit.Store(0)
		h.status.Store(HostStatusDisabled)
		return
	}

	if !h.beginCommand() {
		return
	}

	cmd := HostCommand(h.cmd.Load())
	if cmd < HostCommandNet || cmd > HostCommandPoweroff || h.runner == nil {
		h.exit.Store(HostHelperExitBadInput)
		h.status.Store(HostStatusErr)
		return
	}

	h.exit.Store(0)
	go h.runCommand(cmd)
}

func (h *HostHelper) beginCommand() bool {
	for {
		status := h.status.Load()
		if status == HostStatusRunning {
			return false
		}
		if h.status.CompareAndSwap(status, HostStatusRunning) {
			return true
		}
	}
}

type ExternalHostCommandRunner struct {
	Path      string
	Appliance bool
}

func (r ExternalHostCommandRunner) RunHostCommand(ctx context.Context, cmd HostCommand) HostCommandResult {
	verb, ok := hostCommandVerb(cmd)
	if !ok {
		return HostCommandResult{Status: HostStatusErr, ExitCode: HostHelperExitBadInput}
	}

	path := r.Path
	if path == "" {
		path = DefaultHostHelperPath
	}

	args := []string{verb}
	if cmd == HostCommandUpdate && r.Appliance {
		args = append(args, "--appliance")
	}

	run := exec.CommandContext(ctx, path, args...)
	if err := run.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: ctxErr}
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return HostCommandResult{Status: HostStatusErr, ExitCode: uint32(exitErr.ExitCode()), Err: err}
		}
		return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: err}
	}

	return HostCommandResult{Status: HostStatusOK, ExitCode: 0}
}

func hostCommandVerb(cmd HostCommand) (string, bool) {
	switch cmd {
	case HostCommandNet:
		return "net", true
	case HostCommandUpdate:
		return "update", true
	case HostCommandReboot:
		return "reboot", true
	case HostCommandPoweroff:
		return "poweroff", true
	default:
		return "", false
	}
}

func (h *HostHelper) runCommand(cmd HostCommand) {
	result := h.runner.RunHostCommand(context.Background(), cmd)
	status := result.Status
	switch status {
	case HostStatusOK, HostStatusErr, HostStatusUserCancel, HostStatusDisabled:
	default:
		status = HostStatusErr
	}
	h.exit.Store(result.ExitCode)
	h.status.Store(status)
}

func (h *HostHelper) Status() uint32 {
	return h.status.Load()
}

func (h *HostHelper) ExitCode() uint32 {
	return h.exit.Load()
}

func (h *HostHelper) HandleRead(addr uint32) uint32 {
	offset := hostMMIOOffset(addr)
	switch offset {
	case HostMMIOCommand:
		return h.cmd.Load()
	case HostMMIOStatus:
		return h.Status()
	case HostMMIOExit:
		return h.ExitCode()
	case HostMMIOExit + 1, HostMMIOExit + 2, HostMMIOExit + 3:
		shift := (offset - HostMMIOExit) * 8
		return h.ExitCode() >> shift
	default:
		return 0
	}
}

func (h *HostHelper) HandleWrite(addr uint32, value uint32) {
	offset := hostMMIOOffset(addr)
	switch offset {
	case HostMMIOCommand:
		h.SetCommand(HostCommand(value))
	case HostMMIOTrigger:
		if value != 0 {
			h.Trigger()
		}
	}
}

func hostMMIOOffset(addr uint32) uint32 {
	if addr >= HostMMIOBase && addr <= HostMMIOEnd {
		return addr - HostMMIOBase
	}
	return addr
}
