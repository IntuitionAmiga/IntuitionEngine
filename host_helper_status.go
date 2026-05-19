package main

import "fmt"

const (
	HostHelperExitOK                  uint32 = 0
	HostHelperExitBadInput            uint32 = 2
	HostHelperExitWiFiBadPSK          uint32 = 10
	HostHelperExitWiFiNoSignal        uint32 = 11
	HostHelperExitWiFiUnsupportedAuth uint32 = 12
	HostHelperExitWiFiTimeout         uint32 = 13
	HostHelperExitAptUpdateFailed     uint32 = 20
	HostHelperExitAptUpgradeFailed    uint32 = 21
)

var hostHelperExitStatusText = map[uint32]string{
	HostHelperExitOK:                  "OK",
	HostHelperExitBadInput:            "Failed: bad input",
	HostHelperExitWiFiBadPSK:          "Failed: bad password",
	HostHelperExitWiFiNoSignal:        "Failed: no signal",
	HostHelperExitWiFiUnsupportedAuth: "Failed: unsupported authentication",
	HostHelperExitWiFiTimeout:         "Failed: timeout",
	HostHelperExitAptUpdateFailed:     "Failed: apt-get update",
	HostHelperExitAptUpgradeFailed:    "Failed: apt-get upgrade",
}

func HostHelperExitStatusText(code uint32) string {
	if text, ok := hostHelperExitStatusText[code]; ok {
		return text
	}
	return fmt.Sprintf("Failed: exit %d", code)
}

func HostHelperWiFiJoinStatusText(code uint32, ipAddress string) string {
	if code == HostHelperExitOK {
		if ipAddress != "" {
			return fmt.Sprintf("Connected (%s)", ipAddress)
		}
		return "Connected"
	}
	return HostHelperExitStatusText(code)
}
