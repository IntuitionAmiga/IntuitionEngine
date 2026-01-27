// music_interfaces.go - Common interfaces for music players and parsers

package main

// MusicFile is implemented by all parsed music file types
// Each format implements this with format-specific metadata and data access
type MusicFile interface {
	// GetMetadata returns common metadata fields
	GetMetadata() MusicMetadata
	// GetData returns the raw music data (format-specific)
	GetData() []byte
}

// MusicPlayer is implemented by all music players
// Provides a common interface for playback control
type MusicPlayer interface {
	// Load loads a music file from the given path
	Load(path string) error
	// LoadData loads music data from a byte slice
	LoadData(data []byte) error
	// Play starts playback
	Play()
	// Stop stops playback
	Stop()
	// IsPlaying returns true if currently playing
	IsPlaying() bool
	// DurationSeconds returns the duration in seconds (0 if looping/unknown)
	DurationSeconds() float64
	// DurationText returns a formatted duration string (e.g., "3:45")
	DurationText() string
}

// RegisterMappedPlayer extends MusicPlayer with memory-mapped register control
// Used by players that can be controlled from running programs via MMIO
type RegisterMappedPlayer interface {
	MusicPlayer
	// AttachBus attaches the memory bus for reading embedded music data
	AttachBus(bus MemoryBus)
	// HandlePlayWrite handles writes to play control registers
	HandlePlayWrite(addr uint32, value uint32)
	// HandlePlayRead handles reads from play control registers
	HandlePlayRead(addr uint32) uint32
}
