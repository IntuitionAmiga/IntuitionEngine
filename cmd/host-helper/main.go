package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

const (
	exitOK                  = 0
	exitRuntimeFailure      = 1
	exitBadInput            = 2
	exitWiFiBadPSK          = 10
	exitWiFiNoSignal        = 11
	exitWiFiUnsupportedAuth = 12
	exitWiFiTimeout         = 13
	exitAptUpdateFailed     = 20
	exitAptUpgradeFailed    = 21
)

const maxWiFiPasswordBytes = 64

type commandRunner interface {
	Run(ctx context.Context, path string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error
}

type commandOutputRunner interface {
	commandRunner
	Output(ctx context.Context, path string, args []string, stdin io.Reader, stderr io.Writer) ([]byte, error)
}

type networkManagerClient interface {
	Scan(ctx context.Context) ([]wifiNetwork, error)
	Connect(ctx context.Context, ssid string, password []byte) wifiConnectResult
}

type helperDeps struct {
	commands commandOutputRunner
	nm       networkManagerClient
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, path string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/bin", "LANG=C"}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (execCommandRunner) Output(ctx context.Context, path string, args []string, stdin io.Reader, stderr io.Writer) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/bin", "LANG=C"}
	cmd.Stdin = stdin
	cmd.Stderr = stderr
	return cmd.Output()
}

type nmcliNetworkManagerClient struct {
	commands commandOutputRunner
}

func (c nmcliNetworkManagerClient) Scan(ctx context.Context) ([]wifiNetwork, error) {
	output, err := c.commands.Output(ctx, "/usr/bin/nmcli", []string{
		"--terse", "--fields", "SSID,SIGNAL,SECURITY",
		"device", "wifi", "list", "--rescan", "yes",
	}, nil, io.Discard)
	if err != nil {
		return nil, err
	}
	return parseNMCLIWiFiList(output)
}

func (c nmcliNetworkManagerClient) Connect(ctx context.Context, ssid string, password []byte) wifiConnectResult {
	var stdin io.Reader
	var stderr bytes.Buffer
	args := []string{"--ask", "device", "wifi", "connect", ssid}
	if len(password) > 0 {
		stdin = bytes.NewReader(append(append([]byte(nil), password...), '\n'))
	}
	err := c.commands.Run(ctx, "/usr/bin/nmcli", args, stdin, io.Discard, &stderr)
	if err == nil {
		return wifiConnectResult{}
	}
	return wifiConnectResult{Reason: wifiFailureReasonForNetworkManagerReason(stderr.String() + " " + err.Error()), Err: err}
}

type wifiNetwork struct {
	SSID     string
	Signal   int
	Security string
}

type wifiFailureReason string

const (
	wifiFailureBadPSK          wifiFailureReason = "bad-psk"
	wifiFailureNoSignal        wifiFailureReason = "no-signal"
	wifiFailureUnsupportedAuth wifiFailureReason = "unsupported-auth"
	wifiFailureTimeout         wifiFailureReason = "timeout"
)

type wifiConnectResult struct {
	Reason wifiFailureReason
	Err    error
}

func main() {
	commands := execCommandRunner{}
	deps := helperDeps{
		commands: commands,
		nm:       nmcliNetworkManagerClient{commands: commands},
	}
	code := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, deps)
	log.Printf("intuitionengine-host-helper argv=%q exit=%d", os.Args[1:], code)
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, deps helperDeps) int {
	if deps.commands == nil {
		deps.commands = execCommandRunner{}
	}
	if deps.nm == nil {
		deps.nm = nmcliNetworkManagerClient{commands: deps.commands}
	}

	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing subcommand")
		return exitBadInput
	}

	switch args[0] {
	case "update":
		if len(args) != 1 && !(len(args) == 2 && args[1] == "--appliance") {
			fmt.Fprintln(stderr, "usage: host-helper update [--appliance]")
			return exitBadInput
		}
		return runUpdate(ctx, stdout, stderr, deps.commands)
	case "reboot":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: host-helper reboot")
			return exitBadInput
		}
		return runSystemctl(ctx, "reboot", stdout, stderr, deps.commands)
	case "poweroff":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: host-helper poweroff")
			return exitBadInput
		}
		return runSystemctl(ctx, "poweroff", stdout, stderr, deps.commands)
	case "net", "wifi-scan":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: host-helper wifi-scan")
			return exitBadInput
		}
		return runWiFiScan(ctx, stdout, stderr, deps.nm)
	case "wifi-connect":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: host-helper wifi-connect <ssid>")
			return exitBadInput
		}
		return runWiFiConnect(ctx, args[1], stdin, stderr, deps.nm)
	default:
		fmt.Fprintf(stderr, "unknown subcommand: %s\n", args[0])
		return exitBadInput
	}
}

func runUpdate(ctx context.Context, stdout io.Writer, stderr io.Writer, commands commandRunner) int {
	if err := commands.Run(ctx, "/usr/bin/apt-get", []string{"update"}, nil, stdout, stderr); err != nil {
		return exitAptUpdateFailed
	}
	if err := commands.Run(ctx, "/usr/bin/apt-get", []string{"upgrade", "-y"}, nil, stdout, stderr); err != nil {
		return exitAptUpgradeFailed
	}
	return exitOK
}

func runSystemctl(ctx context.Context, verb string, stdout io.Writer, stderr io.Writer, commands commandRunner) int {
	if err := commands.Run(ctx, "/usr/bin/systemctl", []string{verb}, nil, stdout, stderr); err != nil {
		return exitRuntimeFailure
	}
	return exitOK
}

func runWiFiScan(ctx context.Context, stdout io.Writer, stderr io.Writer, nm networkManagerClient) int {
	networks, err := nm.Scan(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "wifi scan failed: %v\n", err)
		return exitRuntimeFailure
	}
	for _, network := range networks {
		fmt.Fprintf(stdout, "%s\t%d\t%s\n",
			formatWiFiField(network.SSID),
			network.Signal,
			formatWiFiField(network.Security),
		)
	}
	return exitOK
}

func runWiFiConnect(ctx context.Context, ssid string, stdin io.Reader, stderr io.Writer, nm networkManagerClient) int {
	if !validSSID(ssid) {
		fmt.Fprintln(stderr, "invalid ssid")
		return exitBadInput
	}
	password, err := readPassword(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "invalid password: %v\n", err)
		return exitBadInput
	}
	result := nm.Connect(ctx, ssid, password)
	if result.Err == nil && result.Reason == "" {
		return exitOK
	}
	return exitCodeForWiFiFailure(result.Reason)
}

func validSSID(ssid string) bool {
	if ssid == "" || len([]byte(ssid)) > 32 {
		return false
	}
	for _, b := range []byte(ssid) {
		if b == 0 {
			return false
		}
	}
	return true
}

func readPassword(stdin io.Reader) ([]byte, error) {
	password, err := io.ReadAll(io.LimitReader(stdin, maxWiFiPasswordBytes+1))
	if err != nil {
		return nil, err
	}
	password = []byte(strings.TrimRight(string(password), "\r\n"))
	if len(password) > maxWiFiPasswordBytes {
		return nil, fmt.Errorf("too long")
	}
	for _, b := range password {
		if b == 0 {
			return nil, fmt.Errorf("contains NUL")
		}
	}
	return password, nil
}

func exitCodeForWiFiFailure(reason wifiFailureReason) int {
	switch reason {
	case wifiFailureBadPSK:
		return exitWiFiBadPSK
	case wifiFailureNoSignal:
		return exitWiFiNoSignal
	case wifiFailureUnsupportedAuth:
		return exitWiFiUnsupportedAuth
	case wifiFailureTimeout:
		return exitWiFiTimeout
	default:
		return exitRuntimeFailure
	}
}

func wifiFailureReasonForNetworkManagerReason(reason string) wifiFailureReason {
	reason = strings.ToUpper(strings.TrimSpace(reason))
	reason = strings.TrimPrefix(reason, "NM_DEVICE_STATE_REASON_")
	reason = strings.ReplaceAll(reason, "-", "_")
	reason = strings.ReplaceAll(reason, " ", "_")
	switch {
	case strings.Contains(reason, "8021X") || strings.Contains(reason, "UNSUPPORTED") || strings.Contains(reason, "SUPPLICANT_CONFIG_FAILED"):
		return wifiFailureUnsupportedAuth
	case strings.Contains(reason, "NO_SECRETS") || strings.Contains(reason, "NO_SECRET") || strings.Contains(reason, "SUPPLICANT_FAILED"):
		return wifiFailureBadPSK
	case strings.Contains(reason, "SSID_NOT_FOUND") || strings.Contains(reason, "NO_NETWORK_WITH_SSID"):
		return wifiFailureNoSignal
	case strings.Contains(reason, "TIMEOUT") || strings.Contains(reason, "TIMED_OUT"):
		return wifiFailureTimeout
	}
	switch reason {
	case "NO_SECRETS", "SUPPLICANT_FAILED":
		return wifiFailureBadPSK
	case "SSID_NOT_FOUND":
		return wifiFailureNoSignal
	case "SUPPLICANT_CONFIG_FAILED", "8021X_SUPPLICANT_FAILED", "8021X_SUPPLICANT_CONFIG_FAILED":
		return wifiFailureUnsupportedAuth
	case "TIMEOUT":
		return wifiFailureTimeout
	default:
		return ""
	}
}

func parseNMCLIWiFiList(output []byte) ([]wifiNetwork, error) {
	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	networks := make([]wifiNetwork, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := splitNMCLITerseLine(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid nmcli wifi row %q", line)
		}
		var signal int
		if _, err := fmt.Sscanf(fields[1], "%d", &signal); err != nil {
			return nil, fmt.Errorf("invalid nmcli signal %q", fields[1])
		}
		networks = append(networks, wifiNetwork{
			SSID:     fields[0],
			Signal:   signal,
			Security: fields[2],
		})
	}
	return networks, nil
}

func splitNMCLITerseLine(line string) []string {
	var fields []string
	var field strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			field.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == ':':
			fields = append(fields, field.String())
			field.Reset()
		default:
			field.WriteRune(r)
		}
	}
	if escaped {
		field.WriteRune('\\')
	}
	fields = append(fields, field.String())
	return fields
}

func formatWiFiField(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
