package main

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"
)

func TestIExec_M161_Phase4_ExecLibraryPort_PresentFromBoot(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	if _, _, ok := findPublicPortIDByName(client.rig.cpu.memory, "exec.library"); !ok {
		t.Fatalf("exec.library public port missing")
	}
}

func TestIExec_M161_Phase4_ExecLibraryPort_KernelOwned(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	_, portBase, ok := findPublicPortIDByName(client.rig.cpu.memory, "exec.library")
	if !ok {
		t.Fatalf("exec.library public port missing")
	}
	if got := client.rig.cpu.memory[portBase+kdPortOwner]; got != 0xFF {
		t.Fatalf("exec.library owner=%#x, want kernel-owned sentinel 0xff", got)
	}
}

func TestIExec_M161_Phase4_GetIOSM_ExecLibrary(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)

	got := runM161GetIOSMClient(t, client, "exec.library", true)
	if got.findErr != 0 || got.allocErr != 0 || got.replyErr != 0 || got.waitErr != 0 {
		t.Fatalf("GET_IOSM exec.library errors find=%d alloc=%d reply=%d wait=%d", got.findErr, got.allocErr, got.replyErr, got.waitErr)
	}

	vals := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))
	if binary.LittleEndian.Uint32(got.manifest[0:4]) != uint32(vals["IOSM_MAGIC"]) {
		t.Fatalf("exec IOSM magic=%#x", binary.LittleEndian.Uint32(got.manifest[0:4]))
	}
	if got.manifest[8] != byte(vals["IOSM_KIND_LIBRARY"]) {
		t.Fatalf("exec IOSM kind=%d, want library", got.manifest[8])
	}
	if name := cStringFromFixed(got.manifest[16:48]); name != "exec.library" {
		t.Fatalf("exec IOSM name=%q", name)
	}
	if major, minor, patch := binary.LittleEndian.Uint16(got.manifest[10:12]), binary.LittleEndian.Uint16(got.manifest[12:14]), binary.LittleEndian.Uint16(got.manifest[14:16]); major != 1 || minor != 16 || patch != 1 {
		t.Fatalf("exec IOSM version=%d.%d.%d, want 1.16.1", major, minor, patch)
	}
	if copyright := cStringFromFixed(got.manifest[72:120]); copyright != "Copyright \xA9 2026 Zayn Otley" {
		t.Fatalf("exec IOSM copyright=%q", copyright)
	}
	if !bytes.Equal(got.manifest[120:128], make([]byte, 8)) {
		t.Fatalf("exec IOSM reserved2 not zero: % x", got.manifest[120:128])
	}
}

func TestIExec_M161_Phase4_GetIOSM_ExecLibraryZeroShare(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	errBadArg := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["ERR_BADARG"]

	got := runM161GetIOSMClient(t, client, "exec.library", false)
	if got.findErr != 0 {
		t.Fatalf("FindPort(exec.library) err=%d, want 0", got.findErr)
	}
	if got.replyErr != uint64(errBadArg) {
		t.Fatalf("exec.library zero-share reply=%d, want ERR_BADARG (%d)", got.replyErr, errBadArg)
	}
	if got.waitErr != 0 {
		t.Fatalf("exec.library zero-share WaitPort err=%d, want 0", got.waitErr)
	}
}

func TestIExec_M161_Phase4_ExecLibraryIOSM_StaticSecurityShape(t *testing.T) {
	body := string(mustReadRepoBytes(t, "sdk/intuitionos/iexec/iexec.s"))
	for _, want := range []string{
		"kern_iosm_descriptor:",
		"kern_share_backing_for_handle:",
		"bne     r3, r21, .ksbf_badarg",
		"load.b  r20, KD_PORT_OWNER(r1)",
		"bne     r20, r11, .kpiel_no",
		"dc.b    \"exec.library\", 0",
		"dc.w    IOS_VERSION_PATCH",
	} {
		if !bytes.Contains([]byte(body), []byte(want)) {
			t.Fatalf("iexec.s missing %q", want)
		}
	}
}
