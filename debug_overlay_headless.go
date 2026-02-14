//go:build headless

// debug_overlay_headless.go - Stub overlay for headless builds (testing)

package main

// MonitorOverlay is a no-op in headless mode.
type MonitorOverlay struct {
	monitor *MachineMonitor
}

func NewMonitorOverlay(monitor *MachineMonitor) *MonitorOverlay {
	return &MonitorOverlay{monitor: monitor}
}
