package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type trustedRCEntry struct {
	Path string
	Hash string
}

func iemonTrustedPath() string {
	home := iemonHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "trusted")
}

func findIEMONRCFiles() []string {
	var out []string
	wd, err := os.Getwd()
	if err != nil {
		return out
	}
	for {
		candidate := filepath.Join(wd, ".iemonrc")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				out = append(out, abs)
			}
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return out
}

func hashFileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func readTrustedRCEntries() map[string]trustedRCEntry {
	entries := make(map[string]trustedRCEntry)
	path := iemonTrustedPath()
	if path == "" {
		return entries
	}
	f, err := os.Open(path)
	if err != nil {
		return entries
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		entries[fields[0]] = trustedRCEntry{Path: fields[0], Hash: fields[1]}
	}
	return entries
}

func trustIEMONRC(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	hash, err := hashFileSHA256(abs)
	if err != nil {
		return "", err
	}
	trustedPath := iemonTrustedPath()
	if trustedPath == "" {
		return "", fmt.Errorf("IEMon trust store unavailable")
	}
	entries := readTrustedRCEntries()
	entries[abs] = trustedRCEntry{Path: abs, Hash: hash}
	if err := os.MkdirAll(filepath.Dir(trustedPath), 0700); err != nil {
		return "", err
	}
	var lines []string
	for _, entry := range entries {
		lines = append(lines, entry.Path+" "+entry.Hash)
	}
	return hash, os.WriteFile(trustedPath, []byte(strings.Join(lines, "\n")+"\n"), 0600)
}

func iemonRCTrusted(path string) (string, bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false, err
	}
	hash, err := hashFileSHA256(abs)
	if err != nil {
		return "", false, err
	}
	entry, ok := readTrustedRCEntries()[abs]
	return hash, ok && strings.EqualFold(entry.Hash, hash), nil
}

func isAllowedRCCommand(cmd MonitorCommand) bool {
	switch cmd.Name {
	case "b", "bc", "ww", "wc", "layout":
		return true
	case "pg":
		if len(cmd.Args) == 0 {
			return false
		}
		sub := strings.ToLower(cmd.Args[0])
		return sub == "add" || sub == "clear" || sub == "list"
	case "sym":
		return len(cmd.Args) > 0 && strings.ToLower(cmd.Args[0]) == "add"
	case "history":
		return len(cmd.Args) > 0 && strings.ToLower(cmd.Args[0]) == "config"
	case "alias":
		if len(cmd.Args) < 2 {
			return true
		}
		target := ParseCommand(strings.Join(cmd.Args[1:], " "))
		return target.Name != "" && isAllowedRCCommand(target)
	default:
		return strings.HasPrefix(cmd.Name, "bpm")
	}
}

func (m *MachineMonitor) loadIEMONRC(path string) (int, error) {
	hash, trusted, err := iemonRCTrusted(path)
	if err != nil {
		return 0, err
	}
	if !trusted {
		return 0, fmt.Errorf("iemonrc not trusted: %s (sha256 %s)", path, hash)
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	loaded := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		parts, err := splitCommandLine(line)
		if err != nil || len(parts) == 0 {
			return loaded, fmt.Errorf("invalid rc command %q", line)
		}
		cmd := MonitorCommand{Name: strings.ToLower(parts[0]), Args: parts[1:]}
		if !isAllowedRCCommand(cmd) {
			return loaded, fmt.Errorf("rc command not allowed: %s", cmd.Name)
		}
		m.executeCommand(line)
		loaded++
	}
	if err := scanner.Err(); err != nil {
		return loaded, err
	}
	return loaded, nil
}

func (m *MachineMonitor) autoLoadTrustedIEMONRCs() {
	if len(m.cpus) != 1 {
		return
	}
	if m.loadedRC == nil {
		m.loadedRC = make(map[string]string)
	}
	for _, path := range findIEMONRCFiles() {
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		hash, trusted, err := iemonRCTrusted(abs)
		if err != nil {
			m.appendOutput(fmt.Sprintf("iemonrc found at %s (error: %s)", abs, err), colorRed)
			continue
		}
		if !trusted {
			m.appendOutput(fmt.Sprintf("iemonrc found at %s (sha256 %s) - run 'rc trust %s' before loading", abs, hash, abs), colorDim)
			continue
		}
		if m.loadedRC[abs] == hash {
			continue
		}
		count, err := m.loadIEMONRC(abs)
		if err != nil {
			m.appendOutput(fmt.Sprintf("Error loading %s: %s", abs, err), colorRed)
			continue
		}
		m.loadedRC[abs] = hash
		m.appendOutput(fmt.Sprintf("Loaded %d command(s) from trusted %s", count, abs), colorCyan)
	}
}

func (m *MachineMonitor) cmdRC(cmd MonitorCommand) bool {
	sub := ""
	if len(cmd.Args) > 0 {
		sub = strings.ToLower(cmd.Args[0])
	}
	if len(cmd.Args) == 0 || sub == "list" {
		files := findIEMONRCFiles()
		if len(files) == 0 {
			m.appendOutput("No .iemonrc found", colorDim)
			return false
		}
		for _, path := range files {
			hash, trusted, err := iemonRCTrusted(path)
			if err != nil {
				m.appendOutput(fmt.Sprintf("%s (error: %s)", path, err), colorRed)
				continue
			}
			state := "untrusted"
			if trusted {
				state = "trusted"
			}
			m.appendOutput(fmt.Sprintf("%s sha256=%s %s", path, hash, state), colorCyan)
		}
		return false
	}

	path := ""
	if len(cmd.Args) >= 2 {
		path = cmd.Args[1]
	} else if files := findIEMONRCFiles(); len(files) > 0 {
		path = files[0]
	}
	if path == "" {
		m.appendOutput("No .iemonrc found", colorRed)
		return false
	}

	switch sub {
	case "trust":
		hash, err := trustIEMONRC(path)
		if err != nil {
			m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
			return false
		}
		m.appendOutput(fmt.Sprintf("Trusted %s sha256=%s", path, hash), colorCyan)
	case "load":
		count, err := m.loadIEMONRC(path)
		if err != nil {
			m.appendOutput(fmt.Sprintf("Error: %s", err), colorRed)
			return false
		}
		m.appendOutput(fmt.Sprintf("Loaded %d command(s) from %s", count, path), colorCyan)
	default:
		m.appendOutput("Usage: rc list | rc trust [file] | rc load [file]", colorRed)
	}
	return false
}
