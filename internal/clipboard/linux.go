//go:build linux && !headless

package clipboard

import (
	"bytes"
	"os/exec"
	"sync"
)

var (
	linuxClipboardOnce sync.Once
	linuxClipboardTool string
	linuxClipboardErr  error
)

func Init() error {
	_, err := linuxClipboardCommand()
	return err
}

func ReadText() ([]byte, error) {
	args, err := linuxClipboardReadCommand()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil, err
	}
	return out, nil
}

func WriteText(data []byte) error {
	args, err := linuxClipboardWriteCommand()
	if err != nil {
		return err
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

func linuxClipboardCommand() (string, error) {
	linuxClipboardOnce.Do(func() {
		for _, tool := range []string{
			"xsel",
			"xclip",
		} {
			if _, err := exec.LookPath(tool); err == nil {
				linuxClipboardTool = tool
				return
			}
		}
		linuxClipboardErr = errUnsupported
	})
	return linuxClipboardTool, linuxClipboardErr
}

func linuxClipboardReadCommand() ([]string, error) {
	tool, err := linuxClipboardCommand()
	if err != nil {
		return nil, err
	}
	switch tool {
	case "xsel":
		return []string{"xsel", "--clipboard", "--output"}, nil
	case "xclip":
		return []string{"xclip", "-selection", "clipboard", "-o"}, nil
	default:
		return nil, errUnsupported
	}
}

func linuxClipboardWriteCommand() ([]string, error) {
	tool, err := linuxClipboardCommand()
	if err != nil {
		return nil, err
	}
	switch tool {
	case "xsel":
		return []string{"xsel", "--clipboard", "--input"}, nil
	case "xclip":
		return []string{"xclip", "-selection", "clipboard"}, nil
	default:
		return nil, errUnsupported
	}
}
