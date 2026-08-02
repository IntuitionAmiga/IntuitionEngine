package main

import (
	"encoding/binary"
	"os"
	"testing"
	"time"
)

const ie64CProcSmokeResultAddress = 0x00080000
const ie64CProcSmokeResult = uint64(0x494536344350524f)

func TestIE64CProcSmokeImageDefaultJIT(t *testing.T) {
	imagePath := os.Getenv("IE64_TOOLCHAIN_IMAGE")
	if imagePath == "" {
		t.Skip("set IE64_TOOLCHAIN_IMAGE through make test-ie64-toolchain")
	}
	image, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read toolchain smoke image: %v", err)
	}
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
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
	if got := binary.LittleEndian.Uint64(cpu.memory[ie64CProcSmokeResultAddress:]); got != ie64CProcSmokeResult {
		t.Fatalf("toolchain smoke result = %#x, want %#x", got, ie64CProcSmokeResult)
	}
}
