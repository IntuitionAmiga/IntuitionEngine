//go:build darwin && !headless

package clipboard

import (
	"bytes"
	"os/exec"
)

func Init() error {
	return nil
}

func ReadText() ([]byte, error) {
	out, err := exec.Command("pbpaste").Output()
	if err != nil || len(out) == 0 {
		return nil, err
	}
	return out, nil
}

func WriteText(data []byte) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}
