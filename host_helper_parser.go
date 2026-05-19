package main

import (
	"strconv"
	"strings"
)

type HostWiFiNetwork struct {
	SSID     string
	Signal   int
	Security string
}

func ParseHostWiFiScanOutput(output string) []HostWiFiNetwork {
	lines := strings.Split(output, "\n")
	networks := make([]HostWiFiNetwork, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		line = strings.TrimSuffix(line, "\r")
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		ssid := decodeHostWiFiSSID(fields[0])
		if ssid == "" {
			continue
		}
		signal, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			continue
		}
		if signal < 0 {
			signal = 0
		}
		if signal > 100 {
			signal = 100
		}

		security := normalizeHostWiFiField(strings.Join(fields[2:], " "))
		if security == "" {
			security = "open"
		}

		networks = append(networks, HostWiFiNetwork{
			SSID:     ssid,
			Signal:   signal,
			Security: security,
		})
	}
	return networks
}

func FormatHostWiFiScanLine(network HostWiFiNetwork) string {
	return encodeHostWiFiSSID(network.SSID) + "\t" +
		strconv.Itoa(clampHostWiFiSignal(network.Signal)) + "\t" +
		normalizeHostWiFiField(network.Security) + "\n"
}

func encodeHostWiFiSSID(ssid string) string {
	var b strings.Builder
	for i := 0; i < len(ssid); i++ {
		switch ssid[i] {
		case '%':
			b.WriteString("%25")
		case '\t':
			b.WriteString("%09")
		case '\n':
			b.WriteString("%0A")
		case '\r':
			b.WriteString("%0D")
		default:
			b.WriteByte(ssid[i])
		}
	}
	return b.String()
}

func decodeHostWiFiSSID(ssid string) string {
	var b strings.Builder
	for i := 0; i < len(ssid); i++ {
		if ssid[i] != '%' || i+2 >= len(ssid) {
			b.WriteByte(ssid[i])
			continue
		}
		switch ssid[i+1 : i+3] {
		case "09":
			b.WriteByte('\t')
			i += 2
		case "0A", "0a":
			b.WriteByte('\n')
			i += 2
		case "0D", "0d":
			b.WriteByte('\r')
			i += 2
		case "25":
			b.WriteByte('%')
			i += 2
		default:
			b.WriteByte(ssid[i])
		}
	}
	return b.String()
}

func normalizeHostWiFiField(field string) string {
	field = strings.ReplaceAll(field, "\t", " ")
	field = strings.ReplaceAll(field, "\r", " ")
	field = strings.ReplaceAll(field, "\n", " ")
	return strings.TrimSpace(field)
}

func clampHostWiFiSignal(signal int) int {
	if signal < 0 {
		return 0
	}
	if signal > 100 {
		return 100
	}
	return signal
}
