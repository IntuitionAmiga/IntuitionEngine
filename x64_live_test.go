package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestX64LiveMakefileTargets(t *testing.T) {
	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	text := string(makefile)

	for _, want := range []string{
		".PHONY: x86-64-v3",
		".PHONY: x64-live-embed-assets",
		".PHONY: x64-live",
		".PHONY: x64-live-rebuild-golden",
		".PHONY: x64-live-qemu",
		"X64_LIVE_DIR ?= build/x64-live",
		"X64_LIVE_IMG ?= $(X64_LIVE_DIR)/intuition-engine-x64.img",
		"GOOS=linux GOARCH=amd64 GOAMD64=v3 CGO_ENABLED=1",
		"$(GO) build $(GO_FLAGS) -trimpath -pgo=default.pgo",
		"-tags \"$(VM_EMBED_TAGS)\"",
		"-o $(BIN_DIR)/IntuitionEngine_v3 .",
		"x86-64-v3: x64-live-embed-assets",
		"x64-live-embed-assets: sdk-build emutos-release-rom iewarp-runtime-assets intuitionos",
		`test -f "$(EMUTOS_ROM)"`,
		`test -f "$(AROS_ROM)"`,
		`test -f "sdk/examples/prebuilt/ehbasic_ie64.ie64"`,
		"x64-live: x86-64-v3",
		"x64-live-demos",
		`X64_LIVE_OUT_DIR="$(X64_LIVE_DIR)" AROS_RELEASE_DIR="$(AROS_RELEASE_DIR)" ./build_x64_ie_img.sh`,
		"x64-live-rebuild-golden: x86-64-v3",
		`X64_LIVE_OUT_DIR="$(X64_LIVE_DIR)" AROS_RELEASE_DIR="$(AROS_RELEASE_DIR)" ./build_x64_ie_img.sh --rebuild-golden`,
		".PHONY: x64-live-payload-check",
		"x64-live-qemu: $(X64_LIVE_IMG)",
		"$(X64_LIVE_IMG):",
		"OVMF_CODE ?=",
		"qemu-system-x86_64",
		"-cpu host",
		"-bios $(OVMF_CODE)",
		"-drive file=$(X64_LIVE_IMG),format=raw,if=virtio",
		"-audiodev pipewire,id=snd0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Makefile missing %q", want)
		}
	}
}

func TestX64LiveDemoPayloadTargets(t *testing.T) {
	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	text := string(makefile)

	for _, want := range []string{
		".PHONY: x64-live-demos",
		"x64-live-demos: x64-live-payload-check",
		".PHONY: x64-live-payload-check",
		"x64-live-payload-check: x86-64-v3 sdk-build gem-rotozoomer aros-iewarp-library iewarp-runtime-assets x64-live-aros-demos x64-live-ab3d2-assets x64-live-refman-pdfs intuitionos",
		`X64_LIVE_OUT_DIR="$(X64_LIVE_DIR)" AROS_RELEASE_DIR="$(AROS_RELEASE_DIR)" ./build_x64_ie_img.sh --check-payload`,
		".PHONY: aros-iewarp-library",
		"aros-iewarp-library: aros-release-assets",
		`$(MAKE) -C "$(AROS_BUILD_DIR)" kernel-iewarp`,
		`grep -a -q 'Systems/AROS/Libs/iewarp_service.ie64' "$$lib"`,
		".PHONY: iewarp-runtime-assets",
		"iewarp-runtime-assets: sdk-build aros-iewarp-library",
		`cp -f sdk/examples/prebuilt/iewarp_service.ie64 Systems/AROS/Libs/iewarp_service.ie64`,
		`cp -f sdk/examples/prebuilt/iewarp_service.ie64 "$(AROS_RELEASE_DIR)/Libs/iewarp_service.ie64"`,
		".PHONY: x64-live-ab3d2-assets",
		`$(AB3D2_EMBED_ZIP)`,
		`$(AB3D2_EMBED_FILE)`,
		`Using existing AB3D2 embedded assets: $(AB3D2_EMBED_DIR)`,
		`if [ -d "$(AB3D2_SOURCE_DIR)" ] && [ -n "$$(find "$(AB3D2_SOURCE_DIR)" -maxdepth 1 -type f -name 'ab3d2_*.ie68' -print -quit)" ]; then`,
		`rm -f "$(AB3D2_EMBED_DIR)"/ab3d2_*.ie68`,
		`cp "$(AB3D2_SOURCE_DIR)"/ab3d2_*.ie68 "$(AB3D2_EMBED_DIR)/"`,
		`missing cached AB3D2 embedded assets: $(AB3D2_EMBED_DIR)`,
		`if [ -f "$(AB3D2_EMBED_ZIP)" ]; then`,
		`cp "$(AB3D2_SOURCE)" "$(AB3D2_EMBED_FILE)"`,
		"prepare-ab3d2-embed",
		".PHONY: x64-live-aros-demos",
		"x64-live-aros-demos: aros-release-assets rotozoom-textures",
		"vasmm68k_mot -Fhunk -m68020 -devpac -Isdk/include",
		"-o sdk/examples/asm/RotoAPI",
		"-o sdk/examples/asm/RotoHW",
		"AROS_CC ?=",
		"-o sdk/examples/c/RotoAPIc",
		"-o sdk/examples/c/RotoHWc",
		`[ ! -f "$(AROS_ROM)" ] || \`,
		`[ ! -f "$(AROS_RELEASE_DIR)/S/Startup-Sequence" ] || \`,
		`[ ! -f "$(AROS_RELEASE_DIR)/Prefs/ScreenMode" ] || \`,
		`[ ! -f "$(AROS_RELEASE_DIR)/Prefs/Env-Archive/SYS/palette.prefs" ] || \`,
		`[ ! -f "$(AROS_RELEASE_DIR)/Prefs/Env-Archive/SYS/screenmode.prefs" ] || \`,
		`[ ! -f "$(AROS_RELEASE_DIR)/Prefs/Env-Archive/SYS/def_Tool.info" ] || \`,
		`[ ! -f "$(AROS_RELEASE_DIR)/Prefs/Env-Archive/SYS/def_Drawer.info" ] || \`,
		`[ ! -f "$(AROS_RELEASE_DIR)/Devs/Drivers/iegfx.hidd" ] || \`,
		`[ ! -f "$(AROS_RELEASE_DIR)/Libs/iewarp.library" ]`,
		`"$(AROS_SRC_DIR)/arch/m68k-ie/hidd/iegfx/iegfx_hiddclass.c"`,
		"Packaging rebuilt AROS ROM and IE graphics HIDD...",
		"Refreshing packaged ROM and IE graphics HIDD...",
		`cp -f "$$IEGFX_KO" "$$AROSDIR/Devs/Drivers/iegfx.hidd"`,
		`"$(AROS_BUILD_DIR)/bin/linux-x86_64/tools/crosstools/m68k-aros-objcopy"`,
		"AROS release assets missing or incomplete; building them...",
		"Preseeding AROS ScreenMode prefs for IE 1920x1080x8...",
		`./scripts/write-aros-screenmode-prefs.sh "$$AROSDIR/Prefs/Env-Archive/SYS/screenmode.prefs" 1920 1080 8`,
		"Preseeding AROS Palette prefs...",
		`./scripts/write-aros-palette-prefs.sh "$$AROSDIR/Prefs/Env-Archive/SYS/palette.prefs"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Makefile missing live demo payload contract %q", want)
		}
	}
}

func TestAROSRomMakefileBatchesWorkbenchTargets(t *testing.T) {
	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	text := string(makefile)

	forbidden := []string{
		"for target in \\\n\t\tworkbench-c",
		"for target in \\\n\t\tworkbench-devs-fdsk",
	}
	for _, phrase := range forbidden {
		if strings.Contains(text, phrase) {
			t.Fatalf("aros-rom must batch AROS target groups instead of repeatedly rescanning via %q", phrase)
		}
	}
	for _, want := range []string{
		"@$(MAKE) -C \"$(AROS_BUILD_DIR)\" \\\n\t\tworkbench-c",
		"@$(MAKE) -C \"$(AROS_BUILD_DIR)\" \\\n\t\tworkbench-devs-fdsk",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Makefile missing batched AROS target group %q", want)
		}
	}
}

func TestX64LiveQemuDoesNotAlwaysRebuildImage(t *testing.T) {
	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	text := string(makefile)

	forbidden := "intuition-engine-x64.img: x64-live"
	if strings.Contains(text, forbidden) {
		t.Fatalf("x64-live-qemu must not depend on phony x64-live via %q", forbidden)
	}
	if !strings.Contains(text, `test -f "$(X64_LIVE_IMG)" || $(MAKE) x64-live`) {
		t.Fatalf("intuition-engine-x64.img target should only build the image when it is missing")
	}
}

func TestX64LiveBinaryEmbedsAllROMs(t *testing.T) {
	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	text := string(makefile)

	for _, want := range []string{
		`VM_EMBED_TAGS := embed_basic embed_emutos embed_aros`,
		`-tags "$(VM_EMBED_TAGS)"`,
		`IE_BINARY="${SCRIPT_DIR}/bin/IntuitionEngine_v3"`,
	} {
		if !strings.Contains(text+readX64LiveScript(t), want) {
			t.Fatalf("live image embed contract missing %q", want)
		}
	}
}

func TestX64LiveScriptContract(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`UBUNTU_VERSION="26.04"`,
		`UBUNTU_CLOUD_IMG_URL="https://cloud-images.ubuntu.com/releases/26.04/release/ubuntu-26.04-server-cloudimg-amd64.img"`,
		`KERNEL_PKG="linux-lowlatency"`,
		`COMPOSITOR_PKGS="cage,seatd,greetd,xwayland,xwayland-run,libgl1,libegl1,libgles2,libwayland-client0,libxkbcommon0,fonts-dejavu-core,kbd"`,
		`X11_RUNTIME_PKGS="libxrandr2,libxxf86vm1,libxi6,libxcursor1,libxinerama1,libx11-6,libxext6,libxfixes3,libxrender1"`,
		`AUDIO_PKGS="pipewire,pipewire-pulse,wireplumber,pipewire-alsa,alsa-utils,dbus-user-session"`,
		`SECUREBOOT_PKGS="shim-signed,grub-efi-amd64-signed,sbsigntool"`,
		`PLYMOUTH_PKGS="plymouth,plymouth-themes"`,
		`NETWORK_PKGS="network-manager,wpasupplicant,wireless-regdb,iw"`,
		`IE_BINARY="${SCRIPT_DIR}/bin/IntuitionEngine_v3"`,
		`PLYMOUTH_SPLASH="${SCRIPT_DIR}/splash.png"`,
		`C64_MUSIC_SOURCE="${C64_MUSIC_SOURCE:-${HOME}/Music/C64Music}"`,
		`PROJECTAY_MUSIC_SOURCE="${PROJECTAY_MUSIC_SOURCE:-${HOME}/Music/ProjectAY}"`,
		`FINAL_IMAGE_SIZE="8G"`,
		`ROOT_PART_SIZE="6G"`,
		`FATSHARE_LABEL="IESHARE"`,
		`LIVE_OUT_DIR="${X64_LIVE_OUT_DIR:-${SCRIPT_DIR}/build/x64-live}"`,
		`WORK_DIR="${X64_LIVE_WORK_DIR:-${LIVE_OUT_DIR}/work}"`,
		`LOG_FILE="${X64_LIVE_LOG_FILE:-${LIVE_OUT_DIR}/build-x64-live-${TIMESTAMP}.log}"`,
		`OUTPUT_IMG="${X64_LIVE_OUTPUT_IMG:-${LIVE_OUT_DIR}/intuition-engine-x64.img}"`,
		`PAYLOAD_CHECK_ONLY=false`,
		`--check-payload`,
		`payload_require_file "$PLYMOUTH_SPLASH" "restore splash.png" "Plymouth splash image"`,
		`mformat -i "$fat_img" -F -v "${FATSHARE_LABEL}" ::`,
		`local required_cmds=(aria2c curl virt-customize virt-resize virt-filesystems guestfish qemu-img file python3 go sha256sum)`,
		`required_cmds+=(mformat mcopy rsync)`,
		`local archive_path="${OUTPUT_IMG%.img}.zip"`,
		`zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=1, allowZip64=True)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing %q", want)
		}
	}
}

func TestX64LiveScriptSafetyAndSession(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`Refusing destructive user deletion: EXPANDED_IMG`,
		`source image (${UBUNTU_CLOUD_IMG_PATH:-<unset>}) is not the downloaded cloud image`,
		`case " ubuntu cloud-user " in`,
		`UID 1000 occupied by unexpected user`,
		`GID 1000 occupied by unexpected group`,
		`for g in video audio input render seat tty; do getent group "$g" >/dev/null || groupadd -r "$g"; done`,
		`useradd -m -u 1000 -g 1000 -s /bin/bash -G video,audio,input,render,seat,tty ie`,
		`command = "/opt/ie/session.sh"`,
		`exec cage -s -- xwayland-run -- /opt/ie/launch.sh`,
		`if [ -d /var/ie/share ]; then`,
		`cd /var/ie/share || cd /opt/ie`,
		`set -- -ehbasic-host -ehbasic-host-appliance`,
		`set -- "$@" -emutos-drive /var/ie/share/Systems/EmuTOS`,
		`set -- "$@" -aros-drive /var/ie/share/Systems/AROS`,
		`set -- "$@" -intuitionos-root /var/ie/share/Systems/IntuitionOS`,
		`set -- "$@" -intuitionos-image /var/ie/share/Systems/IntuitionOS/Boot/iexec.ie64`,
		`cd /opt/ie`,
		`export IE_LIVE_IMAGE=1`,
		`exec dbus-run-session -- "$0" "$@"`,
		`export XDG_RUNTIME_DIR="/tmp/ie-runtime-$(id -u)"`,
		`export PIPEWIRE_RUNTIME_DIR="$XDG_RUNTIME_DIR"`,
		`export PULSE_RUNTIME_PATH="$XDG_RUNTIME_DIR/pulse"`,
		`export PULSE_SERVER="unix:${XDG_RUNTIME_DIR}/pulse/native"`,
		`mkdir -p "$XDG_RUNTIME_DIR" "$XDG_RUNTIME_DIR/pulse"`,
		`chmod 700 "$XDG_RUNTIME_DIR"`,
		`pipewire >/tmp/ie-pipewire.log 2>&1 &`,
		`[ -S "${XDG_RUNTIME_DIR}/pipewire-0" ] && break`,
		`wireplumber >/tmp/ie-wireplumber.log 2>&1 &`,
		`pipewire-pulse >/tmp/ie-pipewire-pulse.log 2>&1 &`,
		`[ -S "${XDG_RUNTIME_DIR}/pulse/native" ] && break`,
		`pipewire-pulse did not become ready at ${XDG_RUNTIME_DIR}/pulse/native`,
		`pipewire-pulse socket ready at ${XDG_RUNTIME_DIR}/pulse/native`,
		`exec /opt/ie/IntuitionEngine "$@"`,
		`for n in $(seq 1 12); do systemctl mask "getty@tty${n}.service"; done`,
		`NAutoVTs=0`,
		`ReserveVT=0`,
		`control alt keycode  59 = VoidSymbol`,
		`alt keycode 105 = VoidSymbol`,
		`ExecStart=/usr/bin/loadkeys /usr/local/share/kbd/keymaps/ie-no-vt-switch.map`,
		`systemctl enable greetd.service seatd.service`,
		`systemctl enable ie-grow-share.service`,
		`systemctl enable NetworkManager.service`,
		`renderer: NetworkManager`,
		`WantedBy=sysinit.target`,
		`Before=local-fs-pre.target`,
		`growpart "$disk" "$part_num"`,
		`fatresize -s "$after" "$part"`,
		`preserving existing FAT contents without reformatting`,
		`persistent_root=ext4`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing %q", want)
		}
	}

	exportIdx := strings.Index(body, `export IE_LIVE_IMAGE=1`)
	dbusIdx := strings.Index(body, `exec dbus-run-session -- "$0" "$@"`)
	execIdx := strings.Index(body, `exec /opt/ie/IntuitionEngine "$@"`)
	if exportIdx < 0 || dbusIdx < 0 || execIdx < 0 {
		t.Fatalf("live launcher missing IE_LIVE_IMAGE/dbus/final exec contract")
	}
	if exportIdx > dbusIdx {
		t.Fatalf("IE_LIVE_IMAGE export must happen before dbus-run-session re-exec")
	}
	if exportIdx > execIdx {
		t.Fatalf("IE_LIVE_IMAGE export must happen before final IntuitionEngine exec")
	}
}

func TestX64LiveUsesPlymouthSplash(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`PLYMOUTH_PKGS="plymouth,plymouth-themes"`,
		`ALL_PKGS="${KERNEL_PKG},${COMPOSITOR_PKGS},${X11_RUNTIME_PKGS},${AUDIO_PKGS},${SECUREBOOT_PKGS},${PLYMOUTH_PKGS},${SHARE_GROW_PKGS},${NETWORK_PKGS},${HOST_HELPER_PKGS}"`,
		`PLYMOUTH_SPLASH="${SCRIPT_DIR}/splash.png"`,
		`cat > "${WORK_DIR}/intuition-engine.plymouth"`,
		`ModuleName=script`,
		`ImageDir=/usr/share/plymouth/themes/intuition-engine`,
		`ScriptFile=/usr/share/plymouth/themes/intuition-engine/intuition-engine.script`,
		`cat > "${WORK_DIR}/intuition-engine.script"`,
		`logo = Image("splash.png");`,
		`Plymouth.SetRefreshFunction(refresh_callback);`,
		`cat > "${WORK_DIR}/zz-intuition-engine-grub.cfg"`,
		`--mkdir /usr/share/plymouth/themes/intuition-engine`,
		`--upload "${WORK_DIR}/intuition-engine.plymouth:/usr/share/plymouth/themes/intuition-engine/intuition-engine.plymouth"`,
		`--upload "${WORK_DIR}/intuition-engine.script:/usr/share/plymouth/themes/intuition-engine/intuition-engine.script"`,
		`--upload "${PLYMOUTH_SPLASH}:/usr/share/plymouth/themes/intuition-engine/splash.png"`,
		`--upload "${WORK_DIR}/zz-intuition-engine-grub.cfg:/etc/default/grub.d/zz-intuition-engine.cfg"`,
		`update-alternatives --install /usr/share/plymouth/themes/default.plymouth default.plymouth /usr/share/plymouth/themes/intuition-engine/intuition-engine.plymouth 100`,
		`update-alternatives --set default.plymouth /usr/share/plymouth/themes/intuition-engine/intuition-engine.plymouth`,
		`update-initramfs -u -k all`,
		`GRUB_CMDLINE_LINUX_DEFAULT="quiet splash loglevel=0 vt.global_cursor_default=0 fbcon=nodefer video=1920x1080 mitigations=off"`,
		`GRUB_CMDLINE_LINUX=""`,
		`unset GRUB_TERMINAL`,
		`plymouth=${PLYMOUTH_PKGS}`,
		`plymouth_splash=splash.png`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing Plymouth splash contract %q", want)
		}
	}

	for _, forbidden := range []string{
		`splash=0`,
		`GRUB_CMDLINE_LINUX="console=ttyS0`,
		`GRUB_CMDLINE_LINUX_DEFAULT="console=tty1 console=ttyS0`,
		`plymouth-set-default-theme intuition-engine`,
		`plymouth-set-default-theme -R`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("build_x64_ie_img.sh contains forbidden verbose boot pattern %q", forbidden)
		}
	}
}

func TestX64LiveGoldenImageRunsAptUpgrade(t *testing.T) {
	body := readX64LiveScript(t)

	upgrade := `apt-get update; apt-get upgrade -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold; apt-get autoremove -y; apt-get clean`
	verify := `dpkg-query -W linux-lowlatency`
	if !strings.Contains(body, upgrade) {
		t.Fatalf("build_x64_ie_img.sh missing golden apt upgrade contract %q", upgrade)
	}
	upgradeIdx := strings.Index(body, upgrade)
	verifyIdx := strings.Index(body, verify)
	if verifyIdx == -1 {
		t.Fatalf("build_x64_ie_img.sh missing signed-kernel verification contract %q", verify)
	}
	if upgradeIdx > verifyIdx {
		t.Fatalf("golden apt upgrade must run before signed-kernel verification")
	}
}

func TestX64LiveLaunchMatchesDefaultRuntimeMode(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`exec /opt/ie/IntuitionEngine "$@"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("live launcher must preserve the default runtime mode; missing %q", want)
		}
	}

	for _, forbidden := range []string{
		`exec /opt/ie/IntuitionEngine -fullscreen`,
		`exec /opt/ie/IntuitionEngine -basic -fullscreen`,
		`exec /opt/ie/IntuitionEngine -basic "$@"`,
		`exec /opt/ie/IntuitionEngine -basic -ehbasic-host "$@"`,
		`exec /opt/ie/IntuitionEngine -basic -ehbasic-host -ehbasic-host-appliance "$@"`,
		`export IE_TRACE_HOSTIO=1`,
		`IE_TRACE_HOSTIO_FILE=/var/ie/share/ie-hostio.log`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("live launcher has stale forced mode invocation %q", forbidden)
		}
	}
}

func TestX64LiveHostHelperSecurityContract(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`HOST_HELPER_PKGS="polkitd,pkexec,ufw,apparmor,apparmor-utils"`,
		`host_helper=${HOST_HELPER_PKGS}`,
		`set -- -ehbasic-host -ehbasic-host-appliance`,
		`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -pgo=off -ldflags "-s -w" -o "$HOST_HELPER_BINARY" ./cmd/host-helper`,
		`--mkdir /usr/libexec`,
		`--copy-in "${HOST_HELPER_BINARY}:/usr/libexec/"`,
		`chown root:root /usr/libexec/intuitionengine-host-helper`,
		`chmod 0755 /usr/libexec/intuitionengine-host-helper`,
		`ie-host-helper.service`,
		`ExecStart=/usr/libexec/intuitionengine-host-helper serve`,
		`<annotate key="org.freedesktop.policykit.exec.path">/usr/libexec/intuitionengine-host-helper</annotate>`,
		`action.id === "org.intuitionengine.host.run"`,
		`subject.user === "ie"`,
		`polkit.Result.YES`,
		`profile opt.ie.IntuitionEngine /opt/ie/IntuitionEngine flags=(attach_disconnected)`,
		`network unix stream,`,
		`/var/ie/share/ rw,`,
		`/dev/shm/** rwk,`,
		`/tmp/.X11-unix/** rw,`,
		`/tmp/.X[0-9]*-lock r,`,
		`/tmp/dbus-* rw,`,
		`/tmp/tmp*/** rwk,`,
		`/etc/shells r,`,
		`/tmp/xwayland-run*/** rwk,`,
		`/tmp/ie-runtime-[0-9]*/** rw,`,
		`/tmp/ie-*.log rw,`,
		`/home/ie/.config/pulse/ rw,`,
		`/home/ie/.config/pulse/** rwk,`,
		`/proc/[0-9]*/cgroup r,`,
		`/proc/sys/net/core/somaxconn r,`,
		`/usr/lib/x86_64-linux-gnu/dri/** mr,`,
		`/usr/share/glvnd/** r,`,
		`/usr/bin/pkexec ix,`,
		`/usr/libexec/intuitionengine-host-helper Px,`,
		`profile usr.libexec.intuitionengine-host-helper /usr/libexec/intuitionengine-host-helper flags=(attach_disconnected)`,
		`/usr/bin/apt-get Ux,`,
		`/usr/bin/systemctl Cx -> systemctl,`,
		`/run/dbus/system_bus_socket rw,`,
		`/run/intuitionengine-host-helper.sock rw,`,
		`capability chown,`,
		`capability fowner,`,
		`ExecStart=/usr/sbin/apparmor_parser -r /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper`,
		`ExecStart=/usr/sbin/aa-enforce /etc/apparmor.d/opt.ie.IntuitionEngine /etc/apparmor.d/usr.libexec.intuitionengine-host-helper`,
		`Before=greetd.service`,
		`systemctl enable ie-grow-share.service ie-firewall.service ie-no-vt-switch.service ie-apparmor.service ie-host-helper.service`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing HOST helper security contract %q", want)
		}
	}

	for _, forbidden := range []string{
		`usermod -aG netdev ie`,
		`systemctl enable getty@tty2.service`,
		`chown 1000:1000 /opt/ie`,
		`chown -R 1000:1000 /var/ie /opt/ie`,
		`/usr/bin/nmcli`,
		`subject.active === true`,
		`subject.local === true`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("build_x64_ie_img.sh contains forbidden HOST helper security pattern %q", forbidden)
		}
	}
}

func TestX64LiveLaunchesX11BinaryThroughXwayland(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`xwayland,xwayland-run`,
		`exec cage -s -- xwayland-run -- /opt/ie/launch.sh`,
		`X11_RUNTIME_PKGS=`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("live image must launch the X11-linked IE binary through Xwayland; missing %q", want)
		}
	}
}

func TestX64LiveKernelCheckMatchesUbuntu2604LowlatencyPackaging(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`dpkg-query -W linux-lowlatency`,
		`find /boot -maxdepth 1 -type f -name "vmlinuz-*"`,
		`sbverify --list "$KIMG"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing kernel packaging guard %q", want)
		}
	}

	if strings.Contains(body, `apt-get -y purge linux-generic linux-image-generic linux-headers-generic`) {
		t.Fatalf("build_x64_ie_img.sh must not purge linux-generic packages; Ubuntu 26.04 linux-lowlatency depends on generic-named kernel packages")
	}
	if strings.Contains(body, `/boot/vmlinuz-*-lowlatency`) {
		t.Fatalf("build_x64_ie_img.sh must not assume lowlatency kernels have vmlinuz-*-lowlatency filenames on Ubuntu 26.04")
	}
}

func TestX64LiveNoShareDoesNotRequireMtools(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`local required_cmds=(aria2c curl virt-customize virt-resize virt-filesystems guestfish qemu-img file python3 go sha256sum)`,
		`if [[ "${CREATE_SHARE}" == "true" ]]; then`,
		`required_cmds+=(mformat mcopy rsync)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing no-share dependency behavior %q", want)
		}
	}
}

func TestX64LiveStagesDemoPayloadOnIESHARE(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`stage_share_payload`,
		`check_live_payload_inputs`,
		`verify_staged_share_payload`,
		`Live payload manifest inputs are ready`,
		`Staged IESHARE payload matches the live manifest`,
		`payload_require_file "$IE_BINARY" "make x86-64-v3"`,
		`AB3D2_EMBED_DIR="${SCRIPT_DIR}/embedded/ab3d2"`,
		`payload_require_file "${AB3D2_EMBED_DIR}/_build.zip" "make x64-live-ab3d2-assets"`,
		`local ab3d2_demo_inputs=("${AB3D2_EMBED_DIR}"/ab3d2_*.ie68)`,
		`AB3D2 IE68 demos not found: ${AB3D2_EMBED_DIR}/ab3d2_*.ie68`,
		`payload_require_file "${SCRIPT_DIR}/sdk/examples/prebuilt/iewarp_service.ie64" "make iewarp-runtime-assets"`,
		`payload_require_file "${AROS_RELEASE_DIR}/Libs/iewarp_service.ie64" "make iewarp-runtime-assets"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/iexec/iexec.ie64" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/S/Startup-Sequence" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/Tools/Shell" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/LIBS/dos.library" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/L/console.handler" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_api_c.raw" "make rotozoom-textures"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_hw_c.raw" "make rotozoom-textures"`,
		`find "${payload_root}/Demos/m68k" -maxdepth 1 -type f -name 'ab3d2_*.ie68' ! -name '*redux_high*' ! -name '*redux_low*'`,
		`payload_require_file "${payload_root}/_build/ie_unpacked/media/includes/test.lnk" "make x64-live-ab3d2-assets"`,
		`local ab3d2_low_demos=("${payload_root}"/Demos/m68k/ab3d2_ie68_redux_low*.ie68)`,
		`payload_require_file "${payload_root}/_build/ie_media/redux-low/boot.dat" "make x64-live-ab3d2-assets"`,
		`AROS_RELEASE_DIR="${AROS_RELEASE_DIR:-${SCRIPT_DIR}/../AROS/bin/ie-m68k/bin/ie-m68k/AROS}"`,
		`AROS release tree not found: ${AROS_RELEASE_DIR}`,
		`AROS default tool icon not found: ${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info`,
		`AROS default drawer icon not found: ${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info`,
		`AROS IEWarp library not found: ${AROS_RELEASE_DIR}/Libs/iewarp.library`,
		`case-colliding AROS file: keeping`,
		`local demos_dir="${payload_root}/Demos"`,
		`local demos_ie32_dir="${demos_dir}/ie32"`,
		`local demos_ie64_dir="${demos_dir}/ie64"`,
		`local demos_m68k_dir="${demos_dir}/m68k"`,
		`local demos_z80_dir="${demos_dir}/z80"`,
		`local demos_m6502_dir="${demos_dir}/m6502"`,
		`local demos_x86_dir="${demos_dir}/x86"`,
		`local coproc_dir="${payload_root}/IE/Coproc"`,
		`local music_dir="${payload_root}/Music"`,
		`local sdk_dir="${payload_root}/SDK"`,
		`local systems_dir="${payload_root}/Systems"`,
		`local aros_system_dir="${systems_dir}/AROS"`,
		`local aros_demos_dir="${aros_system_dir}/Demos"`,
		`local emutos_system_dir="${systems_dir}/EmuTOS"`,
		`local emutos_demos_dir="${emutos_system_dir}/Demos"`,
		`local intuitionos_system_dir="${systems_dir}/IntuitionOS"`,
		`python3 - "${AROS_RELEASE_DIR}" "$aros_system_dir"`,
		`cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "${payload_root}/Demos.info"`,
		`cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "${systems_dir}/AROS.info"`,
		`cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "${systems_dir}/EmuTOS.info"`,
		`cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info" "${systems_dir}/IntuitionOS.info"`,
		`cp -a "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/." "$intuitionos_system_dir/"`,
		`cp -f "${SCRIPT_DIR}/sdk/intuitionos/iexec/iexec.ie64" "$intuitionos_system_dir/Boot/iexec.ie64"`,
		`"$demos_dir" "$demos_ie32_dir" "$demos_ie64_dir"`,
		`"$demos_m68k_dir" "$demos_z80_dir" "$demos_m6502_dir"`,
		`"$demos_x86_dir" "$coproc_dir" "$music_dir"`,
		`rsync -a --delete "${C64_MUSIC_SOURCE}/" "${music_dir}/C64Music/"`,
		`rsync -a --delete "${PROJECTAY_MUSIC_SOURCE}/" "${music_dir}/ProjectAY/"`,
		`C64 music source not found; leaving Music/C64Music absent`,
		`ProjectAY music source not found; leaving Music/ProjectAY absent`,
		`Required staged payload directory missing: ${payload_root}/Music`,
		`"$sdk_dir/Include" "$sdk_dir/Examples/asm"`,
		`"${SCRIPT_DIR}"/sdk/examples/prebuilt/*.ie*`,
		`"${SCRIPT_DIR}"/sdk/examples/prebuilt/*.prg`,
		`coproc_*.ie*) coproc_files+=("$prebuilt_file") ;;`,
		`iewarp_service.ie64) iewarp_worker_files+=("$prebuilt_file") ;;`,
		`*.prg) emutos_demo_files+=("$prebuilt_file") ;;`,
		`*.iex|*.ie32) cp -f "$prebuilt_file" "$demos_ie32_dir/" ;;`,
		`*.ie64) cp -f "$prebuilt_file" "$demos_ie64_dir/" ;;`,
		`*.ie68) cp -f "$prebuilt_file" "$demos_m68k_dir/" ;;`,
		`*.ie80) cp -f "$prebuilt_file" "$demos_z80_dir/" ;;`,
		`*.ie65) cp -f "$prebuilt_file" "$demos_m6502_dir/" ;;`,
		`*.ie86) cp -f "$prebuilt_file" "$demos_x86_dir/" ;;`,
		`Unsupported prebuilt demo extension`,
		`cp -f "${coproc_files[@]}" "$coproc_dir/"`,
		`cp -f "${emutos_demo_files[@]}" "$emutos_demos_dir/"`,
		`cp -f "${iewarp_worker_files[@]}" "$aros_system_dir/Libs/"`,
		`python3 - "${AB3D2_EMBED_DIR}/_build.zip" "${AB3D2_EMBED_DIR}" "$demos_m68k_dir"`,
		`"redux-low": any(name.startswith("ab3d2_source/_build/ie_media/redux-low/") for name in names)`,
		`Skipping AB3D2 {filename}: missing {profile} runtime media`,
		`no AB3D2 IE68 demos matched available runtime media`,
		`verify_staged_share_payload "$payload_root"`,
		`Forbidden live payload location: coproc worker staged under Demos`,
		`Forbidden live payload content: AB3D2 build intermediates staged in runtime asset root`,
		`Forbidden live payload location: AROS-only payload staged under Demos`,
		`Forbidden legacy IEWarp worker path staged in live payload`,
		`IE/Coproc/coproc_service_ie32.iex`,
		`Systems/AROS/Libs/iewarp_service.ie64`,
		`"${SCRIPT_DIR}/sdk/include/ie32.inc"`,
		`"${SCRIPT_DIR}/sdk/include/ie64.inc"`,
		`"${SCRIPT_DIR}/sdk/include/ie65.inc"`,
		`"${SCRIPT_DIR}/sdk/include/ie68.inc"`,
		`"${SCRIPT_DIR}/sdk/include/ie80.inc"`,
		`"${SCRIPT_DIR}/sdk/include/ie86.inc"`,
		`cp -f "${SCRIPT_DIR}"/sdk/examples/asm/*.asm "$sdk_dir/Examples/asm/"`,
		`cp -f "${SCRIPT_DIR}"/sdk/examples/basic/*.bas "$sdk_dir/Examples/basic/"`,
		`cp -f "${SCRIPT_DIR}"/sdk/examples/c/*.c "$sdk_dir/Examples/c/"`,
		`"${SCRIPT_DIR}/sdk/examples/asm/RotoAPI"`,
		`"${SCRIPT_DIR}/sdk/examples/asm/RotoHW"`,
		`"${SCRIPT_DIR}/sdk/examples/c/RotoAPIc"`,
		`"${SCRIPT_DIR}/sdk/examples/c/RotoHWc"`,
		`cp -f "$aros_demo" "$aros_demos_dir/"`,
		`cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info" "${aros_demos_dir}/$(basename "$aros_demo").info"`,
		`"${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_api_c.raw"`,
		`"${SCRIPT_DIR}/sdk/examples/assets/rotozoomtexture_hw_c.raw"`,
		`"$aros_demos_dir/"`,
		`Systems/AROS/Demos/rotozoomtexture_api_c.raw`,
		`Systems/AROS/Demos/rotozoomtexture_hw_c.raw`,
		`"${AB3D2_EMBED_DIR}/_build.zip"`,
		`"ab3d2_source/_build/ie_media/redux-high/"`,
		`"ab3d2_source/_build/ie_media/redux-low/"`,
		`"ab3d2_source/_build/ie_unpacked/"`,
		`name.removeprefix("ab3d2_source/").lower()`,
		`case-colliding AB3D2 asset differs`,
		`Demos/m68k/ab3d2_ie68_redux_high.ie68`,
		`_build/ie_media/redux-high/boot.dat`,
		`"${demos_dir}/README.TXT"`,
		`"${payload_root}/README.TXT"`,
		`"${systems_dir}/README.TXT"`,
		`AROS files live under Systems/AROS.`,
		`EmuTOS/GEMDOS demo files live under Systems/EmuTOS.`,
		`Music    Music collections copied from the build host when available.`,
		`It contains runnable Intuition Engine demos grouped by guest CPU:`,
		`AB3D2 IE68 demos live under m68k.`,
		`Systems/AROS/Demos`,
		`Systems/EmuTOS/Demos`,
		`Systems/IntuitionOS`,
		`Systems/IntuitionOS/Boot/iexec.ie64`,
		`Systems/IntuitionOS/IOSSYS is the read-only system subtree`,
		`Systems/AROS/Libs contains AROS libraries and private library resources,`,
		`including iewarp_service.ie64 for iewarp.library.`,
		`local payload_entries=("${SHARE_PAYLOAD_ROOT}"/*)`,
		`mcopy -i "$fat_img" -D A -s "${payload_entries[@]}" ::/`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing IESHARE demo payload behavior %q", want)
		}
	}
}

func TestX64LiveGrowShareResizesInitializedShares(t *testing.T) {
	body := readX64LiveScript(t)

	markerIdx := strings.Index(body, `[ -e "$tmp/.ie-share-initialized" ]`)
	resizeIdx := strings.Index(body, `fatresize -s "$after" "$part"`)
	if markerIdx == -1 || resizeIdx == -1 {
		t.Fatalf("expected grow-share marker and fatresize logic")
	}
	if markerIdx < resizeIdx {
		t.Fatalf("initialized-share marker check must not run before fatresize; marker index %d, resize index %d", markerIdx, resizeIdx)
	}
}

func TestX64LiveScriptPartitionFlow(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`virt-filesystems -a "${UBUNTU_CLOUD_IMG_PATH}" --filesystems --long --csv`,
		`header.index("Name"), header.index("Type"), header.index("VFS"), header.index("Label")`,
		`virt-resize --resize "${ROOT_DEV}=${ROOT_PART_SIZE}" --no-extra-partition`,
		`blockdev-getss /dev/sda`,
		`blockdev-getsz /dev/sda`,
		`SHARE_END=$(( total_sectors - 34 ))`,
		`part-list /dev/sda`,
		`part-add /dev/sda p ${SHARE_START} ${SHARE_END}`,
		`part-set-gpt-type /dev/sda ${IESHARE_NUM} EBD0A0A2-B9E5-4433-87C0-68B6B72699C7`,
		`part-set-mbr-id /dev/sda ${IESHARE_NUM} 0x0c`,
		`LABEL=IESHARE /var/ie/share vfat defaults,relatime,nofail,umask=0022,uid=1000,gid=1000 0 0\n`,
		`format_share_partition_rootless`,
		`--no-share`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing %q", want)
		}
	}

	for _, forbidden := range []string{
		`IESAVE`,
		`/var/ie/save`,
		`SAVE_START`,
		`SAVE_END`,
		`mkfs ext4`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("build_x64_ie_img.sh must not create a separate save partition/folder; found %q", forbidden)
		}
	}
}

func TestX64LiveDiscoversAppendedPartitionsByStartOffset(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`find_partition_num_by_start`,
		`target_sector = int(sys.argv[1])`,
		`start = int(m_start.group(1)) // sector_size`,
		`python3 -c`,
		`IESHARE_NUM="$(find_partition_num_by_start "$SHARE_START")"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh must discover appended partitions by start offset; missing %q", want)
		}
	}

	forbidden := `nums[-2], nums[-1]`
	if strings.Contains(body, forbidden) {
		t.Fatalf("build_x64_ie_img.sh must not infer appended partitions from highest partition numbers")
	}
	if strings.Contains(body, `python3 - "$target_start" "$SECTOR_SIZE" <<'PY'`) {
		t.Fatalf("partition parser must not use a Python here-doc that steals stdin from part-list output")
	}
}

func TestX64LiveFormatsShareWithinPartitionBounds(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`IESHARE_SIZE_B="$(find_partition_size_by_start "$SHARE_START")"`,
		`truncate -s "$IESHARE_SIZE_B" "$fat_img"`,
		`mformat -i "$fat_img" -F -v "${FATSHARE_LABEL}" ::`,
		`upload "$fat_img" ${IESHARE_DEV}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh must format IESHARE within partition bounds; missing %q", want)
		}
	}
	if strings.Contains(body, `mformat -i "${OUTPUT_IMG}@@${SHARE_START_B}"`) {
		t.Fatalf("build_x64_ie_img.sh must not run mformat directly on image@@offset without a size bound")
	}
}

func TestX64LiveGoldenCacheHasContentStamp(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`GOLDEN_STAMP_VERSION="x64-live-golden-v39-quiet-plymouth-splash"`,
		`GOLDEN_STAMP_PATH="${GOLDEN_IMG_PATH}.stamp"`,
		`write_golden_stamp`,
		`expected_golden_stamp`,
		`Golden image stamp mismatch; rebuilding`,
		`write_golden_stamp`,
		`plymouth_splash_sha256="$(sha256sum "$PLYMOUTH_SPLASH")"`,
		`plymouth_splash_sha256="${plymouth_splash_sha256%% *}"`,
		`plymouth_splash_sha256=${plymouth_splash_sha256}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing golden stamp contract %q", want)
		}
	}
}

func TestX64LiveUsesCachesWithoutNetworkProbe(t *testing.T) {
	body := readX64LiveScript(t)

	cacheIdx := strings.Index(body, `if [[ -f "$UBUNTU_CLOUD_IMG_PATH" ]]`)
	curlIdx := strings.Index(body, `curl -fsSI "$UBUNTU_CLOUD_IMG_URL"`)
	if cacheIdx == -1 || curlIdx == -1 {
		t.Fatalf("expected cloud image cache check and URL probe")
	}
	if curlIdx < cacheIdx {
		t.Fatalf("cloud image URL probe must run after local cache checks")
	}

	mainIdx := strings.Index(body, `main() {`)
	if mainIdx == -1 {
		t.Fatalf("expected main function")
	}
	mainBody := body[mainIdx:]
	goldenIdx := strings.Index(mainBody, `if check_golden_image; then`)
	downloadIdx := strings.Index(mainBody, `download_ubuntu`)
	if goldenIdx == -1 || downloadIdx == -1 {
		t.Fatalf("expected golden image check and download call")
	}
	if downloadIdx < goldenIdx {
		t.Fatalf("main must check golden cache before calling download_ubuntu")
	}
}

func TestX64LiveArtifactsAreIgnored(t *testing.T) {
	gitignore, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	body := string(gitignore)

	for _, want := range []string{
		"build/",
		"/x64-img-build-work/",
		"/build-x64-live-*.log",
		"/intuition-engine-x64.img",
		"/intuition-engine-x64.zip",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf(".gitignore missing live image artifact ignore %q", want)
		}
	}
}

func TestX64LiveScriptDoesNotRequireHostRoot(t *testing.T) {
	body := readX64LiveScript(t)

	for _, forbidden := range []string{
		`sudo `,
		`modprobe nbd`,
		`qemu-nbd`,
		`mkfs.vfat -F 32 -n "${FATSHARE_LABEL}"`,
		`pulseaudio --`,
		`pulseaudio -`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("build_x64_ie_img.sh should not require host root; found %q", forbidden)
		}
	}
}

func TestX64LiveScriptBashSyntax(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available on this test host")
	}
	if _, err := os.Stat("build_x64_ie_img.sh"); err != nil {
		t.Fatalf("stat build_x64_ie_img.sh: %v", err)
	}
	cmd := exec.Command("bash", "-n", "build_x64_ie_img.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n build_x64_ie_img.sh failed: %v\n%s", err, out)
	}
}

func readX64LiveScript(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("build_x64_ie_img.sh")
	if err != nil {
		t.Fatalf("read build_x64_ie_img.sh: %v", err)
	}
	return string(b)
}
