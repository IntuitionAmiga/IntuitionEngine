package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	bootHostFSKernDataBase  = 0x08D000 // KERN_DATA_BASE (sdk/include/iexec.inc)
	bootHostFSKDCurrentTask = 0        // KD_CURRENT_TASK
)

// BootstrapHostFSDevice is a narrow read-only hostfs bridge for the ROM bootstrap path.
type BootstrapHostFSDevice struct {
	bus *MachineBus

	hostRoot  string
	available bool

	nextHandle uint32
	handles    map[uint32]*os.File
	memHandles map[uint32]*bootstrapHostFSMemHandle
	specials   map[string][]byte

	arg1 uint32
	arg2 uint32
	arg3 uint32
	arg4 uint32
	res1 uint32
	res2 uint32
	err  uint32

	// testShortWriteLimit, when non-zero, caps the effective byte count
	// of BOOT_HOSTFS_WRITE so tests can verify DOS_WRITE accounting
	// honours the actual returned count (not the caller's request).
	testShortWriteLimit uint32
}

type bootstrapHostFSMemHandle struct {
	data []byte
	pos  uint32
}

func NewBootstrapHostFSDevice(bus *MachineBus, hostRoot string) *BootstrapHostFSDevice {
	d := &BootstrapHostFSDevice{
		bus:        bus,
		nextHandle: 1,
		handles:    make(map[uint32]*os.File),
		memHandles: make(map[uint32]*bootstrapHostFSMemHandle),
		specials:   make(map[string][]byte),
	}
	if hostRoot == "" {
		return d
	}
	resolved, err := filepath.EvalSymlinks(hostRoot)
	if err != nil {
		return d
	}
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return d
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return d
	}
	d.hostRoot = absPath
	d.available = true
	return d
}

// SetSpecialFile registers a special file with an independent copy of data.
// Callers commonly pass a slice of live VM memory that the guest may later
// overwrite; the copy insulates subsequent reads from those mutations.
func (d *BootstrapHostFSDevice) SetSpecialFile(rel string, data []byte) {
	if rel == "" {
		return
	}
	d.specials[specialFileKey(rel)] = append([]byte(nil), data...)
}

// SetSpecialFileLive registers a special file aliased to the provided slice
// without copying. Subsequent reads observe whatever the caller has written
// into the slice's backing array. A later SetSpecialFile for the same rel
// replaces the alias with an independent copy.
func (d *BootstrapHostFSDevice) SetSpecialFileLive(rel string, data []byte) {
	if rel == "" {
		return
	}
	d.specials[specialFileKey(rel)] = data
}

func (d *BootstrapHostFSDevice) DeleteSpecialFile(rel string) {
	if rel == "" {
		return
	}
	delete(d.specials, specialFileKey(rel))
}

func specialFileKey(rel string) string {
	return strings.ToUpper(filepath.ToSlash(filepath.Clean(rel)))
}

func (d *BootstrapHostFSDevice) HandleRead(addr uint32) uint32 {
	switch addr {
	case BOOT_HOSTFS_ARG1:
		return d.arg1
	case BOOT_HOSTFS_ARG2:
		return d.arg2
	case BOOT_HOSTFS_ARG3:
		return d.arg3
	case BOOT_HOSTFS_ARG4:
		return d.arg4
	case BOOT_HOSTFS_RES1:
		return d.res1
	case BOOT_HOSTFS_RES2:
		return d.res2
	case BOOT_HOSTFS_ERR:
		return d.err
	}
	return 0
}

func (d *BootstrapHostFSDevice) HandleWrite(addr uint32, val uint32) {
	switch addr {
	case BOOT_HOSTFS_ARG1:
		d.arg1 = val
	case BOOT_HOSTFS_ARG2:
		d.arg2 = val
	case BOOT_HOSTFS_ARG3:
		d.arg3 = val
	case BOOT_HOSTFS_ARG4:
		d.arg4 = val
	case BOOT_HOSTFS_CMD:
		d.dispatch(val)
	}
}

func (d *BootstrapHostFSDevice) dispatch(cmd uint32) {
	d.res1 = 0
	d.res2 = 0
	d.err = 0

	switch cmd {
	case BOOT_HOSTFS_DISCOVER:
		if !d.available {
			d.err = 4
			return
		}
		d.res1 = 1
	case BOOT_HOSTFS_OPEN:
		d.open()
	case BOOT_HOSTFS_READ:
		d.read()
	case BOOT_HOSTFS_CLOSE:
		d.close()
	case BOOT_HOSTFS_STAT:
		d.stat()
	case BOOT_HOSTFS_READDIR:
		d.readDir()
	case BOOT_HOSTFS_CREATE_WRITE:
		d.createWrite()
	case BOOT_HOSTFS_WRITE:
		d.write()
	default:
		d.err = 3
	}
}

// createWrite opens (or creates) a file for writing under the host root,
// truncating it. M15.3 writable SYS: overlay policy: writes are rejected
// for any path whose first component is "IOSSYS" (case-insensitive) so
// the embedded read-only system tree cannot be modified by the guest.
func (d *BootstrapHostFSDevice) createWrite() {
	if !d.available {
		d.err = 4
		return
	}
	rel := d.readCString(d.arg1, 255)
	if rel == "" {
		d.err = 3
		return
	}
	if relPathIsIOSSYS(rel) {
		d.err = 3 // read-only namespace
		return
	}
	hostPath, mkErr := d.resolveForCreate(rel)
	if mkErr != 0 {
		d.err = mkErr
		return
	}
	f, err := os.OpenFile(hostPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		d.err = mapHostErr(err)
		return
	}
	handle := d.nextHandle
	d.nextHandle++
	d.handles[handle] = f
	d.res1 = handle
}

// write appends bytes from the guest buffer to an open hostfs handle.
func (d *BootstrapHostFSDevice) write() {
	f := d.handles[d.arg1]
	if f == nil {
		d.err = 2
		return
	}
	if d.arg2 == 0 {
		d.err = 3
		return
	}
	effective := d.arg3
	if d.testShortWriteLimit > 0 && d.testShortWriteLimit < effective {
		effective = d.testShortWriteLimit
	}
	buf := make([]byte, effective)
	for i := uint32(0); i < effective; i++ {
		b, ok := d.readGuest8(d.arg2 + i)
		if !ok {
			d.err = 5
			return
		}
		buf[i] = b
	}
	n, err := f.Write(buf)
	if err != nil {
		d.err = mapHostErr(err)
		return
	}
	d.res1 = uint32(n)
}

// resolveForCreate walks the relative path, creating any missing parent
// directories (so that writes through `C:Foo` can land in a fresh
// hostRoot without pre-populated `C/` subdir). Returns the final host path.
//
// Security: any "." / ".." segment is rejected outright. Without this
// guard a guest path like "../outside/file" would cause the parent-dir
// auto-create loop to leave `fullPath` as the parent of `hostRoot` and
// then let `os.OpenFile(O_CREATE|O_TRUNC)` truncate or create files
// outside the sandbox. Per-component resolution is also NOFOLLOW: any
// symlink encountered while resolving existing parents or an existing
// leaf is rejected outright even if it points back inside hostRoot, so
// a pre-planted symlink in the writable SYS overlay cannot be used to
// pivot writes onto read-only assets.
func (d *BootstrapHostFSDevice) resolveForCreate(rel string) (string, uint32) {
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == "." || rel == string(filepath.Separator) || rel == "" {
		return "", 3
	}
	if filepath.IsAbs(rel) {
		return "", 5
	}
	segs := strings.Split(rel, string(filepath.Separator))
	if len(segs) == 0 {
		return "", 3
	}
	// Reject ".." (and stray ".") anywhere in the relative path — these
	// are the only way `filepath.Clean` leaves path-traversal tokens in
	// the result, and the parent-dir auto-create loop would otherwise
	// walk out of the sandboxed hostRoot.
	for _, seg := range segs {
		if seg == ".." || seg == "." {
			return "", 5
		}
	}
	fullPath := d.hostRoot
	// Create (or resolve) parent directories case-insensitively.
	for i := 0; i < len(segs)-1; i++ {
		seg := segs[i]
		if seg == "" {
			continue
		}
		next, errCode := d.resolvePathSegmentCI(fullPath, seg)
		if errCode == 4 {
			candidate := filepath.Join(fullPath, seg)
			if err := os.MkdirAll(candidate, 0o755); err != nil {
				return "", mapHostErr(err)
			}
			next = candidate
		} else if errCode != 0 {
			return "", errCode
		}
		fullPath = next
	}
	leaf := segs[len(segs)-1]
	if leaf == "" {
		return "", 3
	}
	// For the leaf, look for an existing case-insensitive match first so
	// that "c:Version" and "C:Version" target the same file.
	// Only errCode 4 (genuinely not-found) is safe to fall through to a
	// lexical join. Any other error — including errCode 5, which covers
	// an existing leaf that is a symlink — must propagate so the caller
	// can't escape the sandbox via a pre-planted symlink in the writable
	// SYS overlay.
	var finalPath string
	match, errCode := d.resolvePathSegmentCI(fullPath, leaf)
	switch errCode {
	case 0:
		finalPath = match
	case 4:
		finalPath = filepath.Join(fullPath, leaf)
	default:
		return "", errCode
	}
	// Belt-and-suspenders: confirm the final host path is actually under
	// hostRoot.
	absFinal, err := filepath.Abs(finalPath)
	if err != nil {
		return "", 3
	}
	ok, err := pathWithinRoot(d.hostRoot, absFinal)
	if err != nil || !ok {
		return "", 5
	}
	return finalPath, 0
}

// relPathIsIOSSYS reports whether the first path component of a
// forward-slash relative path is "IOSSYS" (case-insensitive).
func relPathIsIOSSYS(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "" || clean == "/" {
		return false
	}
	first := clean
	if slash := strings.IndexByte(first, '/'); slash >= 0 {
		first = first[:slash]
	}
	return strings.EqualFold(first, "IOSSYS")
}

func (d *BootstrapHostFSDevice) open() {
	rel := d.readCString(d.arg1, 255)
	if data, ok := d.specialFile(rel); ok {
		handle := d.nextHandle
		d.nextHandle++
		d.memHandles[handle] = &bootstrapHostFSMemHandle{data: data}
		d.res1 = handle
		return
	}
	hostPath, errCode := d.resolveExistingPath(d.arg1)
	if errCode != 0 {
		d.err = errCode
		return
	}
	info, err := os.Stat(hostPath)
	if err != nil {
		d.err = 4
		return
	}
	if info.IsDir() {
		d.err = 3
		return
	}
	f, err := os.Open(hostPath)
	if err != nil {
		d.err = mapHostErr(err)
		return
	}
	handle := d.nextHandle
	d.nextHandle++
	d.handles[handle] = f
	d.res1 = handle
}

func (d *BootstrapHostFSDevice) read() {
	if mh := d.memHandles[d.arg1]; mh != nil {
		if d.arg2 == 0 {
			d.err = 3
			return
		}
		if mh.pos >= uint32(len(mh.data)) {
			d.res1 = 0
			return
		}
		n := d.arg3
		remain := uint32(len(mh.data)) - mh.pos
		if n > remain {
			n = remain
		}
		for i := uint32(0); i < n; i++ {
			if !d.writeGuest8(d.arg2+i, mh.data[mh.pos+i]) {
				d.err = 5
				return
			}
		}
		mh.pos += n
		d.res1 = n
		return
	}
	f := d.handles[d.arg1]
	if f == nil {
		d.err = 2
		return
	}
	if d.arg2 == 0 {
		d.err = 3
		return
	}
	buf := make([]byte, d.arg3)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		d.err = mapHostErr(err)
		return
	}
	for i := 0; i < n; i++ {
		if !d.writeGuest8(d.arg2+uint32(i), buf[i]) {
			d.err = 5
			return
		}
	}
	d.res1 = uint32(n)
}

func (d *BootstrapHostFSDevice) close() {
	if _, ok := d.memHandles[d.arg1]; ok {
		delete(d.memHandles, d.arg1)
		return
	}
	f := d.handles[d.arg1]
	if f == nil {
		d.err = 2
		return
	}
	_ = f.Close()
	delete(d.handles, d.arg1)
}

func (d *BootstrapHostFSDevice) stat() {
	rel := d.readCString(d.arg1, 255)
	if data, ok := d.specialFile(rel); ok {
		if d.arg2 != 0 {
			if !d.writeGuest64(d.arg2+BOOT_HOSTFS_STAT_SIZE_OFF, uint64(len(data))) {
				d.err = 5
				return
			}
			if !d.writeGuest64(d.arg2+BOOT_HOSTFS_STAT_KIND_OFF, uint64(BOOT_HOSTFS_KIND_FILE)) {
				d.err = 5
				return
			}
		}
		d.res1 = uint32(len(data))
		d.res2 = BOOT_HOSTFS_KIND_FILE
		return
	}
	hostPath, errCode := d.resolveExistingPath(d.arg1)
	if errCode != 0 {
		d.err = errCode
		return
	}
	info, err := os.Stat(hostPath)
	if err != nil {
		d.err = mapHostErr(err)
		return
	}
	if d.arg2 != 0 {
		if !d.writeGuest64(d.arg2+BOOT_HOSTFS_STAT_SIZE_OFF, uint64(info.Size())) {
			d.err = 5
			return
		}
		if !d.writeGuest64(d.arg2+BOOT_HOSTFS_STAT_KIND_OFF, uint64(hostFSKind(info))) {
			d.err = 5
			return
		}
	}
	// Mirror size/kind into the MMIO result registers so bootstrap callers
	// can avoid depending on a guest scratch buffer round-trip.
	d.res1 = uint32(info.Size())
	d.res2 = uint32(hostFSKind(info))
}

func (d *BootstrapHostFSDevice) readDir() {
	hostPath, errCode := d.resolveExistingPath(d.arg1)
	if errCode != 0 {
		d.err = errCode
		return
	}
	if d.arg3 == 0 {
		d.err = 3
		return
	}
	info, err := os.Stat(hostPath)
	if err != nil {
		d.err = mapHostErr(err)
		return
	}
	if !info.IsDir() {
		d.err = 3
		return
	}
	entries, err := os.ReadDir(hostPath)
	if err != nil {
		d.err = mapHostErr(err)
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	idx := int(d.arg2)
	if idx < 0 || idx >= len(entries) {
		d.err = 4
		return
	}
	entry := entries[idx]
	kind := BOOT_HOSTFS_KIND_FILE
	if entry.IsDir() {
		kind = BOOT_HOSTFS_KIND_DIR
	}
	if !d.writeGuest64(d.arg3+BOOT_HOSTFS_DIRENT_KIND_OFF, uint64(kind)) {
		d.err = 5
		return
	}
	if !d.writeGuestName(d.arg3+BOOT_HOSTFS_DIRENT_NAME_OFF, entry.Name()) {
		d.err = 5
		return
	}
	d.res1 = 1
	d.res2 = uint32(kind)
}

func (d *BootstrapHostFSDevice) resolveExistingPath(pathPtr uint32) (string, uint32) {
	if !d.available {
		return "", 4
	}
	rel := d.readCString(pathPtr, 255)
	if rel == "" {
		return "", 3
	}
	return d.resolveRelativePath(rel)
}

func (d *BootstrapHostFSDevice) specialFile(rel string) ([]byte, bool) {
	if rel == "" {
		return nil, false
	}
	data, ok := d.specials[specialFileKey(rel)]
	return data, ok
}

func (d *BootstrapHostFSDevice) resolveRelativePath(rel string) (string, uint32) {
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == "." || rel == string(filepath.Separator) || rel == "" {
		return d.hostRoot, 0
	}
	if filepath.IsAbs(rel) {
		return "", 5
	}
	fullPath := d.hostRoot
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if seg == "" || seg == "." {
			continue
		}
		next, errCode := d.resolvePathSegmentCI(fullPath, seg)
		if errCode != 0 {
			return "", errCode
		}
		fullPath = next
	}
	return fullPath, 0
}

func (d *BootstrapHostFSDevice) resolvePathSegmentCI(base string, want string) (string, uint32) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", mapHostErr(err)
	}

	var matched os.DirEntry
	for _, entry := range entries {
		if entry.Name() == want {
			matched = entry
			break
		}
	}
	if matched == nil {
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), want) {
				matched = entry
				break
			}
		}
	}
	if matched == nil {
		return "", 4
	}
	// DirEntry.Type() avoids the extra Lstat syscall when getdents
	// already carries d_type; it still falls back to Lstat on
	// filesystems that report DT_UNKNOWN.
	if matched.Type()&os.ModeSymlink != 0 {
		return "", 5
	}

	fullPath := filepath.Join(base, matched.Name())
	lexicalRel, err := filepath.Rel(d.hostRoot, fullPath)
	if err != nil {
		return "", 3
	}
	if lexicalRel == ".." || strings.HasPrefix(lexicalRel, ".."+string(filepath.Separator)) {
		return "", 5
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", 3
	}
	ok, err := pathWithinRoot(d.hostRoot, absPath)
	if err != nil {
		return "", 3
	}
	if !ok {
		return "", 5
	}
	return absPath, 0
}

func (d *BootstrapHostFSDevice) readCString(ptr uint32, limit int) string {
	if ptr == 0 {
		return ""
	}
	buf := make([]byte, 0, min(limit, 64))
	for i := 0; i < limit; i++ {
		b, ok := d.readGuest8(ptr + uint32(i))
		if !ok {
			return ""
		}
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}

func (d *BootstrapHostFSDevice) currentTaskSlot() uint32 {
	if d.bus == nil {
		return 0
	}
	return uint32(d.bus.Read64(bootHostFSKernDataBase + bootHostFSKDCurrentTask))
}

func (d *BootstrapHostFSDevice) currentTaskPTBR() (uint32, bool) {
	if d.bus == nil {
		return 0, false
	}
	// SYS_BOOT_HOSTFS forwards the caller's live PTBR in arg4. Kernel-direct
	// callers leave arg4 == 0 and pass physical addresses, so (0, true)
	// tells translateGuestVA to treat guest pointers as already-physical.
	if d.arg4 >= MMU_PAGE_SIZE && d.arg4&MMU_PAGE_MASK == 0 {
		return d.arg4, true
	}
	return 0, true
}

// translateGuestVA walks the current task's multi-level page table for
// a guest virtual pointer and returns the resolved physical address.
// PLAN_MAX_RAM.md slice 4: returns uint64 so a leaf PPN above the
// uint32 ceiling is preserved verbatim — the caller routes through
// bus.ReadPhys*/WritePhys* helpers and handles backing-resident pages
// without truncating to a low-memory alias.
func (d *BootstrapHostFSDevice) translateGuestVA(ptr uint32, write bool) (uint64, bool) {
	if d.bus == nil {
		return 0, false
	}
	ptBase, ok := d.currentTaskPTBR()
	if !ok {
		return 0, false
	}
	if ptBase == 0 {
		return uint64(ptr), true
	}
	vpn := (uint64(ptr) >> MMU_PAGE_SHIFT) & PTE_PPN_MASK
	tableAddr := uint64(ptBase)
	var ppn uint64
	var flags byte
	for level := 0; level < PT_LEVELS; level++ {
		idx := ptLevelIndex(vpn, level)
		pteAddr := tableAddr + idx*8
		if pteAddr < tableAddr {
			return 0, false // overflow
		}
		pte, ok := d.bus.ReadPhys64WithFault(pteAddr)
		if !ok {
			return 0, false
		}
		entryPPN, entryFlags := parsePTE(pte)
		if level == PT_LEVELS-1 {
			ppn = entryPPN
			flags = entryFlags
			break
		}
		if entryFlags&PTE_P == 0 {
			return 0, false
		}
		tableAddr = entryPPN << MMU_PAGE_SHIFT
	}
	if flags&PTE_P == 0 {
		return 0, false
	}
	if flags&PTE_U == 0 {
		return 0, false
	}
	if write {
		if flags&PTE_W == 0 {
			return 0, false
		}
	} else if flags&PTE_R == 0 {
		return 0, false
	}
	return (ppn << MMU_PAGE_SHIFT) | uint64(ptr&MMU_PAGE_MASK), true
}

func (d *BootstrapHostFSDevice) readGuest8(ptr uint32) (byte, bool) {
	phys, ok := d.translateGuestVA(ptr, false)
	if !ok {
		return 0, false
	}
	// PLAN_MAX_RAM.md slice 4: route through bus.ReadPhys8 so a leaf
	// PPN above the uint32 ceiling reads from the bound backing
	// instead of aliasing to the low-32-bit window via bus.Read8.
	// Gate with PhysMapped so a PTE whose PPN points outside both the
	// low-memory window and the bound backing fails the access instead
	// of returning a silent zero — matches the widened CPU load/store
	// fault behavior.
	if !d.bus.PhysMapped(phys, 1) {
		return 0, false
	}
	return d.bus.ReadPhys8(phys), true
}

func (d *BootstrapHostFSDevice) writeGuest8(ptr uint32, value byte) bool {
	phys, ok := d.translateGuestVA(ptr, true)
	if !ok {
		return false
	}
	if !d.bus.PhysMapped(phys, 1) {
		return false
	}
	d.bus.WritePhys8(phys, value)
	return true
}

func (d *BootstrapHostFSDevice) writeGuest64(ptr uint32, value uint64) bool {
	for i := 0; i < 8; i++ {
		if !d.writeGuest8(ptr+uint32(i), byte(value>>(8*i))) {
			return false
		}
	}
	return true
}

func (d *BootstrapHostFSDevice) writeGuestName(base uint32, name string) bool {
	if len(name) > BOOT_HOSTFS_DIRENT_NAME_MAX-1 {
		name = name[:BOOT_HOSTFS_DIRENT_NAME_MAX-1]
	}
	for i := 0; i < BOOT_HOSTFS_DIRENT_NAME_MAX; i++ {
		if !d.writeGuest8(base+uint32(i), 0) {
			return false
		}
	}
	for i := 0; i < len(name); i++ {
		if !d.writeGuest8(base+uint32(i), name[i]) {
			return false
		}
	}
	return true
}

func writeBootHostFSName(bus *MachineBus, base uint32, name string) {
	if len(name) > BOOT_HOSTFS_DIRENT_NAME_MAX-1 {
		name = name[:BOOT_HOSTFS_DIRENT_NAME_MAX-1]
	}
	for i := 0; i < BOOT_HOSTFS_DIRENT_NAME_MAX; i++ {
		bus.Write8(base+uint32(i), 0)
	}
	for i := 0; i < len(name); i++ {
		bus.Write8(base+uint32(i), name[i])
	}
}

func hostFSKind(info os.FileInfo) int {
	if info.IsDir() {
		return BOOT_HOSTFS_KIND_DIR
	}
	return BOOT_HOSTFS_KIND_FILE
}

func mapHostErr(err error) uint32 {
	switch {
	case os.IsNotExist(err):
		return 4
	case os.IsPermission(err):
		return 5
	default:
		return 3
	}
}

func pathWithinRoot(root, target string) (bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, fmt.Errorf("rel %q -> %q: %w", root, target, err)
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

func defaultBootstrapHostFSRoot() string {
	if root := os.Getenv("INTUITIONOS_HOST_ROOT"); root != "" {
		return root
	}
	if exe, err := os.Executable(); err == nil {
		if root := defaultBootstrapHostFSRootFromExecutable(exe); bootstrapHostFSRootExists(root) {
			return root
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if abs, err := filepath.Abs(filepath.Join(wd, "sdk", "intuitionos", "system", "SYS")); err == nil {
			if bootstrapHostFSRootExists(abs) {
				return abs
			}
		}
	}
	if abs, err := filepath.Abs(filepath.Join("sdk", "intuitionos", "system", "SYS")); err == nil {
		return abs
	}
	return filepath.Join("sdk", "intuitionos", "system", "SYS")
}

func defaultBootstrapHostFSRootFromExecutable(exe string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	exeDir := filepath.Dir(exe)
	for _, candidate := range []string{
		filepath.Join(exeDir, "..", "sdk", "intuitionos", "system", "SYS"),
		filepath.Join(exeDir, "sdk", "intuitionos", "system", "SYS"),
	} {
		if abs, err := filepath.Abs(candidate); err == nil {
			if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
				return abs
			}
		}
	}
	if abs, err := filepath.Abs(filepath.Join(exeDir, "..", "sdk", "intuitionos", "system", "SYS")); err == nil {
		return abs
	}
	return filepath.Join(exeDir, "..", "sdk", "intuitionos", "system", "SYS")
}

func bootstrapHostFSRootExists(root string) bool {
	info, err := os.Stat(root)
	return err == nil && info.IsDir()
}
