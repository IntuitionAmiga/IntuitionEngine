package main

import (
	"encoding/binary"
	"os"
	"testing"
	"time"
)

const ie64CProcSmokeResultAddress = 0x00080000
const ie64CProcSmokeResult = uint64(0x494536344350524f)

func runIE64CProcImageWithRAM(t *testing.T, environment string, expected, activeRAM, interruptVector uint64) *CPU64 {
	t.Helper()
	imagePath := os.Getenv(environment)
	if imagePath == "" {
		t.Skipf("set %s through make test-ie64-toolchain", environment)
	}
	image, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read toolchain smoke image: %v", err)
	}
	bus := NewMachineBus()
	bus.ApplyProfileVisibleCeiling(activeRAM)
	cpu := NewCPU64(bus)
	cpu.interruptVector = interruptVector
	if err := cpu.LoadFlatProgramBytes(image); err != nil {
		t.Fatalf("load toolchain smoke image: %v", err)
	}
	cpu.jitEnabled = true

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("toolchain smoke image timed out under the default JIT")
	}
	if got := binary.LittleEndian.Uint64(cpu.memory[ie64CProcSmokeResultAddress:]); got != expected {
		t.Fatalf("toolchain result = %#x, want %#x (PC=%#x R1=%#x SP=%#x)",
			got, expected, cpu.PC, cpu.regs[1], cpu.regs[31])
	}
	return cpu
}

func runIE64CProcImage(t *testing.T, environment string, expected uint64) {
	t.Helper()
	runIE64CProcImageWithRAM(t, environment, expected, uint64(DEFAULT_MEMORY_SIZE), 0)
}

func TestIE64CProcSmokeImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_IMAGE", ie64CProcSmokeResult)
}

func TestIE64CProcABIImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_ABI_IMAGE", 0x4142495041535345)
}

func TestIE64CProcLibraryImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_LIB_IMAGE", 0x4c49425041535345)
}

func TestIE64CProcBuiltinImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_BUILTIN_IMAGE", 0x4255494c54494e53)
}

func TestIE64CProcHaltImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_HALT_IMAGE", 0x48414c5450415353)
}

func TestIE64CProcInterruptImageDefaultJIT(t *testing.T) {
	runIE64CProcImageWithRAM(t, "IE64_TOOLCHAIN_INTERRUPT_IMAGE",
		0x494e545250415353, uint64(DEFAULT_MEMORY_SIZE), 0x70000)
}

func TestIE64CProcAtomicMisalignedImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_ATOMIC_MISALIGNED_IMAGE", 0x41544f4d4641554c)
}

func TestIE64CProcAtomicApertureImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_ATOMIC_APERTURE_IMAGE", 0x41544f4d4641554c)
}

func TestIE64CProcAssertImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_ASSERT_IMAGE", 0x4153534552544f4b)
}

func TestIE64CProcAssertFailureImageDefaultJIT(t *testing.T) {
	runIE64CProcImage(t, "IE64_TOOLCHAIN_ASSERT_FAILURE_IMAGE", 0x4153534552544641)
}

func TestIE64CProcStartupRejectsLowRAMDefaultJIT(t *testing.T) {
	cpu := runIE64CProcImageWithRAM(t, "IE64_TOOLCHAIN_IMAGE", 0, 0x9efff, 0)
	if cpu.regs[1] != 1 {
		t.Fatalf("low-RAM startup R1 = %#x, want %#x (PC=%#x SP=%#x)",
			cpu.regs[1], uint64(1), cpu.PC, cpu.regs[31])
	}
}
