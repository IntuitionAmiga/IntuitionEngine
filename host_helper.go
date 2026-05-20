package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	HostMMIOBase = HOST_MMIO_REGION_BASE
	HostMMIOEnd  = HOST_MMIO_REGION_END
)

const DefaultHostHelperPath = "/usr/libexec/intuitionengine-host-helper"
const DefaultPkexecPath = "/usr/bin/pkexec"
const DefaultHostHelperSocketPath = "/run/intuitionengine-host-helper.sock"
const DefaultHostHelperLogPath = "/tmp/ie-host-helper.log"

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

type HostCommandObserver interface {
	HostCommandStarted(cmd HostCommand)
	HostCommandOutput(cmd HostCommand, text string)
	HostCommandCompleted(cmd HostCommand, result HostCommandResult)
}

type HostUpdateConfirmer interface {
	ConfirmHostUpdate(ctx context.Context) bool
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
	confirmer HostUpdateConfirmer
	observer  HostCommandObserver

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

func (h *HostHelper) SetUpdateConfirmer(confirmer HostUpdateConfirmer) {
	h.confirmer = confirmer
}

func (h *HostHelper) SetObserver(observer HostCommandObserver) {
	h.observer = observer
}

func (h *HostHelper) SetCommand(cmd HostCommand) {
	if !isValidHostCommand(cmd) {
		h.cmd.Store(uint32(HostCommandNone))
		return
	}
	h.cmd.Store(uint32(cmd))
}

func (h *HostHelper) Trigger() {
	if !h.enabled {
		h.exit.Store(0)
		h.status.Store(HostStatusDisabled)
		logHostHelperFailure("host helper disabled")
		return
	}

	if !h.beginCommand() {
		return
	}

	cmd := HostCommand(h.cmd.Load())
	if !isValidHostCommand(cmd) || h.runner == nil {
		h.exit.Store(HostHelperExitBadInput)
		h.status.Store(HostStatusErr)
		logHostHelperFailure(fmt.Sprintf("invalid host command or runner: cmd=%d runner-nil=%t", cmd, h.runner == nil))
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
	Path       string
	PkexecPath string
	SocketPath string
	Appliance  bool
	Observer   HostCommandObserver
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

	pkexecPath := r.PkexecPath
	if pkexecPath == "" {
		pkexecPath = DefaultPkexecPath
	}

	args := []string{path, verb}
	if cmd == HostCommandUpdate && r.Appliance {
		args = append(args, "--appliance")
	}

	socketPath := r.SocketPath
	if socketPath == "" {
		socketPath = DefaultHostHelperSocketPath
	}
	if _, err := os.Stat(socketPath); err == nil {
		return r.runViaBroker(ctx, socketPath, args[1:], cmd)
	} else if !errors.Is(err, os.ErrNotExist) {
		logExternalHostCommandFailure(socketPath, args[1:], 1, err, nil)
		return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: err}
	}

	execPath := pkexecPath
	execArgs := args

	run := exec.CommandContext(ctx, execPath, execArgs...)
	output := newHostCommandOutput(cmd, r.Observer, 4096)
	run.Stdout = output
	run.Stderr = output
	err := run.Run()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			logExternalHostCommandFailure(execPath, execArgs, 1, ctxErr, output.Bytes())
			return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: ctxErr}
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode := exitErr.ExitCode()
			logExternalHostCommandFailure(execPath, execArgs, exitCode, err, output.Bytes())
			return HostCommandResult{Status: HostStatusErr, ExitCode: uint32(exitCode), Err: err}
		}
		logExternalHostCommandFailure(execPath, execArgs, 1, err, output.Bytes())
		return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: err}
	}

	return HostCommandResult{Status: HostStatusOK, ExitCode: 0}
}

func (r ExternalHostCommandRunner) runViaBroker(ctx context.Context, socketPath string, args []string, cmd HostCommand) HostCommandResult {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		logExternalHostCommandFailure(socketPath, args, 1, err, nil)
		return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: err}
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	request := hostBrokerRequest{Args: args}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		logExternalHostCommandFailure(socketPath, args, 1, err, nil)
		return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: err}
	}

	output := newHostCommandOutput(cmd, r.Observer, 4096)
	dec := json.NewDecoder(conn)
	for {
		var message hostBrokerMessage
		if err := dec.Decode(&message); err != nil {
			logExternalHostCommandFailure(socketPath, args, 1, err, output.Bytes())
			return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: err}
		}
		switch message.Type {
		case "output":
			_, _ = output.Write([]byte(message.Output))
		case "result", "":
			if message.Output != "" && len(output.Bytes()) == 0 {
				_, _ = output.Write([]byte(message.Output))
			}
			if message.ExitCode != 0 {
				err := fmt.Errorf("host helper broker exited %d", message.ExitCode)
				logExternalHostCommandFailure(socketPath, args, message.ExitCode, err, output.Bytes())
				return HostCommandResult{Status: HostStatusErr, ExitCode: uint32(message.ExitCode), Err: err}
			}
			return HostCommandResult{Status: HostStatusOK, ExitCode: 0}
		default:
			err := fmt.Errorf("host helper broker sent unknown message type %q", message.Type)
			logExternalHostCommandFailure(socketPath, args, 1, err, output.Bytes())
			return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: err}
		}
	}
}

type hostBrokerRequest struct {
	Args []string `json:"args"`
}

type hostBrokerMessage struct {
	Type     string `json:"type,omitempty"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output,omitempty"`
}

type boundedOutput struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func newBoundedOutput(limit int) *boundedOutput {
	return &boundedOutput{limit: limit}
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.limit <= 0 {
		return len(p), nil
	}
	if len(p) >= b.limit {
		b.buf = append(b.buf[:0], p[len(p)-b.limit:]...)
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.limit {
		b.buf = append(b.buf[:0], b.buf[len(b.buf)-b.limit:]...)
	}
	return len(p), nil
}

func (b *boundedOutput) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf...)
}

type hostCommandOutput struct {
	cmd      HostCommand
	observer HostCommandObserver
	output   *boundedOutput
}

func newHostCommandOutput(cmd HostCommand, observer HostCommandObserver, limit int) *hostCommandOutput {
	return &hostCommandOutput{
		cmd:      cmd,
		observer: observer,
		output:   newBoundedOutput(limit),
	}
}

func (w *hostCommandOutput) Write(p []byte) (int, error) {
	if w.output != nil {
		_, _ = w.output.Write(p)
	}
	if w.observer != nil && len(p) > 0 {
		w.observer.HostCommandOutput(w.cmd, string(p))
	}
	return len(p), nil
}

func (w *hostCommandOutput) Bytes() []byte {
	if w == nil || w.output == nil {
		return nil
	}
	return w.output.Bytes()
}

func logExternalHostCommandFailure(pkexecPath string, args []string, exitCode int, err error, output []byte) {
	entry := fmt.Sprintf(
		"%s pkexec=%q args=%q exit=%d err=%v output=%q\n",
		time.Now().Format(time.RFC3339),
		pkexecPath,
		args,
		exitCode,
		err,
		strings.TrimSpace(string(output)),
	)
	writeHostHelperLog(entry)
}

func logHostHelperFailure(message string) {
	writeHostHelperLog(fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339), message))
}

func writeHostHelperLog(entry string) {
	logPaths := []string{DefaultHostHelperLogPath}
	if override := os.Getenv("IE_HOST_HELPER_LOG"); override != "" {
		logPaths = []string{override}
	}
	for _, path := range logPaths {
		if f, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); openErr == nil {
			_, _ = f.WriteString(entry)
			_ = f.Close()
		}
	}
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

func isValidHostCommand(cmd HostCommand) bool {
	return cmd >= HostCommandNet && cmd <= HostCommandPoweroff
}

func (h *HostHelper) runCommand(cmd HostCommand) {
	ctx := context.Background()
	if !h.confirmCommand(ctx, cmd) {
		result := HostCommandResult{Status: HostStatusUserCancel, ExitCode: 0}
		h.completeCommand(cmd, result)
		return
	}

	if h.observer != nil {
		h.observer.HostCommandStarted(cmd)
	}

	runner := h.runner
	if external, ok := runner.(ExternalHostCommandRunner); ok {
		external.Observer = h.observer
		runner = external
	}
	result := runner.RunHostCommand(ctx, cmd)
	h.completeCommand(cmd, result)
}

func (h *HostHelper) completeCommand(cmd HostCommand, result HostCommandResult) {
	switch result.Status {
	case HostStatusOK, HostStatusErr, HostStatusUserCancel, HostStatusDisabled:
	default:
		result.Status = HostStatusErr
	}
	h.exit.Store(result.ExitCode)
	h.status.Store(result.Status)
	if h.observer != nil {
		h.observer.HostCommandCompleted(cmd, result)
	}
}

func (h *HostHelper) confirmCommand(ctx context.Context, cmd HostCommand) bool {
	if cmd != HostCommandUpdate || h.appliance {
		return true
	}
	if h.confirmer == nil {
		return false
	}
	return h.confirmer.ConfirmHostUpdate(ctx)
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
