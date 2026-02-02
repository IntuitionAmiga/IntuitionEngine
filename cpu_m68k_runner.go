package main

type M68KRunner struct {
	cpu         *M68KCPU
	PerfEnabled bool
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
