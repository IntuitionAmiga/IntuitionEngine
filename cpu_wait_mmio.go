package main

import "time"

const cpuWaitSafetyTimeout = 50 * time.Millisecond

type CPUWaitMMIO struct {
	terminal *TerminalMMIO
	video    *VideoChip

	deadlineUsec uint64
}

func NewCPUWaitMMIO(terminal *TerminalMMIO, video *VideoChip) *CPUWaitMMIO {
	return &CPUWaitMMIO{terminal: terminal, video: video}
}

func RegisterCPUWaitMMIO(bus *MachineBus, wait *CPUWaitMMIO) {
	bus.MapIO(CPU_WAIT_REGION_BASE, CPU_WAIT_REGION_END, wait.HandleRead, wait.HandleWrite)
}

func (w *CPUWaitMMIO) HandleRead(addr uint32) uint32 {
	return 0
}

func (w *CPUWaitMMIO) HandleWrite(addr uint32, value uint32) {
	switch addr {
	case WAIT_VBLANK:
		if w.video != nil {
			w.video.WaitForVBlankEdge(cpuWaitSafetyTimeout)
		}
	case WAIT_UNTIL_LO:
		w.deadlineUsec = (w.deadlineUsec & 0xFFFFFFFF00000000) | uint64(value)
	case WAIT_UNTIL_HI:
		w.deadlineUsec = (w.deadlineUsec & 0x00000000FFFFFFFF) | (uint64(value) << 32)
	case WAIT_UNTIL_GO:
		w.waitUntilDeadline()
	}
}

func (w *CPUWaitMMIO) waitUntilDeadline() {
	if w.terminal == nil {
		return
	}
	start := time.Now()
	for {
		if w.terminal.MonotonicUsec() >= w.deadlineUsec {
			return
		}
		if time.Since(start) >= cpuWaitSafetyTimeout {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
