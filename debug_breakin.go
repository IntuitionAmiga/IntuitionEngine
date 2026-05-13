package main

// RequestBreakIn asks the adapter to stop at the next safe instruction
// boundary and report the stop through its existing breakpoint event channel.
func (d *Debug6502) RequestBreakIn() { d.events.RequestBreakIn() }
func (d *DebugZ80) RequestBreakIn()  { d.events.RequestBreakIn() }
func (d *DebugM68K) RequestBreakIn() { d.events.RequestBreakIn() }
func (d *DebugIE32) RequestBreakIn() { d.events.RequestBreakIn() }
func (d *DebugIE64) RequestBreakIn() { d.events.RequestBreakIn() }
func (d *DebugX86) RequestBreakIn()  { d.events.RequestBreakIn() }

func (d *Debug6502) BreakInRequested() bool { return d.events.BreakInRequested() }
func (d *DebugZ80) BreakInRequested() bool  { return d.events.BreakInRequested() }
func (d *DebugM68K) BreakInRequested() bool { return d.events.BreakInRequested() }
func (d *DebugIE32) BreakInRequested() bool { return d.events.BreakInRequested() }
func (d *DebugIE64) BreakInRequested() bool { return d.events.BreakInRequested() }
func (d *DebugX86) BreakInRequested() bool  { return d.events.BreakInRequested() }

func (d *Debug6502) ConsumeBreakIn() bool { return d.events.ConsumeBreakIn() }
func (d *DebugZ80) ConsumeBreakIn() bool  { return d.events.ConsumeBreakIn() }
func (d *DebugM68K) ConsumeBreakIn() bool { return d.events.ConsumeBreakIn() }
func (d *DebugIE32) ConsumeBreakIn() bool { return d.events.ConsumeBreakIn() }
func (d *DebugIE64) ConsumeBreakIn() bool { return d.events.ConsumeBreakIn() }
func (d *DebugX86) ConsumeBreakIn() bool  { return d.events.ConsumeBreakIn() }

func publishAdapterBreakIn(cpu DebuggableCPU, sink *adapterEventSink) {
	publishBreakInAt(sink, cpu.GetPC())
}

func publishBreakInAt(sink *adapterEventSink, pc uint64) {
	sink.Publish(BreakpointEvent{
		Address:   pc,
		IsBreakIn: true,
	})
}
