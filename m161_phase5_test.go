package main

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"
	"time"
)

type m161PortNameByIndexResult struct {
	taskID uint64
	err    uint64
	name   [32]byte
}

func TestIExec_M161_Phase5_PortNameByIndex_Iteration(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)

	want := []string{
		"console.handler",
		"dos.library",
		"hardware.resource",
		"input.device",
		"graphics.library",
	}
	for i, name := range want {
		got := runM161PortNameByIndexClient(t, client, uint32(i), true)
		if got.err != 0 {
			t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX(%d) err=%d, want 0", i, got.err)
		}
		if got.taskID == 0 {
			t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX(%d) taskID=0, want owner task id", i)
		}
		if gotName := cStringFromFixed(got.name[:]); gotName != name {
			t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX(%d) name=%q, want %q", i, gotName, name)
		}
	}

	got := runM161PortNameByIndexClient(t, client, uint32(len(want)), true)
	if got.err != 0 {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX sentinel err=%d, want 0", got.err)
	}
	if got.taskID != 0 {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX sentinel taskID=%d, want 0", got.taskID)
	}
	if !bytes.Equal(got.name[:], make([]byte, len(got.name))) {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX sentinel name=%q, want zeroed buffer", got.name)
	}
}

func TestIExec_M161_Phase5_PortNameByIndex_BadBufferPointer(t *testing.T) {
	client := bootM161Phase3Task0ClientRig(t)
	errBadArg := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["ERR_BADARG"]

	got := runM161PortNameByIndexClient(t, client, 0, false)
	if got.err != uint64(errBadArg) {
		t.Fatalf("SYSINFO_PORT_NAME_BY_INDEX bad buffer err=%d, want ERR_BADARG (%d)", got.err, errBadArg)
	}
}

func runM161PortNameByIndexClient(t *testing.T, client *m161Task0ClientRig, index uint32, goodBuffer bool) m161PortNameByIndexResult {
	t.Helper()

	const (
		offName     = 0x300
		offTaskID   = 0x328
		offErr      = 0x330
		offSentinel = 0x338
	)

	mem := client.rig.cpu.memory
	clear(mem[client.data+offName : client.data+offName+0x40])

	pc := client.t0
	w := func(instr []byte) {
		if pc+uint32(len(instr)) > client.t0+MMU_PAGE_SIZE {
			t.Fatalf("PORT_NAME_BY_INDEX client exceeds executable scratch page: pc=%#x base=%#x", pc, client.t0)
		}
		copy(mem[pc:], instr)
		pc += 8
	}

	sysinfoPortNameByIndex := parseIncConstants(t, filepath.Join("sdk", "include", "iexec.inc"))["SYSINFO_PORT_NAME_BY_INDEX"]

	w(ie64Instr(OP_SUB, 31, IE64_SIZE_L, 1, 31, 0, 16))
	w(ie64Instr(OP_MOVE, 29, IE64_SIZE_L, 1, 0, 0, client.data))
	w(ie64Instr(OP_STORE, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, sysinfoPortNameByIndex))
	w(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, index))
	if goodBuffer {
		w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, client.data+offName))
	} else {
		w(ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0x1ff0000))
	}
	w(ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysGetSysInfo))
	w(ie64Instr(OP_LOAD, 29, IE64_SIZE_Q, 0, 31, 0, 0))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offTaskID))
	w(ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 29, 0, offErr))
	w(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xCAFE))
	w(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 29, 0, offSentinel))
	w(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	resetM161Task0ClientState(client)
	runRigForDuration(client.rig, 300*time.Millisecond)

	if got := binary.LittleEndian.Uint64(mem[client.data+offSentinel:]); got != 0xCAFE {
		output := client.term.DrainOutput()
		t.Fatalf("runM161PortNameByIndexClient(%d): sentinel=%#x err=%d taskID=%d output=%q",
			index,
			got,
			binary.LittleEndian.Uint64(mem[client.data+offErr:]),
			binary.LittleEndian.Uint64(mem[client.data+offTaskID:]),
			output[:min(len(output), 800)])
	}

	var res m161PortNameByIndexResult
	res.taskID = binary.LittleEndian.Uint64(mem[client.data+offTaskID:])
	res.err = binary.LittleEndian.Uint64(mem[client.data+offErr:])
	copy(res.name[:], mem[client.data+offName:client.data+offName+32])
	return res
}

func cStringFromFixed(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
