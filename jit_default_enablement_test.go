package main

import (
	"testing"
	"time"
)

func TestJITDefaults_EnableEveryAvailableBackend(t *testing.T) {
	bus := NewMachineBus()

	if got, want := NewCPU64(bus).jitEnabled, jitAvailable || wasmJITSupported; got != want {
		t.Fatalf("IE64 JIT default = %v, want %v", got, want)
	}
	if got, want := NewM68KCPU(bus).m68kJitEnabled, m68kJitAvailable; got != want {
		t.Fatalf("M68K JIT default = %v, want %v", got, want)
	}
	if got, want := NewCPU_Z80(NewZ80BusAdapter(bus)).jitEnabled, z80JitAvailable; got != want {
		t.Fatalf("Z80 JIT default = %v, want %v", got, want)
	}
	if got, want := NewCPU_X86(NewX86BusAdapter(bus)).x86JitEnabled, x86JitAvailable; got != want {
		t.Fatalf("x86 JIT default = %v, want %v", got, want)
	}
}

func TestJITDefaults_RunnersOptOutOnlyExplicitly(t *testing.T) {
	bus := NewMachineBus()

	z80 := NewCPUZ80Runner(bus, CPUZ80Config{})
	if got, want := z80.JITEnabled, z80JitAvailable; got != want {
		t.Fatalf("Z80 runner JIT default = %v, want %v", got, want)
	}
	if NewCPUZ80Runner(bus, CPUZ80Config{DisableJIT: true}).JITEnabled {
		t.Fatal("Z80 DisableJIT did not disable the runner")
	}

	x86 := NewCPUX86Runner(bus, &CPUX86Config{})
	if got, want := x86.jit, x86JitAvailable; got != want {
		t.Fatalf("x86 runner JIT default = %v, want %v", got, want)
	}
	if NewCPUX86Runner(bus, &CPUX86Config{DisableJIT: true}).jit {
		t.Fatal("x86 DisableJIT did not disable the runner")
	}
}

func TestJITDefaults_CoprocessorWorkersStartEnabled(t *testing.T) {
	bus := NewMachineBus()
	tests := []struct {
		name string
		make func() (*CoprocWorker, error)
		got  func(*CoprocWorker) bool
		want bool
	}{
		{
			name: "z80",
			make: func() (*CoprocWorker, error) { return createZ80Worker(bus, []byte{0x76}, 0) }, // HALT
			got:  func(w *CoprocWorker) bool { return w.debugCPU.(*DebugZ80).cpu.jitEnabled },
			want: z80JitAvailable,
		},
		{
			name: "x86",
			make: func() (*CoprocWorker, error) { return createX86Worker(bus, []byte{0xF4}, 0) }, // HLT
			got:  func(w *CoprocWorker) bool { return w.debugCPU.(*DebugX86).cpu.x86JitEnabled },
			want: x86JitAvailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker, err := tt.make()
			if err != nil {
				t.Fatal(err)
			}
			if got := tt.got(worker); got != tt.want {
				t.Fatalf("worker JIT default = %v, want %v", got, tt.want)
			}
			worker.stopCPU()
			select {
			case <-worker.done:
			case <-time.After(time.Second):
				t.Fatal("worker did not stop")
			}
		})
	}
}
