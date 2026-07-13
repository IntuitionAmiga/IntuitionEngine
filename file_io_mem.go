// file_io_mem.go - in-memory disk volume for the FileIO device.
//
// BASIC's LOAD/BLOAD/SAVE/DIR go through the FileIO device against a host
// directory. The js/wasm browser build has no host filesystem, so on wasm the
// device runs against an in-memory volume seeded at boot (from the server's
// assets folder, fetched over HTTP). Lookups are case-insensitive (matching the
// native device's caseInsensitiveReadPath) but the original file names are
// preserved for DIR, so the listing mirrors the real assets folder. The code is
// pure Go, so it compiles and is testable on every platform.

package main

import (
	"sort"
	"strings"
)

// fileIOMemName strips a leading "./" or "/" and normalises separators, keeping
// the original case for display.
func fileIOMemName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimPrefix(name, "/")
	return name
}

// fileIOMemKey is the canonical, case-insensitive lookup key for a name.
func fileIOMemKey(name string) string {
	return strings.ToLower(fileIOMemName(name))
}

// NewMemoryFileIODevice constructs a FileIO device backed by an in-memory
// volume. Seed files with SetMemFile; guest writes land back in the same maps.
func NewMemoryFileIODevice(bus *MachineBus) *FileIODevice {
	return &FileIODevice{
		bus:      bus,
		memFS:    true,
		memFiles: make(map[string][]byte),
		memNames: make(map[string]string),
	}
}

func (f *FileIODevice) ensureMemMaps() {
	if f.memFiles == nil {
		f.memFiles = make(map[string][]byte)
	}
	if f.memNames == nil {
		f.memNames = make(map[string]string)
	}
	if f.memImported == nil {
		f.memImported = make(map[string]bool)
	}
}

// putMemFile caches data under the canonical key and records the original name.
// Used by SAVE and by any eager seeding.
func (f *FileIODevice) putMemFile(name string, data []byte) {
	f.ensureMemMaps()
	key := fileIOMemKey(name)
	f.memFiles[key] = append([]byte(nil), data...)
	f.memNames[key] = fileIOMemName(name)
}

// RegisterMemPath records a known file path (from the boot manifest) without
// loading its contents; the bytes are fetched lazily on first read.
func (f *FileIODevice) RegisterMemPath(name string) {
	f.ensureMemMaps()
	f.memNames[fileIOMemKey(name)] = fileIOMemName(name)
}

// SetMemFile adds or replaces a file in the in-memory volume, preserving the
// given name's case for DIR. The entry is marked imported so it wins base-name
// resolution over a bundled asset of the same basename (the visitor-import path).
func (f *FileIODevice) SetMemFile(name string, data []byte) {
	f.putMemFile(name, data)
	f.memImported[fileIOMemKey(name)] = true
}

// DeleteMemFile removes whatever entry a read of name would resolve to (exact
// path suffix first, then base name), dropping both its cached bytes and its
// registration. Returns true if an entry was removed. The browser save flow
// uses this to clear an existing file before issuing SAVE, so a poll for the
// result cannot observe the pre-SAVE bytes on an overwrite.
func (f *FileIODevice) DeleteMemFile(name string) bool {
	if f == nil {
		return false
	}
	key, ok := f.resolveMemKey(name)
	if !ok {
		return false
	}
	delete(f.memFiles, key)
	delete(f.memNames, key)
	delete(f.memImported, key)
	return true
}

// candidateKeys returns the canonical keys to try for a name, from the full
// relative path down to the base name. This lets a host-style path that the
// Program Executor or launcher resolved against a base directory
// (e.g. "/run/Demos/ie32/x.iex") still match the manifest's relative key
// ("demos/ie32/x.iex") by suffix.
func (f *FileIODevice) candidateKeys(name string) []string {
	n := strings.ReplaceAll(fileIOMemName(name), "\\", "/")
	segs := strings.Split(n, "/")
	keys := make([]string, 0, len(segs))
	for i := range segs {
		if suffix := strings.Join(segs[i:], "/"); suffix != "" {
			keys = append(keys, strings.ToLower(suffix))
		}
	}
	return keys
}

// pathBaseSlash returns the last segment of a forward-slash path.
func pathBaseSlash(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// resolveMemKey maps a requested name to a known canonical key. A visitor-
// imported entry wins first (so an imported flat basename overrides a bundled
// asset of the same basename, even the exact registered nested path it replaces),
// then an exact path suffix, then by base name anywhere in the tree so a user can
// RUN/LOAD a demo without typing its CPU subfolder. Matches are sorted for
// determinism.
func (f *FileIODevice) resolveMemKey(name string) (string, bool) {
	if f == nil {
		return "", false
	}
	cands := f.candidateKeys(name)
	// An imported file (uploaded by the visitor) takes priority over a bundled
	// asset, regardless of the request naming the bundled nested path.
	for _, key := range cands {
		if f.memImported[key] {
			return key, true
		}
	}
	for _, key := range cands {
		if _, ok := f.memNames[key]; ok {
			return key, true
		}
	}
	base := strings.ToLower(pathBaseSlash(fileIOMemName(name)))
	if base == "" {
		return "", false
	}
	var imported, matches []string
	for key := range f.memNames {
		if pathBaseSlash(key) == base { // keys are already lower-case
			matches = append(matches, key)
			if f.memImported[key] {
				imported = append(imported, key)
			}
		}
	}
	if len(imported) > 0 {
		sort.Strings(imported)
		return imported[0], true
	}
	if len(matches) == 0 {
		return "", false
	}
	sort.Strings(matches)
	return matches[0], true
}

// readMemFile returns the bytes for a name (case-insensitive), fetching them
// lazily via memFetch when the path is known but not yet cached. Used by both
// doReadMem and hostReadFile.
func (f *FileIODevice) readMemFile(name string) ([]byte, bool) {
	key, ok := f.resolveMemKey(name)
	if !ok {
		return nil, false
	}
	return f.loadMemKey(key)
}

// memPathKnown reports whether a name maps to a known file, without loading it.
// Used by hostStatExists for the executor's pre-check.
func (f *FileIODevice) memPathKnown(name string) bool {
	_, ok := f.resolveMemKey(name)
	return ok
}

// loadMemKey returns cached bytes for a canonical key, or lazily fetches them if
// the key is a known path and a fetcher is set.
func (f *FileIODevice) loadMemKey(key string) ([]byte, bool) {
	if data, ok := f.memFiles[key]; ok {
		return data, true
	}
	orig, known := f.memNames[key]
	if !known || f.memFetch == nil {
		return nil, false
	}
	data, ok := f.memFetch(orig)
	if !ok {
		return nil, false
	}
	f.ensureMemMaps()
	f.memFiles[key] = data
	return data, true
}

// lookupMem is the accessor hostReadFile uses (nil-safe on native).
func (f *FileIODevice) lookupMem(name string) ([]byte, bool) {
	return f.readMemFile(name)
}

// doReadMem serves FILE_OP_READ from the in-memory volume (case-insensitive),
// lazily fetching the bytes on first access.
func (f *FileIODevice) doReadMem(rawName string, readMax uint32) {
	data, ok := f.readMemFile(rawName)
	if !ok {
		f.fileStatus = 1
		f.fileErrorCode = FILE_ERR_NOT_FOUND
		f.fileResultLen = 0
		return
	}
	f.writeReadResult(data, rawName, "<memfs>", readMax)
}

// doListMem serves FILE_OP_LIST from the in-memory volume. It lists the direct
// children of the requested directory (root when empty), with subdirectories
// shown with a trailing "/" like the native device, sorted case-insensitively,
// using the same CRLF-delimited, NUL-terminated format and range guard as
// doList.
func (f *FileIODevice) doListMem() {
	dir := f.readFileName()
	entries := f.memDirEntries(dir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.isDir {
			names = append(names, e.name+"/")
		} else {
			names = append(names, e.name)
		}
	}
	data := []byte(strings.Join(names, "\r\n"))
	if len(data) > 0 {
		data = append(data, '\r', '\n')
	}
	if end := uint64(f.fileDataPtr) + uint64(len(data)) + 1; end > busMemMaxBytes || end > f.bus.backingVisibleSize() {
		f.fileStatus = 1
		f.fileErrorCode = FILE_ERR_RANGE
		f.fileResultLen = 0
		return
	}
	for i, b := range data {
		f.writeFileData8(f.fileDataPtr+uint64(i), b)
	}
	f.writeFileData8(f.fileDataPtr+uint64(len(data)), 0)
	f.fileStatus = 0
	f.fileErrorCode = FILE_ERR_OK
	f.fileResultLen = uint32(len(data))
}

// memDirEntries returns the direct children of dir within the in-memory volume,
// using the original file-name case. A path segment that has deeper entries
// beneath it is reported as a directory. (memDirEntry is defined in
// bootstrap_hostfs_mem.go.)
func (f *FileIODevice) memDirEntries(dir string) []memDirEntry {
	base := fileIOMemKey(dir) // lower-case, slash-normalised, no leading ./ or /
	root := base == "" || base == "."
	prefix := ""
	if !root {
		prefix = base + "/"
	}
	// child key (lower) -> entry (original-case name + isDir). A directory wins
	// over a same-named file.
	seen := make(map[string]memDirEntry)
	for _, orig := range f.memNames {
		rel := orig // original relative path
		if !root {
			// Case-insensitive prefix match; byte lengths are equal because
			// lower-casing does not change length.
			if !strings.HasPrefix(strings.ToLower(rel), prefix) {
				continue
			}
			rel = rel[len(prefix):]
		}
		if rel == "" {
			continue
		}
		name := rel
		isDir := false
		if i := strings.IndexByte(rel, '/'); i >= 0 {
			name = rel[:i]
			isDir = true
		}
		key := strings.ToLower(name)
		if cur, ok := seen[key]; !ok || (isDir && !cur.isDir) {
			seen[key] = memDirEntry{name: name, isDir: isDir}
		}
	}
	out := make([]memDirEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].name) < strings.ToLower(out[j].name)
	})
	return out
}
