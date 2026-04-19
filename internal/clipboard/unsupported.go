//go:build !headless && !linux && !darwin && !windows

package clipboard

func Init() error {
	return errUnsupported
}

func ReadText() ([]byte, error) {
	return nil, errUnsupported
}

func WriteText([]byte) error {
	return errUnsupported
}
