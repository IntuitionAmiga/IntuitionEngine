package main

import "testing"

func TestHostHelperExitStatusText(t *testing.T) {
	tests := []struct {
		name string
		code uint32
		want string
	}{
		{name: "success", code: HostHelperExitOK, want: "OK"},
		{name: "bad input", code: HostHelperExitBadInput, want: "Failed: bad input"},
		{name: "bad password", code: HostHelperExitWiFiBadPSK, want: "Failed: bad password"},
		{name: "no signal", code: HostHelperExitWiFiNoSignal, want: "Failed: no signal"},
		{name: "unsupported auth", code: HostHelperExitWiFiUnsupportedAuth, want: "Failed: unsupported authentication"},
		{name: "timeout", code: HostHelperExitWiFiTimeout, want: "Failed: timeout"},
		{name: "apt update", code: HostHelperExitAptUpdateFailed, want: "Failed: apt-get update"},
		{name: "apt upgrade", code: HostHelperExitAptUpgradeFailed, want: "Failed: apt-get upgrade"},
		{name: "unknown", code: 99, want: "Failed: exit 99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HostHelperExitStatusText(tt.code); got != tt.want {
				t.Fatalf("HostHelperExitStatusText(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestHostHelperWiFiJoinStatusText(t *testing.T) {
	tests := []struct {
		name string
		code uint32
		ip   string
		want string
	}{
		{name: "connected with ip", code: HostHelperExitOK, ip: "192.0.2.15", want: "Connected (192.0.2.15)"},
		{name: "connected without ip", code: HostHelperExitOK, want: "Connected"},
		{name: "bad password", code: HostHelperExitWiFiBadPSK, want: "Failed: bad password"},
		{name: "no signal", code: HostHelperExitWiFiNoSignal, want: "Failed: no signal"},
		{name: "unsupported auth", code: HostHelperExitWiFiUnsupportedAuth, want: "Failed: unsupported authentication"},
		{name: "timeout", code: HostHelperExitWiFiTimeout, want: "Failed: timeout"},
		{name: "unknown", code: 42, want: "Failed: exit 42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HostHelperWiFiJoinStatusText(tt.code, tt.ip); got != tt.want {
				t.Fatalf("HostHelperWiFiJoinStatusText(%d, %q) = %q, want %q", tt.code, tt.ip, got, tt.want)
			}
		})
	}
}
