package main

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// GemdosInterceptor intercepts TRAP #1 (GEMDOS) calls from the M68K CPU,
// mapping a host directory as a GEMDOS drive letter. Calls targeting other
// drives pass through to EmuTOS unchanged.
type GemdosInterceptor struct {
	cpu      *M68KCPU
	bus      *MachineBus
	driveNum uint16 // 0=A, 1=B, ... 20=U
	hostRoot string // Canonicalized absolute host path

	mu           sync.Mutex
	currentDir   string             // GEMDOS-relative CWD (e.g. "" or "SUBDIR\CHILD")
	dtaAddr      uint32             // Guest DTA address (snooped from Fsetdta)
	handles      map[int16]*os.File // GEMDOS handle -> host file
	nextHandle   int16
	defaultDrive uint16

	// Fsfirst/Fsnext search state
	searchEntries  []os.DirEntry
	searchDTANames []string // parallel to searchEntries: pre-computed unique 8.3 names
	searchIdx      int
	searchPattern  string
	searchDir      string
	searchAttr     uint16
	searchActive   bool // true when we handled Fsfirst (Fsnext should be ours)

	// Maps host directory → (uppercase short name → original name).
	// Populated during Fsfirst for 8.3 ↔ long name resolution.
	shortNameMaps map[string]map[string]string

	debugTrace bool // enable GEMDOS call tracing

	// Pexec state: saved parent context during child execution
	pexecState *pexecSavedState

	// FNODE/WNODE workaround: populated from Mshrink after enumeration.
	// Used to fix GCC 13 -mshort codegen bug in win_start() that leaves
	// the last WNODE's w_next pointing past the array into FNODE memory.
	fnodeBase     uint32 // p_fbase from dos_shrink
	fnodeSize     uint32 // allocation size (count * sizeof(FNODE))
	returnedCount int    // number of entries successfully returned (Fsnext OK count)
}

// pexecSavedState holds the parent process state saved during Pexec child execution.
type pexecSavedState struct {
	pc       uint32
	sp       uint32
	sr       uint16
	dataRegs [8]uint32
	addrRegs [8]uint32
	tpaBase  uint32
	tpaSize  uint32
}

// NewGemdosInterceptor creates a new interceptor mapping hostRoot as the given drive number.
func NewGemdosInterceptor(cpu *M68KCPU, bus *MachineBus, hostRoot string, driveNum uint16) (*GemdosInterceptor, error) {
	resolved, err := filepath.EvalSymlinks(hostRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve symlinks for %q: %w", hostRoot, err)
	}
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return nil, fmt.Errorf("absolute path for %q: %w", resolved, err)
	}
	return &GemdosInterceptor{
		cpu:           cpu,
		bus:           bus,
		driveNum:      driveNum,
		hostRoot:      absPath,
		handles:       make(map[int16]*os.File),
		nextHandle:    GEMDOS_HANDLE_MIN,
		shortNameMaps: make(map[string]map[string]string),
		debugTrace:    true,
	}, nil
}

// HandleTrap1 intercepts a GEMDOS TRAP #1 call. Returns true if handled
// (caller should skip ProcessException), false to forward to EmuTOS.
func (g *GemdosInterceptor) HandleTrap1() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.ensureDrvbits()

	// Read function number from SP (word at current A7)
	sp := g.cpu.AddrRegs[7]
	funcNum := g.cpu.Read16(sp)

	if g.debugTrace {
		g.logTrap(funcNum, sp)
	}

	switch funcNum {
	case GEMDOS_DSETDRV:
		return g.handleDsetdrv(sp)
	case GEMDOS_DGETDRV:
		return g.handleDgetdrv()
	case GEMDOS_FSETDTA:
		return g.handleFsetdta(sp)
	case GEMDOS_DFREE:
		return g.handleDfree(sp)
	case GEMDOS_DCREATE:
		return g.handleDcreate(sp)
	case GEMDOS_DDELETE:
		return g.handleDdelete(sp)
	case GEMDOS_DSETPATH:
		return g.handleDsetpath(sp)
	case GEMDOS_FCREATE:
		return g.handleFcreate(sp)
	case GEMDOS_FOPEN:
		return g.handleFopen(sp)
	case GEMDOS_FCLOSE:
		return g.handleFclose(sp)
	case GEMDOS_FREAD:
		return g.handleFread(sp)
	case GEMDOS_FWRITE:
		return g.handleFwrite(sp)
	case GEMDOS_FDELETE:
		return g.handleFdelete(sp)
	case GEMDOS_FSEEK:
		return g.handleFseek(sp)
	case GEMDOS_FATTRIB:
		return g.handleFattrib(sp)
	case GEMDOS_DGETPATH:
		return g.handleDgetpath(sp)
	case GEMDOS_FSFIRST:
		return g.handleFsfirst(sp)
	case GEMDOS_FSNEXT:
		return g.handleFsnext()
	case GEMDOS_FRENAME:
		return g.handleFrename(sp)
	case GEMDOS_FDATIME:
		return g.handleFdatime(sp)
	case GEMDOS_MSHRINK:
		if g.handleMshrink(sp) {
			return true
		}
		return false
	case GEMDOS_PEXEC:
		return g.handlePexec(sp)
	case GEMDOS_PTERM:
		return g.handlePterm(sp)
	case 0x00: // Pterm0
		return g.handlePterm0()
	}
	return false
}

// Close releases all open file handles and detaches from the CPU.
func (g *GemdosInterceptor) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for h, f := range g.handles {
		f.Close()
		delete(g.handles, h)
	}
	if g.cpu != nil {
		g.cpu.gemdosHandler = nil
		g.cpu = nil
	}
}

// --- Drive operations ---

func (g *GemdosInterceptor) handleDsetdrv(sp uint32) bool {
	drv := g.cpu.Read16(sp + 2)
	g.defaultDrive = drv
	if drv == g.driveNum {
		drvbits := g.cpu.Read32(GEMDOS_DRVBITS_ADDR) | (1 << g.driveNum)
		g.setD0(uint32(drvbits))
		return true
	}
	return false
}

func (g *GemdosInterceptor) handleDgetdrv() bool {
	if g.defaultDrive == g.driveNum {
		g.setD0(uint32(g.driveNum))
		return true
	}
	return false
}

func (g *GemdosInterceptor) handleFsetdta(sp uint32) bool {
	g.dtaAddr = g.cpu.Read32(sp + 2)
	return false // snoop only — let EmuTOS also process it
}

func (g *GemdosInterceptor) handleDfree(sp uint32) bool {
	bufAddr := g.cpu.Read32(sp + 2)
	drv := g.cpu.Read16(sp + 6)
	// drv: 0=default, 1=A, 2=B, ... 21=U
	effectiveDrv := drv
	if effectiveDrv == 0 {
		effectiveDrv = g.defaultDrive + 1
	}
	if effectiveDrv != g.driveNum+1 {
		return false
	}

	// Report a large fixed size (512MB, 1024-byte clusters)
	clusterSize := uint32(1024)
	totalClusters := uint32(524288)
	freeClusters := uint32(524288)
	sectorSize := uint32(512)
	sectorsPerCluster := clusterSize / sectorSize

	// Write as big-endian via M68K CPU (EmuTOS reads these as M68K longs)
	g.cpu.Write32(bufAddr, freeClusters)
	g.cpu.Write32(bufAddr+4, totalClusters)
	g.cpu.Write32(bufAddr+8, sectorSize)
	g.cpu.Write32(bufAddr+12, sectorsPerCluster)
	g.setD0(GEMDOS_E_OK)
	return true
}

func (g *GemdosInterceptor) handleDcreate(sp uint32) bool {
	pathAddr := g.cpu.Read32(sp + 2)
	gemdosPath := g.readString(pathAddr)
	hostPath, ok := g.resolvePathForOurDrive(gemdosPath)
	if !ok {
		return false
	}
	if err := os.MkdirAll(hostPath, 0o755); err != nil {
		g.setD0(signExtend16to32(GEMDOS_EACCDN))
		return true
	}
	g.setD0(GEMDOS_E_OK)
	return true
}

func (g *GemdosInterceptor) handleDdelete(sp uint32) bool {
	pathAddr := g.cpu.Read32(sp + 2)
	gemdosPath := g.readString(pathAddr)
	hostPath, ok := g.resolvePathForOurDrive(gemdosPath)
	if !ok {
		return false
	}
	if err := os.Remove(hostPath); err != nil {
		g.setD0(signExtend16to32(GEMDOS_EPTHNF))
		return true
	}
	g.setD0(GEMDOS_E_OK)
	return true
}

func (g *GemdosInterceptor) handleDsetpath(sp uint32) bool {
	pathAddr := g.cpu.Read32(sp + 2)
	gemdosPath := g.readString(pathAddr)
	hostPath, ok := g.resolvePathForOurDrive(gemdosPath)
	if !ok {
		return false
	}
	info, err := os.Stat(hostPath)
	if err != nil || !info.IsDir() {
		g.setD0(signExtend16to32(GEMDOS_EPTHNF))
		return true
	}
	// Extract the relative path from hostRoot
	rel, err := filepath.Rel(g.hostRoot, hostPath)
	if err != nil {
		g.setD0(signExtend16to32(GEMDOS_EPTHNF))
		return true
	}
	if rel == "." {
		rel = ""
	}
	g.currentDir = strings.ReplaceAll(rel, "/", "\\")
	g.setD0(GEMDOS_E_OK)
	return true
}

func (g *GemdosInterceptor) handleDgetpath(sp uint32) bool {
	bufAddr := g.cpu.Read32(sp + 2)
	drv := g.cpu.Read16(sp + 6)
	effectiveDrv := drv
	if effectiveDrv == 0 {
		effectiveDrv = g.defaultDrive + 1
	}
	if effectiveDrv != g.driveNum+1 {
		return false
	}
	// Write current directory path with leading backslash
	path := "\\" + g.currentDir
	g.writeString(bufAddr, path)
	g.setD0(GEMDOS_E_OK)
	return true
}

// --- File operations ---

func (g *GemdosInterceptor) handleFcreate(sp uint32) bool {
	fnameAddr := g.cpu.Read32(sp + 2)
	// attr at sp+6 (unused for host files)
	gemdosPath := g.readString(fnameAddr)
	hostPath, ok := g.resolvePathForOurDrive(gemdosPath)
	if !ok {
		return false
	}
	f, err := os.Create(hostPath)
	if err != nil {
		g.setD0(signExtend16to32(GEMDOS_EACCDN))
		return true
	}
	h := g.allocHandle(f)
	if h < 0 {
		f.Close()
		g.setD0(signExtend16to32(GEMDOS_ENHNDL))
		return true
	}
	g.setD0(uint32(h))
	return true
}

func (g *GemdosInterceptor) handleFopen(sp uint32) bool {
	fnameAddr := g.cpu.Read32(sp + 2)
	mode := g.cpu.Read16(sp + 6)
	gemdosPath := g.readString(fnameAddr)
	hostPath, ok := g.resolvePathForOurDrive(gemdosPath)
	if !ok {
		return false
	}
	var flag int
	switch mode & 0x03 {
	case GEMDOS_OPEN_READ:
		flag = os.O_RDONLY
	case GEMDOS_OPEN_WRITE:
		flag = os.O_WRONLY
	default:
		flag = os.O_RDWR
	}
	f, err := os.OpenFile(hostPath, flag, 0o644)
	if err != nil {
		g.setD0(signExtend16to32(GEMDOS_EFILNF))
		return true
	}
	h := g.allocHandle(f)
	if h < 0 {
		f.Close()
		g.setD0(signExtend16to32(GEMDOS_ENHNDL))
		return true
	}
	g.setD0(uint32(h))
	return true
}

func (g *GemdosInterceptor) handleFclose(sp uint32) bool {
	handle := int16(g.cpu.Read16(sp + 2))
	if handle < GEMDOS_HANDLE_MIN {
		return false
	}
	f, ok := g.handles[handle]
	if !ok {
		g.setD0(signExtend16to32(GEMDOS_EIHNDL))
		return true
	}
	f.Close()
	delete(g.handles, handle)
	g.setD0(GEMDOS_E_OK)
	return true
}

func (g *GemdosInterceptor) handleFread(sp uint32) bool {
	handle := int16(g.cpu.Read16(sp + 2))
	if handle < GEMDOS_HANDLE_MIN {
		return false
	}
	count := g.cpu.Read32(sp + 4)
	bufAddr := g.cpu.Read32(sp + 8)
	f, ok := g.handles[handle]
	if !ok {
		g.setD0(signExtend16to32(GEMDOS_EIHNDL))
		return true
	}
	buf := make([]byte, count)
	n, err := f.Read(buf)
	if n > 0 {
		for i := 0; i < n; i++ {
			g.bus.Write8(bufAddr+uint32(i), buf[i])
		}
	}
	if err != nil && n == 0 {
		g.setD0(0) // EOF = 0 bytes read
		return true
	}
	g.setD0(uint32(n))
	return true
}

func (g *GemdosInterceptor) handleFwrite(sp uint32) bool {
	handle := int16(g.cpu.Read16(sp + 2))
	if handle < GEMDOS_HANDLE_MIN {
		return false
	}
	count := g.cpu.Read32(sp + 4)
	bufAddr := g.cpu.Read32(sp + 8)
	f, ok := g.handles[handle]
	if !ok {
		g.setD0(signExtend16to32(GEMDOS_EIHNDL))
		return true
	}
	buf := make([]byte, count)
	for i := uint32(0); i < count; i++ {
		buf[i] = g.bus.Read8(bufAddr + i)
	}
	n, err := f.Write(buf)
	if err != nil {
		g.setD0(signExtend16to32(GEMDOS_EACCDN))
		return true
	}
	g.setD0(uint32(n))
	return true
}

func (g *GemdosInterceptor) handleFdelete(sp uint32) bool {
	fnameAddr := g.cpu.Read32(sp + 2)
	gemdosPath := g.readString(fnameAddr)
	hostPath, ok := g.resolvePathForOurDrive(gemdosPath)
	if !ok {
		return false
	}
	if err := os.Remove(hostPath); err != nil {
		g.setD0(signExtend16to32(GEMDOS_EFILNF))
		return true
	}
	g.setD0(GEMDOS_E_OK)
	return true
}

func (g *GemdosInterceptor) handleFseek(sp uint32) bool {
	offset := int32(g.cpu.Read32(sp + 2))
	handle := int16(g.cpu.Read16(sp + 6))
	mode := g.cpu.Read16(sp + 8)
	if handle < GEMDOS_HANDLE_MIN {
		return false
	}
	f, ok := g.handles[handle]
	if !ok {
		g.setD0(signExtend16to32(GEMDOS_EIHNDL))
		return true
	}
	var whence int
	switch mode {
	case GEMDOS_SEEK_SET:
		whence = 0
	case GEMDOS_SEEK_CUR:
		whence = 1
	case GEMDOS_SEEK_END:
		whence = 2
	default:
		g.setD0(signExtend16to32(GEMDOS_ERANGE))
		return true
	}
	pos, err := f.Seek(int64(offset), whence)
	if err != nil {
		g.setD0(signExtend16to32(GEMDOS_ERANGE))
		return true
	}
	g.setD0(uint32(pos))
	return true
}

func (g *GemdosInterceptor) handleFattrib(sp uint32) bool {
	fnameAddr := g.cpu.Read32(sp + 2)
	wflag := g.cpu.Read16(sp + 6)
	// attr at sp+8 (for set mode)
	gemdosPath := g.readString(fnameAddr)
	hostPath, ok := g.resolvePathForOurDrive(gemdosPath)
	if !ok {
		return false
	}
	if wflag == 0 {
		// Get attributes
		info, err := os.Stat(hostPath)
		if err != nil {
			g.setD0(signExtend16to32(GEMDOS_EFILNF))
			return true
		}
		attr := uint16(GEMDOS_ATTR_ARCHIVE)
		if info.IsDir() {
			attr = GEMDOS_ATTR_DIRECTORY
		}
		if info.Mode()&0o200 == 0 {
			attr |= GEMDOS_ATTR_READONLY
		}
		g.setD0(uint32(attr))
		return true
	}
	// Set attributes — limited on Unix, just return OK
	g.setD0(GEMDOS_E_OK)
	return true
}

func (g *GemdosInterceptor) handleFrename(sp uint32) bool {
	// sp+2: W zero (reserved)
	oldAddr := g.cpu.Read32(sp + 4)
	newAddr := g.cpu.Read32(sp + 8)
	oldPath := g.readString(oldAddr)
	newPath := g.readString(newAddr)
	hostOld, okOld := g.resolvePathForOurDrive(oldPath)
	hostNew, okNew := g.resolvePathForOurDrive(newPath)
	if !okOld || !okNew {
		if okOld != okNew {
			g.setD0(signExtend16to32(GEMDOS_ENSAME))
			return true
		}
		return false
	}
	if err := os.Rename(hostOld, hostNew); err != nil {
		g.setD0(signExtend16to32(GEMDOS_EACCDN))
		return true
	}
	g.setD0(GEMDOS_E_OK)
	return true
}

func (g *GemdosInterceptor) handleFdatime(sp uint32) bool {
	bufAddr := g.cpu.Read32(sp + 2)
	handle := int16(g.cpu.Read16(sp + 6))
	wflag := g.cpu.Read16(sp + 8)
	if handle < GEMDOS_HANDLE_MIN {
		return false
	}
	f, ok := g.handles[handle]
	if !ok {
		g.setD0(signExtend16to32(GEMDOS_EIHNDL))
		return true
	}
	if wflag == 0 {
		// Get date/time
		info, err := f.Stat()
		if err != nil {
			g.setD0(signExtend16to32(GEMDOS_EINTRN))
			return true
		}
		t := info.ModTime()
		timeWord := packGemdosTime(t)
		dateWord := packGemdosDate(t)
		g.cpu.Write16(bufAddr, timeWord)
		g.cpu.Write16(bufAddr+2, dateWord)
	} else {
		// Set date/time — read from buffer, apply to file
		timeWord := g.cpu.Read16(bufAddr)
		dateWord := g.cpu.Read16(bufAddr + 2)
		t := unpackGemdosDateTime(dateWord, timeWord)
		os.Chtimes(f.Name(), t, t)
	}
	g.setD0(GEMDOS_E_OK)
	return true
}

// --- Pexec / Process management ---

func (g *GemdosInterceptor) handlePexec(sp uint32) bool {
	mode := g.cpu.Read16(sp + 2)
	if mode != GEMDOS_PEXEC_LOAD_GO {
		return false
	}

	fnameAddr := g.cpu.Read32(sp + 4)
	cmdlineAddr := g.cpu.Read32(sp + 8)
	fname := g.readString(fnameAddr)
	hostPath, ok := g.resolvePathForOurDrive(fname)
	if !ok {
		return false
	}

	data, err := os.ReadFile(hostPath)
	if err != nil {
		fmt.Printf("[GEMDOS] Pexec: cannot read %s: %v\n", hostPath, err)
		g.setD0(signExtend16to32(GEMDOS_EFILNF))
		return true
	}

	if len(data) < TOS_PRG_HEADER_LEN {
		g.setD0(signExtend16to32(GEMDOS_EPLFMT))
		return true
	}
	magic := binary.BigEndian.Uint16(data[0:2])
	if magic != TOS_PRG_MAGIC {
		g.setD0(signExtend16to32(GEMDOS_EPLFMT))
		return true
	}

	textSize := binary.BigEndian.Uint32(data[2:6])
	dataSize := binary.BigEndian.Uint32(data[6:10])
	bssSize := binary.BigEndian.Uint32(data[10:14])
	symSize := binary.BigEndian.Uint32(data[14:18])

	codeDataSize := textSize + dataSize
	if uint32(len(data)) < TOS_PRG_HEADER_LEN+codeDataSize {
		g.setD0(signExtend16to32(GEMDOS_EPLFMT))
		return true
	}

	// Allocate TPA above EmuTOS-managed memory (at 8MB mark in the 32MB bus).
	tpaSize := uint32(TOS_BASEPAGE_SIZE) + textSize + dataSize + bssSize + 8192
	tpaSize = (tpaSize + 255) &^ 255 // round up to 256-byte boundary
	tpaBase := uint32(PEXEC_TPA_BASE)
	tpaEnd := tpaBase + tpaSize

	bpAddr := tpaBase
	textBase := tpaBase + TOS_BASEPAGE_SIZE
	dataBase := textBase + textSize
	bssBase := dataBase + dataSize

	// Copy TEXT+DATA segments into bus memory
	src := data[TOS_PRG_HEADER_LEN : TOS_PRG_HEADER_LEN+codeDataSize]
	for i, b := range src {
		g.bus.Write8(textBase+uint32(i), b)
	}

	// Zero BSS
	for i := uint32(0); i < bssSize; i++ {
		g.bus.Write8(bssBase+i, 0)
	}

	// Process relocation table
	relocOffset := TOS_PRG_HEADER_LEN + codeDataSize + symSize
	if uint32(len(data)) > relocOffset {
		g.processRelocation(data[relocOffset:], textBase)
	}

	// Set up basepage (256 bytes, all fields written as big-endian via M68K Write32)
	for i := uint32(0); i < TOS_BASEPAGE_SIZE; i += 4 {
		g.cpu.Write32(bpAddr+i, 0)
	}
	g.cpu.Write32(bpAddr+0, tpaBase)  // p_lowtpa
	g.cpu.Write32(bpAddr+4, tpaEnd)   // p_hitpa
	g.cpu.Write32(bpAddr+8, textBase) // p_tbase
	g.cpu.Write32(bpAddr+12, textSize)
	g.cpu.Write32(bpAddr+16, dataBase) // p_dbase
	g.cpu.Write32(bpAddr+20, dataSize)
	g.cpu.Write32(bpAddr+24, bssBase) // p_bbase
	g.cpu.Write32(bpAddr+28, bssSize)
	g.cpu.Write32(bpAddr+32, g.dtaAddr) // p_dta

	// Copy command line into basepage (p_cmdlin at offset 128)
	if cmdlineAddr != 0 {
		cmdLen := g.bus.Read8(cmdlineAddr)
		g.bus.Write8(bpAddr+128, cmdLen)
		for i := uint8(0); i < cmdLen && i < 126; i++ {
			g.bus.Write8(bpAddr+129+uint32(i), g.bus.Read8(cmdlineAddr+1+uint32(i)))
		}
	}

	// Save parent process state (restored on Pterm)
	saved := &pexecSavedState{
		pc:      g.cpu.PC,
		sp:      g.cpu.AddrRegs[7],
		sr:      g.cpu.SR,
		tpaBase: tpaBase,
		tpaSize: tpaSize,
	}
	copy(saved.dataRegs[:], g.cpu.DataRegs[:])
	copy(saved.addrRegs[:], g.cpu.AddrRegs[:])
	g.pexecState = saved

	// Set up child stack at top of TPA.
	// TOS convention: on program entry, 4(SP) = basepage pointer.
	childSP := (tpaEnd - 8) &^ 1 // word-aligned, room for return+basepage
	g.cpu.Write32(childSP+4, bpAddr)
	g.cpu.Write32(childSP, 0) // fake return address

	g.cpu.AddrRegs[7] = childSP
	g.cpu.PC = textBase

	fmt.Printf("[GEMDOS] Pexec: loaded %q at $%06X (text=%d data=%d bss=%d) entry=$%06X sp=$%06X\n",
		fname, textBase, textSize, dataSize, bssSize, textBase, childSP)
	return true
}

// processRelocation applies TOS .PRG relocation to the loaded program.
func (g *GemdosInterceptor) processRelocation(relocData []byte, textBase uint32) {
	if len(relocData) < 4 {
		return
	}
	firstOffset := binary.BigEndian.Uint32(relocData[0:4])
	if firstOffset == 0 {
		return // no relocations
	}

	addr := textBase + firstOffset
	val := g.cpu.Read32(addr)
	g.cpu.Write32(addr, val+textBase)

	pos := uint32(4)
	count := 1
	for pos < uint32(len(relocData)) {
		b := relocData[pos]
		pos++
		if b == 0 {
			break
		}
		if b == 1 {
			addr += 254
			continue
		}
		addr += uint32(b)
		val = g.cpu.Read32(addr)
		g.cpu.Write32(addr, val+textBase)
		count++
	}
	fmt.Printf("[GEMDOS] Pexec: applied %d relocations (base=$%06X)\n", count, textBase)
}

// handleMshrink intercepts Mshrink for our Pexec-loaded TPA.
func (g *GemdosInterceptor) handleMshrink(sp uint32) bool {
	if g.pexecState == nil {
		return false
	}
	addr := g.cpu.Read32(sp + 4) // sp+2 is zero (reserved), sp+4 is block address
	if addr == g.pexecState.tpaBase {
		// Our TPA block — just return success
		g.setD0(GEMDOS_E_OK)
		return true
	}
	return false
}

// handlePterm intercepts Pterm (0x4C) to restore parent state after Pexec child exits.
func (g *GemdosInterceptor) handlePterm(sp uint32) bool {
	if g.pexecState == nil {
		return false
	}
	exitCode := g.cpu.Read16(sp + 2)
	g.restoreParentState(uint32(exitCode))
	return true
}

// handlePterm0 intercepts Pterm0 (0x00) to restore parent state.
func (g *GemdosInterceptor) handlePterm0() bool {
	if g.pexecState == nil {
		return false
	}
	g.restoreParentState(0)
	return true
}

func (g *GemdosInterceptor) restoreParentState(exitCode uint32) {
	saved := g.pexecState
	g.pexecState = nil

	copy(g.cpu.DataRegs[:], saved.dataRegs[:])
	copy(g.cpu.AddrRegs[:], saved.addrRegs[:])
	g.cpu.PC = saved.pc
	g.cpu.SR = saved.sr
	g.cpu.DataRegs[0] = exitCode

	fmt.Printf("[GEMDOS] Pterm: child exited (code=%d), restored parent at PC=$%06X SP=$%06X\n",
		exitCode, saved.pc, saved.sp)
}

// --- Directory search (Fsfirst / Fsnext) ---

func (g *GemdosInterceptor) handleFsfirst(sp uint32) bool {
	fspecAddr := g.cpu.Read32(sp + 2)
	attr := g.cpu.Read16(sp + 6)
	fspec := g.readString(fspecAddr)
	hostDir, pattern, ok := g.resolveSearchPath(fspec)
	if !ok {
		// Not our drive — clear our search state so Fsnext forwards too
		g.searchActive = false
		g.searchEntries = nil
		g.searchDTANames = nil
		return false
	}

	allEntries, err := os.ReadDir(hostDir)
	if err != nil {
		g.setD0(signExtend16to32(GEMDOS_EPTHNF))
		return true
	}

	// Filter out Unix hidden files (names starting with '.').
	// GEMDOS has no concept of hidden dotfiles; including them inflates the
	// FNODE count and can exhaust EmuTOS desktop resources.
	entries := make([]os.DirEntry, 0, len(allEntries))
	for _, e := range allEntries {
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			continue
		}
		entries = append(entries, e)
	}

	g.searchEntries = entries
	g.searchIdx = 0
	g.searchPattern = pattern
	g.searchDir = hostDir
	g.searchAttr = attr
	g.searchActive = true
	g.returnedCount = 0
	g.fnodeBase = 0
	g.fnodeSize = 0

	// Pre-compute unique 8.3 names for all entries (Windows-style ~N disambiguation)
	usedNames := make(map[string]bool)
	dirMap := make(map[string]string)
	dtaNames := make([]string, len(entries))
	for i, entry := range entries {
		base := toGemdos83(entry.Name())
		if base == "" {
			continue
		}
		unique := makeUnique83(base, usedNames)
		usedNames[unique] = true
		dtaNames[i] = unique
		dirMap[unique] = entry.Name()
	}
	g.searchDTANames = dtaNames
	g.shortNameMaps[hostDir] = dirMap

	return g.findNextMatch()
}

func (g *GemdosInterceptor) handleFsnext() bool {
	if !g.searchActive {
		return false // not our search, forward to EmuTOS
	}
	if g.searchEntries == nil {
		g.setD0(signExtend16to32(GEMDOS_ENMFIL))
		return true
	}
	return g.findNextMatch()
}

func (g *GemdosInterceptor) findNextMatch() bool {
	for g.searchIdx < len(g.searchEntries) {
		idx := g.searchIdx
		entry := g.searchEntries[idx]
		g.searchIdx++

		shortName := g.searchDTANames[idx]
		if shortName == "" {
			continue
		}
		if !gemdosWildcardMatch(g.searchPattern, shortName) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		isDir := entry.IsDir()
		if isDir && (g.searchAttr&GEMDOS_ATTR_DIRECTORY) == 0 {
			continue
		}

		g.writeDTAWithName(info, isDir, shortName)
		g.setD0(GEMDOS_E_OK)
		g.returnedCount++
		return true
	}

	g.searchEntries = nil
	g.searchDTANames = nil
	// Keep searchActive=true so subsequent Fsnext calls return ENMFIL from us
	// rather than forwarding to EmuTOS with a zeroed DTA reserved area (which
	// causes Bus Errors). searchActive is cleared by the next Fsfirst call.
	g.setD0(signExtend16to32(GEMDOS_ENMFIL))

	if g.debugTrace {
		fmt.Printf("[GEMDOS] ENMFIL: enumeration done, %d entries returned\n", g.searchIdx)
	}

	return true
}

func (g *GemdosInterceptor) writeDTAWithName(info fs.FileInfo, isDir bool, name string) {
	if g.dtaAddr == 0 {
		return
	}
	// Clear DTA
	for i := uint32(0); i < GEMDOS_DTA_TOTAL; i++ {
		g.bus.Write8(g.dtaAddr+i, 0)
	}

	// Populate reserved area (bytes 0-20) with valid search state.
	// EmuTOS reads this during Fsnext to continue a directory search.
	// Byte 0: search attribute mask
	g.bus.Write8(g.dtaAddr+0, uint8(g.searchAttr))
	// Byte 1: drive number (with bit 6 set = flag for "search in progress")
	g.bus.Write8(g.dtaAddr+1, uint8(g.driveNum)|0x40)
	// Bytes 2-12: FCB search pattern (8+3 space-padded, no dot)
	fcb := gemdosPatternToFCB(g.searchPattern)
	for i := 0; i < 11 && i < len(fcb); i++ {
		g.bus.Write8(g.dtaAddr+2+uint32(i), fcb[i])
	}
	// Bytes 13-14: directory cluster (0 = root for host filesystem)
	// Bytes 15-16: search position (current index)
	g.bus.Write8(g.dtaAddr+15, uint8(g.searchIdx>>8))
	g.bus.Write8(g.dtaAddr+16, uint8(g.searchIdx))

	// Attributes
	attr := uint8(GEMDOS_ATTR_ARCHIVE)
	if isDir {
		attr = GEMDOS_ATTR_DIRECTORY
	}
	if info.Mode()&0o200 == 0 {
		attr |= GEMDOS_ATTR_READONLY
	}
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_ATTR, attr)

	// Time and date
	t := info.ModTime()
	timeWord := packGemdosTime(t)
	dateWord := packGemdosDate(t)
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_TIME, uint8(timeWord>>8))
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_TIME+1, uint8(timeWord))
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_DATE, uint8(dateWord>>8))
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_DATE+1, uint8(dateWord))

	// File size (cap at uint32 max for huge files)
	sz := info.Size()
	if sz > 0xFFFFFFFF {
		sz = 0xFFFFFFFF
	}
	size := uint32(sz)
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_SIZE, uint8(size>>24))
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_SIZE+1, uint8(size>>16))
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_SIZE+2, uint8(size>>8))
	g.bus.Write8(g.dtaAddr+GEMDOS_DTA_SIZE+3, uint8(size))

	// Filename (pre-computed unique 8.3 name, max 12 chars + null fits in 14-byte field)
	for i := 0; i < len(name); i++ {
		g.bus.Write8(g.dtaAddr+GEMDOS_DTA_NAME+uint32(i), name[i])
	}

}

// --- Path resolution ---

// resolvePathForOurDrive checks if a GEMDOS path targets our drive and returns
// the host filesystem path. Returns ("", false) if the path targets another drive.
func (g *GemdosInterceptor) resolvePathForOurDrive(gemdosPath string) (string, bool) {
	path := gemdosPath
	hasDriveLetter := false

	// Check for drive letter prefix (e.g. "U:\path")
	if len(path) >= 2 && path[1] == ':' {
		letter := strings.ToUpper(path[:1])
		drv := uint16(letter[0] - 'A')
		if drv != g.driveNum {
			return "", false
		}
		hasDriveLetter = true
		path = path[2:]
	} else if g.defaultDrive != g.driveNum {
		return "", false
	}

	// Reject path traversal
	normalized := strings.ReplaceAll(path, "\\", "/")
	for _, component := range strings.Split(normalized, "/") {
		if component == ".." {
			return "", false
		}
	}

	// Determine if absolute (starts with \ or /) or relative
	var resolved string
	if len(path) > 0 && (path[0] == '\\' || path[0] == '/') {
		// Absolute from drive root
		resolved = strings.TrimLeft(normalized, "/")
	} else if hasDriveLetter || g.currentDir == "" {
		resolved = normalized
	} else {
		// Relative to current directory
		curDir := strings.ReplaceAll(g.currentDir, "\\", "/")
		if normalized == "" {
			resolved = curDir
		} else {
			resolved = curDir + "/" + normalized
		}
	}

	// Case-insensitive path resolution
	hostPath := g.caseInsensitiveResolve(resolved)
	return hostPath, true
}

// resolveSearchPath splits a GEMDOS filespec (e.g. "U:\DIR\*.TXT") into
// a host directory path and an uppercase wildcard pattern.
func (g *GemdosInterceptor) resolveSearchPath(fspec string) (hostDir, pattern string, ok bool) {
	// Split into directory and pattern parts
	normalized := strings.ReplaceAll(fspec, "\\", "/")

	// Find the last path separator to split dir from pattern
	lastSep := strings.LastIndex(normalized, "/")
	var dirPart, filePart string
	if lastSep >= 0 {
		dirPart = fspec[:lastSep]
		if lastSep+1 < len(fspec) {
			filePart = fspec[lastSep+1:]
		} else {
			filePart = "*.*"
		}
	} else {
		// Check if starts with drive letter
		if len(fspec) >= 2 && fspec[1] == ':' {
			dirPart = fspec[:2]
			filePart = fspec[2:]
		} else {
			dirPart = ""
			filePart = fspec
		}
	}

	if filePart == "" {
		filePart = "*.*"
	}

	// Resolve the directory part
	var hostPath string
	if dirPart == "" {
		// Use current directory on default drive
		if g.defaultDrive != g.driveNum {
			return "", "", false
		}
		if g.currentDir == "" {
			hostPath = g.hostRoot
		} else {
			curDir := strings.ReplaceAll(g.currentDir, "\\", "/")
			hostPath = g.caseInsensitiveResolve(curDir)
		}
	} else {
		var resolveOk bool
		hostPath, resolveOk = g.resolvePathForOurDrive(dirPart)
		if !resolveOk {
			return "", "", false
		}
	}

	return hostPath, strings.ToUpper(filePart), true
}

// caseInsensitiveResolve walks the path components case-insensitively from hostRoot.
// When an exact case-insensitive match fails, it checks whether the component is
// an 8.3 short name that maps to a longer filename (via shortNameMaps from Fsfirst).
func (g *GemdosInterceptor) caseInsensitiveResolve(relPath string) string {
	if relPath == "" || relPath == "." {
		return g.hostRoot
	}

	components := strings.Split(relPath, "/")
	current := g.hostRoot

	for _, comp := range components {
		if comp == "" || comp == "." {
			continue
		}

		entries, err := os.ReadDir(current)
		if err != nil {
			return filepath.Join(current, comp)
		}

		found := false
		// Pass 1: exact case-insensitive match
		for _, e := range entries {
			if strings.EqualFold(e.Name(), comp) {
				current = filepath.Join(current, e.Name())
				found = true
				break
			}
		}
		if found {
			continue
		}

		// Pass 2: check short name map (8.3 name → original name)
		upperComp := strings.ToUpper(comp)
		if m := g.shortNameMaps[current]; m != nil {
			if origName, ok := m[upperComp]; ok {
				current = filepath.Join(current, origName)
				found = true
			}
		}
		if found {
			continue
		}

		// Pass 3: compute 8.3 name for each entry and match
		for _, e := range entries {
			if strings.EqualFold(toGemdos83(e.Name()), upperComp) {
				current = filepath.Join(current, e.Name())
				found = true
				break
			}
		}

		if !found {
			// No match — use as-is (for create operations)
			current = filepath.Join(current, comp)
		}
	}

	return current
}

// --- Helpers ---

func (g *GemdosInterceptor) setD0(val uint32) {
	g.cpu.DataRegs[0] = val
}

func (g *GemdosInterceptor) allocHandle(f *os.File) int16 {
	if g.nextHandle < GEMDOS_HANDLE_MIN || g.nextHandle > GEMDOS_HANDLE_MAX {
		return -1
	}
	h := g.nextHandle
	g.nextHandle++
	g.handles[h] = f
	return h
}

func (g *GemdosInterceptor) readString(addr uint32) string {
	var buf []byte
	for i := uint32(0); i < 256; i++ {
		b := g.bus.Read8(addr + i)
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}

func (g *GemdosInterceptor) writeString(addr uint32, s string) {
	for i := 0; i < len(s); i++ {
		g.bus.Write8(addr+uint32(i), s[i])
	}
	g.bus.Write8(addr+uint32(len(s)), 0) // null terminator
}

// ensureDrvbits unconditionally ORs our drive bit into _drvbits.
// Called on every HandleTrap1 and periodically from the EmuTOS timer goroutine.
// Must be unconditional because EmuTOS may re-initialise _drvbits during boot.
func (g *GemdosInterceptor) ensureDrvbits() {
	bit := uint32(1) << g.driveNum
	current := g.cpu.Read32(GEMDOS_DRVBITS_ADDR)
	if current&bit == 0 {
		g.cpu.Write32(GEMDOS_DRVBITS_ADDR, current|bit)
	}
}

// PollDrvbits is called from the EmuTOS timer goroutine to inject our drive
// bit into _drvbits early enough for the GEM desktop to see it. EmuTOS BIOS
// init writes _drvbits internally (not via TRAP #1), so the lazy injection in
// HandleTrap1 is too late — the desktop reads the drive map before any TRAP #1.
func (g *GemdosInterceptor) PollDrvbits() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ensureDrvbits()
}

// signExtend16to32 sign-extends a 16-bit error code to 32 bits for D0.
func signExtend16to32(v int16) uint32 {
	return uint32(int32(v))
}

// packGemdosTime encodes a time.Time as a GEMDOS time word.
func packGemdosTime(t time.Time) uint16 {
	return uint16(t.Hour())<<11 | uint16(t.Minute())<<5 | uint16(t.Second()/2)
}

// packGemdosDate encodes a time.Time as a GEMDOS date word.
func packGemdosDate(t time.Time) uint16 {
	year := t.Year() - 1980
	if year < 0 {
		year = 0
	}
	return uint16(year)<<9 | uint16(t.Month())<<5 | uint16(t.Day())
}

// unpackGemdosDateTime decodes GEMDOS date and time words into a time.Time.
func unpackGemdosDateTime(date, timeWord uint16) time.Time {
	year := int(date>>9) + 1980
	month := time.Month((date >> 5) & 0x0F)
	day := int(date & 0x1F)
	hour := int(timeWord >> 11)
	min := int((timeWord >> 5) & 0x3F)
	sec := int((timeWord & 0x1F) * 2)
	return time.Date(year, month, day, hour, min, sec, 0, time.Local)
}

// toGemdos83 converts a host filename to a GEMDOS-safe 8.3 format name.
// Returns uppercase name with max 8 chars + "." + max 3 chars extension.
// Names that are already 8.3 compliant are returned as-is (just uppercased).
// Returns "" for names that produce an empty result (e.g., all-dot names).
func toGemdos83(name string) string {
	if name == "" {
		return ""
	}

	// Strip leading dots (Unix hidden files have no GEMDOS equivalent)
	stripped := name
	for len(stripped) > 0 && stripped[0] == '.' {
		stripped = stripped[1:]
	}
	if stripped == "" {
		return ""
	}

	upper := strings.ToUpper(stripped)

	// Find the LAST dot to split name.ext
	lastDot := strings.LastIndex(upper, ".")
	var namePart, extPart string
	if lastDot >= 0 {
		namePart = upper[:lastDot]
		extPart = upper[lastDot+1:]
	} else {
		namePart = upper
		extPart = ""
	}

	// Keep only GEMDOS-valid characters
	namePart = gemdosCleanChars(namePart)
	extPart = gemdosCleanChars(extPart)

	if namePart == "" && extPart == "" {
		return ""
	}
	if namePart == "" {
		namePart = "_"
	}

	// Truncate to 8.3 limits
	if len(namePart) > 8 {
		namePart = namePart[:8]
	}
	if len(extPart) > 3 {
		extPart = extPart[:3]
	}

	if extPart != "" {
		return namePart + "." + extPart
	}
	return namePart
}

// makeUnique83 generates a unique 8.3 name by appending ~N suffixes when the
// base name collides with an already-used name. Follows Windows LFN convention.
func makeUnique83(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}

	lastDot := strings.LastIndex(base, ".")
	namePart, extPart := base, ""
	if lastDot >= 0 {
		namePart = base[:lastDot]
		extPart = base[lastDot:] // includes the dot
	}

	for n := 1; n <= 999; n++ {
		suffix := fmt.Sprintf("~%d", n)
		maxLen := 8 - len(suffix)
		if maxLen < 1 {
			maxLen = 1
		}
		truncated := namePart
		if len(truncated) > maxLen {
			truncated = truncated[:maxLen]
		}
		candidate := truncated + suffix + extPart
		if !used[candidate] {
			return candidate
		}
	}
	return base // shouldn't happen
}

// gemdosCleanChars removes characters that are invalid in GEMDOS filenames.
func gemdosCleanChars(s string) string {
	var buf []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '!' || c == '#' || c == '$' || c == '%' || c == '&' || c == '(' || c == ')' || c == '@' || c == '^' || c == '~' {
			buf = append(buf, c)
		}
	}
	return string(buf)
}

var gemdosFuncNames = map[uint16]string{
	0x0E: "Dsetdrv", 0x19: "Dgetdrv", 0x1A: "Fsetdta", 0x2F: "Fgetdta",
	0x36: "Dfree", 0x39: "Dcreate", 0x3A: "Ddelete", 0x3B: "Dsetpath",
	0x3C: "Fcreate", 0x3D: "Fopen", 0x3E: "Fclose", 0x3F: "Fread",
	0x40: "Fwrite", 0x41: "Fdelete", 0x42: "Fseek", 0x43: "Fattrib",
	0x44: "Mxalloc", 0x47: "Dgetpath", 0x48: "Malloc", 0x49: "Mfree",
	0x4A: "Mshrink", 0x4B: "Pexec", 0x4C: "Pterm",
	0x4E: "Fsfirst", 0x4F: "Fsnext", 0x56: "Frename",
	0x57: "Fdatime",
}

func (g *GemdosInterceptor) logTrap(funcNum uint16, sp uint32) {
	name := gemdosFuncNames[funcNum]
	if name == "" {
		name = fmt.Sprintf("func_0x%02X", funcNum)
	}
	switch funcNum {
	case GEMDOS_FSETDTA:
		addr := g.cpu.Read32(sp + 2)
		fmt.Printf("[GEMDOS] %s dtaAddr=$%08X\n", name, addr)
		// After enumeration, the desktop restores the DTA. Fix WNODE chain
		// if needed (GCC 13 -mshort codegen bug leaves it unterminated).
		if g.fnodeBase != 0 {
			g.fixWnodeChain()
		}
	case GEMDOS_FSFIRST:
		fspecAddr := g.cpu.Read32(sp + 2)
		attr := g.cpu.Read16(sp + 6)
		fspec := g.readString(fspecAddr)
		fmt.Printf("[GEMDOS] %s \"%s\" attr=$%04X dta=$%08X\n", name, fspec, attr, g.dtaAddr)
	case 0x4A: // Mshrink: SP+2:W zero, SP+4:L addr, SP+8:L newsize
		addr := g.cpu.Read32(sp + 4)
		newsize := g.cpu.Read32(sp + 8)
		fmt.Printf("[GEMDOS] %s addr=$%08X newsize=%d\n", name, addr, newsize)
		// After our enumeration, this is dos_shrink(p_fbase, count*sizeof(FNODE))
		if g.searchActive && newsize > 0 {
			g.fnodeBase = addr
			g.fnodeSize = newsize
		}
	case 0x49: // Mfree
		addr := g.cpu.Read32(sp + 2)
		fmt.Printf("[GEMDOS] %s addr=$%08X\n", name, addr)
	case 0x44: // Mxalloc
		size := g.cpu.Read32(sp + 2)
		fmt.Printf("[GEMDOS] %s size=%d\n", name, size)
	case GEMDOS_DSETDRV:
		drv := g.cpu.Read16(sp + 2)
		fmt.Printf("[GEMDOS] %s drv=%d\n", name, drv)
	case GEMDOS_PEXEC:
		mode := g.cpu.Read16(sp + 2)
		fnAddr := g.cpu.Read32(sp + 4)
		fn := g.readString(fnAddr)
		fmt.Printf("[GEMDOS] %s mode=%d fname=%q\n", name, mode, fn)
	case GEMDOS_PTERM:
		code := g.cpu.Read16(sp + 2)
		fmt.Printf("[GEMDOS] %s code=%d\n", name, code)
	case 0x00:
		fmt.Printf("[GEMDOS] Pterm0\n")
	case GEMDOS_FSNEXT:
		// Don't spam
	default:
		// Skip noisy functions
	}
}

// fixWnodeChain works around a GCC 13 -mshort codegen bug in EmuTOS's
// win_start(). The function initializes 7 WNODEs in a linked list but the
// compiled code for `(pw-1)->w_next = NULL` uses displacement $10804 instead
// of $0804 (off by $10000), so the last WNODE's w_next retains the loop value
// pointing one-past-end — right into the FNODE array that follows on the heap.
// We detect and patch this after the desktop's directory enumeration completes.
func (g *GemdosInterceptor) fixWnodeChain() {
	base := g.fnodeBase
	if base == 0 || g.fnodeSize == 0 || g.returnedCount <= 0 {
		return
	}

	const wnodeSize uint32 = 342      // sizeof(WNODE) in EmuTOS with -mshort
	lastWnodeAddr := base - wnodeSize // WNODE[6] should be right before FNODE array
	if lastWnodeAddr < 0x1000 || lastWnodeAddr >= base {
		return
	}

	lastWnext := g.cpu.Read32(lastWnodeAddr)
	if lastWnext == 0 {
		return // Already NULL-terminated, no fix needed
	}
	if lastWnext != base {
		return // Not the expected corrupt value
	}

	// Verify this is really a WNODE array by checking WNODE[5].w_next → WNODE[6]
	prevWnodeAddr := lastWnodeAddr - wnodeSize
	if g.cpu.Read32(prevWnodeAddr) != lastWnodeAddr {
		return
	}

	fmt.Printf("[GEMDOS] WNODE chain fix: WNODE[6] @$%08X w_next=$%08X → NULL (GCC -mshort codegen workaround)\n",
		lastWnodeAddr, lastWnext)
	g.cpu.Write32(lastWnodeAddr, 0)
}

// gemdosPatternToFCB converts a GEMDOS wildcard pattern (e.g. "*.TXT") to
// an 11-byte FCB format (8 name + 3 ext, space-padded, '?' for wildcards).
func gemdosPatternToFCB(pattern string) []byte {
	fcb := []byte("           ") // 11 spaces
	if pattern == "*.*" || pattern == "*" {
		for i := range fcb {
			fcb[i] = '?'
		}
		return fcb
	}
	dot := strings.LastIndex(pattern, ".")
	var namePart, extPart string
	if dot >= 0 {
		namePart = pattern[:dot]
		extPart = pattern[dot+1:]
	} else {
		namePart = pattern
	}
	// Fill name part (0-7)
	for i := 0; i < 8 && i < len(namePart); i++ {
		if namePart[i] == '*' {
			for j := i; j < 8; j++ {
				fcb[j] = '?'
			}
			break
		}
		fcb[i] = namePart[i]
	}
	// Fill extension part (8-10)
	for i := 0; i < 3 && i < len(extPart); i++ {
		if extPart[i] == '*' {
			for j := i; j < 3; j++ {
				fcb[8+j] = '?'
			}
			break
		}
		fcb[8+i] = extPart[i]
	}
	return fcb
}

// gemdosWildcardMatch matches a GEMDOS wildcard pattern against a filename.
// Both pattern and name should be uppercase. In GEMDOS, "*.*" matches all
// files including those without extensions.
func gemdosWildcardMatch(pattern, name string) bool {
	if pattern == "*.*" || pattern == "*" {
		return true
	}
	return wildcardMatch(pattern, name, 0, 0)
}

func wildcardMatch(pattern, name string, pi, ni int) bool {
	for pi < len(pattern) {
		if ni >= len(name) {
			// Remaining pattern must all be *
			for pi < len(pattern) {
				if pattern[pi] != '*' {
					return false
				}
				pi++
			}
			return true
		}
		switch pattern[pi] {
		case '*':
			// Try matching rest of pattern at every position
			for ni <= len(name) {
				if wildcardMatch(pattern, name, pi+1, ni) {
					return true
				}
				ni++
			}
			return false
		case '?':
			pi++
			ni++
		default:
			if pattern[pi] != name[ni] {
				return false
			}
			pi++
			ni++
		}
	}
	return ni >= len(name)
}
