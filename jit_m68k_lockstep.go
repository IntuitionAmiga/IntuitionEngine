//go:build amd64 && (linux || windows || darwin)

package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type m68kJITLockstepMode uint8

const (
	m68kJITLockstepReference m68kJITLockstepMode = iota + 1
	m68kJITLockstepCandidate
)

type m68kJITLockstepSession struct {
	mode         m68kJITLockstepMode
	fromPC       uint32
	toPC         uint32
	maxSamples   int
	pcSyncWindow uint64

	mu        sync.Mutex
	reference map[uint64]m68kJITLockstepSnapshot
	refByPC   map[uint32][]m68kJITLockstepSnapshot
	candByPC  map[uint32]int
	refTail   int
	samples   int
	mismatch  *m68kJITLockstepMismatch
}

type m68kJITLockstepBoundary struct {
	Count       uint64
	BlockPC     uint32
	RetPC       uint32
	RetCount    uint32
	ChainCount  uint32
	ChainBudget uint32
	ExitSignal  bool
	NeedIO      uint32
	NeedHelper  uint32
	Exception   uint32
	NeedInval   uint32
}

type m68kJITLockstepSnapshot struct {
	Count   uint64
	PC      uint32
	SR      uint16
	D       [8]uint32
	A       [8]uint32
	Windows []m68kJITLockstepMemoryWindow
}

type m68kJITLockstepMemoryWindow struct {
	Label string
	Addr  uint32
	Len   uint32
	Hash  uint64
	Data  []byte
}

type m68kJITLockstepMismatch struct {
	Boundary  m68kJITLockstepBoundary
	Reference m68kJITLockstepSnapshot
	Candidate m68kJITLockstepSnapshot
	Reason    string
}

func newM68KJITLockstepReference(fromPC, toPC uint32, maxSamples int) *m68kJITLockstepSession {
	if maxSamples <= 0 {
		maxSamples = 1 << 20
	}
	return &m68kJITLockstepSession{
		mode:         m68kJITLockstepReference,
		fromPC:       fromPC,
		toPC:         toPC,
		maxSamples:   maxSamples,
		pcSyncWindow: 4096,
		reference:    make(map[uint64]m68kJITLockstepSnapshot, 4096),
		refByPC:      make(map[uint32][]m68kJITLockstepSnapshot, 4096),
	}
}

func newM68KJITLockstepCandidate(fromPC, toPC uint32, maxSamples int, reference map[uint64]m68kJITLockstepSnapshot) *m68kJITLockstepSession {
	if maxSamples <= 0 {
		maxSamples = 1 << 20
	}
	refCopy := make(map[uint64]m68kJITLockstepSnapshot, len(reference))
	refByPC := make(map[uint32][]m68kJITLockstepSnapshot, len(reference))
	for k, v := range reference {
		cloned := v.clone()
		refCopy[k] = cloned
		refByPC[cloned.PC] = append(refByPC[cloned.PC], cloned)
	}
	for pc := range refByPC {
		sort.Slice(refByPC[pc], func(i, j int) bool {
			return refByPC[pc][i].Count < refByPC[pc][j].Count
		})
	}
	return &m68kJITLockstepSession{
		mode:         m68kJITLockstepCandidate,
		fromPC:       fromPC,
		toPC:         toPC,
		maxSamples:   maxSamples,
		pcSyncWindow: 4096,
		reference:    refCopy,
		refByPC:      refByPC,
		candByPC:     make(map[uint32]int, len(refByPC)),
	}
}

func (s *m68kJITLockstepSession) ReferenceSnapshot() map[uint64]m68kJITLockstepSnapshot {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[uint64]m68kJITLockstepSnapshot, len(s.reference))
	for k, v := range s.reference {
		out[k] = v.clone()
	}
	return out
}

func (s *m68kJITLockstepSession) Mismatch() *m68kJITLockstepMismatch {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mismatch == nil {
		return nil
	}
	cp := *s.mismatch
	cp.Reference = cp.Reference.clone()
	cp.Candidate = cp.Candidate.clone()
	return &cp
}

func (s *m68kJITLockstepSession) recordReference(cpu *M68KCPU, count uint64) {
	if s == nil || cpu == nil || s.mode != m68kJITLockstepReference {
		return
	}
	inRange := s.inRange(cpu.PC) || s.inRange(cpu.lastExecPC)
	s.mu.Lock()
	defer s.mu.Unlock()
	if inRange {
		s.refTail = 64
	}
	if !inRange && s.refTail <= 0 {
		return
	}
	if s.samples >= s.maxSamples {
		return
	}
	if !inRange {
		s.refTail--
	}
	if _, exists := s.reference[count]; exists {
		return
	}
	snap := m68kJITLockstepCapture(cpu, count)
	s.reference[count] = snap
	s.refByPC[snap.PC] = append(s.refByPC[snap.PC], snap)
	s.samples++
}

func (s *m68kJITLockstepSession) compareCandidate(cpu *M68KCPU, b m68kJITLockstepBoundary) bool {
	if s == nil || cpu == nil || s.mode != m68kJITLockstepCandidate {
		return true
	}
	if !s.inRange(b.BlockPC) && !s.inRange(b.RetPC) && !s.inRange(cpu.PC) {
		return true
	}
	candidate := m68kJITLockstepCapture(cpu, b.Count)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mismatch != nil {
		return false
	}
	if s.samples >= s.maxSamples {
		return true
	}
	s.samples++
	ordinal := s.candByPC[candidate.PC]
	s.candByPC[candidate.PC] = ordinal + 1
	if refs := s.refByPC[candidate.PC]; ordinal < len(refs) {
		ref := refs[ordinal]
		if reason := ref.diff(candidate); reason != "" {
			s.mismatch = &m68kJITLockstepMismatch{
				Boundary:  b,
				Reference: ref.clone(),
				Candidate: candidate,
				Reason:    fmt.Sprintf("PC-ordinal mismatch at %08X visit %d: %s; reference_count=%d candidate_count=%d delta=%d", candidate.PC, ordinal+1, reason, ref.Count, candidate.Count, int64(candidate.Count)-int64(ref.Count)),
			}
			return false
		}
		return true
	}

	ref, ok := s.reference[b.Count]
	if !ok || ref.PC != candidate.PC {
		var (
			best      m68kJITLockstepSnapshot
			bestDelta uint64
			found     bool
		)
		for _, snap := range s.refByPC[candidate.PC] {
			delta := uint64(0)
			if snap.Count > candidate.Count {
				delta = snap.Count - candidate.Count
			} else {
				delta = candidate.Count - snap.Count
			}
			if delta > s.pcSyncWindow {
				continue
			}
			if !found || delta < bestDelta {
				best = snap
				bestDelta = delta
				found = true
			}
		}
		if !found {
			s.mismatch = &m68kJITLockstepMismatch{
				Boundary:  b,
				Candidate: candidate,
				Reason:    fmt.Sprintf("missing reference snapshot at count %d and no %08X reference within +/- %d retired instructions", b.Count, candidate.PC, s.pcSyncWindow),
			}
			return false
		}
		ref = best
		if reason := ref.diff(candidate); reason != "" {
			s.mismatch = &m68kJITLockstepMismatch{
				Boundary:  b,
				Reference: ref.clone(),
				Candidate: candidate,
				Reason:    fmt.Sprintf("PC-synchronized after count miss: %s; reference_count=%d candidate_count=%d delta=%d", reason, ref.Count, candidate.Count, int64(candidate.Count)-int64(ref.Count)),
			}
			return false
		}
		return true
	}
	if reason := ref.diff(candidate); reason != "" {
		s.mismatch = &m68kJITLockstepMismatch{
			Boundary:  b,
			Reference: ref.clone(),
			Candidate: candidate,
			Reason:    reason,
		}
		return false
	}
	return true
}

func (s *m68kJITLockstepSession) inRange(pc uint32) bool {
	if s == nil {
		return false
	}
	if s.fromPC == 0 && s.toPC == 0 {
		return true
	}
	return pc >= s.fromPC && pc <= s.toPC
}

func m68kJITLockstepCapture(cpu *M68KCPU, count uint64) m68kJITLockstepSnapshot {
	snap := m68kJITLockstepSnapshot{
		Count: count,
		PC:    cpu.PC,
		SR:    cpu.SR,
		D:     cpu.DataRegs,
		A:     cpu.AddrRegs,
	}
	snap.Windows = m68kJITLockstepCaptureWindows(cpu)
	return snap
}

func m68kJITLockstepCaptureWindows(cpu *M68KCPU) []m68kJITLockstepMemoryWindow {
	if cpu == nil {
		return nil
	}
	var windows []m68kJITLockstepMemoryWindow
	add := func(label string, addr uint32, size uint32) {
		if size == 0 {
			return
		}
		if uint64(addr) >= uint64(len(cpu.memory)) {
			windows = append(windows, m68kJITLockstepMemoryWindow{Label: label, Addr: addr, Len: size})
			return
		}
		end := uint64(addr) + uint64(size)
		if end > uint64(len(cpu.memory)) {
			end = uint64(len(cpu.memory))
		}
		mem := cpu.memory[int(addr):int(end)]
		w := m68kJITLockstepMemoryWindow{
			Label: label,
			Addr:  addr,
			Len:   uint32(len(mem)),
			Hash:  m68kJITLockstepHash(mem),
		}
		windows = append(windows, w)
	}
	sp := cpu.AddrRegs[7]
	stackStart := uint32(0)
	if sp >= 16 {
		stackStart = sp - 16
	}
	add("stack[A7-16..A7+32)", stackStart, 48)
	add("stack[24(A7)]", sp+24, 4)
	for _, reg := range []struct {
		label string
		addr  uint32
	}{
		{"A1-0x40..A1+0x40", cpu.AddrRegs[1]},
		{"A3-0x40..A3+0x40", cpu.AddrRegs[3]},
	} {
		start := uint32(0)
		if reg.addr >= 0x40 {
			start = reg.addr - 0x40
		}
		add(reg.label, start, 0x80)
	}
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].Addr == windows[j].Addr {
			return windows[i].Label < windows[j].Label
		}
		return windows[i].Addr < windows[j].Addr
	})
	return windows
}

func (s m68kJITLockstepSnapshot) clone() m68kJITLockstepSnapshot {
	out := s
	if s.Windows != nil {
		out.Windows = make([]m68kJITLockstepMemoryWindow, len(s.Windows))
		for i := range s.Windows {
			out.Windows[i] = s.Windows[i]
			if s.Windows[i].Data != nil {
				out.Windows[i].Data = append([]byte(nil), s.Windows[i].Data...)
			}
		}
	}
	return out
}

func (s m68kJITLockstepSnapshot) diff(other m68kJITLockstepSnapshot) string {
	if s.PC != other.PC {
		return fmt.Sprintf("PC reference=%08X candidate=%08X", s.PC, other.PC)
	}
	if s.SR != other.SR {
		return fmt.Sprintf("SR reference=%04X candidate=%04X", s.SR, other.SR)
	}
	for i := 0; i < 8; i++ {
		if s.D[i] != other.D[i] {
			return fmt.Sprintf("D%d reference=%08X candidate=%08X", i, s.D[i], other.D[i])
		}
	}
	for i := 0; i < 8; i++ {
		if s.A[i] != other.A[i] {
			return fmt.Sprintf("A%d reference=%08X candidate=%08X", i, s.A[i], other.A[i])
		}
	}
	if len(s.Windows) != len(other.Windows) {
		return fmt.Sprintf("memory window count reference=%d candidate=%d", len(s.Windows), len(other.Windows))
	}
	for i := range s.Windows {
		a, b := s.Windows[i], other.Windows[i]
		if a.Label != b.Label || a.Addr != b.Addr {
			return fmt.Sprintf("memory window identity reference=%s@%08X candidate=%s@%08X", a.Label, a.Addr, b.Label, b.Addr)
		}
		if a.Len != b.Len || a.Hash != b.Hash || (a.Data != nil && b.Data != nil && !bytes.Equal(a.Data, b.Data)) {
			return fmt.Sprintf("memory window %s@%08X differs", a.Label, a.Addr)
		}
	}
	return ""
}

func m68kJITLockstepHash(data []byte) uint64 {
	const (
		fnvOffset = uint64(1469598103934665603)
		fnvPrime  = uint64(1099511628211)
	)
	h := fnvOffset
	for _, b := range data {
		h ^= uint64(b)
		h *= fnvPrime
	}
	return h
}

func (m *m68kJITLockstepMismatch) String() string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "M68K JIT lockstep mismatch: %s\n", m.Reason)
	fmt.Fprintf(&b, "  count=%d block=%08X ret=%08X retCount=%d chainCount=%d chainBudget=%d exit=%v needIO=%d helper=%d exception=%d inval=%d\n",
		m.Boundary.Count, m.Boundary.BlockPC, m.Boundary.RetPC, m.Boundary.RetCount,
		m.Boundary.ChainCount, m.Boundary.ChainBudget, m.Boundary.ExitSignal,
		m.Boundary.NeedIO, m.Boundary.NeedHelper, m.Boundary.Exception, m.Boundary.NeedInval)
	fmt.Fprintf(&b, "  reference: PC=%08X SR=%04X %s\n", m.Reference.PC, m.Reference.SR, m68kJITLockstepRegsString(m.Reference))
	fmt.Fprintf(&b, "  candidate: PC=%08X SR=%04X %s\n", m.Candidate.PC, m.Candidate.SR, m68kJITLockstepRegsString(m.Candidate))
	return b.String()
}

func m68kJITLockstepRegsString(s m68kJITLockstepSnapshot) string {
	return fmt.Sprintf("D=%08X,%08X,%08X,%08X,%08X,%08X,%08X,%08X A=%08X,%08X,%08X,%08X,%08X,%08X,%08X,%08X",
		s.D[0], s.D[1], s.D[2], s.D[3], s.D[4], s.D[5], s.D[6], s.D[7],
		s.A[0], s.A[1], s.A[2], s.A[3], s.A[4], s.A[5], s.A[6], s.A[7])
}
