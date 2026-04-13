package main

const interp6502TraceCacheSize = 4096

const (
	interp6502TraceTermUnsupported = iota
	interp6502TraceTermHalt
	interp6502TraceTermMaxLen
)

const (
	interp6502TraceMaxPages = 8
	interp6502TraceMaxInstr = 64
)

type interp6502Trace struct {
	startPC     uint16
	endPC       uint16
	instrCount  uint16
	termKind    byte
	fusionID    byte
	actionCount byte
	pageCount   byte
	actions     [interp6502TraceMaxInstr]byte
	pages       [interp6502TraceMaxPages]byte
	pageGens    [interp6502TraceMaxPages]uint32
	valid       bool
}

type interp6502TraceCache struct {
	slots [interp6502TraceCacheSize]interp6502Trace
}
