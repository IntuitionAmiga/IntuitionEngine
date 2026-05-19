package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

type scriptedHostUpdateConfirmer struct {
	result bool
	calls  chan struct{}
}

func newScriptedHostUpdateConfirmer(result bool) *scriptedHostUpdateConfirmer {
	return &scriptedHostUpdateConfirmer{
		result: result,
		calls:  make(chan struct{}, 4),
	}
}

func (c *scriptedHostUpdateConfirmer) ConfirmHostUpdate(ctx context.Context) bool {
	c.calls <- struct{}{}
	return c.result
}

func TestHostUpdateConfirmationInput(t *testing.T) {
	tests := []struct {
		name      string
		key       HostUpdateConfirmationKey
		wantDone  bool
		wantAllow bool
	}{
		{name: "yes", key: HostUpdateConfirmationKeyY, wantDone: true, wantAllow: true},
		{name: "no", key: HostUpdateConfirmationKeyN, wantDone: true, wantAllow: false},
		{name: "escape", key: HostUpdateConfirmationKeyEscape, wantDone: true, wantAllow: false},
		{name: "ignored", key: HostUpdateConfirmationKeyNone, wantDone: false, wantAllow: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirm := NewHostUpdateConfirmation()
			action := confirm.HandleInput(tt.key)
			if action.Done != tt.wantDone {
				t.Fatalf("done = %v, want %v", action.Done, tt.wantDone)
			}
			if action.Allow != tt.wantAllow {
				t.Fatalf("allow = %v, want %v", action.Allow, tt.wantAllow)
			}
			if confirm.Done() != tt.wantDone {
				t.Fatalf("confirmation done = %v, want %v", confirm.Done(), tt.wantDone)
			}
		})
	}
}

func TestHostUpdateConfirmationDefaultsToCancelWhenUnanswered(t *testing.T) {
	confirm := NewHostUpdateConfirmation()
	if confirm.ConfirmHostUpdate(context.Background()) {
		t.Fatal("unanswered confirmation allowed update")
	}
}

func TestTerminalHostUpdateConfirmerAcceptsHostKey(t *testing.T) {
	term := NewTerminalMMIO()
	confirm := NewTerminalHostUpdateConfirmer(term)
	result := make(chan bool, 1)

	go func() {
		result <- confirm.ConfirmHostUpdate(context.Background())
	}()

	deadline := time.After(time.Second)
	for {
		if strings.Contains(term.DrainOutput(), "Proceed?") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("confirmation prompt was not written")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	term.RouteHostKey('Y')

	select {
	case allowed := <-result:
		if !allowed {
			t.Fatal("confirmation rejected Y input")
		}
	case <-time.After(time.Second):
		t.Fatal("confirmation did not complete after Y input")
	}
	if got := term.HandleRead(TERM_IN); got != 0 {
		t.Fatalf("confirmation key leaked to guest input: %#x", got)
	}
}

func TestNewHostHelperDoesNotInstallUnanswerableUpdateConfirmer(t *testing.T) {
	helper := NewHostHelper(HostHelperConfig{
		Enabled:    true,
		Appliance:  false,
		HelperPath: "/tmp/ie-host-helper-test",
	})
	if helper.confirmer != nil {
		t.Fatal("non-appliance host helper installed an unanswerable update confirmer")
	}

	appliance := NewHostHelper(HostHelperConfig{
		Enabled:    true,
		Appliance:  true,
		HelperPath: "/tmp/ie-host-helper-test",
	})
	if appliance.confirmer != nil {
		t.Fatal("appliance host helper installed an update confirmer")
	}
}

func TestNewHostHelperUpdateFailsClosedWhenNoConfirmerIsInstalled(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK})
	helper := NewHostHelper(HostHelperConfig{Enabled: true})
	helper.runner = runner
	helper.SetCommand(HostCommandUpdate)

	helper.Trigger()

	waitForHostStatus(t, helper, HostStatusUserCancel)
	if got := helper.ExitCode(); got != 0 {
		t.Fatalf("exit code = %d, want 0", got)
	}
	select {
	case got := <-runner.calls:
		t.Fatalf("runner invoked without confirmation with command %d", got)
	default:
	}
}

func TestHostHelperUpdateConfirmationAllowsRunner(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK})
	confirmer := newScriptedHostUpdateConfirmer(true)
	helper := NewHostHelperWithRunner(true, false, runner)
	helper.SetUpdateConfirmer(confirmer)
	helper.SetCommand(HostCommandUpdate)

	helper.Trigger()

	select {
	case <-confirmer.calls:
	case <-time.After(time.Second):
		t.Fatal("confirmation was not requested")
	}
	select {
	case got := <-runner.calls:
		if got != HostCommandUpdate {
			t.Fatalf("runner command = %d, want %d", got, HostCommandUpdate)
		}
	case <-time.After(time.Second):
		t.Fatal("runner was not invoked after confirmation")
	}
	runner.release()
	waitForHostStatus(t, helper, HostStatusOK)
}

func TestHostHelperUpdateConfirmationCancelSkipsRunner(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK})
	confirmer := newScriptedHostUpdateConfirmer(false)
	helper := NewHostHelperWithRunner(true, false, runner)
	helper.SetUpdateConfirmer(confirmer)
	helper.SetCommand(HostCommandUpdate)

	helper.Trigger()

	select {
	case <-confirmer.calls:
	case <-time.After(time.Second):
		t.Fatal("confirmation was not requested")
	}
	waitForHostStatus(t, helper, HostStatusUserCancel)
	if got := helper.ExitCode(); got != 0 {
		t.Fatalf("exit code = %d, want 0", got)
	}
	select {
	case got := <-runner.calls:
		t.Fatalf("runner invoked after cancellation with command %d", got)
	default:
	}
}

func TestHostHelperUpdateConfirmationApplianceBypassesPrompt(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK})
	confirmer := newScriptedHostUpdateConfirmer(false)
	helper := NewHostHelperWithRunner(true, true, runner)
	helper.SetUpdateConfirmer(confirmer)
	helper.SetCommand(HostCommandUpdate)

	helper.Trigger()

	select {
	case <-confirmer.calls:
		t.Fatal("confirmation requested in appliance mode")
	default:
	}
	select {
	case got := <-runner.calls:
		if got != HostCommandUpdate {
			t.Fatalf("runner command = %d, want %d", got, HostCommandUpdate)
		}
	case <-time.After(time.Second):
		t.Fatal("runner was not invoked in appliance mode")
	}
	runner.release()
	waitForHostStatus(t, helper, HostStatusOK)
}

func TestHostHelperUpdateConfirmationNotUsedForOtherCommands(t *testing.T) {
	runner := newScriptedHostCommandRunner(HostCommandResult{Status: HostStatusOK})
	confirmer := newScriptedHostUpdateConfirmer(false)
	helper := NewHostHelperWithRunner(true, false, runner)
	helper.SetUpdateConfirmer(confirmer)
	helper.SetCommand(HostCommandReboot)

	helper.Trigger()

	select {
	case <-confirmer.calls:
		t.Fatal("confirmation requested for non-update command")
	default:
	}
	select {
	case got := <-runner.calls:
		if got != HostCommandReboot {
			t.Fatalf("runner command = %d, want %d", got, HostCommandReboot)
		}
	case <-time.After(time.Second):
		t.Fatal("runner was not invoked for non-update command")
	}
	runner.release()
	waitForHostStatus(t, helper, HostStatusOK)
}
