package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestIExec_M161_Phase1_IOSVersionConstants(t *testing.T) {
	path := filepath.Join("sdk", "include", "iexec.inc")
	vals := parseIncConstants(t, path)
	if got := vals["IOS_VERSION_MAJOR"]; got != 1 {
		t.Fatalf("IOS_VERSION_MAJOR=%d, want 1", got)
	}
	if got := vals["IOS_VERSION_MINOR"]; got != 16 {
		t.Fatalf("IOS_VERSION_MINOR=%d, want 16", got)
	}
	if got := vals["IOS_VERSION_PATCH"]; got != 1 {
		t.Fatalf("IOS_VERSION_PATCH=%d, want 1", got)
	}
	if got := fmt.Sprintf("%d.%d.%d", vals["IOS_VERSION_MAJOR"], vals["IOS_VERSION_MINOR"], vals["IOS_VERSION_PATCH"]); got != "1.16.1" {
		t.Fatalf("IOS_VERSION_STRING=%q, want %q", got, "1.16.1")
	}
}

func TestIExec_M161_Phase1_IOSMConstants(t *testing.T) {
	path := filepath.Join("sdk", "include", "iexec.inc")
	vals := parseIncConstants(t, path)

	want := map[string]uint32{
		"IOSM_MAGIC":               0x4C49424D,
		"IOSM_SCHEMA_VERSION":      1,
		"IOSM_KIND_LIBRARY":        1,
		"IOSM_KIND_DEVICE":         2,
		"IOSM_KIND_HANDLER":        3,
		"IOSM_KIND_RESOURCE":       4,
		"IOSM_KIND_COMMAND":        5,
		"IOSM_OFF_MAGIC":           0,
		"IOSM_OFF_SCHEMA_VERSION":  4,
		"IOSM_OFF_NAME":            8,
		"IOSM_OFF_VERSION":         40,
		"IOSM_OFF_REVISION":        42,
		"IOSM_OFF_TYPE":            44,
		"IOSM_OFF_FLAGS":           48,
		"IOSM_OFF_MSG_ABI_VERSION": 52,
		"IOSM_NAME_SIZE":           32,
		"IOSM_SIZE":                96,
		"IOSM_NOTE_TYPE":           0x494F5331,
	}
	for name, wantVal := range want {
		if got := vals[name]; got != wantVal {
			t.Fatalf("%s=%d (0x%X), want %d (0x%X)", name, got, got, wantVal, wantVal)
		}
	}
}

func TestIExec_M161_Phase1_PortOpcodeConstants(t *testing.T) {
	path := filepath.Join("sdk", "include", "iexec.inc")
	vals := parseIncConstants(t, path)

	getIOSM := vals["MSG_GET_IOSM"]
	listResidents := vals["MSG_LIST_RESIDENTS"]
	parseManifest := vals["DOS_OP_PARSE_MANIFEST"]
	if getIOSM == 0 || listResidents == 0 || parseManifest == 0 {
		t.Fatalf("expected non-zero port opcodes, got MSG_GET_IOSM=%#x MSG_LIST_RESIDENTS=%#x DOS_OP_PARSE_MANIFEST=%#x", getIOSM, listResidents, parseManifest)
	}
	if getIOSM == listResidents || getIOSM == parseManifest || listResidents == parseManifest {
		t.Fatalf("port opcodes must be unique, got MSG_GET_IOSM=%#x MSG_LIST_RESIDENTS=%#x DOS_OP_PARSE_MANIFEST=%#x", getIOSM, listResidents, parseManifest)
	}
	for _, existing := range []string{"CON_MSG_CHAR", "CON_MSG_READLINE", "HWRES_MSG_REQUEST", "HWRES_MSG_GRANTED", "HWRES_MSG_DENIED"} {
		if vals[existing] == getIOSM || vals[existing] == listResidents || vals[existing] == parseManifest {
			t.Fatalf("new opcode collides with %s=%#x", existing, vals[existing])
		}
	}
}

func TestIExec_M161_Phase1_DOSParseManifestLayoutConstants(t *testing.T) {
	path := filepath.Join("sdk", "include", "iexec.inc")
	vals := parseIncConstants(t, path)

	want := map[string]uint32{
		"DOS_PMP_PATH_OFF": 0,
		"DOS_PMP_PATH_MAX": 260,
		"DOS_PMP_IOSM_OFF": 272,
		"DOS_PMP_RC_OFF":   400,
	}
	for name, wantVal := range want {
		if got := vals[name]; got != wantVal {
			t.Fatalf("%s=%d, want %d", name, got, wantVal)
		}
	}
}

func TestIExec_M161_Phase1_PortEnumerationConstant(t *testing.T) {
	path := filepath.Join("sdk", "include", "iexec.inc")
	vals := parseIncConstants(t, path)
	if got := vals["SYSINFO_PORT_NAME_BY_INDEX"]; got != 6 {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX=%d, want 6", got)
	}
}
