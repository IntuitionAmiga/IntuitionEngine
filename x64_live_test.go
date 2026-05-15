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
		"x64-live-embed-assets: sdk-build emutos-release-rom aros-iewarp-library intuitionos",
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
		"x64-live-payload-check: x86-64-v3 sdk-build gem-rotozoomer aros-iewarp-library iewarp-runtime-assets x64-live-aros-demos x64-live-ab3d2-assets intuitionos",
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
		"prepare-ab3d2-embed",
		".PHONY: x64-live-aros-demos",
		"x64-live-aros-demos: aros-release-assets",
		"vasmm68k_mot -Fhunk -m68020 -devpac -Isdk/include",
		"-o sdk/examples/asm/RotoAPI",
		"-o sdk/examples/asm/RotoHW",
		"AROS_CC ?=",
		"-o sdk/examples/c/RotoAPIc",
		"-o sdk/examples/c/RotoHWc",
		`[ -f "$(AROS_ROM)" ] && \`,
		`[ -f "$(AROS_RELEASE_DIR)/S/Startup-Sequence" ] && \`,
		`[ -f "$(AROS_RELEASE_DIR)/Prefs/Env-Archive/SYS/def_Tool.info" ] && \`,
		`[ -f "$(AROS_RELEASE_DIR)/Prefs/Env-Archive/SYS/def_Drawer.info" ] && \`,
		`[ -f "$(AROS_RELEASE_DIR)/Libs/iewarp.library" ]`,
		"AROS release assets missing or incomplete; building them...",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Makefile missing live demo payload contract %q", want)
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
		`COMPOSITOR_PKGS="cage,seatd,greetd,xwayland,xwayland-run,mesa-utils,libgl1,libegl1,libgles2,libwayland-client0,libxkbcommon0,fonts-dejavu-core"`,
		`X11_RUNTIME_PKGS="libxrandr2,libxxf86vm1,libxi6,libxcursor1,libxinerama1,libx11-6,libxext6,libxfixes3,libxrender1"`,
		`AUDIO_PKGS="pipewire,pipewire-pulse,wireplumber,pipewire-alsa,alsa-utils,pulseaudio-utils,dbus-user-session"`,
		`SECUREBOOT_PKGS="shim-signed,grub-efi-amd64-signed,sbsigntool"`,
		`OVERLAY_PKG="overlayroot"`,
		`NETWORK_PKGS="network-manager,wpasupplicant,wireless-regdb,iw"`,
		`IE_BINARY="${SCRIPT_DIR}/bin/IntuitionEngine_v3"`,
		`FINAL_IMAGE_SIZE="8G"`,
		`ROOT_PART_SIZE="5G"`,
		`FATSHARE_LABEL="IESHARE"`,
		`LIVE_OUT_DIR="${X64_LIVE_OUT_DIR:-${SCRIPT_DIR}/build/x64-live}"`,
		`WORK_DIR="${X64_LIVE_WORK_DIR:-${LIVE_OUT_DIR}/work}"`,
		`LOG_FILE="${X64_LIVE_LOG_FILE:-${LIVE_OUT_DIR}/build-x64-live-${TIMESTAMP}.log}"`,
		`OUTPUT_IMG="${X64_LIVE_OUTPUT_IMG:-${LIVE_OUT_DIR}/intuition-engine-x64.img}"`,
		`PAYLOAD_CHECK_ONLY=false`,
		`--check-payload`,
		`mformat -i "$fat_img" -F -v "${FATSHARE_LABEL}" ::`,
		`local required_cmds=(aria2c curl virt-customize virt-resize virt-filesystems guestfish qemu-img file zstd tar python3)`,
		`tar -C "$archive_root" -I 'zstd --fast=31 -T0' -cf "$archive_path" "$(basename "$OUTPUT_IMG")" README.md`,
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
		`command = "cage -s -- xwayland-run -- /opt/ie/launch.sh"`,
		`if [ -d /var/ie/share ]; then`,
		`cd /var/ie/share || cd /opt/ie`,
		`set -- "$@" -emutos-drive /var/ie/share/Systems/EmuTOS`,
		`set -- "$@" -aros-drive /var/ie/share/Systems/AROS`,
		`set -- "$@" -intuitionos-root /var/ie/share/Systems/IntuitionOS`,
		`set -- "$@" -intuitionos-image /var/ie/share/Systems/IntuitionOS/Boot/iexec.ie64`,
		`cd /opt/ie`,
		`set --`,
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
		`[ -S "${XDG_RUNTIME_DIR}/pulse/native" ] && pactl info >/tmp/ie-pactl-info.log 2>&1 && break`,
		`pipewire-pulse did not become ready at ${XDG_RUNTIME_DIR}/pulse/native`,
		`pipewire-pulse ready at ${XDG_RUNTIME_DIR}/pulse/native`,
		`exec /opt/ie/IntuitionEngine "$@"`,
		`systemctl mask getty@tty1.service`,
		`systemctl enable greetd.service seatd.service`,
		`systemctl enable getty@tty2.service`,
		`systemctl enable ie-grow-share.service`,
		`systemctl enable NetworkManager.service`,
		`renderer: NetworkManager`,
		`WantedBy=sysinit.target`,
		`Before=local-fs-pre.target`,
		`growpart "$disk" "$part_num"`,
		`fatresize -s "$after" "$part"`,
		`preserving existing FAT contents without reformatting`,
		`overlayroot="tmpfs"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("build_x64_ie_img.sh missing %q", want)
		}
	}
}

func TestX64LiveLaunchUsesDefaultBasicMode(t *testing.T) {
	body := readX64LiveScript(t)

	for _, forbidden := range []string{
		`exec /opt/ie/IntuitionEngine -basic`,
		`exec /opt/ie/IntuitionEngine -fullscreen`,
		`exec /opt/ie/IntuitionEngine -basic -fullscreen`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("live launcher should rely on IntuitionEngine's default BASIC/fullscreen startup, found %q", forbidden)
		}
	}
}

func TestX64LiveLaunchesX11BinaryThroughXwayland(t *testing.T) {
	body := readX64LiveScript(t)

	for _, want := range []string{
		`xwayland,xwayland-run`,
		`command = "cage -s -- xwayland-run -- /opt/ie/launch.sh"`,
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
		`local required_cmds=(aria2c curl virt-customize virt-resize virt-filesystems guestfish qemu-img file zstd tar python3)`,
		`if [[ "${CREATE_SHARE}" == "true" ]]; then`,
		`required_cmds+=(mformat mcopy)`,
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
		`payload_require_file "${SCRIPT_DIR}/embedded/ab3d2/_build.zip" "make x64-live-ab3d2-assets"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/examples/prebuilt/iewarp_service.ie64" "make iewarp-runtime-assets"`,
		`payload_require_file "${AROS_RELEASE_DIR}/Libs/iewarp_service.ie64" "make iewarp-runtime-assets"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/iexec/iexec.ie64" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/S/Startup-Sequence" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/Tools/Shell" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/LIBS/dos.library" "make intuitionos"`,
		`payload_require_file "${SCRIPT_DIR}/sdk/intuitionos/system/SYS/IOSSYS/L/console.handler" "make intuitionos"`,
		`AROS_RELEASE_DIR="${AROS_RELEASE_DIR:-${SCRIPT_DIR}/../AROS/bin/ie-m68k/bin/ie-m68k/AROS}"`,
		`AROS release tree not found: ${AROS_RELEASE_DIR}`,
		`AROS default tool icon not found: ${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info`,
		`AROS default drawer icon not found: ${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Drawer.info`,
		`AROS IEWarp library not found: ${AROS_RELEASE_DIR}/Libs/iewarp.library`,
		`case-colliding AROS file: keeping`,
		`local demos_dir="${payload_root}/Demos"`,
		`local coproc_dir="${payload_root}/IE/Coproc"`,
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
		`"$sdk_dir/Include" "$sdk_dir/Docs" "$sdk_dir/Examples/asm"`,
		`"${SCRIPT_DIR}"/sdk/examples/prebuilt/*.ie*`,
		`"${SCRIPT_DIR}"/sdk/examples/prebuilt/*.prg`,
		`coproc_*.ie*) coproc_files+=("$prebuilt_file") ;;`,
		`iewarp_service.ie64) iewarp_worker_files+=("$prebuilt_file") ;;`,
		`*.prg) emutos_demo_files+=("$prebuilt_file") ;;`,
		`cp -f "${coproc_files[@]}" "$coproc_dir/"`,
		`cp -f "${emutos_demo_files[@]}" "$emutos_demos_dir/"`,
		`cp -f "${iewarp_worker_files[@]}" "$aros_system_dir/Libs/"`,
		`verify_staged_share_payload "$payload_root"`,
		`Forbidden live payload location: coproc worker staged under Demos`,
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
		`cp -f "${SCRIPT_DIR}/sdk/README.md" "$sdk_dir/README.md"`,
		`"${SCRIPT_DIR}/sdk/docs/Coprocessor.md"`,
		`"${SCRIPT_DIR}/sdk/docs/demo-matrix.md"`,
		`"${SCRIPT_DIR}/sdk/docs/ehbasic_ie64.md"`,
		`"${SCRIPT_DIR}/sdk/docs/ie_emutos.md"`,
		`"${SCRIPT_DIR}/sdk/docs/iescript.md"`,
		`"${SCRIPT_DIR}/sdk/docs/iewarp.md"`,
		`"${SCRIPT_DIR}/sdk/docs/iemon.md"`,
		`"${SCRIPT_DIR}/sdk/docs/include-files.md"`,
		`"${SCRIPT_DIR}/sdk/docs/sdk-getting-started.md"`,
		`"${SCRIPT_DIR}/sdk/docs/toolchains.md"`,
		`cp -f "${SCRIPT_DIR}"/sdk/examples/asm/*.asm "$sdk_dir/Examples/asm/"`,
		`cp -f "${SCRIPT_DIR}"/sdk/examples/basic/*.bas "$sdk_dir/Examples/basic/"`,
		`cp -f "${SCRIPT_DIR}"/sdk/examples/c/*.c "$sdk_dir/Examples/c/"`,
		`does not include sdk/bin or other host tool binaries`,
		`"${SCRIPT_DIR}/sdk/examples/basic/rotozoomer_basic.bas"`,
		`"${SCRIPT_DIR}/sdk/examples/asm/RotoAPI"`,
		`"${SCRIPT_DIR}/sdk/examples/asm/RotoHW"`,
		`"${SCRIPT_DIR}/sdk/examples/c/RotoAPIc"`,
		`"${SCRIPT_DIR}/sdk/examples/c/RotoHWc"`,
		`cp -f "$aros_demo" "$aros_demos_dir/"`,
		`cp -f "${AROS_RELEASE_DIR}/Prefs/Env-Archive/SYS/def_Tool.info" "${aros_demos_dir}/$(basename "$aros_demo").info"`,
		`"${SCRIPT_DIR}/embedded/ab3d2/_build.zip"`,
		`"ab3d2_source/_build/ie_media/redux-high/"`,
		`"ab3d2_source/_build/ie_unpacked/"`,
		`fat_name = name.lower()`,
		`case-colliding AB3D2 asset differs`,
		`"${demos_dir}/README.TXT"`,
		`"${payload_root}/README.TXT"`,
		`"${systems_dir}/README.TXT"`,
		`AROS files live under Systems/AROS.`,
		`EmuTOS/GEMDOS demo files live under Systems/EmuTOS.`,
		`Systems/AROS/Demos`,
		`Systems/EmuTOS/Demos`,
		`Systems/IntuitionOS`,
		`Systems/IntuitionOS/Boot/iexec.ie64`,
		`Systems/IntuitionOS/IOSSYS is the read-only system subtree`,
		`The live image keeps OS payloads under Systems/AROS, Systems/EmuTOS, and`,
		`SYS:Libs/iewarp_service.ie64`,
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
		`LABEL=IESHARE /var/ie/share vfat defaults,nofail,umask=0022,uid=1000,gid=1000 0 0\n`,
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
		`GOLDEN_STAMP_VERSION="x64-live-golden-v14-pipewire-pactl-ready"`,
		`GOLDEN_STAMP_PATH="${GOLDEN_IMG_PATH}.stamp"`,
		`write_golden_stamp`,
		`expected_golden_stamp`,
		`Golden image stamp mismatch; rebuilding`,
		`write_golden_stamp`,
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
		"/intuition-engine-x64.tar.zst",
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
