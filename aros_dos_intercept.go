package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ArosDOSDevice provides host filesystem access to AROS via MMIO.
// The AROS packet handler (iehandler) translates AmigaDOS packets
// to MMIO register writes. Writing the command register triggers
// synchronous execution on the Go side.
type ArosDOSDevice struct {
	bus      *MachineBus
	hostRoot string

	// Lock management: key → lock state
	locks    map[uint32]*adosLock
	nextLock uint32

	// File handle management: key → open file
	handles     map[uint32]*os.File
	handleNames map[uint32]string // key → original name for diagnostics
	handleRead  map[uint32]bool   // key → whether first read has occurred
	nextHandle  uint32

	// MMIO register shadow state
	arg1 uint32
	arg2 uint32
	arg3 uint32
	arg4 uint32
	res1 uint32
	res2 uint32

	debugTrace bool
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
		bus:         bus,
		hostRoot:    absPath,
		locks:       make(map[uint32]*adosLock),
		nextLock:    1, // 0 means root/no-lock
		handles:     make(map[uint32]*os.File),
		handleNames: make(map[uint32]string),
		handleRead:  make(map[uint32]bool),
		nextHandle:  1,
		debugTrace:  false,
	}

	// Pre-create root lock at key 0
	d.locks[0] = &adosLock{
		hostPath: absPath,
		isDir:    true,
		mode:     -2, // SHARED_LOCK
	}

	return d, nil
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
		d.res1 = ADOS_DOSTRUE // no-op: iehandler doesn't forward protect bits
	case ADOS_CMD_EXAMINE_FH:
		d.cmdExamineFH()
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
	hostPath, pathOK := d.resolvePath(parent.hostPath, name)
	if !pathOK {
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

func (d *ArosDOSDevice) cmdExamineFH() {
	handleKey := d.arg1
	fibPtr := d.arg2

	f, ok := d.handles[handleKey]
	if !ok {
		if d.debugTrace {
			fmt.Printf("[ADOS] EXAMINE_FH handle=%d → INVALID_LOCK\n", handleKey)
		}
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}

	info, err := f.Stat()
	if err != nil {
		if d.debugTrace {
			fmt.Printf("[ADOS] EXAMINE_FH %q → stat error: %v\n", f.Name(), err)
		}
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	d.fillFIB(fibPtr, info, f.Name())
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
	hostPath, pathOK := d.resolvePath(parent.hostPath, name)
	if !pathOK {
		if d.debugTrace {
			fmt.Printf("[ADOS] FINDINPUT %q (parent=%d) → path resolve failed\n", name, parentKey)
		}
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	f, err := os.Open(hostPath)
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
	d.handles[key] = f
	d.handleNames[key] = name
	d.handleRead[key] = false

	if d.debugTrace {
		fmt.Printf("[ADOS] FINDINPUT %q (parent=%d, path=%q) → handle=%d\n", name, parentKey, hostPath, key)
	}

	d.res1 = key
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
	hostPath, pathOK := d.resolvePath(parent.hostPath, name)
	if !pathOK {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	f, err := os.Create(hostPath)
	if err != nil {
		d.res2 = ADOS_ERROR_WRITE_PROTECTED
		return
	}

	key := d.nextHandle
	d.nextHandle++
	d.handles[key] = f
	d.handleNames[key] = name
	d.handleRead[key] = false

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
	hostPath, pathOK := d.resolvePath(parent.hostPath, name)
	if !pathOK {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	f, err := os.OpenFile(hostPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		d.res2 = ADOS_ERROR_WRITE_PROTECTED
		return
	}

	key := d.nextHandle
	d.nextHandle++
	d.handles[key] = f
	d.handleNames[key] = name
	d.handleRead[key] = false
	d.res1 = key
}

func (d *ArosDOSDevice) cmdRead() {
	handleKey := d.arg1
	bufPtr := d.arg2
	length := d.arg3

	f, ok := d.handles[handleKey]
	if !ok {
		d.res1 = 0xFFFFFFFF // -1
		d.res2 = ADOS_ERROR_INVALID_LOCK
		if d.debugTrace {
			fmt.Printf("[ADOS] READ handle=%d → INVALID_LOCK\n", handleKey)
		}
		return
	}

	buf := make([]byte, length)
	n, err := f.Read(buf)
	if n > 0 {
		for i := 0; i < n; i++ {
			d.bus.Write8(bufPtr+uint32(i), buf[i])
		}
	}

	if err != nil && n == 0 {
		d.res1 = 0 // EOF returns 0, not error
	} else {
		d.res1 = uint32(n)
	}

	// Log first read per handle — shows magic bytes for LoadSeg diagnosis
	if d.debugTrace && n > 0 && !d.handleRead[handleKey] {
		d.handleRead[handleKey] = true
		magic := ""
		if n >= 4 {
			magic = fmt.Sprintf(" magic=%02x%02x%02x%02x", buf[0], buf[1], buf[2], buf[3])
		}
		fmt.Printf("[ADOS] FIRST_READ handle=%d %q len=%d n=%d%s\n",
			handleKey, d.handleNames[handleKey], length, n, magic)
	}
}

func (d *ArosDOSDevice) cmdWrite() {
	handleKey := d.arg1
	bufPtr := d.arg2
	length := d.arg3

	f, ok := d.handles[handleKey]
	if !ok {
		d.res1 = 0xFFFFFFFF // -1
		d.res2 = ADOS_ERROR_INVALID_LOCK
		return
	}

	buf := make([]byte, length)
	for i := uint32(0); i < length; i++ {
		buf[i] = d.bus.Read8(bufPtr + i)
	}

	n, err := f.Write(buf)
	if err != nil {
		d.res1 = 0xFFFFFFFF // -1
		d.res2 = ADOS_ERROR_WRITE_PROTECTED
		return
	}
	d.res1 = uint32(n)
}

func (d *ArosDOSDevice) cmdSeek() {
	handleKey := d.arg1
	offset := int64(int32(d.arg2)) // sign-extend
	mode := d.arg3

	f, ok := d.handles[handleKey]
	if !ok {
		d.res1 = 0xFFFFFFFF // -1
		d.res2 = ADOS_ERROR_INVALID_LOCK
		if d.debugTrace {
			fmt.Printf("[ADOS] SEEK handle=%d → INVALID_LOCK\n", handleKey)
		}
		return
	}

	// Get current position before seeking
	oldPos, err := f.Seek(0, 1) // SEEK_CUR
	if err != nil {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_SEEK_ERROR
		if d.debugTrace {
			fmt.Printf("[ADOS] SEEK handle=%d → SEEK_ERROR (get cur pos): %v\n", handleKey, err)
		}
		return
	}

	var whence int
	switch mode {
	case ADOS_OFFSET_BEGINNING:
		whence = 0 // io.SeekStart
	case ADOS_OFFSET_CURRENT:
		whence = 1 // io.SeekCurrent
	case ADOS_OFFSET_END:
		whence = 2 // io.SeekEnd
	default:
		whence = 1
	}

	newPos, err := f.Seek(offset, whence)
	if err != nil {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_SEEK_ERROR
		if d.debugTrace {
			fmt.Printf("[ADOS] SEEK handle=%d offset=%d mode=%d(whence=%d) → SEEK_ERROR: %v\n",
				handleKey, offset, mode, whence, err)
		}
		return
	}

	d.res1 = uint32(oldPos)

	if d.debugTrace {
		fmt.Printf("[ADOS] SEEK handle=%d offset=%d mode=0x%X(whence=%d) oldPos=%d → newPos=%d\n",
			handleKey, offset, mode, whence, oldPos, newPos)
	}
}

func (d *ArosDOSDevice) cmdClose() {
	handleKey := d.arg1
	if f, ok := d.handles[handleKey]; ok {
		f.Close()
		delete(d.handles, handleKey)
		delete(d.handleNames, handleKey)
		delete(d.handleRead, handleKey)
	}
	d.res1 = ADOS_DOSTRUE
}

func (d *ArosDOSDevice) cmdSetFileSize() {
	handleKey := d.arg1
	newSize := int64(int32(d.arg2))
	mode := d.arg3

	f, ok := d.handles[handleKey]
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
		pos, _ := f.Seek(0, 1)
		absSize = pos + newSize
	case ADOS_OFFSET_END:
		info, _ := f.Stat()
		absSize = info.Size() + newSize
	default:
		absSize = newSize
	}

	if err := f.Truncate(absSize); err != nil {
		d.res1 = 0xFFFFFFFF
		d.res2 = ADOS_ERROR_WRITE_PROTECTED
		return
	}
	d.res1 = uint32(absSize)
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
	hostPath, pathOK := d.resolvePath(parent.hostPath, name)
	if !pathOK {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	err := os.Remove(hostPath)
	if err != nil {
		if os.IsNotExist(err) {
			d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		} else {
			d.res2 = ADOS_ERROR_DELETE_PROTECTED
		}
		return
	}
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
	hostPath, pathOK := d.resolvePath(parent.hostPath, name)
	if !pathOK {
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
	srcPath, srcOK := d.resolvePath(srcParent.hostPath, srcName)
	dstPath, dstOK := d.resolvePath(dstParent.hostPath, dstName)
	if !srcOK || !dstOK {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		d.res2 = ADOS_ERROR_OBJECT_NOT_FOUND
		return
	}
	d.res1 = ADOS_DOSTRUE
}

func (d *ArosDOSDevice) cmdDiskInfo() {
	infoPtr := d.arg1

	// Fill InfoData structure in guest memory (big-endian)
	d.writeBE32(infoPtr+ADOS_ID_NUM_SOFT_ERRORS, 0)
	d.writeBE32(infoPtr+ADOS_ID_UNIT_NUMBER, 0)
	d.writeBE32(infoPtr+ADOS_ID_DISK_STATE, ADOS_ID_VALIDATED)
	d.writeBE32(infoPtr+ADOS_ID_NUM_BLOCKS, 1048576) // ~512MB in 512-byte blocks
	d.writeBE32(infoPtr+ADOS_ID_NUM_BLOCKS_USED, 0)  // report as empty
	d.writeBE32(infoPtr+ADOS_ID_BYTES_PER_BLOCK, 512)
	d.writeBE32(infoPtr+ADOS_ID_DISK_TYPE, ADOS_ID_DOS_DISK)
	d.writeBE32(infoPtr+ADOS_ID_VOLUME_NODE, 0) // filled by handler
	d.writeBE32(infoPtr+ADOS_ID_IN_USE, ADOS_DOSTRUE)

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

// resolvePath joins parent + relative name and sanitizes.
// AmigaDOS uses "/" for parent and "/" as path separator.
func (d *ArosDOSDevice) resolvePath(parentHost string, amigaName string) (string, bool) {
	if amigaName == "" {
		return parentHost, true
	}

	// Convert Amiga path separators: "/" at start means parent
	name := amigaName
	base := parentHost

	// Handle leading "/" (parent directory traversal in AmigaDOS)
	for strings.HasPrefix(name, "/") {
		base = filepath.Dir(base)
		name = name[1:]
		// Don't escape hostRoot
		if !strings.HasPrefix(base, d.hostRoot) {
			base = d.hostRoot
		}
	}

	// Strip volume name prefix (e.g., "IE:" or "IE0:")
	if idx := strings.Index(name, ":"); idx >= 0 {
		name = name[idx+1:]
		base = d.hostRoot // absolute from root after colon
	}

	// Convert remaining "/" to host separator
	name = strings.ReplaceAll(name, "/", string(filepath.Separator))

	if name == "" {
		return base, true
	}

	fullPath := filepath.Join(base, name)
	fullPath = filepath.Clean(fullPath)

	// Security check: must stay within hostRoot
	if !strings.HasPrefix(fullPath, d.hostRoot) {
		return "", false
	}

	return fullPath, true
}

// fillFIB fills a FileInfoBlock structure in guest memory.
func (d *ArosDOSDevice) fillFIB(fibPtr uint32, info fs.FileInfo, hostPath string) {
	// Clear the FIB first
	for i := uint32(0); i < ADOS_FIB_TOTAL_SIZE; i++ {
		d.bus.Write8(fibPtr+i, 0)
	}

	// fib_DiskKey (internal use, set to a hash of the path)
	d.writeBE32(fibPtr+ADOS_FIB_DISK_KEY, simpleHash(hostPath))

	// fib_DirEntryType / fib_EntryType
	isRoot := hostPath == d.hostRoot
	var entryType uint32
	if isRoot {
		entryType = ADOS_ST_ROOT
	} else if info.IsDir() {
		entryType = ADOS_ST_USERDIR
	} else {
		entryType = ADOS_ST_FILE
	}
	d.writeBE32(fibPtr+ADOS_FIB_DIR_ENTRY_TYPE, entryType)
	d.writeBE32(fibPtr+ADOS_FIB_ENTRY_TYPE, entryType)

	// fib_FileName (BSTR: length byte + chars, max 107 chars)
	var name string
	if isRoot {
		// Root directory: use the volume name, not the host dir name
		name = "IE"
	} else {
		name = info.Name()
		if name == "" || name == "." {
			name = "IE"
		}
	}
	d.writeBSTR(fibPtr+ADOS_FIB_FILE_NAME, name, 108)

	// fib_Protection — files in S/ get FIBF_SCRIPT so Shell runs them
	// via Execute. All other files get prot=0 (no special bits).
	prot := d.detectProtection(info, hostPath)
	d.writeBE32(fibPtr+ADOS_FIB_PROTECTION, prot)

	// fib_Size
	size := info.Size()
	d.writeBE32(fibPtr+ADOS_FIB_SIZE, uint32(size))

	// fib_NumBlocks
	blocks := (size + 511) / 512
	if blocks == 0 && !info.IsDir() {
		blocks = 1
	}
	d.writeBE32(fibPtr+ADOS_FIB_NUM_BLOCKS, uint32(blocks))

	// fib_Date (DateStamp: days since 1978-01-01, minutes, ticks)
	d.fillDateStamp(fibPtr+ADOS_FIB_DATE, info.ModTime())

	// fib_Comment (empty BSTR)
	d.bus.Write8(fibPtr+ADOS_FIB_COMMENT, 0)
}

// fillDateStamp writes an AmigaDOS DateStamp at the given address.
// DateStamp: 3 LONGs — days since 1978-01-01, minutes past midnight, ticks (1/50s).
func (d *ArosDOSDevice) fillDateStamp(addr uint32, t time.Time) {
	amigaEpoch := time.Date(1978, 1, 1, 0, 0, 0, 0, time.UTC)
	duration := t.Sub(amigaEpoch)
	if duration < 0 {
		d.writeBE32(addr, 0)
		d.writeBE32(addr+4, 0)
		d.writeBE32(addr+8, 0)
		return
	}

	days := int(duration.Hours() / 24)
	remaining := duration - time.Duration(days)*24*time.Hour
	minutes := int(remaining.Minutes())
	remaining -= time.Duration(minutes) * time.Minute
	ticks := int(remaining.Seconds() * 50) // 50 ticks per second

	d.writeBE32(addr, uint32(days))
	d.writeBE32(addr+4, uint32(minutes))
	d.writeBE32(addr+8, uint32(ticks))
}

// detectProtection determines AmigaDOS protection bits for a host file.
// Files in the S/ directory get FIBF_SCRIPT so the Shell can run them via Execute.
// All other non-directory files get protection=0 (no special bits).
func (d *ArosDOSDevice) detectProtection(info fs.FileInfo, hostPath string) uint32 {
	if info.IsDir() {
		return 0
	}

	// Only files under the S/ directory should be marked as scripts.
	// Use case-insensitive check since the host tree may use lowercase s/.
	rel, err := filepath.Rel(d.hostRoot, hostPath)
	if err == nil {
		first, _, _ := strings.Cut(rel, string(filepath.Separator))
		if strings.EqualFold(first, "S") {
			return ADOS_FIBF_SCRIPT
		}
	}

	return 0
}

// Close releases all open handles and locks.
func (d *ArosDOSDevice) Close() {
	for k, f := range d.handles {
		f.Close()
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
