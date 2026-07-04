// jit_mmap_arena.go - logical arenas layered over platform JIT mappings.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import "strconv"

const execMemArenaMinSize = 64 * 1024

type execMemArena struct {
	start int
	end   int
	used  int
	live  int
}

type execMemArenaState struct {
	arenas        []execMemArena
	current       int
	nextStart     int
	allocToArena  map[int]int
	totalCapacity int
}

func alignExecMemOffset(v int) int {
	return (v + execMemAlign - 1) &^ (execMemAlign - 1)
}

func (s *execMemArenaState) reset() {
	s.arenas = nil
	s.current = 0
	s.nextStart = 0
	s.totalCapacity = 0
	if s.allocToArena != nil {
		clear(s.allocToArena)
	}
}

func (s *execMemArenaState) reserve(totalCapacity, codeBytes int) (int, error) {
	if codeBytes < 0 {
		return 0, errExecMemExhausted(codeBytes, totalCapacity)
	}
	s.totalCapacity = totalCapacity
	if s.allocToArena == nil {
		s.allocToArena = make(map[int]int)
	}
	if idx, offset, ok := s.tryReserveInCurrent(codeBytes); ok {
		s.arenas[idx].live++
		s.allocToArena[offset] = idx
		return offset, nil
	}
	if idx, offset, ok := s.tryReserveInFreeArena(codeBytes); ok {
		s.current = idx
		s.arenas[idx].live++
		s.allocToArena[offset] = idx
		return offset, nil
	}
	idx, offset, ok := s.reserveNewArena(totalCapacity, codeBytes)
	if !ok {
		return 0, errExecMemExhausted(s.nextStart+codeBytes, totalCapacity)
	}
	s.current = idx
	s.arenas[idx].live++
	s.allocToArena[offset] = idx
	return offset, nil
}

func (s *execMemArenaState) tryReserveInCurrent(codeBytes int) (int, int, bool) {
	if len(s.arenas) == 0 || s.current < 0 || s.current >= len(s.arenas) {
		return 0, 0, false
	}
	arena := &s.arenas[s.current]
	offset := alignExecMemOffset(arena.used)
	if offset+codeBytes > arena.end {
		return 0, 0, false
	}
	arena.used = offset + codeBytes
	return s.current, offset, true
}

func (s *execMemArenaState) tryReserveInFreeArena(codeBytes int) (int, int, bool) {
	for idx := range s.arenas {
		arena := &s.arenas[idx]
		if arena.live != 0 {
			continue
		}
		offset := alignExecMemOffset(arena.start)
		if offset+codeBytes > arena.end {
			continue
		}
		arena.used = offset + codeBytes
		return idx, offset, true
	}
	return 0, 0, false
}

func (s *execMemArenaState) reserveNewArena(totalCapacity, codeBytes int) (int, int, bool) {
	start := alignExecMemOffset(s.nextStart)
	if start >= totalCapacity {
		return 0, 0, false
	}
	arenaBytes := execMemArenaMinSize
	if codeBytes > arenaBytes {
		arenaBytes = alignExecMemOffset(codeBytes)
	}
	remaining := totalCapacity - start
	if arenaBytes > remaining {
		arenaBytes = remaining
	}
	end := start + arenaBytes
	if start+codeBytes > end {
		return 0, 0, false
	}
	s.arenas = append(s.arenas, execMemArena{
		start: start,
		end:   end,
		used:  start + codeBytes,
	})
	s.nextStart = end
	return len(s.arenas) - 1, start, true
}

func (s *execMemArenaState) releaseOffset(offset int) bool {
	idx, ok := s.allocToArena[offset]
	if !ok || idx < 0 || idx >= len(s.arenas) {
		return false
	}
	delete(s.allocToArena, offset)
	arena := &s.arenas[idx]
	if arena.live > 0 {
		arena.live--
	}
	if arena.live == 0 {
		arena.used = arena.start
	}
	return true
}

func errExecMemExhausted(need, have int) error {
	return &execMemExhaustedError{need: need, have: have}
}

type execMemExhaustedError struct {
	need int
	have int
}

func (e *execMemExhaustedError) Error() string {
	return "ExecMem exhausted: need " + strconv.Itoa(e.need) + ", have " + strconv.Itoa(e.have)
}

func releaseJITBlockExecMem(block *JITBlock) {
	if block == nil || block.execAddr == 0 || block.execReleased {
		return
	}
	if releaseExecMemAddr(block.execAddr) {
		block.execReleased = true
	}
}
