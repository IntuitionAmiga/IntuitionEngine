// runtime_ipc.go - Unix domain socket IPC for single-instance coordination

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ipcMaxRequestSize = 4096

var allowedExtensions = map[string]bool{
	".ie32": true, ".iex": true, ".ie64": true,
	".ie65": true, ".ie68": true, ".ie80": true, ".ie86": true,
}

type ipcRequest struct {
	Cmd  string `json:"cmd"`
	Path string `json:"path"`
}

type ipcResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// IPCServer listens on Unix socket and dispatches OPEN requests.
type IPCServer struct {
	listener net.Listener
	handler  func(path string) error
	done     chan struct{}
	sockPath string
}

func resolveSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "intuition-engine.sock")
	}
	return "/tmp/intuition-engine.sock"
}

// NewIPCServer creates and binds the IPC Unix socket at the default path.
func NewIPCServer(handler func(string) error) (*IPCServer, error) {
	return newIPCServerAt(resolveSocketPath(), handler)
}

// newIPCServerAt creates and binds the IPC Unix socket at the given path.
func newIPCServerAt(sockPath string, handler func(string) error) (*IPCServer, error) {
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		// Stale socket cleanup: try connecting. If peer is dead, remove and retry.
		conn, dialErr := net.DialTimeout("unix", sockPath, 2*time.Second)
		if dialErr != nil {
			os.Remove(sockPath)
			ln, err = net.Listen("unix", sockPath)
			if err != nil {
				return nil, fmt.Errorf("ipc bind failed: %w", err)
			}
		} else {
			conn.Close()
			return nil, fmt.Errorf("another instance is already running")
		}
	}
	return &IPCServer{listener: ln, handler: handler, done: make(chan struct{}), sockPath: sockPath}, nil
}

// Start begins accepting IPC connections in a goroutine.
func (s *IPCServer) Start() {
	go s.acceptLoop()
}

// Stop closes the listener and waits for the accept loop to exit.
func (s *IPCServer) Stop() {
	s.listener.Close()
	<-s.done
	os.Remove(s.sockPath)
}

func (s *IPCServer) acceptLoop() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *IPCServer) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	buf := make([]byte, ipcMaxRequestSize)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}

	var req ipcRequest
	if err := json.Unmarshal(buf[:n], &req); err != nil {
		s.writeResponse(conn, ipcResponse{Status: "err", Message: "invalid json"})
		return
	}

	if req.Cmd != "open" {
		s.writeResponse(conn, ipcResponse{Status: "err", Message: "unknown command"})
		return
	}

	if err := validateIPCPath(req.Path); err != nil {
		s.writeResponse(conn, ipcResponse{Status: "err", Message: err.Error()})
		return
	}

	if err := s.handler(req.Path); err != nil {
		s.writeResponse(conn, ipcResponse{Status: "err", Message: err.Error()})
		return
	}

	s.writeResponse(conn, ipcResponse{Status: "ok"})
}

func (s *IPCServer) writeResponse(conn net.Conn, resp ipcResponse) {
	data, _ := json.Marshal(resp)
	conn.Write(data)
}

func validateIPCPath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("absolute path required")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if !allowedExtensions[ext] {
		return fmt.Errorf("unsupported extension: %s", ext)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("file not found: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	return nil
}

// SendIPCOpen sends an OPEN request to an existing instance at the default socket.
func SendIPCOpen(path string) error {
	return sendIPCOpenAt(resolveSocketPath(), path)
}

// sendIPCOpenAt sends an OPEN request to an instance at the given socket path.
func sendIPCOpenAt(sockPath, path string) error {
	conn, err := net.DialTimeout("unix", sockPath, 10*time.Second)
	if err != nil {
		return fmt.Errorf("cannot connect to running instance: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	req := ipcRequest{Cmd: "open", Path: path}
	data, _ := json.Marshal(req)
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("send failed: %w", err)
	}

	buf := make([]byte, ipcMaxRequestSize)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}

	var resp ipcResponse
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}
	if resp.Status != "ok" {
		return fmt.Errorf("remote error: %s", resp.Message)
	}
	return nil
}
