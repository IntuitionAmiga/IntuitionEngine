//go:build !headless && !linux && !darwin && !windows

package clipboard

import "errors"

var errUnsupported = errors.New("clipboard unsupported on this platform")

func Init() error {
	return errUnsupported
}

func ReadText() ([]byte, error) {
	return nil, errUnsupported
}

func WriteText([]byte) error {
	return errUnsupported
}
