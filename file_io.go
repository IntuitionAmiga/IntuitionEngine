package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		} else if val == FILE_OP_LIST {
			f.doList()
		}
	}
}

// HandleWrite8 handles byte-level MMIO writes to the File I/O region.
// This allows 8-bit CPUs (Z80, 6502) to set 32-bit register values by writing
// individual bytes. The byte offset within each 4-byte register determines
// which bits are updated. Writing byte 0 of FILE_CTRL triggers the operation.
func (f *FileIODevice) HandleWrite8(addr uint32, value uint8) {
	base := addr &^ 3
	shift := (addr & 3) * 8
	mask := uint32(0xFF) << shift
	assembled := uint32(value) << shift

	switch base {
	case FILE_NAME_PTR:
		f.fileNamePtr = (f.fileNamePtr &^ mask) | assembled
	case FILE_DATA_PTR:
		f.fileDataPtr = (f.fileDataPtr &^ mask) | assembled
	case FILE_DATA_LEN:
		f.fileDataLen = (f.fileDataLen &^ mask) | assembled
	case FILE_CTRL:
		if addr == FILE_CTRL {
			if value == FILE_OP_READ {
				f.doRead()
			} else if value == FILE_OP_WRITE {
				f.doWrite()
			} else if value == FILE_OP_LIST {
				f.doList()
			}
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

func (f *FileIODevice) resolveReadFileName(fileName string) string {
	if strings.HasPrefix(fileName, "_build/ie_unpacked/media/levels/") {
		levelName := strings.TrimPrefix(fileName, "_build/ie_unpacked/media/levels/")
		reduxName := filepath.Join("_build", "ie_media", "redux-high", "levels_editor_uncompressed", levelName)
		if fullPath, ok := f.sanitizePath(reduxName); ok {
			if _, err := os.Stat(fullPath); err == nil {
				return reduxName
			}
		}
	}
	if strings.HasPrefix(fileName, "media/levels/") {
		levelName := strings.TrimPrefix(fileName, "media/levels/")
		reduxName := filepath.Join("_build", "ie_media", "redux-high", "levels_editor_uncompressed", levelName)
		if fullPath, ok := f.sanitizePath(reduxName); ok {
			if _, err := os.Stat(fullPath); err == nil {
				return reduxName
			}
		}
	}
	if strings.HasPrefix(fileName, "media/") {
		unpackedName := filepath.Join("_build", "ie_unpacked", fileName)
		if fullPath, ok := f.sanitizePath(unpackedName); ok {
			if _, err := os.Stat(fullPath); err == nil {
				return unpackedName
			}
		}
	}
	if fileName == "_b" {
		return "_build/ie_media/redux-high/includes/test.lnk"
	}
	if strings.HasPrefix(fileName, "_b/") {
		return "_build/ie_media/redux-high/" + strings.TrimPrefix(fileName, "_b/")
	}
	return fileName
}

func (f *FileIODevice) caseInsensitiveReadPath(fullPath string) (string, bool) {
	rel, err := filepath.Rel(f.baseDir, fullPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	parts := strings.Split(filepath.Clean(rel), string(os.PathSeparator))
	cur := f.baseDir
	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}
		entries, err := os.ReadDir(cur)
		if err != nil {
			return "", false
		}
		match := ""
		for _, entry := range entries {
			name := entry.Name()
			if name == part {
				match = name
				break
			}
			if match == "" && strings.EqualFold(name, part) {
				match = name
			}
		}
		if match == "" {
			return "", false
		}
		cur = filepath.Join(cur, match)
	}
	return cur, true
}

func (f *FileIODevice) reduxReadFallbacks(fileName string) []string {
	const prefix = "_build/ie_media/redux-high/"
	if !strings.HasPrefix(fileName, prefix) {
		return nil
	}
	rel := strings.TrimPrefix(fileName, prefix)
	candidates := make([]string, 0, 4)
	switch {
	case strings.HasPrefix(rel, "levels/"):
		levelAsset := strings.TrimPrefix(rel, "levels/")
		candidates = append(candidates, prefix+"levels_editor_uncompressed/"+levelAsset)
	case strings.HasPrefix(rel, "soundfx/samples/"):
		name := strings.TrimPrefix(rel, "soundfx/samples/")
		candidates = append(candidates,
			"_build/ie_unpacked/media/ab3dsfx/samples/"+name,
			"_build/ie_unpacked/media/ab3dsfx/"+name,
			"_build/ie_unpacked/media/sounds/"+strings.TrimSuffix(name, ".fib"),
		)
	case strings.HasPrefix(rel, "hqn/"):
		candidates = append(candidates, "_build/ie_unpacked/media/hqn/"+strings.TrimPrefix(rel, "hqn/"))
	case strings.HasPrefix(rel, "vectobj/"):
		candidates = append(candidates, "_build/ie_unpacked/media/vectobj/"+strings.TrimPrefix(rel, "vectobj/"))
	}
	candidates = append(candidates, "_build/ie_unpacked/media/"+rel)
	return candidates
}

func (f *FileIODevice) readHostFile(fileName string) ([]byte, string, error, bool) {
	fullPath, ok := f.sanitizePath(fileName)
	if !ok {
		return nil, "", nil, false
	}
	data, err := os.ReadFile(fullPath)
	if os.IsNotExist(err) {
		if resolved, ok := f.caseInsensitiveReadPath(fullPath); ok {
			fullPath = resolved
			data, err = os.ReadFile(fullPath)
		}
	}
	return data, fullPath, err, true
}

// doRead performs the actual file read operation.
func (f *FileIODevice) doRead() {
	fileName := f.resolveReadFileName(f.readFileName())
	data, fullPath, err, ok := f.readHostFile(fileName)
	if !ok {
		f.fileStatus = 1
		f.fileErrorCode = FILE_ERR_PATH_TRAVERSAL
		return
	}
	if err != nil {
		for _, candidate := range f.reduxReadFallbacks(fileName) {
			fallbackData, fallbackPath, fallbackErr, fallbackOK := f.readHostFile(candidate)
			if !fallbackOK {
				continue
			}
			if fallbackErr == nil {
				fileName = candidate
				fullPath = fallbackPath
				data = fallbackData
				err = nil
				break
			}
		}
	}
	traceHostIO("FILEIO", fmt.Sprintf("READ name_ptr=0x%08X", f.fileNamePtr), fileName, fullPath, err, len(data))
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
	traceHostIO("FILEIO", fmt.Sprintf("WRITE name_ptr=0x%08X", f.fileNamePtr), fileName, fullPath, err, len(data))
	if err != nil {
		f.fileStatus = 1
		f.fileErrorCode = FILE_ERR_PERMISSION
		return
	}

	f.fileStatus = 0
	f.fileErrorCode = FILE_ERR_OK
}

// doList writes a sorted, newline-delimited directory listing to FILE_DATA_PTR.
func (f *FileIODevice) doList() {
	dirName := f.readFileName()
	fullPath, ok := f.sanitizePath(dirName)
	if !ok {
		f.fileStatus = 1
		f.fileErrorCode = FILE_ERR_PATH_TRAVERSAL
		f.fileResultLen = 0
		return
	}

	entries, err := os.ReadDir(fullPath)
	if os.IsNotExist(err) {
		if resolved, resolvedOK := f.caseInsensitiveReadPath(fullPath); resolvedOK {
			fullPath = resolved
			entries, err = os.ReadDir(fullPath)
		}
	}
	traceHostIO("FILEIO", fmt.Sprintf("LIST name_ptr=0x%08X", f.fileNamePtr), dirName, fullPath, err, len(entries))
	if err != nil {
		f.fileStatus = 1
		if os.IsNotExist(err) {
			f.fileErrorCode = FILE_ERR_NOT_FOUND
		} else {
			f.fileErrorCode = FILE_ERR_PERMISSION
		}
		f.fileResultLen = 0
		return
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	data := []byte(strings.Join(names, "\r\n"))
	if len(data) > 0 {
		data = append(data, '\r', '\n')
	}

	for i, b := range data {
		f.bus.Write8(f.fileDataPtr+uint32(i), b)
	}
	f.bus.Write8(f.fileDataPtr+uint32(len(data)), 0)

	f.fileStatus = 0
	f.fileErrorCode = FILE_ERR_OK
	f.fileResultLen = uint32(len(data))
}
