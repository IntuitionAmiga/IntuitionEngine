package main

import (
	"context"
	"errors"
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
			result:     HostCommandResult{Status: HostStatusErr, ExitCode: 20, Err: errors.New("update failed")},
			wantStatus: HostStatusErr,
			wantExit:   20,
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

	secondRunner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusErr, ExitCode: 21})
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
	if got := helper.ExitCode(); got != 21 {
		t.Fatalf("exit code after second completion = %d, want 21", got)
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
