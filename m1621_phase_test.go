package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

const (
	execMsgAttachHandler = 0x122
	execMsgDetachHandler = 0x123
	execMsgOpenDevice    = 0x124
	execMsgCloseDevice   = 0x125
	execMsgOpenResource  = 0x126
	execMsgCloseResource = 0x127
	execReplyFlag        = 0x80000000
)

func TestIExec_M1621_ABIFrozenConstantsAndNoPublicSyscalls(t *testing.T) {
	vals := parseIncConstants(t, "sdk/include/iexec.inc")
	want := map[string]uint32{
		"EXEC_MSG_ATTACH_HANDLER": execMsgAttachHandler,
		"EXEC_MSG_DETACH_HANDLER": execMsgDetachHandler,
		"EXEC_MSG_OPEN_DEVICE":    execMsgOpenDevice,
		"EXEC_MSG_CLOSE_DEVICE":   execMsgCloseDevice,
		"EXEC_MSG_OPEN_RESOURCE":  execMsgOpenResource,
		"EXEC_MSG_CLOSE_RESOURCE": execMsgCloseResource,
		"EXEC_REPLY_FLAG":         execReplyFlag,
	}
	for name, value := range want {
		if got, ok := vals[name]; !ok || got != value {
			t.Fatalf("%s=%d ok=%v, want %d", name, got, ok, value)
		}
	}
	for _, forbidden := range []string{
		"SYS_ATTACH_HANDLER",
		"SYS_DETACH_HANDLER",
		"SYS_OPEN_DEVICE",
		"SYS_CLOSE_DEVICE",
		"SYS_OPEN_RESOURCE",
		"SYS_CLOSE_RESOURCE",
		"SYS_M1621_MODULE_HANDLE_OP",
	} {
		if _, ok := vals[forbidden]; ok {
			t.Fatalf("unexpected public syscall constant %s", forbidden)
		}
	}
}

func TestIExec_M1621_OpenDeviceCloseDeviceViaExecLibraryIPC(t *testing.T) {
	rig, term := assembleAndLoadKernel(t)
	images := findAllProgramImages(t, rig.cpu.memory)
	overrideExtraTasks(rig.cpu.memory, images, 1)
	t0 := images[0]
	scratch := uint32(userTask0Stack + 0x100)

	row := m16ModuleRowBase(0)
	clear(rig.cpu.memory[row : row+kdModuleRowStride])
	copy(rig.cpu.memory[row+kdModuleName:row+kdModuleName+portNameLen], append([]byte("input.device"), make([]byte, portNameLen-len("input.device"))...))
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleState:], m16ModStateOnline)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleGeneration:], 7)
	binary.LittleEndian.PutUint16(rig.cpu.memory[row+kdModuleVersion:], 1)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleClass:], modClassDevice)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleOwningTask:], 1)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModulePublicPort:], 1)

	execName := writeTaskImageLiteral(t, rig.cpu.memory, t0, userTask0Code, 0x340, []byte("exec.library\x00"))
	replyName := writeTaskImageLiteral(t, rig.cpu.memory, t0, userTask0Code, 0x360, []byte("m1621.reply\x00"))

	pc := t0
	w := func(instr []byte) { copy(rig.cpu.memory[pc:], instr); pc += 8 }
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, execName))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysFindPort))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 0))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_MOVE, 8, IE64_SIZE_Q, 0, 1, 0, 0)) // exec port
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, replyName))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, pfPublic))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysCreatePort))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 24))
	w(ie64Instr(OP_MOVE, 9, IE64_SIZE_Q, 0, 1, 0, 0)) // reply port
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 4096))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, uint32(memfPublic|memfClear)))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocMem))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 32))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 40))
	w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, 48))
	w(ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 0, 1, 0, 0)) // request VA
	w(ie64Instr(OP_MOVE, 11, IE64_SIZE_Q, 0, 3, 0, 0)) // share handle
	w(ie64Instr(OP_MOVE, 12, IE64_SIZE_L, 1, 0, 0, 1))
	w(ie64Instr(OP_STORE, 12, IE64_SIZE_L, 0, 10, 0, 0))
	for i, v := range []uint32{0x75706e69, 0x65642e74, 0x65636976, 0x00000000} {
		w(ie64Instr(OP_MOVE, 12, IE64_SIZE_L, 1, 0, 0, v))
		w(ie64Instr(OP_STORE, 12, IE64_SIZE_L, 0, 10, 0, uint32(16+i*4)))
	}
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 1, 29, 0, 0))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, execMsgOpenDevice))
	w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_LOAD, 6, IE64_SIZE_Q, 1, 29, 0, 48))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 64))
	w(ie64Instr(OP_STORE, 5, IE64_SIZE_Q, 1, 29, 0, 72))
	w(ie64Instr(OP_STORE, 6, IE64_SIZE_Q, 1, 29, 0, 80))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysPutMsg))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 56))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWaitPort))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 0))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_STORE, 4, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_STORE, 6, IE64_SIZE_Q, 1, 29, 0, 24))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 1, 29, 0, 0))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, execMsgCloseDevice))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch))
	w(ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_MOVE, 6, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysPutMsg))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWaitPort))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 32))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 40))
	w(ie64Instr(OP_STORE, 4, IE64_SIZE_Q, 1, 29, 0, 48))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 56))
	yieldPC := pc
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	w(ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(int32(yieldPC)-int32(pc))))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(2 * time.Second)
	rig.cpu.running.Store(false)
	<-done

	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+56:]); got != 0xCAFE {
		output := term.DrainOutput()
		var dbg []uint64
		for off := uint32(64); off < 160; off += 8 {
			dbg = append(dbg, binary.LittleEndian.Uint64(rig.cpu.memory[scratch+off:]))
		}
		t.Fatalf("task did not reach sentinel, got 0x%X debug=%v output=%q", got, dbg, output[:min(len(output), 400)])
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+0:]); got != uint64(execMsgOpenDevice|execReplyFlag) {
		t.Fatalf("open reply type=0x%X, want 0x%X", got, execMsgOpenDevice|execReplyFlag)
	}
	token := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+8:])
	if token == 0 {
		t.Fatal("OpenDevice returned zero token")
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+16:]); got != 0 {
		t.Fatalf("OpenDevice err=%d, want 0", got)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+24:]); got != 0 {
		t.Fatalf("OpenDevice reply share=%d, want 0", got)
	}
	if got := token >> 56; got != modClassDevice {
		t.Fatalf("token class=%d, want MODCLASS_DEVICE", got)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+32:]); got != uint64(execMsgCloseDevice|execReplyFlag) {
		t.Fatalf("close reply type=0x%X, want 0x%X", got, execMsgCloseDevice|execReplyFlag)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+48:]); got != 0 {
		t.Fatalf("CloseDevice err=%d, want 0", got)
	}
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleOpenCount:]); got != 0 {
		t.Fatalf("device open_count=%d after close, want 0", got)
	}
}

func TestIExec_M1621_OpenDeviceRejectsNonZeroNamePadding(t *testing.T) {
	rig, term := assembleAndLoadKernel(t)
	images := findAllProgramImages(t, rig.cpu.memory)
	overrideExtraTasks(rig.cpu.memory, images, 1)
	t0 := images[0]
	scratch := uint32(userTask0Stack + 0x180)

	row := m16ModuleRowBase(0)
	clear(rig.cpu.memory[row : row+kdModuleRowStride])
	copy(rig.cpu.memory[row+kdModuleName:row+kdModuleName+portNameLen], append([]byte("input"), make([]byte, portNameLen-len("input"))...))
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleState:], m16ModStateOnline)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleGeneration:], 9)
	binary.LittleEndian.PutUint16(rig.cpu.memory[row+kdModuleVersion:], 1)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleClass:], modClassDevice)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModuleOwningTask:], 1)
	binary.LittleEndian.PutUint32(rig.cpu.memory[row+kdModulePublicPort:], 1)

	execName := writeTaskImageLiteral(t, rig.cpu.memory, t0, userTask0Code, 0x3A0, []byte("exec.library\x00"))
	replyName := writeTaskImageLiteral(t, rig.cpu.memory, t0, userTask0Code, 0x3C0, []byte("m1621.badpad\x00"))

	pc := t0
	w := func(instr []byte) { copy(rig.cpu.memory[pc:], instr); pc += 8 }
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, execName))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysFindPort))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 0))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, replyName))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, pfPublic))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysCreatePort))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 4096))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, uint32(memfPublic|memfClear)))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocMem))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 1, 29, 0, 24))
	w(ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 0, 1, 0, 0))
	w(ie64Instr(OP_MOVE, 11, IE64_SIZE_Q, 0, 3, 0, 0))
	w(ie64Instr(OP_MOVE, 12, IE64_SIZE_L, 1, 0, 0, 1))
	w(ie64Instr(OP_STORE, 12, IE64_SIZE_L, 0, 10, 0, 0))
	w(ie64Instr(OP_MOVE, 12, IE64_SIZE_L, 1, 0, 0, 0x75706e69)) // "inpu"
	w(ie64Instr(OP_STORE, 12, IE64_SIZE_L, 0, 10, 0, 16))
	w(ie64Instr(OP_MOVE, 12, IE64_SIZE_L, 1, 0, 0, 0x58580074)) // "t\0XX": malformed non-zero padding
	w(ie64Instr(OP_STORE, 12, IE64_SIZE_L, 0, 10, 0, 20))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 1, 29, 0, 0))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, execMsgOpenDevice))
	w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0))
	w(ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_LOAD, 6, IE64_SIZE_Q, 1, 29, 0, 24))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysPutMsg))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch+64))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 32))
	w(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWaitPort))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, scratch))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 0))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, 8))
	w(ie64Instr(OP_STORE, 4, IE64_SIZE_Q, 1, 29, 0, 16))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, 24))
	yieldPC := pc
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	w(ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(int32(yieldPC)-int32(pc))))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(500 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+24:]); got != 0xCAFE {
		output := term.DrainOutput()
		t.Fatalf("task did not reach sentinel, got 0x%X output=%q", got, output[:min(len(output), 400)])
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+0:]); got != uint64(execMsgOpenDevice|execReplyFlag) {
		t.Fatalf("reply type=0x%X, want 0x%X", got, execMsgOpenDevice|execReplyFlag)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+8:]); got != 0 {
		t.Fatalf("malformed request returned token=%d, want 0", got)
	}
	if got := binary.LittleEndian.Uint64(rig.cpu.memory[scratch+16:]); got != errBadArg {
		t.Fatalf("malformed request err=%d, want ERR_BADARG (%d)", got, errBadArg)
	}
	if got := binary.LittleEndian.Uint32(rig.cpu.memory[row+kdModuleOpenCount:]); got != 0 {
		t.Fatalf("device open_count=%d after malformed request, want 0", got)
	}
}

func TestIExec_M1621_DocsDescribeExecLibraryFastPathAndOnlineOnly(t *testing.T) {
	plan := mustReadRepoFile(t, "sdk/docs/IntuitionOS/M16.2.1-plan.md")
	for _, fragment := range []string{
		"kernel-serviced `exec.library` port",
		"does not demand-load absent non-library modules",
		"opaque tokens",
	} {
		if !strings.Contains(plan, fragment) {
			t.Fatalf("M16.2.1 plan missing contract fragment %q", fragment)
		}
	}
}
