//go:build headless

package clipboard

import "sync"

var (
	headlessMu   sync.Mutex
	headlessData []byte
)

func Init() error {
	return nil
}

func ReadText() ([]byte, error) {
	headlessMu.Lock()
	defer headlessMu.Unlock()
	if len(headlessData) == 0 {
		return nil, nil
	}
	data := make([]byte, len(headlessData))
	copy(data, headlessData)
	return data, nil
}

func WriteText(data []byte) error {
	headlessMu.Lock()
	defer headlessMu.Unlock()
	headlessData = append(headlessData[:0], data...)
	return nil
}
