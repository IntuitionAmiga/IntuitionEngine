package main

type TraceRingEntry struct {
	CPUName  string
	PC       uint64
	HexBytes string
	Mnemonic string
}

type TimelineEvent struct {
	Seq        uint64
	Kind       string
	CPUID      int
	PC         uint64
	SnapshotID uint64
	Detail     string
}

type DebugTraceRing struct {
	enabled bool
	size    int
	buf     []TraceRingEntry
	next    int
	full    bool
}

func NewDebugTraceRing(size int) *DebugTraceRing {
	if size <= 0 {
		size = 4096
	}
	return &DebugTraceRing{size: size, buf: make([]TraceRingEntry, size)}
}

func (r *DebugTraceRing) SetEnabled(enabled bool) {
	if r != nil {
		r.enabled = enabled
	}
}

func (r *DebugTraceRing) Resize(size int) {
	if r == nil {
		return
	}
	if size <= 0 {
		size = 4096
	}
	r.size = size
	r.buf = make([]TraceRingEntry, size)
	r.next = 0
	r.full = false
}

func (r *DebugTraceRing) Enabled() bool {
	return r != nil && r.enabled
}

func (r *DebugTraceRing) Add(entry TraceRingEntry) {
	if r == nil || !r.enabled || r.size <= 0 {
		return
	}
	r.buf[r.next] = entry
	r.next = (r.next + 1) % r.size
	if r.next == 0 {
		r.full = true
	}
}

func (r *DebugTraceRing) Tail(n int) []TraceRingEntry {
	if r == nil || r.size <= 0 {
		return nil
	}
	count := r.next
	if r.full {
		count = r.size
	}
	if n <= 0 || n > count {
		n = count
	}
	out := make([]TraceRingEntry, 0, n)
	start := (r.next - n + r.size) % r.size
	for i := 0; i < n; i++ {
		out = append(out, r.buf[(start+i)%r.size])
	}
	return out
}
