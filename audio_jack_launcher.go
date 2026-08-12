//go:build linux && cgo && jack

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/xthexder/go-jack"
)

const (
	jackSampleRate = 44100
	jackPeriodSize = 64
)

type jackServerHandle struct{ cmd *exec.Cmd }

func (h *jackServerHandle) stop() {
	if h == nil || h.cmd == nil || h.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-h.cmd.Process.Pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = h.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-h.cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
}

// startOwnedJACK uses the fixed appliance configuration. Device choice remains
// caller controlled so the launcher and ALSA fallback can share one device.
func startOwnedJACK(device string) (*jackServerHandle, error) {
	device = jackALSADevice(device)
	path, err := exec.LookPath("jackd")
	if err != nil {
		return nil, fmt.Errorf("jackd is unavailable: %w", err)
	}
	cmd := exec.Command(path, "-R", "-P95", "-dalsa", "-d"+device, "-r44100", "-p64", "-n3")
	cmd.Env = append(os.Environ(), "JACK_NO_AUDIO_RESERVATION=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start jackd: %w", err)
	}
	h := &jackServerHandle{cmd: cmd}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, status := jack.ClientOpen("IntuitionEngine-probe", jack.NoStartServer)
		opened := jackClientOpenSucceeded(client, status)
		if client != nil {
			_ = client.Close()
		}
		if opened {
			return h, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.stop()
	return nil, fmt.Errorf("jackd did not accept clients within five seconds")
}

// jackALSADevice gives an explicit JACK device precedence. When it is unset,
// use the same card selection as the appliance ALSA fallback.
func jackALSADevice(device string) string {
	if device = strings.TrimSpace(device); device != "" {
		return device
	}
	card := strings.TrimSpace(os.Getenv("IE_JACK_ALSA_CARD"))
	if card == "" {
		card = "0"
	}
	return "hw:" + card
}
