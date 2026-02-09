package main

import (
	"os"
	"path/filepath"
	"strings"
)

// FileIODevice implements a memory-mapped I/O device for file operations.
// It allows the VM to read and write files to a restricted directory on the host.
type FileIODevice struct {
	bus           *MachineBus
	baseDir       string
	fileNamePtr   uint32
	fileDataPtr   uint32
	fileDataLen   uint32
	fileStatus    uint32
	fileResultLen uint32
	fileErrorCode uint32
}

// NewFileIODevice creates a new File I/O device.
func NewFileIODevice(bus *MachineBus, baseDir string) *FileIODevice {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		absBase = baseDir
	}
	return &FileIODevice{
		bus:     bus,
		baseDir: absBase,
	}
}

// HandleRead handles MMIO reads from the File I/O region.
func (f *FileIODevice) HandleRead(addr uint32) uint32 {
	switch addr {
	case FILE_NAME_PTR:
		return f.fileNamePtr
	case FILE_DATA_PTR:
		return f.fileDataPtr
	case FILE_DATA_LEN:
		return f.fileDataLen
	case FILE_STATUS:
		return f.fileStatus
	case FILE_RESULT_LEN:
		return f.fileResultLen
	case FILE_ERROR_CODE:
		return f.fileErrorCode
	}
	return 0
}

// HandleWrite handles MMIO writes to the File I/O region.
func (f *FileIODevice) HandleWrite(addr uint32, val uint32) {
	switch addr {
	case FILE_NAME_PTR:
		f.fileNamePtr = val
	case FILE_DATA_PTR:
		f.fileDataPtr = val
	case FILE_DATA_LEN:
		f.fileDataLen = val
	case FILE_CTRL:
		if val == FILE_OP_READ {
			f.doRead()
		} else if val == FILE_OP_WRITE {
			f.doWrite()
		}
	}
}

// sanitizePath ensures the given path is safe and within baseDir.
func (f *FileIODevice) sanitizePath(path string) (string, bool) {
	// Reject absolute paths and paths containing ".."
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return "", false
	}

	// Join with baseDir and clean the path
	fullPath := filepath.Join(f.baseDir, path)

	// Final check: must be inside baseDir
	rel, err := filepath.Rel(f.baseDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}

	return fullPath, true
}

// readFileName reads a null-terminated string from the bus.
func (f *FileIODevice) readFileName() string {
	var name []byte
	addr := f.fileNamePtr
	for {
		b := f.bus.Read8(addr)
		if b == 0 {
			break
		}
		name = append(name, b)
		addr++
		// Safety limit for filename length
		if len(name) > 255 {
			break
		}
	}
	return string(name)
}

// doRead performs the actual file read operation.
func (f *FileIODevice) doRead() {
	fileName := f.readFileName()
	fullPath, ok := f.sanitizePath(fileName)
	if !ok {
		f.fileStatus = 1
		f.fileErrorCode = FILE_ERR_PATH_TRAVERSAL
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		f.fileStatus = 1
		if os.IsNotExist(err) {
			f.fileErrorCode = FILE_ERR_NOT_FOUND
		} else {
			f.fileErrorCode = FILE_ERR_PERMISSION
		}
		return
	}

	// Write data to bus
	for i, b := range data {
		f.bus.Write8(f.fileDataPtr+uint32(i), b)
	}

	f.fileStatus = 0
	f.fileErrorCode = FILE_ERR_OK
	f.fileResultLen = uint32(len(data))
}

// doWrite performs the actual file write operation.
func (f *FileIODevice) doWrite() {
	fileName := f.readFileName()
	fullPath, ok := f.sanitizePath(fileName)
	if !ok {
		f.fileStatus = 1
		f.fileErrorCode = FILE_ERR_PATH_TRAVERSAL
		return
	}

	// Read data from bus
	data := make([]byte, f.fileDataLen)
	for i := uint32(0); i < f.fileDataLen; i++ {
		data[i] = f.bus.Read8(f.fileDataPtr + i)
	}

	err := os.WriteFile(fullPath, data, 0644)
	if err != nil {
		f.fileStatus = 1
		f.fileErrorCode = FILE_ERR_PERMISSION
		return
	}

	f.fileStatus = 0
	f.fileErrorCode = FILE_ERR_OK
}
