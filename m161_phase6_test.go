package main

import (
	"strings"
	"testing"
	"time"
)

func TestIExec_M161_Phase6_VERSION_NoArgs(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION\n", 8*time.Second)
	for _, want := range []string{
		"IntuitionOS 1.16.6\r\n",
		"exec.library 1.16.6 (2026-04-25)\r\n",
		"Copyright \xA9 2026 Zayn Otley\r\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("VERSION output missing %q\noutput=%q", want, output[:min(len(output), 1200)])
		}
	}
	if strings.Contains(output, "IntuitionOS 0.18") {
		t.Fatalf("VERSION output contains stale 0.18 label: %q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_ByName_Library(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION dos.library\n", 8*time.Second)
	if !strings.Contains(output, "dos.library 15.0") {
		t.Fatalf("VERSION dos.library missing resident manifest output\noutput=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_ByName_LibraryFallthrough(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION intuition.library\n", 8*time.Second)
	if !strings.Contains(output, "intuition.library 12.0.1") {
		t.Fatalf("VERSION intuition.library missing file-fallback manifest output\noutput=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_ByPath_Library(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION LIBS:intuition.library\n", 8*time.Second)
	if !strings.Contains(output, "intuition.library 12.0.1") {
		t.Fatalf("VERSION LIBS:intuition.library missing path manifest output\noutput=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_ByName_Service(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION console.handler\n", 8*time.Second)
	if !strings.Contains(output, "console.handler 1.0.1") {
		t.Fatalf("VERSION console.handler missing resident manifest output\noutput=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_ByPath_Handler(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION L:console.handler\n", 8*time.Second)
	if !strings.Contains(output, "console.handler 1.0.1") {
		t.Fatalf("VERSION L:console.handler missing path manifest output\noutput=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_ByName_Command_Fallthrough(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION Dir\n", 8*time.Second)
	if !strings.Contains(output, "Dir 1.0.1") {
		t.Fatalf("VERSION Dir missing file-fallback manifest output\noutput=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_ByPath_Command(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION C:Dir\n", 8*time.Second)
	if !strings.Contains(output, "Dir 1.0.1") {
		t.Fatalf("VERSION C:Dir missing path manifest output\noutput=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_ALL_ResidentOnly(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION ALL\n", 8*time.Second)
	for _, want := range []string{
		"exec.library 1.16.6",
		"console.handler 1.0.1",
		"dos.library 15.0",
		"hardware.resource 1.0.1",
		"input.device 1.0.1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("VERSION ALL missing %q\noutput=%q", want, output[:min(len(output), 1600)])
		}
	}
	if strings.Contains(output, "Dir 1.0.1") {
		t.Fatalf("VERSION ALL must be resident-only, but included command output=%q", output[:min(len(output), 1600)])
	}
	if strings.Contains(output, "graphics.library 11.0.1") {
		t.Fatalf("VERSION ALL must not synthesize non-resident graphics output=%q", output[:min(len(output), 1600)])
	}
}

func TestIExec_M161_Phase6_VERSION_UnknownName(t *testing.T) {
	output := bootAndInjectCommand(t, "\nVERSION nope\n", 8*time.Second)
	if !strings.Contains(output, "VERSION: not found in resident ports or LIBS:, DEVS:, RESOURCES:, L:, C:") {
		t.Fatalf("VERSION nope missing fallback-surface diagnostic\noutput=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M161_Phase6_VERSION_PrintsU16IOSMVersions(t *testing.T) {
	body := string(mustReadRepoBytes(t, "sdk/intuitionos/iexec/cmd/version.s"))
	start := strings.Index(body, ".ver_send_decimal:")
	end := strings.Index(body, ".ver_send_crlf:")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("VERSION decimal formatter labels missing")
	}
	formatter := body[start:end]
	if strings.Contains(formatter, "store.b r1") || strings.Contains(formatter, "load.b  r14") {
		t.Fatalf("VERSION decimal formatter truncates IOSM u16 fields through byte storage")
	}
	for _, want := range []string{"store.w r1, 8(sp)", "load.w  r14, 8(sp)", "load.l  r16, 16(sp)", "move.l  r28, #10000"} {
		if !strings.Contains(formatter, want) {
			t.Fatalf("VERSION decimal formatter missing %q", want)
		}
	}
}

func TestIExec_M161_Phase6_VERSION_DoesNotWaitOnUnknownPublicPorts(t *testing.T) {
	body := string(mustReadRepoBytes(t, "sdk/intuitionos/iexec/cmd/version.s"))
	for _, label := range []string{".ver_query_resident:", ".ver_all_print_one:"} {
		start := strings.Index(body, label)
		if start < 0 {
			t.Fatalf("VERSION source missing %s", label)
		}
		putMsg := strings.Index(body[start:], "syscall #SYS_PUT_MSG")
		guard := strings.Index(body[start:], "jsr     .ver_is_iosm_port_name")
		wait := strings.Index(body[start:], "syscall #SYS_WAIT_PORT")
		if guard < 0 || putMsg < 0 || wait < 0 {
			t.Fatalf("%s missing IOSM guard/PutMsg/WaitPort", label)
		}
		if guard > putMsg {
			t.Fatalf("%s sends MSG_GET_IOSM before checking IOSM-capable resident names", label)
		}
		if putMsg > wait {
			t.Fatalf("%s WaitPort appears before PutMsg", label)
		}
	}
	for _, want := range []string{
		".ver_is_iosm_port_name:",
		"prog_version_name_intuition:",
		"move.q  r2, #ERR_NOTFOUND",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("VERSION source missing non-IOSM public-port guard fragment %q", want)
		}
	}
}
