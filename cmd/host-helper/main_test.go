package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
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
				{Path: "/usr/bin/apt-get", Args: []string{"upgrade", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}},
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
				{Path: "/usr/bin/apt-get", Args: []string{"upgrade", "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}},
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

func TestNetworkManagerSecurityFlagMapping(t *testing.T) {
	tests := []struct {
		name     string
		apFlags  uint32
		wpaFlags uint32
		rsnFlags uint32
		want     string
	}{
		{name: "open", want: "open"},
		{name: "wpa psk", wpaFlags: nmAPSecKeyMgmtPSK, want: "wpa2-psk"},
		{name: "rsn psk", rsnFlags: nmAPSecKeyMgmtPSK, want: "wpa2-psk"},
		{name: "sae", rsnFlags: nmAPSecKeyMgmtSAE, want: "wpa3-sae"},
		{name: "8021x", rsnFlags: nmAPSecKeyMgmt8021X, want: string(wifiFailureUnsupportedAuth)},
		{name: "privacy only", apFlags: nmAPFlagPrivacy, want: string(wifiFailureUnsupportedAuth)},
		{name: "unknown secured", rsnFlags: 1, want: string(wifiFailureUnsupportedAuth)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := securityStringForNMFlags(tt.apFlags, tt.wpaFlags, tt.rsnFlags); got != tt.want {
				t.Fatalf("securityStringForNMFlags() = %q, want %q", got, tt.want)
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

func TestNetworkManagerDBusClientScansAndConnects(t *testing.T) {
	bus := newFakeNetworkManagerBus()
	client := networkManagerDBusClient{bus: bus}

	networks, err := client.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	wantNetworks := []wifiNetwork{
		{SSID: "Office", Signal: 92, Security: "wpa2-psk"},
		{SSID: "Café", Signal: 37, Security: "wpa3-sae"},
	}
	if !reflect.DeepEqual(networks, wantNetworks) {
		t.Fatalf("networks = %#v, want %#v", networks, wantNetworks)
	}

	result := client.Connect(context.Background(), "Café", []byte("secret"))
	if result.Err != nil {
		t.Fatalf("Connect() error = %v", result.Err)
	}
	if !bus.called(nmWirelessInterface + ".RequestScan") {
		t.Fatalf("NetworkManager RequestScan was not called: %#v", bus.calls)
	}
	call := bus.firstCall(nmInterface + ".AddAndActivateConnection2")
	if call == nil {
		t.Fatalf("AddAndActivateConnection2 was not called: %#v", bus.calls)
	}
	settings, ok := call.Args[0].(map[string]map[string]dbus.Variant)
	if !ok {
		t.Fatalf("settings arg = %T", call.Args[0])
	}
	if got := settings["802-11-wireless"]["ssid"].Value(); !reflect.DeepEqual(got, []byte("Café")) {
		t.Fatalf("ssid setting = %#v", got)
	}
	if got := settings["802-11-wireless-security"]["key-mgmt"].Value(); got != "sae" {
		t.Fatalf("key-mgmt = %#v, want sae", got)
	}
	if got := settings["802-11-wireless-security"]["psk"].Value(); got != "secret" {
		t.Fatalf("psk = %#v, want secret", got)
	}
	if got := call.Args[3].(map[string]dbus.Variant)["persist"].Value(); got != "volatile" {
		t.Fatalf("persist option = %#v, want volatile", got)
	}
}

func TestNetworkManagerDBusClientReturnsStateReasonOnDeactivation(t *testing.T) {
	bus := newFakeNetworkManagerBus()
	bus.properties["/org/freedesktop/NetworkManager/ActiveConnection/1"][nmActiveInterface+".State"] = dbus.MakeVariant(nmActiveStateDeactivated)
	bus.properties["/org/freedesktop/NetworkManager/ActiveConnection/1"][nmActiveInterface+".StateReason"] = dbus.MakeVariant([]uint32{
		nmActiveStateDeactivated,
		nmActiveReasonNoSecrets,
	})

	client := networkManagerDBusClient{bus: bus}
	result := client.Connect(context.Background(), "Office", []byte("wrong"))
	if result.Reason != wifiFailureBadPSK {
		t.Fatalf("Connect() reason = %q, want %q (err=%v)", result.Reason, wifiFailureBadPSK, result.Err)
	}
}

func TestNetworkManagerDBusClientRejectsPrivacyOnlyAP(t *testing.T) {
	bus := newFakeNetworkManagerBus()
	bus.properties["/org/freedesktop/NetworkManager/AccessPoint/0"][nmAPInterface+".Flags"] = dbus.MakeVariant(nmAPFlagPrivacy)
	bus.properties["/org/freedesktop/NetworkManager/AccessPoint/0"][nmAPInterface+".WpaFlags"] = dbus.MakeVariant(uint32(0))
	bus.properties["/org/freedesktop/NetworkManager/AccessPoint/0"][nmAPInterface+".RsnFlags"] = dbus.MakeVariant(uint32(0))

	client := networkManagerDBusClient{bus: bus}
	networks, err := client.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if networks[0].Security != string(wifiFailureUnsupportedAuth) {
		t.Fatalf("privacy-only AP security = %q, want unsupported-auth", networks[0].Security)
	}
	result := client.Connect(context.Background(), "Office", []byte("password"))
	if result.Reason != wifiFailureUnsupportedAuth {
		t.Fatalf("Connect() reason = %q, want unsupported-auth", result.Reason)
	}
}

func TestNetworkManagerDBusClientWaitsForScanCompletion(t *testing.T) {
	bus := newFakeNetworkManagerBus()
	bus.properties["/org/freedesktop/NetworkManager/Devices/0"][nmWirelessInterface+".LastScan"] = dbus.MakeVariant(int64(100))
	bus.scanCompletesAfterLastScanReads = 2

	client := networkManagerDBusClient{bus: bus}
	if _, err := client.Scan(context.Background()); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got := bus.propertyReads["/org/freedesktop/NetworkManager/Devices/0 "+nmWirelessInterface+".LastScan"]; got < 2 {
		t.Fatalf("LastScan reads = %d, want at least 2", got)
	}
	if !bus.called(nmWirelessInterface + ".GetAllAccessPoints") {
		t.Fatalf("GetAllAccessPoints was not called after scan completion: %#v", bus.calls)
	}
}

func TestNetworkManagerDBusClientRefreshesAPListBeforeConnect(t *testing.T) {
	bus := newFakeNetworkManagerBus()
	bus.hideAPsUntilScan = true

	client := networkManagerDBusClient{bus: bus}
	result := client.Connect(context.Background(), "Office", []byte("secret"))
	if result.Err != nil {
		t.Fatalf("Connect() error = %v", result.Err)
	}
	requestScan := bus.firstCallIndex(nmWirelessInterface + ".RequestScan")
	getAPs := bus.firstCallIndex(nmWirelessInterface + ".GetAllAccessPoints")
	if requestScan < 0 || getAPs < 0 {
		t.Fatalf("scan/AP calls missing: %#v", bus.calls)
	}
	if requestScan > getAPs {
		t.Fatalf("GetAllAccessPoints ran before RequestScan: %#v", bus.calls)
	}
}

func TestNetworkManagerDBusClientFallbackUsesUnsavedConnection(t *testing.T) {
	bus := newFakeNetworkManagerBus()
	bus.add2Unknown = true

	client := networkManagerDBusClient{bus: bus}
	result := client.Connect(context.Background(), "Office", []byte("secret"))
	if result.Err != nil {
		t.Fatalf("Connect() error = %v", result.Err)
	}
	if !bus.called(nmSettingsInterface + ".AddConnectionUnsaved") {
		t.Fatalf("fallback AddConnectionUnsaved was not called: %#v", bus.calls)
	}
	if !bus.called(nmInterface + ".ActivateConnection") {
		t.Fatalf("fallback ActivateConnection was not called: %#v", bus.calls)
	}
	if bus.called(nmInterface + ".AddAndActivateConnection") {
		t.Fatalf("fallback used persistent AddAndActivateConnection: %#v", bus.calls)
	}
	if bus.called("org.freedesktop.NetworkManager.Settings.Connection.Delete") {
		t.Fatalf("fallback deleted active settings connection before/during activation: %#v", bus.calls)
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

type fakeDBusCall struct {
	Method string
	Path   dbus.ObjectPath
	Args   []any
}

type fakeNetworkManagerBus struct {
	properties                      map[dbus.ObjectPath]map[string]dbus.Variant
	propertyReads                   map[string]int
	calls                           []fakeDBusCall
	add2Unknown                     bool
	hideAPsUntilScan                bool
	scanned                         bool
	scanCompletesAfterLastScanReads int
}

func newFakeNetworkManagerBus() *fakeNetworkManagerBus {
	return &fakeNetworkManagerBus{
		properties: map[dbus.ObjectPath]map[string]dbus.Variant{
			"/org/freedesktop/NetworkManager/Devices/0": {
				nmDeviceInterface + ".DeviceType": dbus.MakeVariant(nmDeviceTypeWiFi),
			},
			"/org/freedesktop/NetworkManager/AccessPoint/0": {
				nmAPInterface + ".Ssid":     dbus.MakeVariant([]byte("Office")),
				nmAPInterface + ".Strength": dbus.MakeVariant(byte(92)),
				nmAPInterface + ".WpaFlags": dbus.MakeVariant(nmAPSecKeyMgmtPSK),
				nmAPInterface + ".RsnFlags": dbus.MakeVariant(uint32(0)),
			},
			"/org/freedesktop/NetworkManager/AccessPoint/1": {
				nmAPInterface + ".Ssid":     dbus.MakeVariant([]byte("Café")),
				nmAPInterface + ".Strength": dbus.MakeVariant(byte(37)),
				nmAPInterface + ".WpaFlags": dbus.MakeVariant(uint32(0)),
				nmAPInterface + ".RsnFlags": dbus.MakeVariant(nmAPSecKeyMgmtSAE),
			},
			"/org/freedesktop/NetworkManager/ActiveConnection/1": {
				nmActiveInterface + ".State": dbus.MakeVariant(nmActiveStateActivated),
			},
		},
		propertyReads: map[string]int{},
	}
}

func (b *fakeNetworkManagerBus) Object(dest string, path dbus.ObjectPath) dbus.BusObject {
	return fakeDBusObject{bus: b, dest: dest, path: path}
}

func (b *fakeNetworkManagerBus) Close() error { return nil }

func (b *fakeNetworkManagerBus) called(method string) bool {
	return b.firstCall(method) != nil
}

func (b *fakeNetworkManagerBus) firstCall(method string) *fakeDBusCall {
	for i := range b.calls {
		if b.calls[i].Method == method {
			return &b.calls[i]
		}
	}
	return nil
}

func (b *fakeNetworkManagerBus) firstCallIndex(method string) int {
	for i := range b.calls {
		if b.calls[i].Method == method {
			return i
		}
	}
	return -1
}

type fakeDBusObject struct {
	bus  *fakeNetworkManagerBus
	dest string
	path dbus.ObjectPath
}

func (o fakeDBusObject) Call(method string, flags dbus.Flags, args ...any) *dbus.Call {
	return o.CallWithContext(context.Background(), method, flags, args...)
}

func (o fakeDBusObject) CallWithContext(_ context.Context, method string, _ dbus.Flags, args ...any) *dbus.Call {
	o.bus.calls = append(o.bus.calls, fakeDBusCall{Method: method, Path: o.path, Args: append([]any(nil), args...)})
	call := &dbus.Call{}
	switch method {
	case nmInterface + ".GetDevices":
		call.Body = []any{[]dbus.ObjectPath{"/org/freedesktop/NetworkManager/Devices/0"}}
	case nmWirelessInterface + ".GetAllAccessPoints":
		if o.bus.hideAPsUntilScan && !o.bus.scanned {
			call.Body = []any{[]dbus.ObjectPath{}}
			break
		}
		call.Body = []any{[]dbus.ObjectPath{
			"/org/freedesktop/NetworkManager/AccessPoint/0",
			"/org/freedesktop/NetworkManager/AccessPoint/1",
		}}
	case nmWirelessInterface + ".RequestScan":
		o.bus.scanned = true
	case nmInterface + ".AddAndActivateConnection2":
		if o.bus.add2Unknown {
			call.Err = errors.New("org.freedesktop.DBus.Error.UnknownMethod: unknown method")
			break
		}
		call.Body = []any{
			dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings/1"),
			dbus.ObjectPath("/org/freedesktop/NetworkManager/ActiveConnection/1"),
			map[string]dbus.Variant{},
		}
	case nmInterface + ".AddAndActivateConnection":
		call.Body = []any{
			dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings/1"),
			dbus.ObjectPath("/org/freedesktop/NetworkManager/ActiveConnection/1"),
		}
	case nmSettingsInterface + ".AddConnectionUnsaved":
		call.Body = []any{
			dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings/1"),
		}
	case nmInterface + ".ActivateConnection":
		call.Body = []any{
			dbus.ObjectPath("/org/freedesktop/NetworkManager/ActiveConnection/1"),
		}
	case "org.freedesktop.NetworkManager.Settings.Connection.Delete":
	default:
		call.Err = errors.New("unexpected DBus call " + method)
	}
	return call
}

func (o fakeDBusObject) Go(method string, flags dbus.Flags, ch chan *dbus.Call, args ...any) *dbus.Call {
	return o.GoWithContext(context.Background(), method, flags, ch, args...)
}

func (o fakeDBusObject) GoWithContext(ctx context.Context, method string, flags dbus.Flags, ch chan *dbus.Call, args ...any) *dbus.Call {
	call := o.CallWithContext(ctx, method, flags, args...)
	if ch != nil {
		ch <- call
	}
	return call
}

func (o fakeDBusObject) AddMatchSignal(string, string, ...dbus.MatchOption) *dbus.Call {
	return &dbus.Call{}
}

func (o fakeDBusObject) RemoveMatchSignal(string, string, ...dbus.MatchOption) *dbus.Call {
	return &dbus.Call{}
}

func (o fakeDBusObject) GetProperty(name string) (dbus.Variant, error) {
	key := string(o.path) + " " + name
	o.bus.propertyReads[key]++
	if name == nmWirelessInterface+".LastScan" && o.bus.scanCompletesAfterLastScanReads > 0 &&
		o.bus.propertyReads[key] >= o.bus.scanCompletesAfterLastScanReads {
		if o.bus.properties[o.path] == nil {
			o.bus.properties[o.path] = map[string]dbus.Variant{}
		}
		o.bus.properties[o.path][name] = dbus.MakeVariant(int64(200))
	}
	if props := o.bus.properties[o.path]; props != nil {
		if value, ok := props[name]; ok {
			return value, nil
		}
	}
	return dbus.Variant{}, errors.New("missing property " + name)
}

func (o fakeDBusObject) StoreProperty(name string, value any) error {
	return o.SetProperty(name, value)
}

func (o fakeDBusObject) SetProperty(name string, value any) error {
	variant, ok := value.(dbus.Variant)
	if !ok {
		variant = dbus.MakeVariant(value)
	}
	if o.bus.properties[o.path] == nil {
		o.bus.properties[o.path] = map[string]dbus.Variant{}
	}
	o.bus.properties[o.path][name] = variant
	return nil
}

func (o fakeDBusObject) Destination() string { return o.dest }
func (o fakeDBusObject) Path() dbus.ObjectPath {
	return o.path
}
