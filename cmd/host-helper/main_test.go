package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestRunUpdateExitCodes(t *testing.T) {
	tests := []struct {
		name    string
		failAt  int
		want    int
		wantRun []recordedCommand
	}{
		{
			name: "success",
			want: exitOK,
			wantRun: []recordedCommand{
				{Path: "/usr/bin/apt-get", Args: []string{"update"}},
				{Path: "/usr/bin/apt-get", Args: []string{"upgrade", "-y"}},
			},
		},
		{
			name:   "apt update failure",
			failAt: 1,
			want:   exitAptUpdateFailed,
			wantRun: []recordedCommand{
				{Path: "/usr/bin/apt-get", Args: []string{"update"}},
			},
		},
		{
			name:   "apt upgrade failure",
			failAt: 2,
			want:   exitAptUpgradeFailed,
			wantRun: []recordedCommand{
				{Path: "/usr/bin/apt-get", Args: []string{"update"}},
				{Path: "/usr/bin/apt-get", Args: []string{"upgrade", "-y"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &recordingCommandRunner{failAt: tt.failAt}
			code := run(context.Background(), []string{"update"}, strings.NewReader(""), io.Discard, io.Discard, helperDeps{commands: runner})
			if code != tt.want {
				t.Fatalf("run(update) = %d, want %d", code, tt.want)
			}
			if !reflect.DeepEqual(runner.commands, tt.wantRun) {
				t.Fatalf("commands = %#v, want %#v", runner.commands, tt.wantRun)
			}
		})
	}
}

func TestRunWiFiConnectExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		result wifiConnectResult
		want   int
	}{
		{name: "success", want: exitOK},
		{name: "bad psk", result: wifiConnectResult{Reason: wifiFailureBadPSK, Err: errors.New("no secrets")}, want: exitWiFiBadPSK},
		{name: "ssid not found", result: wifiConnectResult{Reason: wifiFailureNoSignal, Err: errors.New("ssid not found")}, want: exitWiFiNoSignal},
		{name: "unsupported auth", result: wifiConnectResult{Reason: wifiFailureUnsupportedAuth, Err: errors.New("eap unsupported")}, want: exitWiFiUnsupportedAuth},
		{name: "timeout", result: wifiConnectResult{Reason: wifiFailureTimeout, Err: errors.New("timeout")}, want: exitWiFiTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nm := &scriptedNetworkManagerClient{connectResult: tt.result}
			code := run(context.Background(), []string{"wifi-connect", "Office"}, strings.NewReader("password"), io.Discard, io.Discard, helperDeps{nm: nm})
			if code != tt.want {
				t.Fatalf("run(wifi-connect) = %d, want %d", code, tt.want)
			}
			if nm.connectedSSID != "Office" {
				t.Fatalf("connectedSSID = %q, want Office", nm.connectedSSID)
			}
			if string(nm.connectedPassword) != "password" {
				t.Fatalf("connectedPassword = %q, want password", string(nm.connectedPassword))
			}
		})
	}
}

func TestRunNetAliasesWiFiScan(t *testing.T) {
	nm := &scriptedNetworkManagerClient{
		networks: []wifiNetwork{{SSID: "Office", Signal: 92, Security: "wpa2-psk"}},
	}
	var stdout bytes.Buffer
	code := run(context.Background(), []string{"net"}, strings.NewReader(""), &stdout, io.Discard, helperDeps{nm: nm})
	if code != exitOK {
		t.Fatalf("run(net) = %d, want %d", code, exitOK)
	}
	if stdout.String() != "Office\t92\twpa2-psk\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestNetworkManagerReasonMapping(t *testing.T) {
	tests := []struct {
		reason string
		want   wifiFailureReason
	}{
		{reason: "NM_DEVICE_STATE_REASON_NO_SECRETS", want: wifiFailureBadPSK},
		{reason: "supplicant_failed", want: wifiFailureBadPSK},
		{reason: "No secrets were provided", want: wifiFailureBadPSK},
		{reason: "SSID_NOT_FOUND", want: wifiFailureNoSignal},
		{reason: "SUPPLICANT_CONFIG_FAILED", want: wifiFailureUnsupportedAuth},
		{reason: "8021X_SUPPLICANT_FAILED", want: wifiFailureUnsupportedAuth},
		{reason: "TIMEOUT", want: wifiFailureTimeout},
		{reason: "CONNECTION_REMOVED", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			if got := wifiFailureReasonForNetworkManagerReason(tt.reason); got != tt.want {
				t.Fatalf("wifiFailureReasonForNetworkManagerReason(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestRunRejectsInvalidInputWithoutAction(t *testing.T) {
	tests := []struct {
		name string
		args []string
		in   string
	}{
		{name: "unknown subcommand", args: []string{"bogus"}},
		{name: "extra update arg", args: []string{"update", "--unexpected"}},
		{name: "missing ssid", args: []string{"wifi-connect"}},
		{name: "ssid too long", args: []string{"wifi-connect", strings.Repeat("a", 33)}, in: "password"},
		{name: "ssid nul", args: []string{"wifi-connect", "Office\x00Guest"}, in: "password"},
		{name: "password too long", args: []string{"wifi-connect", "Office"}, in: strings.Repeat("a", maxWiFiPasswordBytes+1)},
		{name: "password nul", args: []string{"wifi-connect", "Office"}, in: "abc\x00def"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &recordingCommandRunner{}
			nm := &scriptedNetworkManagerClient{}
			code := run(context.Background(), tt.args, strings.NewReader(tt.in), io.Discard, io.Discard, helperDeps{commands: runner, nm: nm})
			if code != exitBadInput {
				t.Fatalf("run(%v) = %d, want %d", tt.args, code, exitBadInput)
			}
			if len(runner.commands) != 0 {
				t.Fatalf("commands ran for invalid input: %#v", runner.commands)
			}
			if nm.connectCalls != 0 {
				t.Fatalf("wifi connect calls = %d, want 0", nm.connectCalls)
			}
		})
	}
}

func TestRunAcceptsValidSSIDBytesWithoutShellEscaping(t *testing.T) {
	tests := []string{
		"Bob's WiFi",
		"Research & Dev",
		"Café",
		"$(not a shell)",
		"Office:Lab",
	}

	for _, ssid := range tests {
		t.Run(ssid, func(t *testing.T) {
			nm := &scriptedNetworkManagerClient{}
			code := run(context.Background(), []string{"wifi-connect", ssid}, strings.NewReader("password"), io.Discard, io.Discard, helperDeps{nm: nm})
			if code != exitOK {
				t.Fatalf("run(wifi-connect %q) = %d, want %d", ssid, code, exitOK)
			}
			if nm.connectedSSID != ssid {
				t.Fatalf("connectedSSID = %q, want %q", nm.connectedSSID, ssid)
			}
		})
	}
}

func TestRunWiFiScanFormatsNetworkManagerOutput(t *testing.T) {
	nm := &scriptedNetworkManagerClient{
		networks: []wifiNetwork{
			{SSID: "Office", Signal: 92, Security: "wpa2-psk"},
			{SSID: " Lab\tNet ", Signal: 37, Security: "wpa3\nsae"},
		},
	}
	var stdout bytes.Buffer
	code := run(context.Background(), []string{"wifi-scan"}, strings.NewReader(""), &stdout, io.Discard, helperDeps{nm: nm})
	if code != exitOK {
		t.Fatalf("run(wifi-scan) = %d, want %d", code, exitOK)
	}
	want := "Office\t92\twpa2-psk\n Lab Net \t37\twpa3 sae\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestNMCLIClientScansAndConnectsThroughNetworkManager(t *testing.T) {
	runner := &recordingCommandRunner{
		output: []byte("Office\\:Lab:92:wpa2-psk\nCafé:37:wpa3-sae\n"),
	}
	client := nmcliNetworkManagerClient{commands: runner}

	networks, err := client.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	wantNetworks := []wifiNetwork{
		{SSID: "Office:Lab", Signal: 92, Security: "wpa2-psk"},
		{SSID: "Café", Signal: 37, Security: "wpa3-sae"},
	}
	if !reflect.DeepEqual(networks, wantNetworks) {
		t.Fatalf("networks = %#v, want %#v", networks, wantNetworks)
	}

	result := client.Connect(context.Background(), "Bob's WiFi", []byte("secret"))
	if result.Err != nil {
		t.Fatalf("Connect() error = %v", result.Err)
	}
	if len(runner.outputs) != 1 || runner.outputs[0].Path != "/usr/bin/nmcli" {
		t.Fatalf("scan output commands = %#v", runner.outputs)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("connect commands = %#v", runner.commands)
	}
	connect := runner.commands[0]
	wantArgs := []string{"--ask", "device", "wifi", "connect", "Bob's WiFi"}
	if connect.Path != "/usr/bin/nmcli" || !reflect.DeepEqual(connect.Args, wantArgs) {
		t.Fatalf("connect command = %#v, want path /usr/bin/nmcli args %#v", connect, wantArgs)
	}
	if connect.Stdin != "secret\n" {
		t.Fatalf("connect stdin = %q, want password on stdin", connect.Stdin)
	}
}

func TestRunAllowsRaw64ByteWPAKey(t *testing.T) {
	nm := &scriptedNetworkManagerClient{}
	password := strings.Repeat("a", maxWiFiPasswordBytes)
	code := run(context.Background(), []string{"wifi-connect", "Office"}, strings.NewReader(password), io.Discard, io.Discard, helperDeps{nm: nm})
	if code != exitOK {
		t.Fatalf("run(wifi-connect) = %d, want %d", code, exitOK)
	}
	if string(nm.connectedPassword) != password {
		t.Fatalf("password length = %d, want %d", len(nm.connectedPassword), maxWiFiPasswordBytes)
	}
}

type recordedCommand struct {
	Path  string
	Args  []string
	Stdin string
}

type recordingCommandRunner struct {
	failAt   int
	commands []recordedCommand
	outputs  []recordedCommand
	output   []byte
}

func (r *recordingCommandRunner) Run(_ context.Context, path string, args []string, stdin io.Reader, _ io.Writer, _ io.Writer) error {
	r.commands = append(r.commands, recordedCommand{Path: path, Args: append([]string(nil), args...), Stdin: readAllString(stdin)})
	if r.failAt > 0 && len(r.commands) == r.failAt {
		return errors.New("scripted command failure")
	}
	return nil
}

func (r *recordingCommandRunner) Output(_ context.Context, path string, args []string, stdin io.Reader, _ io.Writer) ([]byte, error) {
	r.outputs = append(r.outputs, recordedCommand{Path: path, Args: append([]string(nil), args...), Stdin: readAllString(stdin)})
	return append([]byte(nil), r.output...), nil
}

func readAllString(r io.Reader) string {
	if r == nil {
		return ""
	}
	data, _ := io.ReadAll(r)
	return string(data)
}

type scriptedNetworkManagerClient struct {
	networks          []wifiNetwork
	scanErr           error
	connectResult     wifiConnectResult
	connectCalls      int
	connectedSSID     string
	connectedPassword []byte
}

func (c *scriptedNetworkManagerClient) Scan(context.Context) ([]wifiNetwork, error) {
	return c.networks, c.scanErr
}

func (c *scriptedNetworkManagerClient) Connect(_ context.Context, ssid string, password []byte) wifiConnectResult {
	c.connectCalls++
	c.connectedSSID = ssid
	c.connectedPassword = append([]byte(nil), password...)
	return c.connectResult
}
