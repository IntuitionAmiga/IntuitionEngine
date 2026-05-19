package main

import (
	"flag"
	"io"
	"testing"
)

func TestHostHelperApplianceFlagThreadsIntoHelper(t *testing.T) {
	var config hostHelperFlagConfig
	flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	registerHostHelperFlags(flagSet, &config)

	if err := flagSet.Parse([]string{
		"-ehbasic-host",
		"-ehbasic-host-appliance",
		"-ehbasic-host-helper", "/tmp/test-host-helper",
	}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	helper := NewHostHelper(config.HostHelperConfig())
	if !helper.enabled {
		t.Fatal("helper.enabled = false, want true")
	}
	if !helper.appliance {
		t.Fatal("helper.appliance = false, want true")
	}

	runner, ok := helper.runner.(ExternalHostCommandRunner)
	if !ok {
		t.Fatalf("helper.runner = %T, want ExternalHostCommandRunner", helper.runner)
	}
	if runner.Path != "/tmp/test-host-helper" {
		t.Fatalf("runner.Path = %q, want %q", runner.Path, "/tmp/test-host-helper")
	}
}

func TestInstallHostHelperUpdateConfirmerForInteractiveHost(t *testing.T) {
	term := NewTerminalMMIO()
	helper := NewHostHelper(HostHelperConfig{Enabled: true})
	confirmer := installHostHelperUpdateConfirmer(helper, hostHelperFlagConfig{enabled: true}, term)
	if confirmer == nil {
		t.Fatal("interactive host helper did not install an update confirmer")
	}
	if helper.confirmer != confirmer {
		t.Fatal("helper confirmer was not wired to the installed confirmer")
	}

	appliance := NewHostHelper(HostHelperConfig{Enabled: true, Appliance: true})
	if got := installHostHelperUpdateConfirmer(appliance, hostHelperFlagConfig{enabled: true, appliance: true}, term); got != nil {
		t.Fatal("appliance host helper installed an update confirmer")
	}
	if appliance.confirmer != nil {
		t.Fatal("appliance helper confirmer should remain nil")
	}
}
