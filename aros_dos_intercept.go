package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const arosDOSMaxPacket = 1 << 20
const arosDOSReadAheadSize = 64 * 1024

type resolveReason uint8

const (
	resolveOK resolveReason = iota
	resolveNotFound
	resolveWrongType
	resolveExists
)

type arosFileOps interface {
	ReadAt([]byte, int64) (int, error)
	WriteAt([]byte, int64) (int, error)
	Seek(int64, int) (int64, error)
	Stat() (fs.FileInfo, error)
	Truncate(int64) error
	Close() error
}

var arosOpenFile = func(name string, flag int, perm fs.FileMode) (arosFileOps, error) {
	return os.OpenFile(name, flag, perm)
}

type arosHandleMode uint8

const (
	arosHandleRead arosHandleMode = iota
	arosHandleOutput
	arosHandleUpdate
)

type arosReadAhead struct {
	start int64
	data  []byte
}

type arosFileHandle struct {
	file      arosFileOps
	name      string
	hostPath  string
	pos       int64
	mode      arosHandleMode
	dirty     bool
	firstRead bool
	cache     arosReadAhead
}

// ArosDOSDevice provides host filesystem access to AROS via MMIO.
// The AROS packet handler (iehandler) translates AmigaDOS packets
// to MMIO register writes. Writing the command register triggers
// synchronous execution on the Go side.
type ArosDOSDevice struct {
	bus      *MachineBus
	hostRoot string
	symbols  *SymbolTable

	// Lock management: key → lock state
	locks    map[uint32]*adosLock
	nextLock uint32

	// File handle management: key -> open file
	handles    map[uint32]*arosFileHandle
	nextHandle uint32

	// MMIO register shadow state
	arg1 uint32
	arg2 uint32
	arg3 uint32
	arg4 uint32
	res1 uint32
	res2 uint32

	debugTrace bool

	// dirNameCache maps directory path → (lowercase name → actual name on disk)
	dirNameCache map[string]map[string]string
}

type adosLock struct {
	hostPath   string
	isDir      bool
	mode       int32
	dirEntries []fs.DirEntry
	dirIdx     int
}

// NewArosDOSDevice creates a new AROS DOS device mapping hostRoot as an AmigaDOS volume.
func NewArosDOSDevice(bus *MachineBus, hostRoot string) (*ArosDOSDevice, error) {
	resolved, err := filepath.EvalSymlinks(hostRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve symlinks for %q: %w", hostRoot, err)
	}
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return nil, fmt.Errorf("absolute path for %q: %w", resolved, err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", absPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", absPath)
	}

	d := &ArosDOSDevice{
		bus:          bus,
		hostRoot:     absPath,
		locks:        make(map[uint32]*adosLock),
		nextLock:     1, // 0 means root/no-lock
		handles:      make(map[uint32]*arosFileHandle),
		nextHandle:   1,
		debugTrace:   false,
		dirNameCache: make(map[string]map[string]string),
	}

	// Pre-create root lock at key 0
	d.locks[0] = &adosLock{
		hostPath: absPath,
		isDir:    true,
		mode:     -2, // SHARED_LOCK
	}

	return d, nil
}

func (d *ArosDOSDevice) SetSymbolTable(symbols *SymbolTable) {
	if d == nil {
		return
	}
	d.symbols = symbols
}

// HandleRead handles MMIO reads from the AROS DOS region.
func (d *ArosDOSDevice) HandleRead(addr uint32) uint32 {
	switch addr {
	case AROS_DOS_ARG1:
		return d.arg1
	case AROS_DOS_ARG2:
		return d.arg2
	case AROS_DOS_ARG3:
		return d.arg3
	case AROS_DOS_ARG4:
		return d.arg4
	case AROS_DOS_RESULT1:
		return d.res1
	case AROS_DOS_RESULT2:
		return d.res2
	case AROS_DOS_STATUS:
		return 0 // always ready
	}
	return 0
}

// HandleWrite handles MMIO writes to the AROS DOS region.
func (d *ArosDOSDevice) HandleWrite(addr uint32, val uint32) {
	switch addr {
	case AROS_DOS_ARG1:
		d.arg1 = val
	case AROS_DOS_ARG2:
		d.arg2 = val
	case AROS_DOS_ARG3:
		d.arg3 = val
	case AROS_DOS_ARG4:
		d.arg4 = val
	case AROS_DOS_CMD:
		d.dispatch(val)
	}
}

func (d *ArosDOSDevice) dispatch(cmd uint32) {
	d.res1 = ADOS_DOSFALSE
	d.res2 = ADOS_ERR_NONE

	if d.debugTrace && cmd != ADOS_CMD_READ && cmd != ADOS_CMD_SEEK && cmd != ADOS_CMD_WRITE {
		fmt.Printf("[ADOS] dispatch cmd=%d arg1=0x%X arg2=0x%X arg3=0x%X arg4=0x%X\n",
			cmd, d.arg1, d.arg2, d.arg3, d.arg4)
	}

	switch cmd {
	case ADOS_CMD_LOCK:
		d.cmdLock()
	case ADOS_CMD_UNLOCK:
		d.cmdUnlock()
	case ADOS_CMD_EXAMINE:
		d.cmdExamine()
	case ADOS_CMD_EXNEXT:
		d.cmdExNext()
	case ADOS_CMD_FINDINPUT:
		d.cmdFindInput()
	case ADOS_CMD_FINDOUTPUT:
		d.cmdFindOutput()
	case ADOS_CMD_FINDUPDATE:
		d.cmdFindUpdate()
	case ADOS_CMD_READ:
		d.cmdRead()
	case ADOS_CMD_WRITE:
		d.cmdWrite()
	case ADOS_CMD_SEEK:
		d.cmdSeek()
	case ADOS_CMD_CLOSE:
		d.cmdClose()
	case ADOS_CMD_PARENT:
		d.cmdParent()
	case ADOS_CMD_DELETE:
		d.cmdDelete()
	case ADOS_CMD_CREATEDIR:
		d.cmdCreateDir()
	case ADOS_CMD_RENAME:
		d.cmdRename()
	case ADOS_CMD_DISKINFO:
		d.cmdDiskInfo()
	case ADOS_CMD_DUPLOCK:
		d.cmdDupLock()
	case ADOS_CMD_SAMELOCK:
		d.cmdSameLock()
	case ADOS_CMD_IS_FS:
		d.res1 = ADOS_DOSTRUE
	case ADOS_CMD_SET_FILESIZE:
		d.cmdSetFileSize()
	case ADOS_CMD_SET_PROTECT:
		d.cmdSetProtect()
	case ADOS_CMD_EXAMINE_FH:
		d.cmdExamineFH()
	case ADOS_CMD_LOADSEG_SYMS:
		d.cmdLoadSegSymbols()
	case ADOS_CMD_EXAMINE_ALL:
		d.cmdExamineAll()
	default:
		d.res1 = ADOS_DOSFALSE
		d.res2 = ADOS_ERROR_ACTION_NOT_KNOWN
	}
}

// --- Lock operations ---

func (d *ArosDOSDevice) cmdLock() {
	namePtr := d.arg1
	parentKey := d.arg2

	parent, ok := d.locks[parentKey]
	if !ok {
		if d.debugTrace {
			name := d.readString(namePtr)
			fmt.Printf("[ADOS] LOCK %q (parent=%d) → INVALID_LOCK\n", name, parentKey)
		}
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}

	name := d.readString(namePtr)
	hostPath, reason := d.resolveOpenReadDOSPath(parent.hostPath, name)
	if reason != resolveOK {
		if d.debugTrace {
			fmt.Printf("[ADOS] LOCK %q (parent=%d) → path resolve failed\n", name, parentKey)
		}
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	info, err := os.Stat(hostPath)
	if err != nil {
		if d.debugTrace {
			fmt.Printf("[ADOS] LOCK %q (parent=%d, path=%q) → NOT_FOUND\n", name, parentKey, hostPath)
		}
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	key := d.nextLock
	d.nextLock++
	d.locks[key] = &adosLock{
		hostPath: hostPath,
		isDir:    info.IsDir(),
		mode:     int32(d.arg3),
	}

	if d.debugTrace {
		fmt.Printf("[ADOS] LOCK %q parent=%d → key=%d (dir=%v, host=%q)\n", name, parentKey, key, info.IsDir(), hostPath)
	}

	d.res1 = key
}

func (d *ArosDOSDevice) cmdUnlock() {
	key := d.arg1
	if key == 0 {
		d.res1 = ADOS_DOSTRUE
		return
	}
	if d.debugTrace {
		if l, ok := d.locks[key]; ok {
			fmt.Printf("[ADOS] UNLOCK key=%d (host=%q)\n", key, l.hostPath)
		}
	}
	delete(d.locks, key)
	d.res1 = ADOS_DOSTRUE
}

func (d *ArosDOSDevice) cmdDupLock() {
	srcKey := d.arg1
	src, ok := d.locks[srcKey]
	if !ok {
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}

	key := d.nextLock
	d.nextLock++
	d.locks[key] = &adosLock{
		hostPath: src.hostPath,
		isDir:    src.isDir,
		mode:     -2, // AmigaDOS: DupLock always returns SHARED_LOCK
	}
	d.res1 = key
	if d.debugTrace {
		fmt.Printf("[ADOS] DUPLOCK src=%d → key=%d (host=%q)\n", srcKey, key, src.hostPath)
	}
}

func (d *ArosDOSDevice) cmdSameLock() {
	key1 := d.arg1
	key2 := d.arg2

	path1 := d.hostRoot
	if l, ok := d.locks[key1]; ok {
		path1 = l.hostPath
	}
	path2 := d.hostRoot
	if l, ok := d.locks[key2]; ok {
		path2 = l.hostPath
	}

	if path1 == path2 {
		d.res1 = ADOS_LOCK_SAME
		if d.debugTrace {
			fmt.Printf("[ADOS] SAMELOCK %d vs %d → SAME (%q)\n", key1, key2, path1)
		}
	} else if strings.HasPrefix(path1, d.hostRoot) && strings.HasPrefix(path2, d.hostRoot) {
		d.res1 = ADOS_LOCK_SAME_VOLUME
		if d.debugTrace {
			fmt.Printf("[ADOS] SAMELOCK %d vs %d → SAME_VOLUME (%q vs %q)\n", key1, key2, path1, path2)
		}
	} else {
		d.res1 = ADOS_LOCK_DIFFERENT
		if d.debugTrace {
			fmt.Printf("[ADOS] SAMELOCK %d vs %d → DIFFERENT (%q vs %q)\n", key1, key2, path1, path2)
		}
	}
}

func (d *ArosDOSDevice) cmdParent() {
	key := d.arg1
	lock, ok := d.locks[key]
	if !ok {
		if key == 0 {
			// Parent of root is null
			d.res1 = 0
			d.res2 = ADOS_ERR_NONE
			return
		}
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}

	// Check if already at root
	if lock.hostPath == d.hostRoot {
		d.res1 = 0
		d.res2 = ADOS_ERR_NONE
		return
	}

	parentPath := filepath.Dir(lock.hostPath)
	// Safety: don't go above hostRoot
	if !strings.HasPrefix(parentPath, d.hostRoot) {
		d.res1 = 0
		d.res2 = ADOS_ERR_NONE
		return
	}

	newKey := d.nextLock
	d.nextLock++
	d.locks[newKey] = &adosLock{
		hostPath: parentPath,
		isDir:    true,
		mode:     -2,
	}
	d.res1 = newKey
}

// --- Directory examination ---

func (d *ArosDOSDevice) cmdExamine() {
	key := d.arg1
	fibPtr := d.arg2

	lock, ok := d.locks[key]
	if !ok && key != 0 {
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}
	if key == 0 {
		lock = d.locks[0]
	}

	info, err := os.Stat(lock.hostPath)
	if err != nil {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	d.fillFIB(fibPtr, info, lock.hostPath)

	// Reset directory enumeration for EXNEXT
	if info.IsDir() {
		entries, err := os.ReadDir(lock.hostPath)
		if err != nil {
			entries = nil
		}
		lock.dirEntries = entries
		lock.dirIdx = 0
	}

	if d.debugTrace {
		name := info.Name()
		if lock.hostPath == d.hostRoot || name == "" || name == "." {
			name = "IE"
		}
		fmt.Printf("[ADOS] EXAMINE key=%d → %q (dir=%v, size=%d, path=%s)\n", key, name, info.IsDir(), info.Size(), lock.hostPath)
	}

	d.res1 = ADOS_DOSTRUE
}

func (d *ArosDOSDevice) cmdExNext() {
	key := d.arg1
	fibPtr := d.arg2

	lock, ok := d.locks[key]
	if !ok && key != 0 {
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}
	if key == 0 {
		lock = d.locks[0]
	}

	if lock.dirEntries == nil || lock.dirIdx >= len(lock.dirEntries) {
		d.res1 = ADOS_DOSFALSE
		d.res2 = ADOS_ERROR_NO_MORE_ENTRIES
		return
	}

	entry := lock.dirEntries[lock.dirIdx]
	lock.dirIdx++

	entryPath := filepath.Join(lock.hostPath, entry.Name())
	info, err := entry.Info()
	if err != nil {
		// Skip entries we can't stat, try next
		d.cmdExNext()
		return
	}

	d.fillFIB(fibPtr, info, entryPath)

	if d.debugTrace {
		fmt.Printf("[ADOS] EXNEXT key=%d [%d] → %q (dir=%v, size=%d)\n",
			key, lock.dirIdx-1, entry.Name(), info.IsDir(), info.Size())
	}

	d.res1 = ADOS_DOSTRUE
}

func (d *ArosDOSDevice) cmdExamineAll() {
	reqPtr := d.arg1
	req := make([]byte, ADOS_EXALL_REQ_SIZE)
	if err := ReadGuestBytes(d.bus, reqPtr, 0, req); err != nil {
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}
	lockKey := binary.BigEndian.Uint32(req[ADOS_EXALL_REQ_LOCK_KEY:])
	bufferPtr := binary.BigEndian.Uint32(req[ADOS_EXALL_REQ_BUFFER:])
	bufferSize := binary.BigEndian.Uint32(req[ADOS_EXALL_REQ_BUFFER_LEN:])
	exAllType := binary.BigEndian.Uint32(req[ADOS_EXALL_REQ_TYPE:])
	controlPtr := binary.BigEndian.Uint32(req[ADOS_EXALL_REQ_CONTROL:])

	if exAllType < ADOS_ED_NAME_TYPE || exAllType > ADOS_ED_COMMENT_TYPE {
		d.res2 = ADOS_ERROR_BAD_NUMBER
		return
	}

	lock, ok := d.locks[lockKey]
	if !ok && lockKey != 0 {
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}
	if lockKey == 0 {
		lock = d.locks[0]
	}
	if !lock.isDir {
		d.res2 = ADOS_ERROR_OBJECT_WRONG_TYPE
		return
	}

	control := make([]byte, 16)
	if err := ReadGuestBytes(d.bus, controlPtr, 0, control); err != nil {
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}
	lastKey := binary.BigEndian.Uint32(control[ADOS_EAC_LAST_KEY:])
	if binary.BigEndian.Uint32(control[ADOS_EAC_MATCH_STRING:]) != 0 ||
		binary.BigEndian.Uint32(control[ADOS_EAC_MATCH_FUNC:]) != 0 {
		d.res2 = ADOS_ERROR_ACTION_NOT_KNOWN
		return
	}
	if bufferSize > arosDOSMaxPacket {
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}
	if err := ValidateGuestSpan(d.bus, bufferPtr, 0, uint64(bufferSize)); err != nil {
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}

	entries, err := os.ReadDir(lock.hostPath)
	if err != nil {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}
	start := int(lastKey)
	if start > len(entries) {
		start = len(entries)
	}

	out := make([]byte, int(bufferSize))
	count := uint32(0)
	prevOff := -1
	cursor := 0
	nextIndex := start
	structSize := exAllStructSize(exAllType)
	for i := start; i < len(entries); i++ {
		entry := entries[i]
		info, err := entry.Info()
		if err != nil {
			nextIndex = i + 1
			continue
		}
		entrySize := align2(structSize + len(entry.Name()) + 1 + exAllCommentSize(exAllType))
		if cursor+entrySize > len(out) {
			nextIndex = i
			if count == 0 {
				d.res2 = ADOS_ERROR_NO_FREE_STORE
				return
			}
			break
		}
		entryOff := cursor
		nameOff := entryOff + structSize
		commentOff := nameOff + len(entry.Name()) + 1
		arosPutBE32(out, entryOff+ADOS_ED_NEXT, 0)
		arosPutBE32(out, entryOff+ADOS_ED_NAME, bufferPtr+uint32(nameOff))
		arosPutCString(out, nameOff, entry.Name(), len(entry.Name())+1)

		entryType := uint32(ADOS_ST_FILE)
		if info.IsDir() {
			entryType = ADOS_ST_USERDIR
		}
		if exAllType >= ADOS_ED_TYPE_TYPE {
			arosPutBE32(out, entryOff+ADOS_ED_TYPE, entryType)
		}
		if exAllType >= ADOS_ED_SIZE_TYPE {
			arosPutBE32(out, entryOff+ADOS_ED_SIZE, uint32(info.Size()))
		}
		if exAllType >= ADOS_ED_PROTECTION_TYPE {
			arosPutBE32(out, entryOff+ADOS_ED_PROT, d.detectProtection(info, filepath.Join(lock.hostPath, entry.Name())))
		}
		if exAllType >= ADOS_ED_DATE_TYPE {
			fillDateStampBytes(out[entryOff+ADOS_ED_DAYS:entryOff+ADOS_ED_DAYS+12], info.ModTime())
		}
		if exAllType >= ADOS_ED_COMMENT_TYPE {
			arosPutBE32(out, entryOff+ADOS_ED_COMMENT, bufferPtr+uint32(commentOff))
			arosPutCString(out, commentOff, "", 1)
		}
		if prevOff >= 0 {
			arosPutBE32(out, prevOff+ADOS_ED_NEXT, bufferPtr+uint32(entryOff))
		}
		prevOff = entryOff
		cursor += entrySize
		count++
		nextIndex = i + 1
	}

	if count == 0 {
		arosPutBE32(control, ADOS_EAC_ENTRIES, 0)
		_ = WriteGuestBytes(d.bus, controlPtr, 0, control)
		d.res1 = ADOS_DOSFALSE
		d.res2 = ADOS_ERROR_NO_MORE_ENTRIES
		return
	}
	if err := WriteGuestBytes(d.bus, bufferPtr, 0, out[:cursor]); err != nil {
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}
	arosPutBE32(control, ADOS_EAC_ENTRIES, count)
	arosPutBE32(control, ADOS_EAC_LAST_KEY, uint32(nextIndex))
	if err := WriteGuestBytes(d.bus, controlPtr, 0, control); err != nil {
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}
	d.res1 = ADOS_DOSTRUE
}

func exAllStructSize(exAllType uint32) int {
	switch exAllType {
	case ADOS_ED_NAME_TYPE:
		return ADOS_ED_TYPE
	case ADOS_ED_TYPE_TYPE:
		return ADOS_ED_SIZE
	case ADOS_ED_SIZE_TYPE:
		return ADOS_ED_PROT
	case ADOS_ED_PROTECTION_TYPE:
		return ADOS_ED_DAYS
	case ADOS_ED_DATE_TYPE:
		return ADOS_ED_COMMENT
	case ADOS_ED_COMMENT_TYPE:
		return ADOS_ED_OWNER_UID
	default:
		return 0
	}
}

func exAllCommentSize(exAllType uint32) int {
	if exAllType >= ADOS_ED_COMMENT_TYPE {
		return 1
	}
	return 0
}

func align2(v int) int {
	return (v + 1) &^ 1
}

func (d *ArosDOSDevice) cmdExamineFH() {
	handleKey := d.arg1
	fibPtr := d.arg2

	h, ok := d.handles[handleKey]
	if !ok {
		if d.debugTrace {
			fmt.Printf("[ADOS] EXAMINE_FH handle=%d → INVALID_LOCK\n", handleKey)
		}
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}

	info, err := h.file.Stat()
	if err != nil {
		if d.debugTrace {
			fmt.Printf("[ADOS] EXAMINE_FH %q → stat error: %v\n", h.hostPath, err)
		}
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	d.fillFIB(fibPtr, info, h.hostPath)
	if d.debugTrace {
		prot := uint32(d.bus.Read8(fibPtr+ADOS_FIB_PROTECTION)) << 24
		prot |= uint32(d.bus.Read8(fibPtr+ADOS_FIB_PROTECTION+1)) << 16
		prot |= uint32(d.bus.Read8(fibPtr+ADOS_FIB_PROTECTION+2)) << 8
		prot |= uint32(d.bus.Read8(fibPtr + ADOS_FIB_PROTECTION + 3))
		fmt.Fprintf(os.Stderr, "[ADOS] EXAMINE_FH handle=%d %q → prot=0x%08X fib@0x%08X\n",
			handleKey, info.Name(), prot, fibPtr)
	}
	d.res1 = ADOS_DOSTRUE
}

// --- File operations ---

func (d *ArosDOSDevice) cmdFindInput() {
	namePtr := d.arg1
	parentKey := d.arg2

	parent, ok := d.locks[parentKey]
	if !ok {
		if parentKey == 0 {
			parent = d.locks[0]
		} else {
			if d.debugTrace {
				name := d.readString(namePtr)
				fmt.Printf("[ADOS] FINDINPUT %q (parent=%d) → INVALID_LOCK\n", name, parentKey)
			}
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}

	name := d.readString(namePtr)
	hostPath, reason := d.resolveOpenReadDOSPath(parent.hostPath, name)
	if reason != resolveOK {
		if d.debugTrace {
			fmt.Printf("[ADOS] FINDINPUT %q (parent=%d) → path resolve failed\n", name, parentKey)
		}
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	info, err := os.Stat(hostPath)
	if err != nil {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}
	if !info.Mode().IsRegular() {
		d.res2 = ADOS_ERROR_OBJECT_WRONG_TYPE
		return
	}

	f, err := arosOpenFile(hostPath, os.O_RDONLY, 0)
	if err != nil {
		if d.debugTrace {
			fmt.Printf("[ADOS] FINDINPUT %q (parent=%d, path=%q) → %v\n", name, parentKey, hostPath, err)
		}
		if os.IsNotExist(err) {
			d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		} else {
			d.res2 = ADOS_ERROR_READ_PROTECTED
		}
		return
	}

	key := d.nextHandle
	d.nextHandle++
	d.handles[key] = &arosFileHandle{
		file:      f,
		name:      name,
		hostPath:  hostPath,
		mode:      arosHandleRead,
		firstRead: true,
	}

	if d.debugTrace {
		fmt.Printf("[ADOS] FINDINPUT %q (parent=%d, path=%q) → handle=%d\n", name, parentKey, hostPath, key)
	}

	d.res1 = key
}

func (d *ArosDOSDevice) cmdLoadSegSymbols() {
	namePtr := d.arg1
	parentKey := d.arg2
	base := d.arg3

	if d.symbols == nil {
		d.res1 = ADOS_DOSTRUE
		d.res2 = ADOS_ERR_NONE
		return
	}

	parent, ok := d.locks[parentKey]
	if !ok {
		if parentKey == 0 {
			parent = d.locks[0]
		} else {
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}

	name := d.readString(namePtr)
	hostPath, reason := d.resolveOpenReadDOSPath(parent.hostPath, name)
	if reason != resolveOK {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	sidecar, err := loadGuestSymbolSidecar(d.symbols, "M68K", hostPath, uint64(base))
	if err != nil {
		if d.debugTrace {
			fmt.Printf("[ADOS] LOADSEG_SYMS %q base=$%08X → sidecar %q disabled: %v\n", name, base, sidecar, err)
		}
		d.res2 = ADOS_ERROR_BAD_NUMBER
		return
	}
	if d.debugTrace && sidecar != "" {
		fmt.Printf("[ADOS] LOADSEG_SYMS %q → loaded %s at base $%08X\n", name, sidecar, base)
	}
	d.res1 = ADOS_DOSTRUE
	d.res2 = ADOS_ERR_NONE
}

func (d *ArosDOSDevice) cmdFindOutput() {
	namePtr := d.arg1
	parentKey := d.arg2

	parent, ok := d.locks[parentKey]
	if !ok {
		if parentKey == 0 {
			parent = d.locks[0]
		} else {
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}

	name := d.readString(namePtr)
	hostPath, reason := d.resolveCreateDOSPath(parent.hostPath, name)
	switch reason {
	case resolveOK, resolveExists:
	case resolveWrongType:
		d.res2 = ADOS_ERROR_OBJECT_WRONG_TYPE
		return
	default:
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	f, err := arosOpenFile(hostPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC|arosOpenNoFollow, 0644)
	if err != nil {
		d.res2 = mapWriteErr(err)
		return
	}

	key := d.nextHandle
	d.nextHandle++
	d.handles[key] = &arosFileHandle{
		file:      f,
		name:      name,
		hostPath:  hostPath,
		mode:      arosHandleOutput,
		firstRead: true,
		dirty:     true,
	}
	d.invalidateHandleCachesForPath(hostPath)
	d.invalidatePathCaches(hostPath)

	if d.debugTrace {
		fmt.Printf("[ADOS] FINDOUTPUT %q → handle=%d\n", name, key)
	}

	d.res1 = key
}

func (d *ArosDOSDevice) cmdFindUpdate() {
	namePtr := d.arg1
	parentKey := d.arg2

	parent, ok := d.locks[parentKey]
	if !ok {
		if parentKey == 0 {
			parent = d.locks[0]
		} else {
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}

	name := d.readString(namePtr)
	hostPath, reason := d.resolveUpdateDOSPath(parent.hostPath, name)
	createdMissing := false
	if reason == resolveNotFound {
		hostPath, reason = d.resolveCreateDOSPath(parent.hostPath, name)
		createdMissing = reason == resolveOK
	}
	if reason == resolveWrongType {
		d.res2 = ADOS_ERROR_OBJECT_WRONG_TYPE
		return
	}
	if reason != resolveOK && reason != resolveExists {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	f, err := arosOpenFile(hostPath, os.O_RDWR|os.O_CREATE|arosOpenNoFollow, 0644)
	if err != nil {
		d.res2 = mapWriteErr(err)
		return
	}

	key := d.nextHandle
	d.nextHandle++
	d.handles[key] = &arosFileHandle{
		file:      f,
		name:      name,
		hostPath:  hostPath,
		mode:      arosHandleUpdate,
		firstRead: true,
		dirty:     createdMissing,
	}
	if createdMissing {
		d.invalidatePathCaches(hostPath)
	}
	d.res1 = key
}

func (h *arosFileHandle) clearCache() {
	h.cache.start = 0
	h.cache.data = nil
}

func (h *arosFileHandle) cacheContains(pos int64) bool {
	if len(h.cache.data) == 0 {
		return false
	}
	return pos >= h.cache.start && pos <= h.cache.start+int64(len(h.cache.data))
}

func (d *ArosDOSDevice) readFromHandle(h *arosFileHandle, dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	if len(h.cache.data) == 0 || h.pos < h.cache.start || h.pos >= h.cache.start+int64(len(h.cache.data)) {
		fillLen := arosDOSReadAheadSize
		if len(dst) > fillLen {
			fillLen = len(dst)
		}
		buf := make([]byte, fillLen)
		n, err := h.file.ReadAt(buf, h.pos)
		if n > 0 {
			h.cache.start = h.pos
			h.cache.data = buf[:n]
		} else {
			h.clearCache()
		}
		if err != nil && err != io.EOF {
			return 0, err
		}
	}

	offset := int(h.pos - h.cache.start)
	if offset < 0 || offset >= len(h.cache.data) {
		return 0, io.EOF
	}
	n := copy(dst, h.cache.data[offset:])
	h.pos += int64(n)
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

func (d *ArosDOSDevice) cmdRead() {
	handleKey := d.arg1
	bufPtr := d.arg2
	length := d.arg3

	h, ok := d.handles[handleKey]
	if !ok {
		d.res1 = 0xFFFFFFFF // -1
		d.res2 = ADOS_ERROR_INVALID_LOCK
		if d.debugTrace {
			fmt.Printf("[ADOS] READ handle=%d → INVALID_LOCK\n", handleKey)
		}
		return
	}

	if length > arosDOSMaxPacket {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}

	buf := make([]byte, int(length))
	n, err := d.readFromHandle(h, buf)
	if n > 0 {
		if err := WriteGuestBytes(d.bus, bufPtr, 0, buf[:n]); err != nil {
			d.res1 = 0xFFFFFFFF
			d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
			return
		}
	}

	if err != nil && n == 0 {
		d.res1 = 0 // EOF returns 0, not error
	} else {
		d.res1 = uint32(n)
	}

	// Log first read per handle - shows magic bytes for LoadSeg diagnosis.
	if d.debugTrace && n > 0 && h.firstRead {
		h.firstRead = false
		magic := ""
		if n >= 4 {
			magic = fmt.Sprintf(" magic=%02x%02x%02x%02x", buf[0], buf[1], buf[2], buf[3])
		}
		fmt.Printf("[ADOS] FIRST_READ handle=%d %q len=%d n=%d%s\n",
			handleKey, h.name, length, n, magic)
	}
}

func (d *ArosDOSDevice) cmdWrite() {
	handleKey := d.arg1
	bufPtr := d.arg2
	length := d.arg3

	h, ok := d.handles[handleKey]
	if !ok {
		d.res1 = 0xFFFFFFFF // -1
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}

	if length > arosDOSMaxPacket {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}

	buf := make([]byte, int(length))
	if err := ReadGuestBytes(d.bus, bufPtr, 0, buf); err != nil {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}

	n, err := h.file.WriteAt(buf, h.pos)
	if err != nil {
		d.res1 = 0xFFFFFFFF // -1
		d.res2 = mapWriteErr(err)
		return
	}
	h.pos += int64(n)
	h.dirty = true
	d.invalidateHandleCachesForPath(h.hostPath)
	d.invalidatePathCaches(h.hostPath)
	d.res1 = uint32(n)
}

func (d *ArosDOSDevice) cmdSeek() {
	handleKey := d.arg1
	offset := int64(int32(d.arg2)) // sign-extend
	mode := d.arg3

	h, ok := d.handles[handleKey]
	if !ok {
		d.res1 = 0xFFFFFFFF // -1
		d.res2 = ADOS_ERROR_INVALID_LOCK
		if d.debugTrace {
			fmt.Printf("[ADOS] SEEK handle=%d → INVALID_LOCK\n", handleKey)
		}
		return
	}

	oldPos := h.pos

	var newPos int64
	switch mode {
	case ADOS_OFFSET_BEGINNING:
		newPos = offset
	case ADOS_OFFSET_CURRENT:
		newPos = h.pos + offset
	case ADOS_OFFSET_END:
		info, err := h.file.Stat()
		if err != nil {
			d.res1 = 0xFFFFFFFF
			d.res2 = ADOS_ERROR_SEEK_ERROR
			return
		}
		newPos = info.Size() + offset
	default:
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_BAD_NUMBER
		return
	}
	if newPos < 0 {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_SEEK_ERROR
		return
	}
	if _, err := h.file.Seek(newPos, io.SeekStart); err != nil {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_SEEK_ERROR
		return
	}
	h.pos = newPos
	if !h.cacheContains(newPos) {
		h.clearCache()
	}

	d.res1 = uint32(oldPos)

	if d.debugTrace {
		fmt.Printf("[ADOS] SEEK handle=%d offset=%d mode=0x%X oldPos=%d → newPos=%d\n",
			handleKey, offset, mode, oldPos, newPos)
	}
}

func (d *ArosDOSDevice) cmdClose() {
	handleKey := d.arg1
	if h, ok := d.handles[handleKey]; ok {
		h.clearCache()
		h.file.Close()
		if h.dirty && (h.mode == arosHandleOutput || h.mode == arosHandleUpdate) {
			d.invalidatePathCaches(h.hostPath)
		}
		delete(d.handles, handleKey)
	}
	d.res1 = ADOS_DOSTRUE
}

func (d *ArosDOSDevice) cmdSetFileSize() {
	handleKey := d.arg1
	newSize := int64(int32(d.arg2))
	mode := d.arg3

	h, ok := d.handles[handleKey]
	if !ok {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}

	// Compute absolute size based on mode
	var absSize int64
	switch mode {
	case ADOS_OFFSET_BEGINNING:
		absSize = newSize
	case ADOS_OFFSET_CURRENT:
		absSize = h.pos + newSize
	case ADOS_OFFSET_END:
		info, _ := h.file.Stat()
		absSize = info.Size() + newSize
	default:
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_BAD_NUMBER
		return
	}

	if err := h.file.Truncate(absSize); err != nil {
		d.res1 = 0xFFFFFFFF
		d.res2 = mapWriteErr(err)
		return
	}
	h.dirty = true
	d.invalidateHandleCachesForPath(h.hostPath)
	d.invalidatePathCaches(h.hostPath)
	d.res1 = uint32(absSize)
}

func (d *ArosDOSDevice) cmdSetProtect() {
	parentKey := d.arg1
	namePtr := d.arg2
	protect := d.arg3

	parent, ok := d.locks[parentKey]
	if !ok {
		if parentKey == 0 {
			parent = d.locks[0]
		} else {
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}

	name := d.readString(namePtr)
	hostPath, reason := d.resolveSourceDOSPath(parent.hostPath, name)
	if reason == resolveWrongType {
		d.res2 = ADOS_ERROR_OBJECT_WRONG_TYPE
		return
	}
	if reason != resolveOK {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}
	info, err := os.Lstat(hostPath)
	if err != nil {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		d.res2 = ADOS_ERROR_OBJECT_WRONG_TYPE
		return
	}

	mode := info.Mode().Perm()
	if protect&ADOS_FIBF_READ != 0 {
		mode &^= 0o400
	} else {
		mode |= 0o400
	}
	if protect&ADOS_FIBF_WRITE != 0 {
		mode &^= 0o200
	} else {
		mode |= 0o200
	}
	if protect&ADOS_FIBF_EXECUTE != 0 {
		mode &^= 0o100
	} else {
		mode |= 0o100
	}
	if err := os.Chmod(hostPath, mode); err != nil {
		d.res2 = mapWriteErr(err)
		return
	}
	d.res1 = ADOS_DOSTRUE
}

// --- Filesystem operations ---

func (d *ArosDOSDevice) cmdDelete() {
	parentKey := d.arg1
	namePtr := d.arg2

	parent, ok := d.locks[parentKey]
	if !ok {
		if parentKey == 0 {
			parent = d.locks[0]
		} else {
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}

	name := d.readString(namePtr)
	hostPath, reason := d.resolveSourceDOSPath(parent.hostPath, name)
	if reason == resolveWrongType {
		d.res2 = ADOS_ERROR_OBJECT_WRONG_TYPE
		return
	}
	if reason != resolveOK {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	err := os.Remove(hostPath)
	if err != nil {
		d.res2 = mapDeleteErr(err)
		return
	}
	d.invalidatePathCaches(hostPath)
	d.res1 = ADOS_DOSTRUE
}

func (d *ArosDOSDevice) cmdCreateDir() {
	parentKey := d.arg1
	namePtr := d.arg2

	parent, ok := d.locks[parentKey]
	if !ok {
		if parentKey == 0 {
			parent = d.locks[0]
		} else {
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}

	name := d.readString(namePtr)
	hostPath, reason := d.resolveCreateDOSPath(parent.hostPath, name)
	switch reason {
	case resolveOK:
	case resolveExists, resolveWrongType:
		d.res2 = ADOS_ERROR_OBJECT_EXISTS
		return
	default:
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	if err := os.Mkdir(hostPath, 0755); err != nil {
		if os.IsExist(err) {
			d.res2 = ADOS_ERROR_OBJECT_EXISTS
		} else {
			d.res2 = ADOS_ERROR_WRITE_PROTECTED
		}
		return
	}
	d.invalidatePathCaches(hostPath)

	key := d.nextLock
	d.nextLock++
	d.locks[key] = &adosLock{
		hostPath: hostPath,
		isDir:    true,
		mode:     -1, // EXCLUSIVE_LOCK
	}
	d.res1 = key
}

func (d *ArosDOSDevice) cmdRename() {
	srcParentKey := d.arg1
	srcNamePtr := d.arg2
	dstParentKey := d.arg3
	dstNamePtr := d.arg4

	srcParent, ok := d.locks[srcParentKey]
	if !ok {
		if srcParentKey == 0 {
			srcParent = d.locks[0]
		} else {
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}
	dstParent, ok := d.locks[dstParentKey]
	if !ok {
		if dstParentKey == 0 {
			dstParent = d.locks[0]
		} else {
			d.res2 = ADOS_ERROR_INVALID_LOCK
			return
		}
	}

	srcName := d.readString(srcNamePtr)
	dstName := d.readString(dstNamePtr)
	srcPath, srcReason := d.resolveSourceDOSPath(srcParent.hostPath, srcName)
	if srcReason == resolveWrongType {
		d.res2 = ADOS_ERROR_OBJECT_WRONG_TYPE
		return
	}
	if srcReason != resolveOK {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}
	dstPath, dstReason := d.resolveCreateDOSPath(dstParent.hostPath, dstName)
	switch dstReason {
	case resolveOK:
	case resolveExists, resolveWrongType:
		d.res2 = ADOS_ERROR_OBJECT_EXISTS
		return
	default:
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		d.res2 = mapRenameErr(err)
		return
	}
	d.invalidatePathCaches(srcPath)
	d.invalidatePathCaches(dstPath)
	d.res1 = ADOS_DOSTRUE
}

func (d *ArosDOSDevice) cmdDiskInfo() {
	infoPtr := d.arg1

	buf := make([]byte, ADOS_INFO_DATA_SIZE)
	arosPutBE32(buf, ADOS_ID_NUM_SOFT_ERRORS, 0)
	arosPutBE32(buf, ADOS_ID_UNIT_NUMBER, 0)
	arosPutBE32(buf, ADOS_ID_DISK_STATE, ADOS_ID_VALIDATED)
	arosPutBE32(buf, ADOS_ID_NUM_BLOCKS, 1048576) // about 512 MB in 512-byte blocks
	arosPutBE32(buf, ADOS_ID_NUM_BLOCKS_USED, 0)  // report as empty
	arosPutBE32(buf, ADOS_ID_BYTES_PER_BLOCK, 512)
	arosPutBE32(buf, ADOS_ID_DISK_TYPE, ADOS_ID_DOS_DISK)
	arosPutBE32(buf, ADOS_ID_VOLUME_NODE, 0) // filled by handler
	arosPutBE32(buf, ADOS_ID_IN_USE, ADOS_DOSTRUE)
	if err := WriteGuestBytes(d.bus, infoPtr, 0, buf); err != nil {
		d.res2 = ADOS_ERROR_OBJECT_TOO_LARGE
		return
	}

	d.res1 = ADOS_DOSTRUE
}

// --- Helpers ---

// readString reads a null-terminated string from guest memory.
func (d *ArosDOSDevice) readString(addr uint32) string {
	if addr == 0 {
		return ""
	}
	var buf []byte
	for i := 0; i < 256; i++ {
		b := d.bus.Read8(addr + uint32(i))
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}

// writeBE32 writes a 32-bit value to guest memory in big-endian byte order.
func (d *ArosDOSDevice) writeBE32(addr uint32, value uint32) {
	d.bus.Write8(addr, byte(value>>24))
	d.bus.Write8(addr+1, byte(value>>16))
	d.bus.Write8(addr+2, byte(value>>8))
	d.bus.Write8(addr+3, byte(value))
}

// writeBE16 writes a 16-bit value to guest memory in big-endian byte order.
func (d *ArosDOSDevice) writeBE16(addr uint32, value uint16) {
	d.bus.Write8(addr, byte(value>>8))
	d.bus.Write8(addr+1, byte(value))
}

func arosPutBE32(buf []byte, off int, value uint32) {
	binary.BigEndian.PutUint32(buf[off:off+4], value)
}

func arosPutBE16(buf []byte, off int, value uint16) {
	binary.BigEndian.PutUint16(buf[off:off+2], value)
}

func arosPutCString(buf []byte, off int, s string, maxLen int) int {
	n := len(s)
	if n > maxLen-1 {
		n = maxLen - 1
	}
	copy(buf[off:off+n], s[:n])
	buf[off+n] = 0
	return n + 1
}

func arosPutBSTR(buf []byte, off int, s string, maxLen int) {
	n := len(s)
	if n > maxLen-1 {
		n = maxLen - 1
	}
	buf[off] = byte(n)
	copy(buf[off+1:off+1+n], s[:n])
}

// writeString writes a null-terminated string to guest memory.
func (d *ArosDOSDevice) writeString(addr uint32, s string, maxLen int) {
	n := len(s)
	if n > maxLen-1 {
		n = maxLen - 1
	}
	for i := 0; i < n; i++ {
		d.bus.Write8(addr+uint32(i), s[i])
	}
	d.bus.Write8(addr+uint32(n), 0) // null terminator
}

// writeBSTR writes a BSTR (length-prefixed string) to guest memory.
func (d *ArosDOSDevice) writeBSTR(addr uint32, s string, maxLen int) {
	n := len(s)
	if n > maxLen-1 {
		n = maxLen - 1
	}
	d.bus.Write8(addr, byte(n))
	for i := 0; i < n; i++ {
		d.bus.Write8(addr+1+uint32(i), s[i])
	}
}

func (d *ArosDOSDevice) resolveOpenReadDOSPath(parentHost, amigaName string) (string, resolveReason) {
	base, leaf := d.normalizeDOSPath(parentHost, amigaName)
	return d.resolveOpenReadPath(base, leaf)
}

func (d *ArosDOSDevice) resolveSourceDOSPath(parentHost, amigaName string) (string, resolveReason) {
	base, leaf := d.normalizeDOSPath(parentHost, amigaName)
	return d.resolveSourcePath(base, leaf)
}

func (d *ArosDOSDevice) resolveUpdateDOSPath(parentHost, amigaName string) (string, resolveReason) {
	base, leaf := d.normalizeDOSPath(parentHost, amigaName)
	return d.resolveUpdatePath(base, leaf)
}

func (d *ArosDOSDevice) resolveCreateDOSPath(parentHost, amigaName string) (string, resolveReason) {
	base, leaf := d.normalizeDOSPath(parentHost, amigaName)
	return d.resolveCreatePath(base, leaf)
}

func (d *ArosDOSDevice) normalizeDOSPath(parentHost string, amigaName string) (string, string) {
	if amigaName == "" {
		return parentHost, ""
	}
	name := amigaName

	for strings.HasPrefix(name, "/") {
		name = name[1:]
		parentHost = filepath.Dir(parentHost)
		if !d.containsPath(parentHost) {
			parentHost = d.hostRoot
		}
	}

	if idx := strings.Index(name, ":"); idx >= 0 {
		name = name[idx+1:]
		parentHost = d.hostRoot
	}

	name = strings.ReplaceAll(name, "/", string(filepath.Separator))
	return parentHost, name
}

func (d *ArosDOSDevice) resolveOpenReadPath(parentHost string, leafName string) (string, resolveReason) {
	if filepath.IsAbs(leafName) {
		return "", resolveNotFound
	}
	if leafName == "" {
		return parentHost, resolveOK
	}
	candidate := d.caseInsensitiveResolve(parentHost, leafName)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", resolveNotFound
	}
	resolved = filepath.Clean(resolved)
	if !d.containsPath(resolved) {
		return "", resolveNotFound
	}
	return resolved, resolveOK
}

func (d *ArosDOSDevice) resolveSourcePath(parentHost string, leafName string) (string, resolveReason) {
	hostPath, info, reason := d.resolveLeafLstat(parentHost, leafName)
	if reason != resolveOK {
		return "", reason
	}
	mode := info.Mode()
	if mode.IsRegular() || mode.IsDir() || mode&os.ModeSymlink != 0 {
		return hostPath, resolveOK
	}
	return "", resolveWrongType
}

func (d *ArosDOSDevice) resolveUpdatePath(parentHost string, leafName string) (string, resolveReason) {
	hostPath, info, reason := d.resolveLeafLstat(parentHost, leafName)
	if reason != resolveOK {
		return "", reason
	}
	if info.Mode().IsRegular() {
		return hostPath, resolveOK
	}
	return "", resolveWrongType
}

func (d *ArosDOSDevice) resolveCreatePath(parentHost string, leafName string) (string, resolveReason) {
	if filepath.IsAbs(leafName) || leafName == "" {
		return "", resolveNotFound
	}
	hostPath, info, reason := d.resolveLeafLstat(parentHost, leafName)
	if reason == resolveNotFound {
		hostPath, reason = d.resolveMissingLeafPath(parentHost, leafName)
		if reason == resolveOK {
			return hostPath, resolveOK
		}
		return "", reason
	}
	if reason != resolveOK {
		return "", reason
	}
	if info.Mode().IsRegular() {
		return hostPath, resolveExists
	}
	return hostPath, resolveWrongType
}

func (d *ArosDOSDevice) resolveLeafLstat(parentHost string, leafName string) (string, fs.FileInfo, resolveReason) {
	if filepath.IsAbs(leafName) || leafName == "" {
		return "", nil, resolveNotFound
	}
	hostPath, reason := d.resolveMissingLeafPath(parentHost, leafName)
	if reason != resolveOK {
		return "", nil, reason
	}
	info, err := os.Lstat(hostPath)
	if err != nil {
		return "", nil, resolveNotFound
	}
	return hostPath, info, resolveOK
}

func (d *ArosDOSDevice) resolveMissingLeafPath(parentHost string, leafName string) (string, resolveReason) {
	if filepath.IsAbs(leafName) || leafName == "" {
		return "", resolveNotFound
	}
	candidate := filepath.Join(parentHost, leafName)
	dir, base := filepath.Split(candidate)
	if base == "" {
		return "", resolveNotFound
	}
	dir = d.caseInsensitiveResolve(d.hostRoot, mustRelOrEmpty(d.hostRoot, dir))
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", resolveNotFound
	}
	resolvedDir = filepath.Clean(resolvedDir)
	if !d.containsPath(resolvedDir) {
		return "", resolveNotFound
	}
	return filepath.Join(resolvedDir, base), resolveOK
}

func mustRelOrEmpty(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return ""
	}
	return rel
}

func (d *ArosDOSDevice) containsPath(path string) bool {
	rel, err := filepath.Rel(d.hostRoot, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolvePath joins parent + relative name and sanitizes.
// AmigaDOS uses "/" for parent and "/" as path separator.
func (d *ArosDOSDevice) resolvePath(parentHost string, amigaName string) (string, bool) {
	host, reason := d.resolveOpenReadDOSPath(parentHost, amigaName)
	return host, reason == resolveOK
}

// caseInsensitiveResolve walks path components case-insensitively from base.
// Uses a cached directory listing to avoid repeated os.ReadDir calls.
func (d *ArosDOSDevice) caseInsensitiveResolve(base string, relPath string) string {
	components := strings.Split(relPath, string(filepath.Separator))
	current := base

	for _, comp := range components {
		if comp == "" || comp == "." {
			continue
		}

		// Try exact match first (fast path, no syscall for cached negative)
		exact := filepath.Join(current, comp)
		if _, err := os.Stat(exact); err == nil {
			current = exact
			continue
		}

		// Look up in cache
		actual := d.resolveNameInDir(current, comp)
		current = filepath.Join(current, actual)
	}

	return current
}

// resolveNameInDir returns the actual on-disk name matching comp case-insensitively.
func (d *ArosDOSDevice) resolveNameInDir(dir string, comp string) string {
	lowerComp := strings.ToLower(comp)

	cache, ok := d.dirNameCache[dir]
	if !ok {
		// Build cache for this directory
		cache = make(map[string]string)
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				cache[strings.ToLower(e.Name())] = e.Name()
			}
		}
		d.dirNameCache[dir] = cache
	}

	if actual, found := cache[lowerComp]; found {
		return actual
	}
	return comp // not found — use as-is (for create operations)
}

func (d *ArosDOSDevice) invalidatePathCaches(hostPath string) {
	if hostPath == "" {
		return
	}
	d.invalidateDirCache(filepath.Dir(hostPath))
	if info, err := os.Stat(hostPath); err == nil && info.IsDir() {
		d.invalidateDirCache(hostPath)
	}
}

func (d *ArosDOSDevice) invalidateHandleCachesForPath(hostPath string) {
	for _, h := range d.handles {
		if h.hostPath == hostPath {
			h.clearCache()
		}
	}
}

func (d *ArosDOSDevice) invalidateDirCache(dir string) {
	if dir == "" || dir == "." {
		return
	}
	delete(d.dirNameCache, dir)
	for _, lock := range d.locks {
		if lock.hostPath == dir {
			lock.dirEntries = nil
			lock.dirIdx = 0
		}
	}
}

// fillFIB fills a FileInfoBlock structure in guest memory.
func (d *ArosDOSDevice) fillFIB(fibPtr uint32, info fs.FileInfo, hostPath string) {
	buf := d.buildFIB(info, hostPath)
	_ = WriteGuestBytes(d.bus, fibPtr, 0, buf)
}

func (d *ArosDOSDevice) buildFIB(info fs.FileInfo, hostPath string) []byte {
	buf := make([]byte, ADOS_FIB_TOTAL_SIZE)
	arosPutBE32(buf, ADOS_FIB_DISK_KEY, simpleHash(hostPath))

	isRoot := hostPath == d.hostRoot
	var entryType uint32
	if isRoot {
		entryType = ADOS_ST_ROOT
	} else if info.IsDir() {
		entryType = ADOS_ST_USERDIR
	} else {
		entryType = ADOS_ST_FILE
	}
	arosPutBE32(buf, ADOS_FIB_DIR_ENTRY_TYPE, entryType)
	arosPutBE32(buf, ADOS_FIB_ENTRY_TYPE, entryType)

	var name string
	if isRoot {
		name = "IE"
	} else {
		name = info.Name()
		if name == "" || name == "." {
			name = "IE"
		}
	}
	arosPutBSTR(buf, ADOS_FIB_FILE_NAME, name, 108)

	prot := d.detectProtection(info, hostPath)
	arosPutBE32(buf, ADOS_FIB_PROTECTION, prot)

	size := info.Size()
	arosPutBE32(buf, ADOS_FIB_SIZE, uint32(size))

	blocks := (size + 511) / 512
	if blocks == 0 && !info.IsDir() {
		blocks = 1
	}
	arosPutBE32(buf, ADOS_FIB_NUM_BLOCKS, uint32(blocks))

	fillDateStampBytes(buf[ADOS_FIB_DATE:], info.ModTime())

	return buf
}

// fillDateStamp writes an AmigaDOS DateStamp at the given address.
// DateStamp: 3 LONGs — days since 1978-01-01, minutes past midnight, ticks (1/50s).
func (d *ArosDOSDevice) fillDateStamp(addr uint32, t time.Time) {
	var buf [12]byte
	fillDateStampBytes(buf[:], t)
	_ = WriteGuestBytes(d.bus, addr, 0, buf[:])
}

func fillDateStampBytes(buf []byte, t time.Time) {
	amigaEpoch := time.Date(1978, 1, 1, 0, 0, 0, 0, time.UTC)
	duration := t.Sub(amigaEpoch)
	if duration < 0 {
		arosPutBE32(buf, 0, 0)
		arosPutBE32(buf, 4, 0)
		arosPutBE32(buf, 8, 0)
		return
	}

	days := int(duration.Hours() / 24)
	remaining := duration - time.Duration(days)*24*time.Hour
	minutes := int(remaining.Minutes())
	remaining -= time.Duration(minutes) * time.Minute
	ticks := int(remaining.Seconds() * 50) // 50 ticks per second

	arosPutBE32(buf, 0, uint32(days))
	arosPutBE32(buf, 4, uint32(minutes))
	arosPutBE32(buf, 8, uint32(ticks))
}

// detectProtection determines AmigaDOS protection bits for a host file.
// Maps host Unix execute permission to Amiga executable status. Files in
// the S/ directory also get FIBF_SCRIPT so the Shell can run them via Execute.
// AmigaDOS uses negative logic for RWED: bit set = permission DENIED.
func (d *ArosDOSDevice) detectProtection(info fs.FileInfo, hostPath string) uint32 {
	if info.IsDir() {
		return 0
	}

	var prot uint32
	perm := info.Mode().Perm()
	if perm&0o400 == 0 {
		prot |= ADOS_FIBF_READ
	}
	if perm&0o200 == 0 {
		prot |= ADOS_FIBF_WRITE
		prot |= ADOS_FIBF_DELETE
	}
	// If the host file lacks user-execute permission, deny Amiga execute.
	if perm&0o100 == 0 {
		prot |= ADOS_FIBF_EXECUTE
	}

	// Files under the S/ directory get the SCRIPT bit so Shell runs them via Execute.
	rel, err := filepath.Rel(d.hostRoot, hostPath)
	if err == nil {
		first, _, _ := strings.Cut(rel, string(filepath.Separator))
		if strings.EqualFold(first, "S") {
			prot |= ADOS_FIBF_SCRIPT
		}
	}

	return prot
}

func mapWriteErr(err error) uint32 {
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return ADOS_ERROR_DISK_FULL
	case errors.Is(err, syscall.EROFS):
		return ADOS_ERROR_DISK_WRITE_PROTECTED
	case errors.Is(err, syscall.EBUSY):
		return ADOS_ERROR_OBJECT_IN_USE
	case os.IsPermission(err):
		return ADOS_ERROR_DISK_WRITE_PROTECTED
	default:
		return ADOS_ERROR_WRITE_PROTECTED
	}
}

func mapDeleteErr(err error) uint32 {
	switch {
	case os.IsNotExist(err):
		return ADOS_ERROR_OBJECT_NOT_FOUND
	case errors.Is(err, syscall.EBUSY):
		return ADOS_ERROR_OBJECT_IN_USE
	case errors.Is(err, syscall.EROFS):
		return ADOS_ERROR_DISK_WRITE_PROTECTED
	case os.IsPermission(err):
		return ADOS_ERROR_DELETE_PROTECTED
	default:
		return ADOS_ERROR_DELETE_PROTECTED
	}
}

func mapRenameErr(err error) uint32 {
	switch {
	case os.IsNotExist(err):
		return ADOS_ERROR_OBJECT_NOT_FOUND
	case os.IsExist(err):
		return ADOS_ERROR_OBJECT_EXISTS
	case errors.Is(err, syscall.EBUSY):
		return ADOS_ERROR_OBJECT_IN_USE
	case errors.Is(err, syscall.ENOSPC):
		return ADOS_ERROR_DISK_FULL
	case errors.Is(err, syscall.EROFS):
		return ADOS_ERROR_DISK_WRITE_PROTECTED
	case os.IsPermission(err):
		return ADOS_ERROR_DISK_WRITE_PROTECTED
	default:
		return ADOS_ERROR_OBJECT_NOT_FOUND
	}
}

// Close releases all open handles and locks.
func (d *ArosDOSDevice) Close() {
	for k, h := range d.handles {
		h.clearCache()
		h.file.Close()
		delete(d.handles, k)
	}
	for k := range d.locks {
		if k != 0 { // preserve root lock
			delete(d.locks, k)
		}
	}
}

func simpleHash(s string) uint32 {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}
