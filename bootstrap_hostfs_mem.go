// bootstrap_hostfs_mem.go - in-memory store for the BootstrapHostFS device.
//
// The js/wasm build has no host filesystem, so the hostfs device runs against
// the in-memory specials store instead of os. Reads and stat already consult
// specials first (see open/read/stat in bootstrap_hostfs.go); this file adds
// the write and directory-listing halves so the full LOAD/BLOAD/SAVE/WRITE/
// STAT/READDIR surface works without os. The code is pure Go (no os, no
// syscall), so it compiles and is testable on every platform, not only wasm.

package main

import (
	"os"
	"path"
	"sort"
	"strings"
)

// bootstrapHostFSMemWriteHandle is an open-for-write handle whose bytes are
// buffered in memory and committed to the specials store on close.
type bootstrapHostFSMemWriteHandle struct {
	name string
	buf  []byte
}

// NewMemoryHostFSDevice constructs a hostfs device backed entirely by the
// in-memory specials store. Seed files with SetSpecialFile before or after
// construction; guest writes land back in the same store. Used by the js/wasm
// browser build, which has no host filesystem.
func NewMemoryHostFSDevice(bus *MachineBus) *BootstrapHostFSDevice {
	return &BootstrapHostFSDevice{
		bus:             bus,
		available:       true,
		memFS:           true,
		nextHandle:      1,
		handles:         make(map[uint32]*os.File),
		memHandles:      make(map[uint32]*bootstrapHostFSMemHandle),
		memWriteHandles: make(map[uint32]*bootstrapHostFSMemWriteHandle),
		specials:        make(map[string][]byte),
	}
}

// validateMemCreatePath applies the same lexical rejection as the native
// resolveForCreate, in the same order and with the same error codes: empty,
// "." or "/" is a bad argument (3); absolute paths and any traversal segment
// are access violations (5). The store has no host root to escape, but the
// guest-visible contract must match native HostFS.
func validateMemCreatePath(rel string) uint32 {
	clean := path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if clean == "." || clean == "/" || clean == "" {
		return 3
	}
	if strings.HasPrefix(clean, "/") {
		return 5
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." || seg == "." {
			return 5
		}
	}
	return 0
}

// memDirKnown reports whether rel names a synthetic directory in the specials
// store: the root, or a case-insensitive prefix of at least one deeper key.
// Used by STAT so directories that READDIR lists also stat as KIND_DIR.
func (d *BootstrapHostFSDevice) memDirKnown(rel string) bool {
	key := specialFileKey(rel)
	if key == "." || key == "/" || key == "" {
		return true
	}
	prefix := key + "/"
	for k := range d.specials {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// readDirMem serves BOOT_HOSTFS_READDIR from the specials store: it lists the
// direct children of the requested directory (root when the path is empty,
// "." or "/"), sorted case-insensitively, and emits the entry at index arg2.
func (d *BootstrapHostFSDevice) readDirMem() {
	if d.arg3 == 0 {
		d.err = 3
		return
	}
	dir := d.readCString(d.arg1, 255)
	entries := d.memDirEntries(dir)
	idx := int(d.arg2)
	if idx < 0 || idx >= len(entries) {
		d.err = 4
		return
	}
	entry := entries[idx]
	kind := uint32(BOOT_HOSTFS_KIND_FILE)
	if entry.isDir {
		kind = BOOT_HOSTFS_KIND_DIR
	}
	if !d.writeGuest64(d.arg3+BOOT_HOSTFS_DIRENT_KIND_OFF, uint64(kind)) {
		d.err = 5
		return
	}
	if !d.writeGuestName(d.arg3+BOOT_HOSTFS_DIRENT_NAME_OFF, entry.name) {
		d.err = 5
		return
	}
	d.res1 = 1
	d.res2 = kind
}

// memDirEntry is one child of a directory listing: a file or a synthesised
// virtual directory (a name that has deeper keys beneath it).
type memDirEntry struct {
	name  string
	isDir bool
}

// memDirEntries returns the direct children of dir within the specials store,
// using the canonical (uppercase, slash-separated) key form. A name that also
// prefixes a deeper key is reported as a directory, so guests can browse into
// embedded directory trees; a directory always wins over a same-named file.
func (d *BootstrapHostFSDevice) memDirEntries(dir string) []memDirEntry {
	base := specialFileKey(dir)
	root := base == "." || base == "/" || base == ""
	prefix := ""
	if !root {
		prefix = base + "/"
	}
	isDir := make(map[string]bool)
	for key := range d.specials {
		rest := key
		if !root {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			rest = key[len(prefix):]
		}
		if rest == "" {
			continue
		}
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			// A deeper path exists under this name: it is a directory.
			isDir[rest[:i]] = true
		} else if _, ok := isDir[rest]; !ok {
			// A leaf key: a file, unless a directory of the same name was
			// (or will be) recorded, in which case the directory wins.
			isDir[rest] = false
		}
	}
	out := make([]memDirEntry, 0, len(isDir))
	for name, dir := range isDir {
		out = append(out, memDirEntry{name: name, isDir: dir})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].name) < strings.ToLower(out[j].name)
	})
	return out
}
