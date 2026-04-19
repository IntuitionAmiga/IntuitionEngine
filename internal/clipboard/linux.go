//go:build linux && !headless

package clipboard

import (
	"bytes"
	"os/exec"
)

func Init() error {
	return nil
}

func ReadText() ([]byte, error) {
	for _, args := range [][]string{
		{"xsel", "--clipboard", "--output"},
		{"xclip", "-selection", "clipboard", "-o"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			return out, nil
		}
	}
	return nil, nil
}

func WriteText(data []byte) error {
	for _, args := range [][]string{
		{"xsel", "--clipboard", "--input"},
		{"xclip", "-selection", "clipboard"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = bytes.NewReader(data)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return nil
}
