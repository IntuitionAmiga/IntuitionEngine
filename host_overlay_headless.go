//go:build headless

package main

type HostOverlay struct{}

func NewHostOverlay() *HostOverlay {
	return &HostOverlay{}
}

func (o *HostOverlay) HostCommandStarted(cmd HostCommand) {}

func (o *HostOverlay) HostCommandOutput(cmd HostCommand, text string) {}

func (o *HostOverlay) HostCommandCompleted(cmd HostCommand, result HostCommandResult) {}

func hostCommandTitle(cmd HostCommand) string {
	switch cmd {
	case HostCommandNet:
		return "HOST NET"
	case HostCommandUpdate:
		return "HOST UPDATE"
	case HostCommandReboot:
		return "HOST REBOOT"
	case HostCommandPoweroff:
		return "HOST POWEROFF"
	default:
		return "HOST"
	}
}
