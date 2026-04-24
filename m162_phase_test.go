package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func TestIExec_M162_ClassConstantsAndInternalAliases(t *testing.T) {
	vals := parseIncConstants(t, "sdk/include/iexec.inc")
	want := map[string]uint32{
		"MODCLASS_LIBRARY":  1,
		"MODCLASS_DEVICE":   2,
		"MODCLASS_HANDLER":  3,
		"MODCLASS_RESOURCE": 4,
		"SYS_ADD_HANDLER":   50,
		"SYS_ADD_DEVICE":    51,
		"SYS_ADD_RESOURCE":  52,
	}
	for name, value := range want {
		if got := vals[name]; got != value {
			t.Fatalf("%s=%d, want %d", name, got, value)
		}
	}
}

func TestIExec_M162_ShippedNonLibrariesSelfRegister(t *testing.T) {
	checks := []struct {
		path    string
		syscall string
	}{
		{"sdk/intuitionos/iexec/handler/console_handler.s", "SYS_ADD_HANDLER"},
		{"sdk/intuitionos/iexec/resource/hardware_resource.s", "SYS_ADD_RESOURCE"},
		{"sdk/intuitionos/iexec/dev/input_device.s", "SYS_ADD_DEVICE"},
	}
	for _, tc := range checks {
		src := mustReadRepoFile(t, tc.path)
		if !strings.Contains(src, "syscall #"+tc.syscall) {
			t.Fatalf("%s does not self-register with %s", tc.path, tc.syscall)
		}
		if !strings.Contains(src, "SYS_CREATE_PORT") {
			t.Fatalf("%s no longer creates its compat public port", tc.path)
		}
	}
}

func TestIExec_M162_BootPublishesNonLibraryRegistryRows(t *testing.T) {
	rig, term := assembleAndLoadKernel(t)
	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()

	deadline := time.Now().Add(5 * time.Second)
	want := map[string]uint32{
		"console.handler":   modClassHandler,
		"hardware.resource": modClassResource,
		"input.device":      modClassDevice,
	}
	for time.Now().Before(deadline) {
		all := true
		for name, class := range want {
			rowIndex, ok := m16FindModuleRowByName(rig.cpu.memory, name)
			if !ok {
				all = false
				break
			}
			row := m16ModuleRowBase(rowIndex)
			if binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleState:]) != m16ModStateOnline ||
				binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleClass:]) != class ||
				binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleGeneration:]) == 0 ||
				binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModulePublicPort:]) == 0 {
				all = false
				break
			}
		}
		if all {
			rig.cpu.running.Store(false)
			<-done
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	rig.cpu.running.Store(false)
	<-done
	output := term.DrainOutput()
	var rows []string
	for i := uint32(0); i < kdModuleMax; i++ {
		row := m16ModuleRowBase(i)
		name := strings.TrimRight(string(rig.cpu.memory[row+kdModuleName:row+kdModuleName+portNameLen]), "\x00")
		if name == "" {
			continue
		}
		rows = append(rows, name)
	}
	t.Fatalf("non-library rows did not all become ONLINE; rows=%v output=%q", rows, output[:min(len(output), 1200)])
}

func TestIExec_M162_OpenLibraryExRejectsOnlineNonLibraryRows(t *testing.T) {
	rig, term := assembleAndLoadKernel(t)
	images := findAllProgramImages(t, rig.cpu.memory)
	overrideExtraTasks(rig.cpu.memory, images, 1)
	t0 := images[0]
	scratch0 := uint32(userTask0Stack + 0x100)

	row := m16ModuleRowBase(0)
	clear(rig.cpu.memory[row : row+kdModuleRowStride])
	copy(rig.cpu.memory[row+kdModuleName:row+kdModuleName+portNameLen], append([]byte("input.device"), make([]byte, portNameLen-len("input.device"))...))
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleState:], m16ModStateOnline)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleGeneration:], 3)
	binary.LittleEndian.PutUint16(rig.cpu.memory[row+kdModuleVersion:], 1)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleClass:], modClassDevice)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleOwningTask:], 1)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModulePublicPort:], 1)

	nameAddr := writeTaskImageLiteral(t, rig.cpu.memory, t0, userTask0Code, 0x340, []byte("input.device\x00"))

	pc := t0
	w := func(instr []byte) { copy(rig.cpu.memory[pc:], instr); pc += 8 }
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, nameAddr))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 1))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysOpenLibraryEx))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 0))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 16))
	yieldPC := pc
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	w(ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(int32(yieldPC)-int32(pc))))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(500 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch0+16:]); got != 0xCAFE {
		output := term.DrainOutput()
		t.Fatalf("task 0 did not reach sentinel, got 0x%X output=%q", got, output[:min(len(output), 400)])
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch0+0:]); got != 0 {
		t.Fatalf("OpenLibraryEx returned token=%d for device row, want 0", got)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch0+8:]); got != errBadArg {
		t.Fatalf("OpenLibraryEx err=%d for device row, want ERR_BADARG (%d)", got, errBadArg)
	}
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleOpenCount:]); got != 0 {
		t.Fatalf("device row open_count=%d after rejected OpenLibraryEx, want 0", got)
	}
}

func TestIExec_M162_AddDeviceRejectsLibraryLoadingRowWithWaiters(t *testing.T) {
	rig, term := assembleAndLoadKernel(t)
	images := findAllProgramImages(t, rig.cpu.memory)
	overrideExtraTasks(rig.cpu.memory, images, 1)
	markTrustedBootTaskLayouts(rig.cpu.memory, maxTasks)
	t0 := images[0]
	scratch0 := uint32(userTask0Stack + 0x100)
	name := "rogue.device"
	nameAddr := writeTaskImageLiteral(t, rig.cpu.memory, t0, userTask0Code, 0x340, []byte(name+"\x00"))

	rowIndex := uint32(0xFFFFFFFF)
	for i := uint32(0); i < kdModuleMax; i++ {
		row := m16ModuleRowBase(i)
		if rig.cpu.memory[row+kdModuleName] == 0 {
			rowIndex = i
			break
		}
	}
	if rowIndex == 0xFFFFFFFF {
		t.Fatal("no free module row found")
	}
	row := m16ModuleRowBase(rowIndex)
	clear(rig.cpu.memory[row : row+kdModuleRowStride])
	copy(rig.cpu.memory[row+kdModuleName:row+kdModuleName+portNameLen], append([]byte(name), make([]byte, portNameLen-len(name))...))
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleState:], m16ModStateLoading)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleClass:], modClassLibrary)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleGeneration:], 7)
	binary.LittleEndian.PutUint64(rig.cpu.memory[row+kdModuleWaitersHead:], 1)

	waiter0 := m16ModuleWaiterBase(0)
	binary.LittleEndian.PutUint32(rig.cpu.memory[waiter0+kdModuleWaiterTask:], 0xDEAD)
	binary.LittleEndian.PutUint16(rig.cpu.memory[waiter0+kdModuleWaiterMinVer:], 1)
	binary.LittleEndian.PutUint16(rig.cpu.memory[waiter0+kdModuleWaiterSigBit:], 16)
	binary.LittleEndian.PutUint32(rig.cpu.memory[waiter0+kdModuleWaiterOutcome:], scratch0+64)
	binary.LittleEndian.PutUint32(rig.cpu.memory[waiter0+12:], rowIndex)
	binary.LittleEndian.PutUint64(rig.cpu.memory[waiter0+kdModuleWaiterNext:], 0)

	pc := t0
	w := func(instr []byte) { copy(rig.cpu.memory[pc:], instr); pc += 8 }
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, nameAddr))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, pfPublic))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysCreatePort))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, nameAddr))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 1))
	w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch0))
	w(ie64Instr(OP_LOAD, 4, IE64_SIZE_Q, 0, 29, 0, 8))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAddDevice))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch0))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 24))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 32))
	yieldPC := pc
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	w(ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(int32(yieldPC)-int32(pc))))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(500 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch0+32:]); got != 0xCAFE {
		output := term.DrainOutput()
		t.Fatalf("task 0 did not reach sentinel, got 0x%X output=%q", got, output[:min(len(output), 400)])
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch0+16:]); got != 0 {
		t.Fatalf("CreatePort err=%d, want 0", got)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch0+24:]); got != errBadArg {
		t.Fatalf("AddDevice err=%d, want ERR_BADARG (%d)", got, errBadArg)
	}
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleState:]); got != m16ModStateLoading {
		t.Fatalf("row state=%d after rejected AddDevice, want LOADING (%d)", got, m16ModStateLoading)
	}
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleClass:]); got != modClassLibrary {
		t.Fatalf("row class=%d after rejected AddDevice, want library (%d)", got, modClassLibrary)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[row+kdModuleWaitersHead:]); got != 1 {
		t.Fatalf("waiter head=%d after rejected AddDevice, want preserved token 1", got)
	}
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[waiter0+kdModuleWaiterOutcome:]); got != scratch0+64 {
		t.Fatalf("waiter outcome=0x%X after rejected AddDevice, want preserved 0x%X", got, scratch0+64)
	}
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleOpenCount:]); got != 0 {
		t.Fatalf("row open_count=%d after rejected AddDevice, want 0", got)
	}
}

func TestIExec_M162_StartupSequenceDoesNotLaunchModulePaths(t *testing.T) {
	body := mustReadRepoFile(t, "sdk/intuitionos/iexec/assets/system/S/Startup-Sequence")
	for _, forbidden := range []string{"L/", "DEVS/", "RESOURCES/", ".handler", ".device", ".resource"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Startup-Sequence still contains module launch fragment %q:\n%s", forbidden, body)
		}
	}
}

func TestIExec_M162_DOSBootPolicyLaunchesHardwareBeforeInputBeforeShell(t *testing.T) {
	src := mustReadRepoFile(t, "sdk/intuitionos/iexec/lib/dos_library.s")
	hw := strings.Index(src, "move.l  r1, #BOOT_MANIFEST_ID_HWRES")
	input := strings.Index(src, "move.l  r1, #BOOT_MANIFEST_ID_INPUT")
	shell := strings.Index(src, "Launch the boot shell")
	if hw < 0 || input < 0 || shell < 0 {
		t.Fatalf("dos.library missing M16.2 eager policy or shell launch markers")
	}
	if !(hw < input && input < shell) {
		t.Fatalf("unexpected eager launch order: hw=%d input=%d shell=%d", hw, input, shell)
	}
}

func TestIExec_M162_HWResOpsRequireCurrentResourceRegistryGeneration(t *testing.T) {
	src := mustReadRepoFile(t, "sdk/intuitionos/iexec/iexec.s")
	for _, fragment := range []string{
		"m162_hwres_current_generation_check:",
		"move.l  r14, #MODCLASS_RESOURCE",
		"beq     r10, r11, .do_add_resource",
		"jsr     m162_hwres_current_generation_check",
	} {
		if !strings.Contains(src, fragment) {
			t.Fatalf("iexec.s missing hardware.resource generation guard fragment %q", fragment)
		}
	}
}
