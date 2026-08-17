package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestHostSDKPublicHeaderContract(t *testing.T) {
	header, err := os.ReadFile("sdk/include/intuitionengine.h")
	if err != nil {
		t.Fatal(err)
	}
	text := string(header)
	for _, want := range []string{
		"IE_TARGET_IE64", "IE_TARGET_M68K", "IE_TARGET_Z80", "IE_TARGET_6502", "IE_TARGET_X86",
		"IE_HAS_BANK_WINDOWS", "IE_HAS_X86_PORT_IO", "IE_HAS_IE64_CONTROL_REGISTERS", "IE_HAS_IE64_ATOMICS", "IE_HAS_IE64_FPU",
		"IE_VIDEO_", "IE_AUDIO_", "IE_INPUT_", "IE_FILE_", "IE_NET_", "IE_COPROC_", "IE_VOODOO_",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("public header is missing %q", want)
		}
	}
	for _, forbidden := range []string{"sdk/include/ie64.h", "ie64/platform.h"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("public header mentions removed private header %q", forbidden)
		}
	}
	defineRE := regexp.MustCompile(`(?m)^#define ([A-Z0-9_]+) 0x([0-9a-fA-F]+)u$`)
	defines := make(map[string]uint64)
	for _, match := range defineRE.FindAllStringSubmatch(text, -1) {
		value, err := strconv.ParseUint(match[2], 16, 64)
		if err != nil {
			t.Fatalf("parse %s: %v", match[1], err)
		}
		defines[match[1]] = value
	}
	for name, want := range map[string]uint64{
		"IE_VIDEO_CTRL":              VIDEO_CTRL,
		"IE_VIDEO_CTRL_ENABLE":       videoCtrlEnable,
		"IE_VIDEO_CTRL_PRESENT_HOLD": videoCtrlPresentationHold,
		"IE_INPUT_TERM_OUT":          TERM_OUT,
		"IE_AUDIO_BASE":              AUDIO_CTRL,
		"IE_FILE_BASE":               FILE_IO_BASE,
		"IE_EXEC_BASE":               EXEC_BASE,
		"IE_COPROC_BASE":             COPROC_BASE,
		"IE_NET_SOCKET_BASE":         HOST_SOCKET_BASE,
		"IE_VOODOO_BASE":             VOODOO_BASE,
	} {
		if got, ok := defines[name]; !ok || got != want {
			t.Errorf("public header %s = 0x%X, want executable-source value 0x%X", name, got, want)
		}
	}
}

func TestHostSDKVideoPresentationHoldDocumentation(t *testing.T) {
	documentation, err := os.ReadFile("sdk/docs/include-files-host-sdk.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"IE_VIDEO_CTRL_ENABLE",
		"IE_VIDEO_CTRL_PRESENT_HOLD",
		"retained",
		"framebuffer",
		"final blit completion",
	} {
		if !strings.Contains(string(documentation), want) {
			t.Errorf("host SDK include documentation is missing %q", want)
		}
	}
}

func TestHostSDKAssemblyIncludeInventory(t *testing.T) {
	for _, name := range []string{"ie32.inc", "ie64.inc", "ie68.inc", "ie80.inc", "ie65.inc", "ie86.inc"} {
		if _, err := os.Stat(filepath.Join("sdk", "include", name)); err != nil {
			t.Fatalf("missing public assembly include %s: %v", name, err)
		}
	}
	for _, removed := range []string{"ie64.h", filepath.Join("ie64", "platform.h")} {
		if _, err := os.Stat(filepath.Join("sdk", "include", removed)); !os.IsNotExist(err) {
			t.Fatalf("removed private header remains %s", removed)
		}
	}
}

func TestHostSDKLinuxARM64DistributionContract(t *testing.T) {
	armScript, err := os.ReadFile("scripts/dist-host-sdk-linux-arm64.sh")
	if err != nil {
		t.Fatal(err)
	}
	armText := string(armScript)
	genericScript, err := os.ReadFile("scripts/dist-host-sdk-linux-amd64.sh")
	if err != nil {
		t.Fatal(err)
	}
	armText += "\n" + string(genericScript)
	for _, want := range []string{
		"GOOS=linux",
		"GOARCH=arm64",
		"aarch64",
		"qemu-aarch64",
		"--sysroot",
		"ELF",
		"sha256sum",
		"cmp",
	} {
		if !strings.Contains(armText, want) {
			t.Errorf("ARM64 Host SDK distributor is missing %q", want)
		}
	}

	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makeText := string(makefile)
	for _, want := range []string{
		"dist-host-sdk-linux-arm64",
		"test-host-sdk-arm64",
		"intuition-engine-host-sdk-linux-arm64.tar.xz",
		"QEMU_AARCH64",
	} {
		if !strings.Contains(makeText, want) {
			t.Errorf("Makefile is missing ARM64 Host SDK contract %q", want)
		}
	}
}

func TestHostSDKLinuxARM64ReleaseAndPayloadContract(t *testing.T) {
	for _, name := range []string{"build_x64_ie_img.sh", "Makefile"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		wants := []string{"intuition-engine-host-sdk-linux-amd64", "intuition-engine-host-sdk-linux-arm64"}
		if name == "build_x64_ie_img.sh" {
			wants = append(wants, "HOST_SDK_ARCHIVES", "HOST_SDK_CHECKSUMS", "HOST_SDK_EXTRACTORS")
		}
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s is missing shared ARM64 payload artefact %q", name, want)
			}
		}
	}
	stageScript, err := os.ReadFile("scripts/stage_ieshare_payload.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stageScript), "build_x64_ie_img.sh") ||
		!strings.Contains(string(stageScript), "stage_share_payload") ||
		!strings.Contains(string(stageScript), "verify_staged_share_payload") {
		t.Error("shared IESHARE staging entrypoint does not delegate the complete payload contract")
	}

	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makeText := string(makefile)
	for _, archive := range []string{
		"intuition-engine-host-sdk-linux-amd64.tar.xz*",
		"intuition-engine-host-sdk-linux-arm64.tar.xz*",
	} {
		if !strings.Contains(makeText, archive) {
			t.Errorf("browser manifest does not exclude %q", archive)
		}
	}

	index, err := os.ReadFile("intuitionengine.com/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "intuition-engine-host-sdk-linux-arm64.tar.xz") {
		t.Error("website does not offer the ARM64 Host SDK download")
	}

	gitignore, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	for _, exception := range []string{
		"!intuitionengine.com/assets/intuition-engine-host-sdk-linux-arm64.tar.xz",
		"!intuitionengine.com/assets/intuition-engine-host-sdk-linux-arm64.tar.xz.sha256",
	} {
		if !strings.Contains(string(gitignore), exception) {
			t.Errorf(".gitignore does not retain the ARM64 release asset %q", exception)
		}
	}
}

func TestHostSDKWindowsDistributionContract(t *testing.T) {
	windowsScript, err := os.ReadFile("scripts/dist-host-sdk-windows-amd64.sh")
	if err != nil {
		t.Fatal(err)
	}
	windowsText := string(windowsScript)
	genericScript, err := os.ReadFile("scripts/dist-host-sdk-linux-amd64.sh")
	if err != nil {
		t.Fatal(err)
	}
	windowsText += "\n" + string(genericScript)
	for _, want := range []string{
		"HOST_SDK_ARCH=amd64",
		"HOST_SDK_GOOS=windows",
		"CGO_ENABLED=0",
		"GOOS=windows",
		"GOARCH=amd64",
		".exe",
		"x86_64-w64-mingw32-gcc",
		"host_gcc_libdir",
		"bootstrap_windows_guest_tools",
		"non-system DLL",
		"HOST_SDK_ARCHIVE_FORMAT=zip",
		"HOST_SDK_NAME=intuition-engine-host-sdk-windows-amd64",
	} {
		if !strings.Contains(windowsText, want) {
			t.Errorf("Windows Host SDK distributor is missing %q", want)
		}
	}

	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makeText := string(makefile)
	for _, want := range []string{
		"dist-host-sdk-windows-amd64",
		"test-host-sdk-windows-amd64",
		"intuition-engine-host-sdk-windows-amd64.zip",
	} {
		if !strings.Contains(makeText, want) {
			t.Errorf("Makefile is missing Windows Host SDK contract %q", want)
		}
	}
}

func TestHostSDKWindowsReleaseAndPayloadContract(t *testing.T) {
	buildScript, err := os.ReadFile("build_x64_ie_img.sh")
	if err != nil {
		t.Fatal(err)
	}
	buildText := string(buildScript)
	windowsScript, err := os.ReadFile("scripts/dist-host-sdk-windows-amd64.sh")
	if err != nil {
		t.Fatal(err)
	}
	buildText += "\n" + string(windowsScript)
	genericScript, err := os.ReadFile("scripts/dist-host-sdk-linux-amd64.sh")
	if err != nil {
		t.Fatal(err)
	}
	buildText += "\n" + string(genericScript)
	for _, want := range []string{
		"intuition-engine-host-sdk-windows-amd64.zip",
		"intuition-engine-host-sdk-windows-amd64.zip.sha256",
		"unzip",
		"objdump -p",
		"zip -X",
	} {
		if !strings.Contains(buildText, want) {
			t.Errorf("shared payload producer is missing Windows Host SDK contract %q", want)
		}
	}

	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makeText := string(makefile)
	for _, want := range []string{
		"dist-host-sdk-windows-amd64",
		"! -name 'intuition-engine-host-sdk-windows-amd64.zip*'",
	} {
		if !strings.Contains(makeText, want) {
			t.Errorf("Makefile is missing Windows Host SDK payload contract %q", want)
		}
	}

	index, err := os.ReadFile("intuitionengine.com/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"intuition-engine-host-sdk-windows-amd64.zip",
		"intuition-engine-host-sdk-windows-amd64.zip.sha256",
	} {
		if !strings.Contains(string(index), want) {
			t.Errorf("website is missing Windows Host SDK link %q", want)
		}
	}

	gitignore, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"!intuitionengine.com/assets/intuition-engine-host-sdk-windows-amd64.zip",
		"!intuitionengine.com/assets/intuition-engine-host-sdk-windows-amd64.zip.sha256",
	} {
		if !strings.Contains(string(gitignore), want) {
			t.Errorf(".gitignore does not retain Windows Host SDK asset %q", want)
		}
	}
}
