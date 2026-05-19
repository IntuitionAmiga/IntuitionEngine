package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type scriptedHostCommandRunner struct {
	result HostCommandResult
	ready  chan struct{}
	calls  chan HostCommand
	once   sync.Once
}

func newScriptedHostCommandRunner(result HostCommandResult) *scriptedHostCommandRunner {
	return &scriptedHostCommandRunner{
		result: result,
		ready:  make(chan struct{}),
		calls:  make(chan HostCommand, 4),
	}
}

func (r *scriptedHostCommandRunner) RunHostCommand(ctx context.Context, cmd HostCommand) HostCommandResult {
	r.calls <- cmd
	select {
	case <-r.ready:
	case <-ctx.Done():
		return HostCommandResult{Status: HostStatusErr, ExitCode: 1, Err: ctx.Err()}
	}
	return r.result
}

func (r *scriptedHostCommandRunner) release() {
	r.once.Do(func() { close(r.ready) })
}

func TestHostHelperStateTransitions(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		cmd        HostCommand
		result     HostCommandResult
		wantStatus uint32
		wantExit   uint32
		wantCall   bool
	}{
		{
			name:       "ok",
			enabled:    true,
			cmd:        HostCommandNet,
			result:     HostCommandResult{Status: HostStatusOK, ExitCode: 0},
			wantStatus: HostStatusOK,
			wantCall:   true,
		},
		{
			name:       "err",
			enabled:    true,
			cmd:        HostCommandUpdate,
			result:     HostCommandResult{Status: HostStatusErr, ExitCode: HostHelperExitAptUpdateFailed, Err: errors.New("update failed")},
			wantStatus: HostStatusErr,
			wantExit:   HostHelperExitAptUpdateFailed,
			wantCall:   true,
		},
		{
			name:       "cancel",
			enabled:    true,
			cmd:        HostCommandUpdate,
			result:     HostCommandResult{Status: HostStatusUserCancel, ExitCode: 0},
			wantStatus: HostStatusUserCancel,
			wantCall:   true,
		},
		{
			name:       "disabled",
			enabled:    false,
			cmd:        HostCommandPoweroff,
			result:     HostCommandResult{Status: HostStatusOK},
			wantStatus: HostStatusDisabled,
			wantCall:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newScriptedHostCommandRunner(tt.result)
			helper := NewHostHelperWithRunner(tt.enabled, false, runner)
			if tt.cmd == HostCommandUpdate && tt.wantCall {
				helper.SetUpdateConfirmer(newScriptedHostUpdateConfirmer(true))
			}

			helper.SetCommand(tt.cmd)
			helper.Trigger()

			if got := helper.Status(); got != tt.wantStatus && got != HostStatusRunning {
				t.Fatalf("initial status = %d, want RUNNING or terminal %d", got, tt.wantStatus)
			}

			if tt.wantCall {
				select {
				case got := <-runner.calls:
					if got != tt.cmd {
						t.Fatalf("runner command = %d, want %d", got, tt.cmd)
					}
				case <-time.After(time.Second):
					t.Fatal("runner was not invoked")
				}
				runner.release()
			}

			waitForHostStatus(t, helper, tt.wantStatus)
			if got := helper.ExitCode(); got != tt.wantExit {
				t.Fatalf("exit code = %d, want %d", got, tt.wantExit)
			}

			if !tt.wantCall {
				select {
				case got := <-runner.calls:
					t.Fatalf("runner invoked while disabled with command %d", got)
				default:
				}
			}
		})
	}
}

func TestHostHelperRejectsOverlappingTriggers(t *testing.T) {
	firstRunner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK, ExitCode: 0})
	helper := NewHostHelperWithRunner(true, false, firstRunner)
	helper.SetUpdateConfirmer(newScriptedHostUpdateConfirmer(true))

	helper.SetCommand(HostCommandUpdate)
	helper.Trigger()
	select {
	case got := <-firstRunner.calls:
		if got != HostCommandUpdate {
			t.Fatalf("first runner command = %d, want %d", got, HostCommandUpdate)
		}
	case <-time.After(time.Second):
		t.Fatal("first runner was not invoked")
	}

	secondRunner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusErr, ExitCode: HostHelperExitAptUpgradeFailed})
	helper.runner = secondRunner
	helper.SetCommand(HostCommandPoweroff)
	helper.Trigger()

	select {
	case got := <-secondRunner.calls:
		t.Fatalf("overlapping trigger invoked runner with command %d", got)
	case <-time.After(10 * time.Millisecond):
	}
	if got := helper.Status(); got != HostStatusRunning {
		t.Fatalf("status after overlapping trigger = %d, want RUNNING", got)
	}

	firstRunner.release()
	waitForHostStatus(t, helper, HostStatusOK)
	if got := helper.ExitCode(); got != 0 {
		t.Fatalf("exit code after first completion = %d, want 0", got)
	}

	helper.Trigger()
	select {
	case got := <-secondRunner.calls:
		if got != HostCommandPoweroff {
			t.Fatalf("second runner command = %d, want %d", got, HostCommandPoweroff)
		}
	case <-time.After(time.Second):
		t.Fatal("second runner was not invoked after first command completed")
	}
	secondRunner.release()
	waitForHostStatus(t, helper, HostStatusErr)
	if got := helper.ExitCode(); got != HostHelperExitAptUpgradeFailed {
		t.Fatalf("exit code after second completion = %d, want %d", got, HostHelperExitAptUpgradeFailed)
	}
}

func TestNewHostHelperConfigInstallsRunnerOnlyWhenEnabled(t *testing.T) {
	disabled := NewHostHelper(HostHelperConfig{})
	if disabled.enabled {
		t.Fatal("default host helper should be disabled")
	}
	if disabled.runner != nil {
		t.Fatalf("disabled host helper runner = %#v, want nil", disabled.runner)
	}

	enabled := NewHostHelper(HostHelperConfig{
		Enabled:    true,
		Appliance:  true,
		HelperPath: "/tmp/ie-host-helper-test",
	})
	if !enabled.enabled {
		t.Fatal("enabled host helper should be enabled")
	}
	if !enabled.appliance {
		t.Fatal("enabled host helper should preserve appliance mode")
	}
	runner, ok := enabled.runner.(ExternalHostCommandRunner)
	if !ok {
		t.Fatalf("enabled host helper runner = %T, want ExternalHostCommandRunner", enabled.runner)
	}
	if runner.Path != "/tmp/ie-host-helper-test" {
		t.Fatalf("runner path = %q, want configured helper path", runner.Path)
	}
	if !runner.Appliance {
		t.Fatal("runner appliance mode = false, want true")
	}
}

func TestExternalHostCommandRunnerUsesFixedHelperArgv(t *testing.T) {
	dir := t.TempDir()
	helperPath := filepath.Join(dir, "helper")
	pkexecPath := filepath.Join(dir, "pkexec")
	argsPath := filepath.Join(dir, "args")
	helperScript := "#!/bin/sh\nexit 21\n"
	if err := os.WriteFile(helperPath, []byte(helperScript), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	pkexecScript := fmt.Sprintf("#!/bin/sh\nprintf '%%s|%%s|%%s|%%s' \"$0\" \"$1\" \"$2\" \"$#\" > %q\nexec \"$@\"\n", argsPath)
	if err := os.WriteFile(pkexecPath, []byte(pkexecScript), 0o755); err != nil {
		t.Fatalf("write pkexec: %v", err)
	}

	result := (ExternalHostCommandRunner{Path: helperPath, PkexecPath: pkexecPath}).RunHostCommand(context.Background(), HostCommandUpdate)
	if result.Status != HostStatusErr {
		t.Fatalf("status = %d, want ERR", result.Status)
	}
	if result.ExitCode != HostHelperExitAptUpgradeFailed {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, HostHelperExitAptUpgradeFailed)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read helper args: %v", err)
	}
	want := pkexecPath + "|" + helperPath + "|update|2"
	if string(args) != want {
		t.Fatalf("helper args = %q, want %q", args, want)
	}
}

func TestExternalHostCommandRunnerPassesApplianceArgForUpdateOnly(t *testing.T) {
	tests := []struct {
		name      string
		cmd       HostCommand
		appliance bool
		want      string
	}{
		{
			name:      "update appliance",
			cmd:       HostCommandUpdate,
			appliance: true,
			want:      "update|--appliance|3",
		},
		{
			name:      "update normal",
			cmd:       HostCommandUpdate,
			appliance: false,
			want:      "update||2",
		},
		{
			name:      "reboot appliance",
			cmd:       HostCommandReboot,
			appliance: true,
			want:      "reboot||2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			helperPath := filepath.Join(dir, "helper")
			pkexecPath := filepath.Join(dir, "pkexec")
			argsPath := filepath.Join(dir, "args")
			helperScript := "#!/bin/sh\nexit 0\n"
			if err := os.WriteFile(helperPath, []byte(helperScript), 0o755); err != nil {
				t.Fatalf("write helper: %v", err)
			}
			pkexecScript := fmt.Sprintf("#!/bin/sh\nprintf '%%s|%%s|%%s' \"$2\" \"$3\" \"$#\" > %q\nexec \"$@\"\n", argsPath)
			if err := os.WriteFile(pkexecPath, []byte(pkexecScript), 0o755); err != nil {
				t.Fatalf("write pkexec: %v", err)
			}

			result := (ExternalHostCommandRunner{
				Path:       helperPath,
				PkexecPath: pkexecPath,
				Appliance:  tt.appliance,
			}).RunHostCommand(context.Background(), tt.cmd)
			if result.Status != HostStatusOK {
				t.Fatalf("status = %d, want OK", result.Status)
			}
			if result.ExitCode != 0 {
				t.Fatalf("exit code = %d, want 0", result.ExitCode)
			}

			args, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatalf("read helper args: %v", err)
			}
			if string(args) != tt.want {
				t.Fatalf("helper args = %q, want %q", args, tt.want)
			}
		})
	}
}

func TestExternalHostCommandRunnerRejectsInvalidCommand(t *testing.T) {
	result := (ExternalHostCommandRunner{Path: "/no/such/helper"}).RunHostCommand(context.Background(), HostCommandNone)
	if result.Status != HostStatusErr {
		t.Fatalf("status = %d, want ERR", result.Status)
	}
	if result.ExitCode != HostHelperExitBadInput {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, HostHelperExitBadInput)
	}
}

func waitForHostStatus(t *testing.T, helper *HostHelper, want uint32) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := helper.Status(); got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("status = %d after timeout, want %d", helper.Status(), want)
}
