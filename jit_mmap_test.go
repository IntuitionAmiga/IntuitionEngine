// jit_mmap_test.go - Tests for executable memory allocation

//go:build (amd64 || arm64) && linux

package main

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// mapPermsForAddr parses /proc/self/maps and returns the permission string
// ("rwxp" style) for the mapping that contains addr. Returns empty string
// if addr is not in any mapping.
func mapPermsForAddr(t *testing.T, addr uintptr) string {
	t.Helper()
	f, err := os.Open("/proc/self/maps")
	if err != nil {
		t.Fatalf("open /proc/self/maps: %v", err)
	}
	defer f.Close()

	scan := bufio.NewScanner(f)
	// /proc/self/maps lines can be long for VMAs with heavy fragmentation.
	scan.Buffer(make([]byte, 64*1024), 1024*1024)

	for scan.Scan() {
		line := scan.Text()
		// Format: START-END PERMS OFFSET DEV INODE PATH
		// e.g., "7f0000000000-7f0000001000 rw-p 00000000 00:00 0"
		dash := strings.IndexByte(line, '-')
		if dash < 0 {
			continue
		}
		space := strings.IndexByte(line, ' ')
		if space < 0 || space < dash {
			continue
		}
		startStr := line[:dash]
		endStr := line[dash+1 : space]
		start, err1 := strconv.ParseUint(startStr, 16, 64)
		end, err2 := strconv.ParseUint(endStr, 16, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if uintptr(start) <= addr && addr < uintptr(end) {
			rest := strings.TrimLeft(line[space:], " ")
			permEnd := strings.IndexByte(rest, ' ')
			if permEnd < 0 {
				return rest
			}
			return rest[:permEnd]
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatalf("scan /proc/self/maps: %v", err)
	}
	return ""
}

func TestExecMem_AllocAndFree(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	if em.writable == nil {
		t.Fatal("writable view should not be nil")
	}
	if em.exec == nil {
		t.Fatal("exec view should not be nil")
	}
	if em.Used() != 0 {
		t.Fatalf("Used = %d, want 0", em.Used())
	}
}

func TestExecMem_AllocAndCall(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	var code []byte
	switch runtime.GOARCH {
	case "amd64":
		// x86-64: MOV EAX, 42; RET
		code = []byte{
			0xB8, 0x2A, 0x00, 0x00, 0x00, // MOV EAX, 42
			0xC3, // RET
		}
	case "arm64":
		// ARM64: MOV X0, #42; RET
		code = []byte{
			0x40, 0x05, 0x80, 0xD2, // MOV X0, #42
			0xC0, 0x03, 0x5F, 0xD6, // RET
		}
	default:
		t.Skip("unsupported architecture")
	}

	addr, err := em.Write(code)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if addr == 0 {
		t.Fatal("addr should not be 0")
	}

	// Call the function and verify it returns 42.
	// Go function values are pointers to a runtime.funcval struct whose
	// first field (fn) is the code pointer. We need two levels of
	// indirection: the function value must be a pointer to a memory
	// location that contains the code address.
	result := callNativeRet(addr)
	if result != 42 {
		t.Fatalf("result = %d, want 42", result)
	}
	runtime.KeepAlive(em)
}

func TestExecMem_Alignment(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	// Write a 3-byte sequence, then a 5-byte sequence.
	// Second write should be 16-byte aligned.
	addr1, err := em.Write([]byte{0x90, 0x90, 0x90}) // 3 bytes
	if err != nil {
		t.Fatalf("first Write failed: %v", err)
	}

	addr2, err := em.Write([]byte{0x90, 0x90, 0x90, 0x90, 0x90}) // 5 bytes
	if err != nil {
		t.Fatalf("second Write failed: %v", err)
	}

	if addr2%16 != 0 {
		t.Fatalf("addr2 = 0x%X, not 16-byte aligned", addr2)
	}
	if addr2 <= addr1 {
		t.Fatalf("addr2 (0x%X) should be after addr1 (0x%X)", addr2, addr1)
	}
}

func TestExecMem_Reset(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	em.Write([]byte{0x90, 0x90, 0x90})
	if em.Used() == 0 {
		t.Fatal("Used should be > 0 after Write")
	}

	em.Reset()
	if em.Used() != 0 {
		t.Fatalf("Used = %d after Reset, want 0", em.Used())
	}
}

func TestExecMem_Exhausted(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	// Try to write more than the buffer size
	bigCode := make([]byte, 5000)
	_, err = em.Write(bigCode)
	if err == nil {
		t.Fatal("expected error when writing beyond capacity")
	}
}

// M15.6 G1: dual-mapping W^X tests.
//
// The JIT host memory backend must map the same physical pages twice:
// a writable view (RW, not executable) and an execution view (RX, not
// writable). At no point does any view have both PROT_WRITE and
// PROT_EXEC. Execution runs against the execution view; emit and
// PatchRel32At go through the writable view.

func TestExecMem_DualViewPermissions(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	writableAddr := uintptr(unsafe.Pointer(&em.writable[0]))
	execAddr := uintptr(unsafe.Pointer(&em.exec[0]))

	if writableAddr == execAddr {
		t.Fatalf("writable view and execution view share the same address 0x%X; "+
			"dual-mapping requires distinct virtual addresses aliasing the same physical pages",
			writableAddr)
	}

	writablePerms := mapPermsForAddr(t, writableAddr)
	execPerms := mapPermsForAddr(t, execAddr)

	// Writable view: read + write, no execute.
	if !strings.HasPrefix(writablePerms, "rw-") {
		t.Errorf("writable view perms = %q, want \"rw-\" prefix (read+write, no execute)",
			writablePerms)
	}
	if strings.Contains(writablePerms, "x") {
		t.Errorf("writable view perms = %q, must not include execute", writablePerms)
	}

	// Execution view: read + execute, no write.
	if !strings.HasPrefix(execPerms, "r-x") {
		t.Errorf("execution view perms = %q, want \"r-x\" prefix (read+execute, no write)",
			execPerms)
	}
	if strings.Contains(execPerms, "w") {
		t.Errorf("execution view perms = %q, must not include write", execPerms)
	}
}

func TestExecMem_NoRWXView(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	for label, addr := range map[string]uintptr{
		"writable": uintptr(unsafe.Pointer(&em.writable[0])),
		"exec":     uintptr(unsafe.Pointer(&em.exec[0])),
	} {
		perms := mapPermsForAddr(t, addr)
		if strings.Contains(perms, "w") && strings.Contains(perms, "x") {
			t.Errorf("%s view perms = %q: W^X violation, no view may have both write and execute",
				label, perms)
		}
	}
}

func TestExecMem_WriteReturnsExecView(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	addr, err := em.Write([]byte{0x90, 0x90, 0x90, 0x90})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	execBase := uintptr(unsafe.Pointer(&em.exec[0]))
	execEnd := execBase + uintptr(len(em.exec))
	if addr < execBase || addr >= execEnd {
		t.Errorf("Write returned 0x%X, want address inside execution view [0x%X, 0x%X)",
			addr, execBase, execEnd)
	}

	writableBase := uintptr(unsafe.Pointer(&em.writable[0]))
	writableEnd := writableBase + uintptr(len(em.writable))
	if addr >= writableBase && addr < writableEnd {
		t.Errorf("Write returned writable-view address 0x%X; must return execution-view address",
			addr)
	}
}

func TestExecMem_ExecViewReadsReflectWritableWrites(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	code := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	execAddr, err := em.Write(code)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got := (*[6]byte)(unsafe.Pointer(execAddr))
	for i := range code {
		if got[i] != code[i] {
			t.Errorf("exec view byte[%d] = 0x%02X, want 0x%02X (same physical pages)",
				i, got[i], code[i])
		}
	}
}

func TestPatchRel32At_WritesThroughWritableView(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	code := []byte{0xE9, 0x00, 0x00, 0x00, 0x00} // JMP rel32, disp = 0
	execAddr, err := em.Write(code)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	patchExecAddr := execAddr + 1 // displacement field

	targetExec := execAddr + 100
	PatchRel32At(patchExecAddr, targetExec)

	execBytes := (*[4]byte)(unsafe.Pointer(patchExecAddr))
	disp := int32(uint32(execBytes[0]) |
		uint32(execBytes[1])<<8 |
		uint32(execBytes[2])<<16 |
		uint32(execBytes[3])<<24)
	wantDisp := int32(targetExec - (patchExecAddr + 4))
	if disp != wantDisp {
		t.Errorf("disp through exec view = %d, want %d", disp, wantDisp)
	}

	if uintptr(execBytes[0])+uintptr(execBytes[1])+uintptr(execBytes[2])+uintptr(execBytes[3]) == 0 {
		t.Error("patch did not land (all zeros); PatchRel32At must write through the writable view backing")
	}
}

func TestPatchRel32At_ExecViewIsReadOnly(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	_, err = em.Write([]byte{0xE9, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	execPerms := mapPermsForAddr(t, uintptr(unsafe.Pointer(&em.exec[0])))
	if strings.Contains(execPerms, "w") {
		t.Fatalf("execution view must remain read-only after Write; perms = %q", execPerms)
	}

	// Probe /proc/self/maps after a probable patch path mutates writable:
	// writable must still NOT be executable.
	writablePerms := mapPermsForAddr(t, uintptr(unsafe.Pointer(&em.writable[0])))
	if strings.Contains(writablePerms, "x") {
		t.Fatalf("writable view must remain non-executable after Write; perms = %q", writablePerms)
	}
}

func TestExecMem_MultipleWrites(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem failed: %v", err)
	}
	defer em.Free()

	var retCode []byte
	switch runtime.GOARCH {
	case "amd64":
		retCode = []byte{0xC3} // RET
	case "arm64":
		retCode = []byte{0xC0, 0x03, 0x5F, 0xD6} // RET
	default:
		t.Skip("unsupported architecture")
	}

	// Write multiple blocks
	addrs := make([]uintptr, 10)
	for i := range 10 {
		addr, err := em.Write(retCode)
		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
		addrs[i] = addr
	}

	// All addresses should be unique and aligned
	seen := make(map[uintptr]bool)
	for i, addr := range addrs {
		if seen[addr] {
			t.Fatalf("duplicate address at index %d: 0x%X", i, addr)
		}
		seen[addr] = true
		if addr%16 != 0 {
			t.Fatalf("addr[%d] = 0x%X, not 16-byte aligned", i, addr)
		}
	}
}
