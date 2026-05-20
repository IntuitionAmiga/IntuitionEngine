package main

import (
	"os"
	"strings"
	"testing"
)

func TestHostHelperLiveImageSecurityContracts(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`useradd -m -u 1000 -g 1000 -s /bin/bash -G video,audio,input,render,seat,tty ie`,
		`systemctl mask getty@tty1.service getty@tty2.service`,
		`<annotate key="org.freedesktop.policykit.exec.path">/usr/libexec/intuitionengine-host-helper</annotate>`,
		`action.id === "org.intuitionengine.host.run"`,
		`subject.user === "ie"`,
		`/usr/bin/pkexec ix,`,
		`/usr/libexec/intuitionengine-host-helper Px,`,
		`profile usr.libexec.intuitionengine-host-helper /usr/libexec/intuitionengine-host-helper flags=(attach_disconnected)`,
		`/run/dbus/system_bus_socket rw,`,
		`dbus send`,
		`peer=(name=org.freedesktop.NetworkManager),`,
		`/usr/bin/apt-get Cx -> apt,`,
		`/usr/bin/systemctl Cx -> systemctl,`,
		`Before=greetd.service`,
		`ufw default deny incoming`,
		`ufw default allow outgoing`,
		`ufw --force enable`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("live image security contract missing %q", want)
		}
	}

	for _, forbidden := range []string{
		`-G video,audio,input,render,seat,tty,netdev`,
		`usermod -aG netdev ie`,
		`usermod -aG sudo ie`,
		`systemctl enable getty@tty2.service`,
		`/usr/bin/nmcli`,
		`pkexec /bin/bash`,
		`chown 1000:1000 /opt/ie`,
		`chown -R 1000:1000 /var/ie /opt/ie`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("live image security contract contains forbidden pattern %q", forbidden)
		}
	}
}

func TestHostHelperAptProfileCoversEmpiricalExecAllowlist(t *testing.T) {
	body := readX64LiveScript(t)
	allowlist, err := os.ReadFile("cmd/host-helper/testdata/apt_child_exec_allowlist.txt")
	if err != nil {
		t.Fatalf("read apt allowlist fixture: %v", err)
	}
	for _, line := range strings.Split(string(allowlist), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(body, line) {
			t.Fatalf("AppArmor apt profile missing empirical exec allowlist entry %q", line)
		}
	}
}
