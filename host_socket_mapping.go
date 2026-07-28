package main

type hostSocketCloser interface {
	CloseAll()
}

type HostSocketMapping struct {
	bus     *MachineBus
	backend arosHostSocketBackend
	device  *ArosHostSocketDevice
	mapped  bool
}

func hostSocketModeEnabled(mode string) bool {
	switch mode {
	case "ie64", "ie32", "m68k", "x86", "6502", "z80", "intuitionos", "emutos", "aros":
		return true
	default:
		return false
	}
}

func hostSocketRuntimeMode(mode runtimeMode) string {
	switch mode {
	case modeIE64:
		return "ie64"
	case modeIntuitionOS:
		return "intuitionos"
	case modeBasic:
		return "basic"
	case modeIE32:
		return "ie32"
	case modeX86:
		return "x86"
	case modeM68KBare:
		return "m68k"
	case modeEmuTOS:
		return "emutos"
	case modeAros:
		return "aros"
	case mode6502:
		return "6502"
	case modeZ80:
		return "z80"
	default:
		return ""
	}
}

func NewHostSocketMapping(bus *MachineBus) *HostSocketMapping {
	return &HostSocketMapping{bus: bus}
}

func (m *HostSocketMapping) Configure(mode string) {
	if m == nil || m.bus == nil {
		return
	}
	if m.mapped {
		m.bus.UnmapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END)
		m.mapped = false
	}
	if closer, ok := m.backend.(hostSocketCloser); ok {
		closer.CloseAll()
	}
	m.backend = nil
	m.device = nil
	if !hostSocketModeEnabled(mode) {
		return
	}
	m.backend = NewUnixArosHostSocketBackend()
	m.device = NewArosHostSocketDevice(m.bus, m.backend, true)
	m.bus.MapIO(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, m.device.HandleRead, m.device.HandleWrite)
	m.bus.MapIOByte(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, m.device.HandleWrite8)
	m.bus.MapIOByteRead(AROS_HOST_SOCKET_REGION_BASE, AROS_HOST_SOCKET_REGION_END, m.device.HandleRead8)
	m.mapped = true
}
