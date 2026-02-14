package main

import "sync"

type M68KRunner struct {
	cpu         *M68KCPU
	PerfEnabled bool

	execMu     sync.Mutex
	execDone   chan struct{}
	execActive bool
}

func NewM68KRunner(cpu *M68KCPU) *M68KRunner {
	return &M68KRunner{cpu: cpu}
}

func (r *M68KRunner) LoadProgram(filename string) error {
	return r.cpu.LoadProgram(filename)
}

func (r *M68KRunner) Reset() {
	r.cpu.Reset()
}

func (r *M68KRunner) Execute() {
	r.cpu.PerfEnabled = r.PerfEnabled
	r.cpu.ExecuteInstruction()
}

func (r *M68KRunner) CPU() *M68KCPU {
	return r.cpu
}

func (r *M68KRunner) IsRunning() bool {
	return r.cpu.Running()
}

func (r *M68KRunner) StartExecution() {
	r.execMu.Lock()
	defer r.execMu.Unlock()
	if r.execActive {
		return
	}
	r.execActive = true
	r.cpu.SetRunning(true)
	r.execDone = make(chan struct{})
	go func() {
		defer func() {
			r.execMu.Lock()
			r.execActive = false
			close(r.execDone)
			r.execMu.Unlock()
		}()
		r.Execute()
	}()
}

func (r *M68KRunner) Stop() {
	r.execMu.Lock()
	if !r.execActive {
		r.cpu.SetRunning(false)
		r.execMu.Unlock()
		return
	}
	r.cpu.SetRunning(false)
	done := r.execDone
	r.execMu.Unlock()
	<-done
}
