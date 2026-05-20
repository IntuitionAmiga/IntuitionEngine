package main

import (
	"strings"
	"testing"
)

func TestHostHelperLiveImageSecurityContracts(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`useradd -m -u 1000 -g 1000 -s /bin/bash -G video,audio,input,render,seat,tty ie`,
		`for n in $(seq 1 12); do systemctl mask "getty@tty${n}.service"; done`,
		`NAutoVTs=0`,
		`ReserveVT=0`,
		`ExecStart=/usr/bin/loadkeys /usr/local/share/kbd/keymaps/ie-no-vt-switch.map`,
		`<annotate key="org.freedesktop.policykit.exec.path">/usr/libexec/intuitionengine-host-helper</annotate>`,
		`action.id === "org.intuitionengine.host.run"`,
		`subject.user === "ie"`,
		`chmod 0755 /usr/libexec/intuitionengine-host-helper`,
		`ExecStart=/usr/libexec/intuitionengine-host-helper serve`,
		`/run/intuitionengine-host-helper.sock rw,`,
		`/usr/bin/pkexec ix,`,
		`/usr/libexec/intuitionengine-host-helper Px,`,
		`profile opt.ie.IntuitionEngine /opt/ie/IntuitionEngine flags=(attach_disconnected)`,
		`profile usr.libexec.intuitionengine-host-helper /usr/libexec/intuitionengine-host-helper flags=(attach_disconnected)`,
		`network unix stream,`,
		`/var/ie/share/ rw,`,
		`/tmp/tmp*/** rwk,`,
		`/etc/shells r,`,
		`/home/ie/.config/pulse/ rw,`,
		`/home/ie/.config/pulse/** rwk,`,
		`/run/dbus/system_bus_socket rw,`,
		`dbus send`,
		`peer=(name=org.freedesktop.NetworkManager),`,
		`/usr/bin/apt-get Ux,`,
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
		`subject.active === true`,
		`subject.local === true`,
		`chown 1000:1000 /opt/ie`,
		`chown -R 1000:1000 /var/ie /opt/ie`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("live image security contract contains forbidden pattern %q", forbidden)
		}
	}
}

func TestHostHelperAptUsesUnconfinedTransition(t *testing.T) {
	body := readX64LiveScript(t)
	if !strings.Contains(body, `/usr/bin/apt-get Ux,`) {
		t.Fatal("host helper AppArmor profile must run apt-get with Ux")
	}
	if strings.Contains(body, `profile apt `) {
		t.Fatal("host helper AppArmor profile must not use a confined apt child profile")
	}
}
