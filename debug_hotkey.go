package main

import (
	"io"
	"sync"
)

const defaultBreakInHotkey byte = 0x04 // Ctrl-D, matching SoftICE-style break-in muscle memory.

// BreakInHotkeyListener bridges a host-side byte stream to MachineMonitor
// break-in requests. Display backends may use their own key systems; this
// listener is for TTY-style frontends and tests.
type BreakInHotkeyListener struct {
	monitor *MachineMonitor
	reader  io.Reader
	key     byte

	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func NewBreakInHotkeyListener(monitor *MachineMonitor, reader io.Reader) *BreakInHotkeyListener {
	return &BreakInHotkeyListener{
		monitor: monitor,
		reader:  reader,
		key:     defaultBreakInHotkey,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (l *BreakInHotkeyListener) SetKey(key byte) {
	if key != 0 {
		l.key = key
	}
}

func (l *BreakInHotkeyListener) Start() {
	if l == nil || l.monitor == nil || l.reader == nil {
		return
	}
	go l.loop()
}

func (l *BreakInHotkeyListener) Stop() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		close(l.stop)
		if closer, ok := l.reader.(io.Closer); ok {
			_ = closer.Close()
		}
		<-l.done
	})
}

func (l *BreakInHotkeyListener) loop() {
	defer close(l.done)
	buf := make([]byte, 1)
	for {
		select {
		case <-l.stop:
			return
		default:
		}
		n, err := l.reader.Read(buf)
		if n > 0 && buf[0] == l.key {
			l.monitor.RequestBreakIn()
		}
		if err != nil {
			return
		}
	}
}
