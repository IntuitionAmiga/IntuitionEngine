package main

type relativeMouseCursorAction int

const (
	relativeMouseCursorNone relativeMouseCursorAction = iota
	relativeMouseCursorCapture
	relativeMouseCursorVisible
	relativeMouseCursorRestorePolicy
)

type relativeMouseCaptureInput struct {
	guestRelative      bool
	mouseOverride      bool
	hostX              int
	hostY              int
	releaseRequested   bool
	recaptureRequested bool
}

type relativeMouseCaptureOutput struct {
	clearDeltas     bool
	addDX           int32
	addDY           int32
	suppressButtons bool
	cursorAction    relativeMouseCursorAction
}

type relativeMouseCaptureState struct {
	active       bool
	captured     bool
	hostReleased bool
	lastHostX    int
	lastHostY    int
}

func (s *relativeMouseCaptureState) Update(in relativeMouseCaptureInput) relativeMouseCaptureOutput {
	var out relativeMouseCaptureOutput

	if !in.guestRelative {
		if s.active || s.captured || s.hostReleased {
			out.clearDeltas = true
			out.cursorAction = relativeMouseCursorRestorePolicy
		}
		s.active = false
		s.captured = false
		s.hostReleased = false
		s.lastHostX = in.hostX
		s.lastHostY = in.hostY
		return out
	}

	if !s.active {
		s.active = true
		s.lastHostX = in.hostX
		s.lastHostY = in.hostY
		if !in.mouseOverride {
			out.clearDeltas = true
		}
		if in.releaseRequested {
			s.captured = false
			s.hostReleased = true
			out.cursorAction = relativeMouseCursorVisible
			return out
		}
		s.captured = true
		s.hostReleased = false
		out.cursorAction = relativeMouseCursorCapture
		return out
	}

	if s.hostReleased {
		s.lastHostX = in.hostX
		s.lastHostY = in.hostY
		if in.recaptureRequested {
			s.captured = true
			s.hostReleased = false
			out.suppressButtons = true
			out.cursorAction = relativeMouseCursorCapture
		}
		return out
	}

	if !s.captured {
		s.lastHostX = in.hostX
		s.lastHostY = in.hostY
		if in.recaptureRequested {
			s.captured = true
			out.cursorAction = relativeMouseCursorCapture
		}
		return out
	}

	if in.releaseRequested {
		s.captured = false
		s.hostReleased = true
		s.lastHostX = in.hostX
		s.lastHostY = in.hostY
		out.cursorAction = relativeMouseCursorVisible
		return out
	}

	if in.mouseOverride {
		s.lastHostX = in.hostX
		s.lastHostY = in.hostY
		return out
	}

	dx := in.hostX - s.lastHostX
	dy := in.hostY - s.lastHostY
	s.lastHostX = in.hostX
	s.lastHostY = in.hostY
	out.addDX = int32(dx)
	out.addDY = int32(dy)
	return out
}
