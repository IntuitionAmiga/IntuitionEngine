package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestHostIOTraceFlags(t *testing.T) {
	previousEnabled := hostIOTraceEnabled
	previousFile := hostIOTraceFile
	t.Cleanup(func() {
		hostIOTraceEnabled = previousEnabled
		hostIOTraceFile = previousFile
	})

	hostIOTraceEnabled = false
	hostIOTraceFile = ""

	flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	registerHostIOTraceFlags(flagSet)

	if err := flagSet.Parse([]string{
		"-trace-host-io",
		"-trace-host-io-file", "/tmp/ie-hostio.log",
	}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if !hostIOTraceEnabled {
		t.Fatal("hostIOTraceEnabled = false, want true")
	}
	if hostIOTraceFile != "/tmp/ie-hostio.log" {
		t.Fatalf("hostIOTraceFile = %q, want /tmp/ie-hostio.log", hostIOTraceFile)
	}
}

func TestHostIOTraceFileFlagEnablesTracing(t *testing.T) {
	previousEnabled := hostIOTraceEnabled
	previousFile := hostIOTraceFile
	previousEnvEnabled, hadEnvEnabled := os.LookupEnv("IE_TRACE_HOSTIO")
	previousEnvFile, hadEnvFile := os.LookupEnv("IE_TRACE_HOSTIO_FILE")
	t.Cleanup(func() {
		hostIOTraceEnabled = previousEnabled
		hostIOTraceFile = previousFile
		if hadEnvEnabled {
			_ = os.Setenv("IE_TRACE_HOSTIO", previousEnvEnabled)
		} else {
			_ = os.Unsetenv("IE_TRACE_HOSTIO")
		}
		if hadEnvFile {
			_ = os.Setenv("IE_TRACE_HOSTIO_FILE", previousEnvFile)
		} else {
			_ = os.Unsetenv("IE_TRACE_HOSTIO_FILE")
		}
	})

	_ = os.Unsetenv("IE_TRACE_HOSTIO")
	_ = os.Unsetenv("IE_TRACE_HOSTIO_FILE")
	hostIOTraceEnabled = false
	hostIOTraceFile = filepath.Join(t.TempDir(), "hostio.log")

	traceHostIO("FILEIO", "READ", "guest.dat", "/tmp/guest.dat", nil, 12)

	got, err := os.ReadFile(hostIOTraceFile)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", hostIOTraceFile, err)
	}
	if !strings.Contains(string(got), `FILEIO READ "guest.dat" -> "/tmp/guest.dat" size=12`) {
		t.Fatalf("trace log = %q, want FILEIO READ trace", got)
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
