package main

import "testing"

func testX86PollLoopCode(addr uint32, mask uint32, jcc byte) []byte {
	code := make([]byte, 12)
	code[0] = 0xA1 // MOV EAX, moffs32
	code[1] = byte(addr)
	code[2] = byte(addr >> 8)
	code[3] = byte(addr >> 16)
	code[4] = byte(addr >> 24)
	code[5] = 0xA9 // TEST EAX, imm32
	code[6] = byte(mask)
	code[7] = byte(mask >> 8)
	code[8] = byte(mask >> 16)
	code[9] = byte(mask >> 24)
	code[10] = jcc
	code[11] = 0xF4 // rel8 -12 back to loop start
	return code
}

func TestCPUX86_TryFastMMIOPollLoop_JNZ(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.EIP = 0x1000
	cpu.running.Store(true)

	reads := 0
	bus.SetVideoStatusReader(func(addr uint32) uint32 {
		reads++
		if reads < 3 {
			return 2
		}
		return 0
	})

	copy(cpu.memory[0x1000:], testX86PollLoopCode(0x0000F008, 2, 0x75))

	if !cpu.tryFastMMIOPollLoop() {
		t.Fatal("expected fast MMIO poll loop to match")
	}
	if cpu.EIP != 0x100C {
		t.Fatalf("EIP = 0x%X, want 0x100C", cpu.EIP)
	}
	if cpu.EAX != 0 {
		t.Fatalf("EAX = 0x%X, want 0 after loop exits", cpu.EAX)
	}
	if reads != 3 {
		t.Fatalf("reads = %d, want 3", reads)
	}
}

func TestCPUX86_TryFastMMIOPollLoop_JZ(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.EIP = 0x1000
	cpu.running.Store(true)

	reads := 0
	bus.SetVideoStatusReader(func(addr uint32) uint32 {
		reads++
		if reads < 4 {
			return 0
		}
		return 2
	})

	copy(cpu.memory[0x1000:], testX86PollLoopCode(0x0000F008, 2, 0x74))

	if !cpu.tryFastMMIOPollLoop() {
		t.Fatal("expected fast MMIO poll loop to match")
	}
	if cpu.EIP != 0x100C {
		t.Fatalf("EIP = 0x%X, want 0x100C", cpu.EIP)
	}
	if cpu.EAX != 2 {
		t.Fatalf("EAX = 0x%X, want 2 after loop exits", cpu.EAX)
	}
	if reads != 4 {
		t.Fatalf("reads = %d, want 4", reads)
	}
}

func TestCPUX86_TryFastMMIOPollLoop_GenericMMIO(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.EIP = 0x1000
	cpu.running.Store(true)

	const guestAddr = 0x0000F044
	const hostAddr = 0x000F0044
	reads := 0
	bus.MapIO(hostAddr, hostAddr+3, func(addr uint32) uint32 {
		reads++
		if reads < 2 {
			return 2
		}
		return 0
	}, nil)

	copy(cpu.memory[0x1000:], testX86PollLoopCode(guestAddr, 2, 0x75))

	if !cpu.tryFastMMIOPollLoop() {
		t.Fatal("expected generic MMIO fast poll loop to match")
	}
	if cpu.EIP != 0x100C {
		t.Fatalf("EIP = 0x%X, want 0x100C", cpu.EIP)
	}
	if cpu.EAX != 0 {
		t.Fatalf("EAX = 0x%X, want 0 after loop exits", cpu.EAX)
	}
	if reads != 2 {
		t.Fatalf("reads = %d, want 2", reads)
	}
}

func TestCPUX86_TryFastMMIOPollLoop_IgnoresNonLoop(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.EIP = 0x1000
	cpu.running.Store(true)

	copy(cpu.memory[0x1000:], []byte{
		0xA1, 0x08, 0xF0, 0x00, 0x00,
		0xA9, 0x02, 0x00, 0x00, 0x00,
		0x75, 0x00, // not a back-edge
	})

	if cpu.tryFastMMIOPollLoop() {
		t.Fatal("non-loop sequence should not match fast MMIO polling")
	}
}

func TestCPUX86_Read32_TranslatedIOUsesSingleBusRead32(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()

	const guestAddr = 0x0000F044
	const hostAddr = 0x000F0044
	reads := 0
	bus.MapIO(hostAddr, hostAddr+3, func(addr uint32) uint32 {
		reads++
		return 0x11223344
	}, nil)

	got := cpu.read32(guestAddr)
	if got != 0x11223344 {
		t.Fatalf("read32 = 0x%08X, want 0x11223344", got)
	}
	if reads != 1 {
		t.Fatalf("read handler calls = %d, want 1", reads)
	}
}

func TestCPUX86_Write32_TranslatedIOUsesSingleBusWrite32(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()

	const guestAddr = 0x0000F020
	const hostAddr = 0x000F0020
	writes := 0
	var gotAddr uint32
	var gotValue uint32
	bus.MapIO(hostAddr, hostAddr+3, nil, func(addr uint32, value uint32) {
		writes++
		gotAddr = addr
		gotValue = value
	})

	cpu.write32(guestAddr, 0x55667788)
	if writes != 1 {
		t.Fatalf("write handler calls = %d, want 1", writes)
	}
	if gotAddr != hostAddr {
		t.Fatalf("write addr = 0x%X, want 0x%X", gotAddr, hostAddr)
	}
	if gotValue != 0x55667788 {
		t.Fatalf("write value = 0x%08X, want 0x55667788", gotValue)
	}
}
