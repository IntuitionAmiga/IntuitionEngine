package main

import "strings"

const hostWiFiPasswordMaxLen = 64

type HostWiFiPickerMode int

const (
	HostWiFiPickerModeList HostWiFiPickerMode = iota
	HostWiFiPickerModePassword
	HostWiFiPickerModeSubmitted
	HostWiFiPickerModeCancelled
)

type HostWiFiPickerKey int

const (
	HostWiFiPickerKeyNone HostWiFiPickerKey = iota
	HostWiFiPickerKeyUp
	HostWiFiPickerKeyDown
	HostWiFiPickerKeyEnter
	HostWiFiPickerKeyR
	HostWiFiPickerKeyEscape
	HostWiFiPickerKeyBackspace
)

type HostWiFiPickerAction int

const (
	HostWiFiPickerActionNone HostWiFiPickerAction = iota
	HostWiFiPickerActionRescan
	HostWiFiPickerActionSubmit
	HostWiFiPickerActionCancel
)

type HostWiFiPickerInput struct {
	Key  HostWiFiPickerKey
	Rune rune
}

type HostWiFiJoinRequest struct {
	SSID     string
	Password string
}

type HostWiFiPicker struct {
	networks []HostWiFiNetwork
	selected int
	mode     HostWiFiPickerMode
	password []rune
	submit   HostWiFiJoinRequest
}

func NewHostWiFiPicker(networks []HostWiFiNetwork) *HostWiFiPicker {
	p := &HostWiFiPicker{}
	p.SetNetworks(networks)
	return p
}

func (p *HostWiFiPicker) SetNetworks(networks []HostWiFiNetwork) {
	p.networks = append(p.networks[:0], networks...)
	p.mode = HostWiFiPickerModeList
	p.password = p.password[:0]
	p.submit = HostWiFiJoinRequest{}
	p.clampSelection()
}

func (p *HostWiFiPicker) HandleInput(input HostWiFiPickerInput) HostWiFiPickerAction {
	if p.mode == HostWiFiPickerModeSubmitted || p.mode == HostWiFiPickerModeCancelled {
		return HostWiFiPickerActionNone
	}

	if input.Key == HostWiFiPickerKeyEscape {
		p.mode = HostWiFiPickerModeCancelled
		return HostWiFiPickerActionCancel
	}

	switch p.mode {
	case HostWiFiPickerModeList:
		return p.handleListInput(input)
	case HostWiFiPickerModePassword:
		return p.handlePasswordInput(input)
	default:
		return HostWiFiPickerActionNone
	}
}

func (p *HostWiFiPicker) SelectedIndex() int {
	return p.selected
}

func (p *HostWiFiPicker) Mode() HostWiFiPickerMode {
	return p.mode
}

func (p *HostWiFiPicker) Password() string {
	return string(p.password)
}

func (p *HostWiFiPicker) SubmittedRequest() HostWiFiJoinRequest {
	return p.submit
}

func (p *HostWiFiPicker) handleListInput(input HostWiFiPickerInput) HostWiFiPickerAction {
	switch input.Key {
	case HostWiFiPickerKeyUp:
		if p.selected > 0 {
			p.selected--
		}
	case HostWiFiPickerKeyDown:
		if p.selected+1 < len(p.networks) {
			p.selected++
		}
	case HostWiFiPickerKeyR:
		return HostWiFiPickerActionRescan
	case HostWiFiPickerKeyEnter:
		if len(p.networks) == 0 {
			return HostWiFiPickerActionNone
		}
		network := p.networks[p.selected]
		if hostWiFiNetworkNeedsPassword(network) {
			p.password = p.password[:0]
			p.mode = HostWiFiPickerModePassword
			return HostWiFiPickerActionNone
		}
		p.submitRequest(network.SSID, "")
		return HostWiFiPickerActionSubmit
	}
	return HostWiFiPickerActionNone
}

func (p *HostWiFiPicker) handlePasswordInput(input HostWiFiPickerInput) HostWiFiPickerAction {
	switch input.Key {
	case HostWiFiPickerKeyBackspace:
		if len(p.password) > 0 {
			p.password = p.password[:len(p.password)-1]
		}
	case HostWiFiPickerKeyEnter:
		if len(p.networks) == 0 {
			p.mode = HostWiFiPickerModeList
			return HostWiFiPickerActionNone
		}
		p.submitRequest(p.networks[p.selected].SSID, string(p.password))
		return HostWiFiPickerActionSubmit
	}

	if input.Rune >= ' ' && input.Rune != 0x7f && len(p.password) < hostWiFiPasswordMaxLen {
		p.password = append(p.password, input.Rune)
	}
	return HostWiFiPickerActionNone
}

func (p *HostWiFiPicker) submitRequest(ssid string, password string) {
	p.submit = HostWiFiJoinRequest{SSID: ssid, Password: password}
	p.mode = HostWiFiPickerModeSubmitted
}

func (p *HostWiFiPicker) clampSelection() {
	if len(p.networks) == 0 {
		p.selected = 0
		return
	}
	if p.selected < 0 {
		p.selected = 0
	}
	if p.selected >= len(p.networks) {
		p.selected = len(p.networks) - 1
	}
}

func hostWiFiNetworkNeedsPassword(network HostWiFiNetwork) bool {
	security := strings.TrimSpace(strings.ToLower(network.Security))
	return security != "" && security != "open"
}
