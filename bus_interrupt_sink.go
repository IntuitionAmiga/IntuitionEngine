package main

type InterruptMask uint8

const (
	IntMaskVBI     InterruptMask = 1 << 0
	IntMaskDLI     InterruptMask = 1 << 1
	IntMaskBlitter InterruptMask = 1 << 2
)

type InterruptSink interface {
	Pulse(mask InterruptMask)
}

type LevelTriggeredInterruptSink interface {
	Assert(mask InterruptMask)
	Deassert(mask InterruptMask)
	Ack(mask InterruptMask)
	SetMask(mask InterruptMask, masked bool)
}

type interruptLevelState struct {
	active InterruptMask
	masked InterruptMask
}

func (s *interruptLevelState) assert(mask InterruptMask) bool {
	s.active |= mask
	return s.pending()
}

func (s *interruptLevelState) deassert(mask InterruptMask) bool {
	s.active &^= mask
	return s.pending()
}

func (s *interruptLevelState) ack(mask InterruptMask) bool {
	return s.pending()
}

func (s *interruptLevelState) setMask(mask InterruptMask, masked bool) bool {
	if masked {
		s.masked |= mask
		return s.pending()
	}
	s.masked &^= mask
	return s.pending()
}

func (s *interruptLevelState) pending() bool {
	return s.active&^s.masked != 0
}

// pendingMask returns the set of currently-active, unmasked causes. Level-
// triggered sinks record this (not the call argument) so that acknowledging or
// masking one source does not lose another that is still pending.
func (s *interruptLevelState) pendingMask() InterruptMask {
	return s.active &^ s.masked
}
