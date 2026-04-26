package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	dosResidentAdd    = 0x321
	dosResidentRemove = 0x322
	dosResidentList   = 0x323
	dosRcliMagic      = 0x494C4352
)

func TestIExec_M1643_ABIConstantsAndVersionBumps(t *testing.T) {
	vals := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))
	want := map[string]uint32{
		"IOS_VERSION_PATCH":       7,
		"DOS_RESIDENT_ADD":        dosResidentAdd,
		"DOS_RESIDENT_REMOVE":     dosResidentRemove,
		"DOS_RESIDENT_LIST":       dosResidentList,
		"DOS_RCMD_NAME_OFF":       0,
		"DOS_RCMD_NAME_MAX":       260,
		"DOS_RCMD_RC_OFF":         260,
		"DOS_RCMD_IOSM_OFF":       272,
		"DOS_RCLI_MAGIC":          dosRcliMagic,
		"DOS_RCLI_VERSION":        1,
		"DOS_RCLI_HDR_SIZE":       32,
		"DOS_RCLI_REC_SIZE":       64,
		"DOS_RCLI_FLAG_TRUNCATED": 1,
		"RSIV_VERSION":            1,
		"RSIV_HEADER_SIZE":        32,
		"RSIV_RECORD_SIZE":        64,
	}
	for name, value := range want {
		if got := vals[name]; got != value {
			t.Fatalf("%s=%#x, want %#x", name, got, value)
		}
	}
}

func TestIExec_M1643_DOSResidentCommandCacheSourceContract(t *testing.T) {
	src := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")
	for _, want := range []string{
		"#DOS_RESIDENT_ADD",
		"#DOS_RESIDENT_REMOVE",
		"#DOS_RESIDENT_LIST",
		".dos_do_resident_add:",
		".dos_do_resident_remove:",
		".dos_do_resident_list:",
		".dos_resident_find_by_command:",
		".dos_run_launch_resident_elf:",
		"#DOS_RCLI_MAGIC",
		"#DOS_RCLI_FLAG_TRUNCATED",
		"#IOSM_KIND_COMMAND",
		"#MODF_ASLR_CAPABLE",
		"prog_doslib_resident_cmd_cache:",
		"DOS_RCMD_CACHE_ROW_SZ",
		".dos_resident_cache_find_free_or_match:",
		".dos_resident_cache_copy_iosm:",
		".dos_resident_cache_free_row:",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("dos.library missing M16.4.3 resident command cache fragment %q", want)
		}
	}
	for _, forbidden := range []string{
		"A full cache implementation snapshots validated command ELF bytes here",
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("dos.library still contains resident-cache scaffold fragment %q", forbidden)
		}
	}
	if strings.Contains(src, "SYS_RESIDENT") {
		t.Fatal("DOS resident command cache must not add a new resident syscall")
	}
	if strings.Contains(src, "store.q r23, 704(r29)              ; resolved command path") ||
		strings.Contains(src, "load.q  r1, 704(r29)") {
		t.Fatal("resident add must not keep resolved command path in ELF parser scratch 704(r29)")
	}
}

func TestIExec_M1643_ResidentCommandSurfaceUsesDOSCache(t *testing.T) {
	src := mustReadRepoFile(t, "sdk/intuitionos/iexec/cmd/resident.s")
	for _, want := range []string{
		"#DOS_RESIDENT_ADD",
		"#DOS_RESIDENT_REMOVE",
		"#DOS_RESIDENT_LIST",
		"#SYS_FREE_MEM",
		"Resident: all remove unsupported",
		"Resident: added",
		"Resident: removed",
		"Resident usage: Resident [<name> ADD|REMOVE]",
		"dc.w    1\n    dc.w    2\n    dc.w    0",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("Resident command missing M16.4.3 fragment %q", want)
		}
	}
}

func TestIExec_M1643_ShellCommandResidentAppearsInResidentAll(t *testing.T) {
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	output := bootAndInjectCommandWithBootstrapHostRoot(t, hostRoot, "\nRESIDENT C:Dir ADD\nRESIDENT C:List ADD\nRESIDENT all\n", 18*time.Second)
	if strings.Count(output, "Resident: added") < 2 {
		t.Fatalf("Resident command ADD did not report success output=%q", output[:min(len(output), 1200)])
	}
	if !strings.Contains(output, "Dir") {
		t.Fatalf("Resident ALL did not include DOS-owned command resident Dir output=%q", output[:min(len(output), 1200)])
	}
	if !strings.Contains(output, "List") {
		t.Fatalf("Resident ALL did not include DOS-owned command resident List output=%q", output[:min(len(output), 1200)])
	}
}

func TestIExec_M1643_ShellCommandResidentRemoveUpdatesInventory(t *testing.T) {
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	output := bootAndInjectCommandWithBootstrapHostRoot(t, hostRoot, "\nRESIDENT C:Dir ADD\nRESIDENT C:List ADD\nRESIDENT C:Dir REMOVE\nRESIDENT all\n", 20*time.Second)
	if strings.Contains(output, "GURU MEDITATION") {
		t.Fatalf("Resident command REMOVE crashed output=%q", output[:min(len(output), 1400)])
	}
	if !strings.Contains(output, "Resident: removed") {
		t.Fatalf("Resident command REMOVE did not report success output=%q", output[:min(len(output), 1400)])
	}
	if strings.Contains(output, "\r\nDir\r\n") {
		t.Fatalf("Resident ALL still listed removed Dir command output=%q", output[:min(len(output), 1400)])
	}
	if !strings.Contains(output, "List") {
		t.Fatalf("Resident ALL lost remaining List command after Dir removal output=%q", output[:min(len(output), 1400)])
	}
}

func TestIExec_M1643_DOSRunLaunchesCommandFromResidentCacheAfterHostRemoval(t *testing.T) {
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	writeHostRootFileBytes(t, hostRoot, "S/Startup-Sequence", []byte("RESIDENT C:Dir ADD\nECHO READY\n"))
	rig, term := assembleAndLoadKernelWithBootstrapHostRoot(t, hostRoot)
	rig.cpu.running.Store(true)
	bootDone := make(chan struct{})
	go func() { rig.cpu.Execute(); close(bootDone) }()
	time.Sleep(8 * time.Second)
	rig.cpu.running.Store(false)
	<-bootDone
	startupOutput := term.DrainOutput()
	if !strings.Contains(startupOutput, "Resident: added") || !strings.Contains(startupOutput, "READY") {
		t.Fatalf("startup did not resident C:Dir before host removal output=%q", startupOutput[:min(len(startupOutput), 1400)])
	}
	if err := os.Remove(filepath.Join(hostRoot, "IOSSYS", "C", "Dir")); err != nil {
		t.Fatalf("remove host-backed C/Dir after residency: %v", err)
	}

	for _, ch := range "DIR C:\n" {
		term.EnqueueByte(byte(ch))
	}
	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(8 * time.Second)
	rig.cpu.running.Store(false)
	<-done
	output := term.DrainOutput()
	if strings.Contains(output, "Unknown Command") || strings.Contains(output, "GURU MEDITATION") {
		t.Fatalf("resident cached DIR did not launch after host file removal output=%q", output[:min(len(output), 1600)])
	}
	for _, want := range []string{"Version", "List"} {
		if !strings.Contains(output, want) {
			t.Fatalf("resident cached DIR output missing %q after host file removal output=%q", want, output[:min(len(output), 1600)])
		}
	}
}

func TestIExec_M1643_DocsMentionUniversalUserlandResidency(t *testing.T) {
	for _, rel := range []string{
		"sdk/docs/IntuitionOS/IExec.md",
		"sdk/docs/IntuitionOS/ELF.md",
		"sdk/docs/IntuitionOS/HostFS.md",
		"sdk/docs/IntuitionOS/Toolchain.md",
		"IntuitionOS_Roadmap.md",
		"sdk/intuitionos/iexec/assets/system/S/Help",
	} {
		body := mustReadRepoFile(t, rel)
		for _, want := range []string{
			"Universal Userland Residency",
			"DOS_RESIDENT_ADD",
			"resident command cache",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing M16.4.3 doc fragment %q", rel, want)
			}
		}
	}
}
