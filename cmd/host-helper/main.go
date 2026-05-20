package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
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
const defaultBrokerSocketPath = "/run/intuitionengine-host-helper.sock"
const maxBrokerOutputBytes = 4096

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

type dbusBus interface {
	Object(dest string, path dbus.ObjectPath) dbus.BusObject
	Close() error
}

type networkManagerDBusClient struct {
	bus dbusBus
}

type helperDeps struct {
	commands commandOutputRunner
	nm       networkManagerClient
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, path string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/bin", "LANG=C", "DEBIAN_FRONTEND=noninteractive"}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (execCommandRunner) Output(ctx context.Context, path string, args []string, stdin io.Reader, stderr io.Writer) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/bin", "LANG=C", "DEBIAN_FRONTEND=noninteractive"}
	cmd.Stdin = stdin
	cmd.Stderr = stderr
	return cmd.Output()
}

func newNetworkManagerClient() (networkManagerClient, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}
	return networkManagerDBusClient{bus: conn}, nil
}

const (
	nmDestination       = "org.freedesktop.NetworkManager"
	nmObjectPath        = dbus.ObjectPath("/org/freedesktop/NetworkManager")
	nmSettingsPath      = dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings")
	nmInterface         = "org.freedesktop.NetworkManager"
	nmSettingsInterface = "org.freedesktop.NetworkManager.Settings"
	nmDeviceInterface   = "org.freedesktop.NetworkManager.Device"
	nmWirelessInterface = "org.freedesktop.NetworkManager.Device.Wireless"
	nmAPInterface       = "org.freedesktop.NetworkManager.AccessPoint"
	nmActiveInterface   = "org.freedesktop.NetworkManager.Connection.Active"

	nmDeviceTypeWiFi             = uint32(2)
	nmActiveStateActivated       = uint32(2)
	nmActiveStateDeactivated     = uint32(4)
	nmActiveReasonNoSecrets      = uint32(9)
	nmActiveReasonLoginFailed    = uint32(10)
	nmActiveReasonConnectTimeout = uint32(6)
	nmAPFlagPrivacy              = uint32(0x1)
	nmAPSecKeyMgmtPSK            = uint32(0x100)
	nmAPSecKeyMgmt8021X          = uint32(0x200)
	nmAPSecKeyMgmtSAE            = uint32(0x400)
)

func (c networkManagerDBusClient) Scan(ctx context.Context) ([]wifiNetwork, error) {
	device, err := c.firstWiFiDevice(ctx)
	if err != nil {
		return nil, err
	}
	wireless := c.bus.Object(nmDestination, device)
	if err := c.requestScanAndWait(ctx, wireless); err != nil {
		return nil, err
	}

	var paths []dbus.ObjectPath
	if err := wireless.CallWithContext(ctx, nmWirelessInterface+".GetAllAccessPoints", 0).Store(&paths); err != nil {
		return nil, err
	}
	networks := make([]wifiNetwork, 0, len(paths))
	for _, path := range paths {
		ap := c.bus.Object(nmDestination, path)
		ssid, err := getDBusByteSliceProperty(ap, nmAPInterface+".Ssid")
		if err != nil || len(ssid) == 0 {
			continue
		}
		strength, _ := getDBusByteProperty(ap, nmAPInterface+".Strength")
		apFlags, _ := getDBusUint32Property(ap, nmAPInterface+".Flags")
		wpaFlags, _ := getDBusUint32Property(ap, nmAPInterface+".WpaFlags")
		rsnFlags, _ := getDBusUint32Property(ap, nmAPInterface+".RsnFlags")
		networks = append(networks, wifiNetwork{
			SSID:     string(ssid),
			Signal:   int(strength),
			Security: securityStringForNMFlags(apFlags, wpaFlags, rsnFlags),
		})
	}
	return networks, nil
}

func (c networkManagerDBusClient) Connect(ctx context.Context, ssid string, password []byte) wifiConnectResult {
	device, ap, security, err := c.findWiFiAccessPoint(ctx, ssid)
	if err != nil {
		return wifiConnectResult{Reason: wifiFailureReasonForNetworkManagerReason(err.Error()), Err: err}
	}
	if security == string(wifiFailureUnsupportedAuth) {
		return wifiConnectResult{Reason: wifiFailureUnsupportedAuth, Err: fmt.Errorf("unsupported authentication")}
	}
	settings := buildWiFiConnectionSettings(ssid, password, security)
	options := map[string]dbus.Variant{"persist": dbus.MakeVariant("volatile")}
	nm := c.bus.Object(nmDestination, nmObjectPath)
	var connPath dbus.ObjectPath
	var activePath dbus.ObjectPath
	call := nm.CallWithContext(ctx, nmInterface+".AddAndActivateConnection2", 0, settings, device, ap, options)
	if call.Err == nil {
		var result map[string]dbus.Variant
		if err := call.Store(&connPath, &activePath, &result); err != nil {
			return wifiConnectResult{Err: err}
		}
		return c.waitForActivation(ctx, activePath)
	}
	if !isUnknownDBusMethod(call.Err) {
		return wifiConnectResult{Reason: wifiFailureReasonForNetworkManagerReason(call.Err.Error()), Err: call.Err}
	}
	settingsObj := c.bus.Object(nmDestination, nmSettingsPath)
	call = settingsObj.CallWithContext(ctx, nmSettingsInterface+".AddConnectionUnsaved", 0, settings)
	if call.Err != nil {
		return wifiConnectResult{Reason: wifiFailureReasonForNetworkManagerReason(call.Err.Error()), Err: call.Err}
	}
	if err := call.Store(&connPath); err != nil {
		return wifiConnectResult{Err: err}
	}
	call = nm.CallWithContext(ctx, nmInterface+".ActivateConnection", 0, connPath, device, ap)
	if call.Err != nil {
		return wifiConnectResult{Reason: wifiFailureReasonForNetworkManagerReason(call.Err.Error()), Err: call.Err}
	}
	if err := call.Store(&activePath); err != nil {
		return wifiConnectResult{Err: err}
	}
	return c.waitForActivation(ctx, activePath)
}

func (c networkManagerDBusClient) requestScanAndWait(ctx context.Context, wireless dbus.BusObject) error {
	before, _ := getDBusInt64Property(wireless, nmWirelessInterface+".LastScan")
	if err := wireless.CallWithContext(ctx, nmWirelessInterface+".RequestScan", 0, map[string]dbus.Variant{}).Err; err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		lastScan, err := getDBusInt64Property(wireless, nmWirelessInterface+".LastScan")
		if err != nil || lastScan > before || before < 0 && lastScan >= 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("NetworkManager scan did not complete: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c networkManagerDBusClient) firstWiFiDevice(ctx context.Context) (dbus.ObjectPath, error) {
	devices, err := c.getDevices(ctx)
	if err != nil {
		return "", err
	}
	for _, path := range devices {
		deviceType, err := getDBusUint32Property(c.bus.Object(nmDestination, path), nmDeviceInterface+".DeviceType")
		if err == nil && deviceType == nmDeviceTypeWiFi {
			return path, nil
		}
	}
	return "", fmt.Errorf("no wifi device")
}

func (c networkManagerDBusClient) getDevices(ctx context.Context) ([]dbus.ObjectPath, error) {
	var devices []dbus.ObjectPath
	err := c.bus.Object(nmDestination, nmObjectPath).CallWithContext(ctx, nmInterface+".GetDevices", 0).Store(&devices)
	return devices, err
}

func (c networkManagerDBusClient) findWiFiAccessPoint(ctx context.Context, ssid string) (dbus.ObjectPath, dbus.ObjectPath, string, error) {
	device, err := c.firstWiFiDevice(ctx)
	if err != nil {
		return "", "", "", err
	}
	wireless := c.bus.Object(nmDestination, device)
	if err := c.requestScanAndWait(ctx, wireless); err != nil {
		return "", "", "", err
	}
	var paths []dbus.ObjectPath
	if err := wireless.CallWithContext(ctx, nmWirelessInterface+".GetAllAccessPoints", 0).Store(&paths); err != nil {
		return "", "", "", err
	}
	for _, path := range paths {
		ap := c.bus.Object(nmDestination, path)
		apSSID, err := getDBusByteSliceProperty(ap, nmAPInterface+".Ssid")
		if err != nil || string(apSSID) != ssid {
			continue
		}
		apFlags, _ := getDBusUint32Property(ap, nmAPInterface+".Flags")
		wpaFlags, _ := getDBusUint32Property(ap, nmAPInterface+".WpaFlags")
		rsnFlags, _ := getDBusUint32Property(ap, nmAPInterface+".RsnFlags")
		return device, path, securityStringForNMFlags(apFlags, wpaFlags, rsnFlags), nil
	}
	return "", "", "", fmt.Errorf("SSID_NOT_FOUND")
}

func (c networkManagerDBusClient) waitForActivation(ctx context.Context, activePath dbus.ObjectPath) wifiConnectResult {
	if activePath == "" || activePath == "/" {
		return wifiConnectResult{}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := getDBusUint32Property(c.bus.Object(nmDestination, activePath), nmActiveInterface+".State")
		if err == nil && state == nmActiveStateActivated {
			return wifiConnectResult{}
		}
		if err == nil && state == nmActiveStateDeactivated {
			reason, reasonErr := getActiveConnectionStateReason(c.bus.Object(nmDestination, activePath))
			if reasonErr != nil {
				return wifiConnectResult{Err: reasonErr}
			}
			return wifiConnectResult{Reason: wifiFailureReasonForNetworkManagerActiveReason(reason), Err: fmt.Errorf("NetworkManager activation failed: %d", reason)}
		}
		select {
		case <-ctx.Done():
			return wifiConnectResult{Reason: wifiFailureTimeout, Err: ctx.Err()}
		case <-ticker.C:
		}
	}
}

func buildWiFiConnectionSettings(ssid string, password []byte, security string) map[string]map[string]dbus.Variant {
	settings := map[string]map[string]dbus.Variant{
		"connection": {
			"type": dbus.MakeVariant("802-11-wireless"),
		},
		"802-11-wireless": {
			"mode": dbus.MakeVariant("infrastructure"),
			"ssid": dbus.MakeVariant([]byte(ssid)),
		},
	}
	if security != "open" {
		keyMgmt := "wpa-psk"
		if strings.Contains(security, "wpa3") {
			keyMgmt = "sae"
		}
		settings["802-11-wireless-security"] = map[string]dbus.Variant{
			"key-mgmt": dbus.MakeVariant(keyMgmt),
			"psk":      dbus.MakeVariant(string(password)),
		}
	}
	return settings
}

func securityStringForNMFlags(apFlags, wpaFlags, rsnFlags uint32) string {
	flags := wpaFlags | rsnFlags
	switch {
	case flags&nmAPSecKeyMgmt8021X != 0:
		return string(wifiFailureUnsupportedAuth)
	case flags&nmAPSecKeyMgmtSAE != 0:
		return "wpa3-sae"
	case flags&nmAPSecKeyMgmtPSK != 0:
		return "wpa2-psk"
	case flags != 0:
		return string(wifiFailureUnsupportedAuth)
	case apFlags&nmAPFlagPrivacy != 0:
		return string(wifiFailureUnsupportedAuth)
	default:
		return "open"
	}
}

func wifiFailureReasonForNetworkManagerActiveReason(reason uint32) wifiFailureReason {
	switch reason {
	case nmActiveReasonNoSecrets, nmActiveReasonLoginFailed:
		return wifiFailureBadPSK
	case nmActiveReasonConnectTimeout:
		return wifiFailureTimeout
	default:
		return ""
	}
}

func getDBusUint32Property(obj dbus.BusObject, name string) (uint32, error) {
	variant, err := obj.GetProperty(name)
	if err != nil {
		return 0, err
	}
	switch value := variant.Value().(type) {
	case uint32:
		return value, nil
	case byte:
		return uint32(value), nil
	default:
		return 0, fmt.Errorf("property %s has type %T", name, value)
	}
}

func getDBusInt64Property(obj dbus.BusObject, name string) (int64, error) {
	variant, err := obj.GetProperty(name)
	if err != nil {
		return 0, err
	}
	switch value := variant.Value().(type) {
	case int64:
		return value, nil
	case int32:
		return int64(value), nil
	case uint64:
		if value <= uint64(^uint64(0)>>1) {
			return int64(value), nil
		}
	case uint32:
		return int64(value), nil
	case int:
		return int64(value), nil
	}
	return 0, fmt.Errorf("property %s has type %T", name, variant.Value())
}

func getDBusByteProperty(obj dbus.BusObject, name string) (byte, error) {
	variant, err := obj.GetProperty(name)
	if err != nil {
		return 0, err
	}
	value, ok := variant.Value().(byte)
	if !ok {
		return 0, fmt.Errorf("property %s has type %T", name, variant.Value())
	}
	return value, nil
}

func getDBusByteSliceProperty(obj dbus.BusObject, name string) ([]byte, error) {
	variant, err := obj.GetProperty(name)
	if err != nil {
		return nil, err
	}
	switch value := variant.Value().(type) {
	case []byte:
		return value, nil
	case string:
		return []byte(value), nil
	default:
		return nil, fmt.Errorf("property %s has type %T", name, value)
	}
}

func getActiveConnectionStateReason(obj dbus.BusObject) (uint32, error) {
	variant, err := obj.GetProperty(nmActiveInterface + ".StateReason")
	if err != nil {
		return 0, err
	}
	value := variant.Value()
	switch reason := value.(type) {
	case []uint32:
		if len(reason) >= 2 {
			return reason[1], nil
		}
	case []interface{}:
		if len(reason) >= 2 {
			return uint32FromDBusValue(reason[1])
		}
	case struct {
		State  uint32
		Reason uint32
	}:
		return reason.Reason, nil
	}
	return 0, fmt.Errorf("StateReason has type %T", value)
}

func uint32FromDBusValue(value any) (uint32, error) {
	switch v := value.(type) {
	case uint32:
		return v, nil
	case byte:
		return uint32(v), nil
	case int:
		if v >= 0 {
			return uint32(v), nil
		}
	}
	return 0, fmt.Errorf("unexpected uint32 DBus value %T", value)
}

func isUnknownDBusMethod(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UnknownMethod") || strings.Contains(err.Error(), "Unknown method")
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
	nm, err := newNetworkManagerClient()
	if err != nil {
		log.Printf("NetworkManager DBus unavailable at startup: %v", err)
	}
	deps := helperDeps{
		commands: commands,
		nm:       nm,
	}
	args := os.Args[1:]
	var code int
	if len(args) > 0 && args[0] == "serve" {
		code = runBroker(context.Background(), args[1:], os.Stderr, deps)
	} else {
		code = run(context.Background(), args, os.Stdin, os.Stdout, os.Stderr, deps)
	}
	log.Printf("intuitionengine-host-helper argv=%q exit=%d", os.Args[1:], code)
	os.Exit(code)
}

type hostBrokerRequest struct {
	Args []string `json:"args"`
}

type hostBrokerResponse struct {
	Type     string `json:"type,omitempty"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output,omitempty"`
}

func runBroker(ctx context.Context, args []string, stderr io.Writer, deps helperDeps) int {
	socketPath := defaultBrokerSocketPath
	if len(args) == 2 && args[0] == "--socket" {
		socketPath = args[1]
	} else if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: host-helper serve [--socket PATH]")
		return exitBadInput
	}

	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintf(stderr, "listen %s: %v\n", socketPath, err)
		return exitRuntimeFailure
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(socketPath)
	}()

	if err := os.Chmod(socketPath, 0o660); err != nil {
		fmt.Fprintf(stderr, "chmod %s: %v\n", socketPath, err)
		return exitRuntimeFailure
	}
	group, err := user.LookupGroup("ie")
	if err != nil {
		fmt.Fprintf(stderr, "lookup ie group: %v\n", err)
		return exitRuntimeFailure
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		fmt.Fprintf(stderr, "parse ie gid %q: %v\n", group.Gid, err)
		return exitRuntimeFailure
	}
	if err := os.Chown(socketPath, 0, gid); err != nil {
		fmt.Fprintf(stderr, "chown %s: %v\n", socketPath, err)
		return exitRuntimeFailure
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return exitOK
			}
			fmt.Fprintf(stderr, "accept %s: %v\n", socketPath, err)
			return exitRuntimeFailure
		}
		handleBrokerConn(ctx, conn, deps)
	}
}

func handleBrokerConn(ctx context.Context, conn net.Conn, deps helperDeps) {
	defer conn.Close()

	var request hostBrokerRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(hostBrokerResponse{Type: "result", ExitCode: exitBadInput, Output: err.Error()})
		return
	}
	if !brokerArgsAllowed(request.Args) {
		_ = json.NewEncoder(conn).Encode(hostBrokerResponse{Type: "result", ExitCode: exitBadInput, Output: "disallowed host-helper command"})
		return
	}

	output := newBrokerOutput(conn, maxBrokerOutputBytes)
	exitCode := run(ctx, request.Args, nil, output, output, deps)
	_ = output.EncodeResult(exitCode)
}

func brokerArgsAllowed(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "net", "reboot", "poweroff":
		return len(args) == 1
	case "update":
		return len(args) == 1 || len(args) == 2 && args[1] == "--appliance"
	default:
		return false
	}
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

type brokerOutput struct {
	mu      sync.Mutex
	encoder *json.Encoder
	output  *boundedOutput
}

func newBrokerOutput(w io.Writer, limit int) *brokerOutput {
	return &brokerOutput{
		encoder: json.NewEncoder(w),
		output:  newBoundedOutput(limit),
	}
}

func (b *brokerOutput) Write(p []byte) (int, error) {
	if b.output != nil {
		_, _ = b.output.Write(p)
	}
	if len(p) == 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.encoder.Encode(hostBrokerResponse{Type: "output", Output: string(p)}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (b *brokerOutput) EncodeResult(exitCode int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.encoder.Encode(hostBrokerResponse{
		Type:     "result",
		ExitCode: exitCode,
		Output:   string(b.output.Bytes()),
	})
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, deps helperDeps) int {
	if deps.commands == nil {
		deps.commands = execCommandRunner{}
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
		if deps.nm == nil {
			nm, err := newNetworkManagerClient()
			if err != nil {
				fmt.Fprintf(stderr, "NetworkManager DBus unavailable: %v\n", err)
				return exitRuntimeFailure
			}
			deps.nm = nm
		}
		return runWiFiScan(ctx, stdout, stderr, deps.nm)
	case "wifi-connect":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: host-helper wifi-connect <ssid>")
			return exitBadInput
		}
		if deps.nm == nil {
			nm, err := newNetworkManagerClient()
			if err != nil {
				fmt.Fprintf(stderr, "NetworkManager DBus unavailable: %v\n", err)
				return exitRuntimeFailure
			}
			deps.nm = nm
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
	args := []string{
		"upgrade",
		"-y",
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold",
	}
	if err := commands.Run(ctx, "/usr/bin/apt-get", args, nil, stdout, stderr); err != nil {
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
		if strings.Contains(err.Error(), "no wifi device") {
			return exitOK
		}
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

func formatWiFiField(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
