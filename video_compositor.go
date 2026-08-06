// video_compositor.go - Video Compositor for Intuition Engine

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine

License: GPLv3 or later
*/

/*
video_compositor.go - Video Compositor for Multiple Video Sources

This module implements a compositor that blends multiple video sources for display:
- Collects frames from registered VideoSource implementations
- Composites frames based on layer order (z-order)
- Outputs the final blended frame to the display

Signal Flow:
1. Video sources (VideoChip, VGA, future cards) register with compositor
2. Compositor runs at 60Hz refresh rate
3. Each frame, compositor collects frames from all enabled sources
4. Frames are blended in layer order (higher layer on top)
5. Final frame is sent to VideoOutput

Architecture:
                    ┌─────────────┐
  CPU → VGA VRAM → │   VideoVGA  │ ──┐
                    └─────────────┘   │     ┌─────────────┐     ┌─────────┐
                                      ├───→ │ Compositor  │ ──→ │ Display │
                    ┌─────────────┐   │     └─────────────┘     └─────────┘
  CPU → Chip VRAM → │  VideoChip  │ ──┘
                    └─────────────┘
*/

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Compositor constants
const (
	COMPOSITOR_REFRESH_RATE     = 60
	COMPOSITOR_REFRESH_INTERVAL = time.Second / COMPOSITOR_REFRESH_RATE
)

type compositorState int

const (
	compositorStopped compositorState = iota
	compositorRunning
	compositorStopping
	compositorClosed
)

type PresentationScaleMode int

const (
	ScaleAspectFit PresentationScaleMode = iota
	ScaleStretchFill
)

type registeredSource struct {
	id          uint64
	source      VideoSource
	lastEnabled bool
}

type videoScheduledTask struct {
	id   uint64
	tick func()
}

type VideoScheduler struct {
	mu       sync.Mutex
	interval time.Duration
	tasks    []videoScheduledTask
	nextID   uint64
	done     chan struct{}
	loopDone chan struct{}
	running  bool
	manual   bool
}

func NewVideoScheduler(interval time.Duration) *VideoScheduler {
	return &VideoScheduler{interval: interval}
}

func NewManualVideoScheduler() *VideoScheduler {
	return &VideoScheduler{manual: true}
}

func (s *VideoScheduler) Register(tick func()) uint64 {
	if tick == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := s.nextID
	s.tasks = append(s.tasks, videoScheduledTask{id: id, tick: tick})
	return id
}

func (s *VideoScheduler) Unregister(id uint64) {
	if id == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, task := range s.tasks {
		if task.id == id {
			copy(s.tasks[i:], s.tasks[i+1:])
			s.tasks = s.tasks[:len(s.tasks)-1]
			return
		}
	}
}

func (s *VideoScheduler) Start() {
	s.mu.Lock()
	if s.running || s.manual {
		s.mu.Unlock()
		return
	}
	interval := s.interval
	if interval <= 0 {
		interval = COMPOSITOR_REFRESH_INTERVAL
	}
	s.done = make(chan struct{})
	s.loopDone = make(chan struct{})
	done := s.done
	loopDone := s.loopDone
	s.running = true
	s.mu.Unlock()
	go s.loop(interval, done, loopDone)
}

func (s *VideoScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	done := s.done
	loopDone := s.loopDone
	s.done = nil
	s.loopDone = nil
	s.running = false
	s.mu.Unlock()
	close(done)
	<-loopDone
}

func (s *VideoScheduler) TickManual() {
	s.tickAll()
}

func (s *VideoScheduler) loop(interval time.Duration, done <-chan struct{}, loopDone chan<- struct{}) {
	defer close(loopDone)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s.tickAll()
		}
	}
}

func (s *VideoScheduler) tickAll() {
	s.mu.Lock()
	tasks := make([]videoScheduledTask, len(s.tasks))
	copy(tasks, s.tasks)
	s.mu.Unlock()
	for _, task := range tasks {
		safeCall("VideoScheduler.Tick", task.tick)
	}
}

// VideoCompositor blends multiple video sources into a single output
type VideoCompositor struct {
	mu                  sync.Mutex
	outputMu            sync.Mutex
	output              VideoOutput
	sources             []registeredSource
	nextSourceID        uint64
	finalFrame          []byte
	outputBuf           []byte
	onFrameComplete     func()      // pixel consumer: composed ticks only
	onFrameTiming       func()      // logical-frame/timing: every tick, incl. skipped
	pixelConsumerActive func() bool // when true, a live pixel consumer disables the skip
	done                chan struct{}
	frameWidth          int
	frameHeight         int
	scaleMode           PresentationScaleMode
	pendingResolution   atomic.Uint64
	lockedResolution    bool
	prevHasContent      bool
	frameCounter        uint64
	frameTimestamp      time.Time
	scheduler           *VideoScheduler
	schedulerTaskID     uint64
	hardwareDisabled    bool
	lastHardwareFrame   uint64
	lastHardwareLayers  []CompositorFrameLayer
	frameLeaseRings     map[uint64]*VideoFrameLeaseRing
	frameLeaseBytes     map[uint64]int
	lastSourceGens      map[uint64]uint64
	skipStreak          uint64
	softwareFrameRing   *VideoFrameLeaseRing
	softwareFrameBytes  int
	finalFrameLease     *VideoFrameLease
	lastSnapshotFrame   uint64
	lastSnapshot        []byte
	blendJobs           chan blendStripJob
	blendWorkerStop     chan struct{}
	blendWorkerWG       sync.WaitGroup
	softwareDirtyRects  []FrameDirtyRect
	forceFullFrame      bool

	// Tile-based retained composition state. tileSlotFrame records, per
	// software lease slot, the frame whose complete composite that slot's
	// pixels still hold; tileHistory records which tiles each recent frame
	// repainted. See video_compositor_tiles.go.
	tileSlotFrame   []uint64
	tileHistory     []tileDirtyEntry
	tileLayerSig    uint64
	tileGridW       int
	tileGridH       int
	tileStats       tileCompositeStats
	tileSerialWS    tileWorkspace
	tileCurBits     []uint64
	tileNeedBits    []uint64
	tilePlans       []tileLayerPlan
	tileRectScratch []FrameDirtyRect

	// indexedScratch holds reusable index storage per source for layers that
	// go to the backend as palette indices rather than RGBA.
	indexedScratch map[uint64]*IndexedLayerData

	// uploadPlan owns the regional upload plan and its packing scratch.
	// Guarded by outputMu, which is the lock held across updateOutput.
	uploadPlan uploadPlanner

	// resamplePlans caches horizontal resample plans by source and
	// destination width, so a steady scale allocates nothing per frame.
	resamplePlans map[uint64]*resamplePlan
	scaledRowBuf  []byte

	compositorRunning atomic.Bool
	state             compositorState
	stopRequested     bool
	loopDone          chan struct{}
}

// crtPresentationController is deliberately narrower than VideoOutput. It
// lets IEScript control the host presentation effect without exposing backend
// implementation details to guest-facing video APIs.
type crtPresentationController interface {
	crtIsRequested() bool
	setCRTRequested(bool)
	toggleCRTRequested() bool
}

// crtPresentationModeController exposes the full host presentation cycle to
// IEScript. The string boundary keeps this optional controller available to
// headless builds, which do not compile the Ebiten CRT mode type.
type crtPresentationModeController interface {
	crtModeRequested() string
	setCRTModeRequested(string) bool
	cycleCRTModeRequested() string
}

type presentationScreenshotOutput interface {
	TakePresentationScreenshot(path string) error
}

type compositionScreenshotOutput interface {
	TakeCompositionScreenshot(path string) error
}

func (c *VideoCompositor) CRTEnabled() (bool, bool) {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	controller, ok := c.output.(crtPresentationController)
	if !ok {
		return false, false
	}
	return controller.crtIsRequested(), true
}

func (c *VideoCompositor) SetCRTEnabled(enabled bool) bool {
	c.outputMu.Lock()
	controller, ok := c.output.(crtPresentationController)
	c.outputMu.Unlock()
	if !ok {
		return false
	}
	controller.setCRTRequested(enabled)
	c.mu.Lock()
	c.forceFullFrame = true
	c.mu.Unlock()
	return true
}

func (c *VideoCompositor) ToggleCRT() (bool, bool) {
	c.outputMu.Lock()
	controller, ok := c.output.(crtPresentationController)
	c.outputMu.Unlock()
	if !ok {
		return false, false
	}
	enabled := controller.toggleCRTRequested()
	c.mu.Lock()
	c.forceFullFrame = true
	c.mu.Unlock()
	return enabled, true
}

func (c *VideoCompositor) CRTMode() (string, bool) {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	controller, ok := c.output.(crtPresentationModeController)
	if !ok {
		return "", false
	}
	return controller.crtModeRequested(), true
}

func (c *VideoCompositor) SetCRTMode(mode string) bool {
	c.outputMu.Lock()
	controller, ok := c.output.(crtPresentationModeController)
	c.outputMu.Unlock()
	if !ok || !controller.setCRTModeRequested(mode) {
		return false
	}
	c.mu.Lock()
	c.forceFullFrame = true
	c.mu.Unlock()
	return true
}

func (c *VideoCompositor) CycleCRTMode() (string, bool) {
	c.outputMu.Lock()
	controller, ok := c.output.(crtPresentationModeController)
	c.outputMu.Unlock()
	if !ok {
		return "", false
	}
	mode := controller.cycleCRTModeRequested()
	c.mu.Lock()
	c.forceFullFrame = true
	c.mu.Unlock()
	return mode, true
}

// RequestFullComposite invalidates the unchanged-frame fast path after a host
// presentation mode transition changes the selected compositor backend.
func (c *VideoCompositor) RequestFullComposite() {
	c.mu.Lock()
	c.forceFullFrame = true
	c.mu.Unlock()
}

// TakePresentationScreenshot captures the next displayed frame when the
// selected video output supports final-presentation capture.
func (c *VideoCompositor) TakePresentationScreenshot(path string) error {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	output, ok := c.output.(presentationScreenshotOutput)
	if !ok {
		return fmt.Errorf("presentation screenshot unavailable")
	}
	return output.TakePresentationScreenshot(path)
}

// TakeCompositionScreenshot captures the GPU composition before presentation
// filtering. It is a diagnostic counterpart to TakePresentationScreenshot.
func (c *VideoCompositor) TakeCompositionScreenshot(path string) error {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	output, ok := c.output.(compositionScreenshotOutput)
	if !ok {
		return fmt.Errorf("composition screenshot unavailable")
	}
	return output.TakeCompositionScreenshot(path)
}

type blendStripJob struct {
	srcFrame []byte
	width    int
	startY   int
	endY     int
	wg       *sync.WaitGroup
}

// NewVideoCompositor creates a new video compositor
func NewVideoCompositor(output VideoOutput) *VideoCompositor {
	return &VideoCompositor{
		output:      output,
		sources:     make([]registeredSource, 0),
		done:        make(chan struct{}),
		frameWidth:  DefaultPresentationWidth,
		frameHeight: DefaultPresentationHeight,
		scaleMode:   ScaleStretchFill,
		scheduler:   NewVideoScheduler(COMPOSITOR_REFRESH_INTERVAL),
	}
}

// RegisterSource adds a video source to the compositor
func (c *VideoCompositor) RegisterSource(source VideoSource) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registerSourceLocked(source)
}

// RegisterSourceWithID adds a video source and returns its unregister handle.
func (c *VideoCompositor) RegisterSourceWithID(source VideoSource) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registerSourceLocked(source)
}

func (c *VideoCompositor) registerSourceLocked(source VideoSource) uint64 {
	c.nextSourceID++
	id := c.nextSourceID
	c.sources = append(c.sources, registeredSource{id: id, source: source, lastEnabled: source != nil && source.IsEnabled()})
	c.forceFullFrame = true
	c.sortSourcesByLayerLocked()
	return id
}

func (c *VideoCompositor) sortSourcesByLayerLocked() {
	sort.SliceStable(c.sources, func(i, j int) bool {
		return c.sources[i].source.GetLayer() < c.sources[j].source.GetLayer()
	})
}

// UnregisterSource removes a source by the id returned from RegisterSourceWithID.
func (c *VideoCompositor) UnregisterSource(id uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.sources {
		if c.sources[i].id == id {
			copy(c.sources[i:], c.sources[i+1:])
			c.sources[len(c.sources)-1] = registeredSource{}
			c.sources = c.sources[:len(c.sources)-1]
			c.forceFullFrame = true
			return true
		}
	}
	return false
}

// SetDimensions sets the output frame dimensions
func (c *VideoCompositor) SetDimensions(width, height int) {
	c.mu.Lock()
	if c.lockedResolution {
		c.mu.Unlock()
		return
	}
	cfg, out, changed := c.prepareResolutionLocked(width, height)
	c.mu.Unlock()
	c.applyDisplayConfig(out, cfg, changed)
}

func (c *VideoCompositor) NotifyResolutionChange(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	// A zero packed value is a safe "no pending resolution" sentinel because
	// this API rejects non-positive dimensions before packing.
	packed := (uint64(uint32(width)) << 32) | uint64(uint32(height))
	c.pendingResolution.Store(packed)
}

func (c *VideoCompositor) LockResolution(width, height int) {
	c.mu.Lock()
	c.lockedResolution = true
	cfg, out, changed := c.prepareResolutionLocked(width, height)
	c.mu.Unlock()
	c.applyDisplayConfig(out, cfg, changed)
}

func (c *VideoCompositor) UnlockResolution() {
	c.mu.Lock()
	c.lockedResolution = false
	c.mu.Unlock()
}

func (c *VideoCompositor) prepareResolutionLocked(width, height int) (DisplayConfig, VideoOutput, bool) {
	var cfg DisplayConfig
	if width <= 0 || height <= 0 {
		return cfg, nil, false
	}
	if width == c.frameWidth && height == c.frameHeight {
		return cfg, nil, false
	}
	c.frameWidth = width
	c.frameHeight = height
	c.clearSoftwareFrameLeaseLocked()
	c.softwareFrameRing = nil
	c.softwareFrameBytes = 0
	c.finalFrame = make([]byte, width*height*BYTES_PER_PIXEL)
	c.outputBuf = make([]byte, width*height*BYTES_PER_PIXEL)
	c.hardwareDisabled = false
	c.forceFullFrame = true
	c.invalidateTileStateLocked()
	c.clearHardwareSnapshotLocked()

	if c.output != nil {
		cfg = c.output.GetDisplayConfig()
		cfg.Width = width
		cfg.Height = height
		return cfg, c.output, true
	}
	return cfg, nil, false
}

func (c *VideoCompositor) applyDisplayConfig(out VideoOutput, cfg DisplayConfig, changed bool) {
	if !changed || out == nil {
		return
	}
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	if err := out.SetDisplayConfig(cfg); err != nil {
		fmt.Printf("Compositor: Error applying display config: %v\n", err)
	}
}

// Start begins the compositor refresh loop
func (c *VideoCompositor) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == compositorClosed {
		return errors.New("video compositor is closed")
	}
	if c.state == compositorRunning || c.state == compositorStopping {
		return nil
	}
	if c.finalFrame == nil {
		c.finalFrame = make([]byte, c.frameWidth*c.frameHeight*BYTES_PER_PIXEL)
	}
	if len(c.outputBuf) != len(c.finalFrame) {
		c.outputBuf = make([]byte, len(c.finalFrame))
	}
	c.done = make(chan struct{})
	c.stopRequested = false
	loopDone := make(chan struct{})
	c.loopDone = loopDone
	c.compositorRunning.Store(true)
	c.state = compositorRunning
	c.schedulerTaskID = c.scheduler.Register(c.composite)
	c.scheduler.Start()
	go func() {
		defer func() {
			c.scheduler.Stop()
			c.scheduler.Unregister(c.schedulerTaskID)
			c.mu.Lock()
			if c.state == compositorStopping {
				c.state = compositorStopped
			}
			c.compositorRunning.Store(false)
			c.mu.Unlock()
			close(loopDone)
		}()
		<-c.done
	}()
	return nil
}

// Stop halts the compositor refresh loop and waits for it to exit.
func (c *VideoCompositor) Stop() {
	c.mu.Lock()
	if c.state == compositorStopping {
		loopDone := c.loopDone
		c.mu.Unlock()
		if loopDone != nil {
			<-loopDone
		}
		return
	}
	if c.state != compositorRunning {
		c.mu.Unlock()
		return
	}
	if !c.stopRequested {
		c.state = compositorStopping
		c.stopRequested = true
		close(c.done)
	}
	loopDone := c.loopDone
	c.mu.Unlock()
	<-loopDone
}

// Close stops the compositor and releases registered source references.
func (c *VideoCompositor) Close() error {
	c.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopBlendWorkersLocked()
	c.clearHardwareSnapshotLocked()
	c.clearSoftwareFrameLeaseLocked()
	c.state = compositorClosed
	c.sources = nil
	return nil
}

// composite collects and blends frames from all enabled sources
func (c *VideoCompositor) composite() {
	var perfT0 time.Time
	if perfAcctOn {
		perfT0 = time.Now()
		defer perfSubsysAcct.VideoFramePath.AddSince(perfT0, 1)
	}
	c.mu.Lock()

	if !c.lockedResolution {
		packed := c.pendingResolution.Swap(0)
		if packed != 0 {
			width := int(uint32(packed >> 32))
			height := int(uint32(packed))
			cfg, out, changed := c.prepareResolutionLocked(width, height)
			c.mu.Unlock()
			c.applyDisplayConfig(out, cfg, changed)
			c.mu.Lock()
		}
	}
	if c.finalFrame == nil {
		c.finalFrame = make([]byte, c.frameWidth*c.frameHeight*BYTES_PER_PIXEL)
	}
	if len(c.outputBuf) != len(c.finalFrame) {
		c.outputBuf = make([]byte, len(c.finalFrame))
	}

	for i := range c.sources {
		source := c.sources[i].source
		enabled := source.IsEnabled()
		if enabled != c.sources[i].lastEnabled {
			c.forceFullFrame = true
			c.sources[i].lastEnabled = enabled
		}
		if ticker, ok := source.(FrameTicker); ok {
			safeCall("TickFrame", ticker.TickFrame)
		}
	}

	// Frame-generation gate: if every enabled source tracks a frame
	// generation and none has advanced since the last composite, this
	// tick has no new pixels — skip the collect/copy/blend/upload work.
	// Source TickFrame calls above (VBlank edges, chip state) have
	// already run, so guest-visible timing is unaffected.
	if c.canSkipUnchangedCompositeLocked() {
		// No new pixels this tick, but the logical frame still advances:
		// bump frame metadata the skip would otherwise bypass and deliver
		// the unconditional timing callback (frame count, VBL, frameChan
		// wake). The pixel consumer is deliberately NOT invoked.
		c.frameCounter++
		c.frameTimestamp = time.Now()
		cbTiming := c.onFrameTiming
		c.mu.Unlock()
		if cbTiming != nil {
			cbTiming()
		}
		return
	}

	useHardwareCompositor := c.canUseHardwareCompositorLocked()
	layers, hasContent := c.collectCompositeLayers(useHardwareCompositor)
	shouldOutput := hasContent || c.prevHasContent
	frameID := c.frameCounter + 1

	var outputFrame []byte
	var outputLease *VideoFrameLease
	var outputRegions []FrameDirtyRect
	forceFullFrame := c.forceFullFrame
	var hwUpdate *CompositorFrameUpdate
	if shouldOutput && useHardwareCompositor {
		hwUpdate = &CompositorFrameUpdate{
			FrameID:            frameID,
			PresentationWidth:  c.frameWidth,
			PresentationHeight: c.frameHeight,
			HasContent:         hasContent,
			Layers:             layers,
		}
		c.storeHardwareSnapshotLayersLocked(frameID, layers)
	} else {
		c.renderLayersSoftwareLocked(layers, frameID)
		c.clearHardwareSnapshotLocked()
		if hasContent {
			c.prevHasContent = true
			outputFrame, outputLease = c.prepareSoftwareOutputFrameLocked()
			if !forceFullFrame {
				outputRegions = cloneFrameDirtyRects(c.softwareDirtyRects)
			}
		} else if c.prevHasContent {
			c.prevHasContent = false
			outputFrame, outputLease = c.prepareSoftwareOutputFrameLocked()
			outputRegions = nil
		}
	}

	c.frameCounter = frameID
	c.frameTimestamp = time.Now()
	out := c.output
	cbTiming := c.onFrameTiming
	cbPixels := c.onFrameComplete
	c.mu.Unlock()
	defer releaseFrameLayerLeases(layers)
	defer func() {
		if outputLease != nil {
			outputLease.Release()
		}
	}()

	if hwUpdate != nil {
		if c.updateHardwareOutput(out, *hwUpdate) {
			c.mu.Lock()
			c.prevHasContent = hasContent
			c.mu.Unlock()
		} else {
			c.mu.Lock()
			c.hardwareDisabled = true
			c.renderLayersSoftwareLocked(layers, frameID)
			c.clearHardwareSnapshotLocked()
			if hasContent {
				c.prevHasContent = true
				outputFrame, outputLease = c.prepareSoftwareOutputFrameLocked()
				if !forceFullFrame {
					outputRegions = cloneFrameDirtyRects(c.softwareDirtyRects)
				}
			} else if c.prevHasContent {
				c.prevHasContent = false
				outputFrame, outputLease = c.prepareSoftwareOutputFrameLocked()
				outputRegions = nil
			}
			c.mu.Unlock()
			c.updateOutput(out, outputFrame, outputRegions)
		}
	} else {
		c.updateOutput(out, outputFrame, outputRegions)
	}
	if cbTiming != nil {
		cbTiming()
	}
	if cbPixels != nil {
		cbPixels()
	}
	if shouldOutput {
		c.mu.Lock()
		c.forceFullFrame = false
		c.mu.Unlock()
	}
}

// canSkipUnchangedCompositeLocked reports whether this compositor tick
// can skip all pixel work because no enabled source published a new
// frame since the last composite. Conservative by construction: any
// enabled source that does not implement FrameGenerationSource, a
// pending force-full-frame, an active pixel consumer (recorder/capture),
// or a scanline-compositing frame all disable the skip. On a false return
// the caller proceeds to composite and this function has already
// recorded the new generations.
func (c *VideoCompositor) canSkipUnchangedCompositeLocked() bool {
	if !compositeSkipEnabled() {
		return false
	}
	if c.forceFullFrame || !c.prevHasContent {
		return false
	}
	// A live pixel consumer (recorder/capture) needs materialised pixels
	// every tick, so it disables the skip. The plain timing callback does
	// not: it fires unconditionally on the skip path.
	if c.pixelConsumerActive != nil && c.pixelConsumerActive() {
		return false
	}
	if c.frameCounter == 0 {
		return false
	}
	unchanged := true
	enabledSources := 0
	for i := range c.sources {
		source := c.sources[i].source
		if !source.IsEnabled() {
			continue
		}
		enabledSources++
		if selector, ok := source.(ScanlineCompositingSource); ok && selector.NeedsScanlineCompositing() {
			return false
		}
		gen, ok := source.(FrameGenerationSource)
		if !ok {
			return false
		}
		id := c.sources[i].id
		current := gen.FrameGeneration()
		if c.lastSourceGens == nil {
			c.lastSourceGens = make(map[uint64]uint64)
		}
		if last, seen := c.lastSourceGens[id]; !seen || last != current {
			c.lastSourceGens[id] = current
			unchanged = false
		}
	}
	if compositorSkipTraceOn && unchanged {
		c.skipStreak++
		if c.skipStreak == 1 || c.skipStreak%120 == 0 {
			fmt.Printf("compositor-skip: streak=%d enabled=%d gens=%v\n", c.skipStreak, enabledSources, c.lastSourceGens)
		}
	} else if compositorSkipTraceOn && !unchanged && c.skipStreak > 0 {
		fmt.Printf("compositor-skip: streak ended at %d\n", c.skipStreak)
		c.skipStreak = 0
	}
	return unchanged
}

var compositorSkipTraceOn = os.Getenv("IE_COMPOSITOR_SKIP_TRACE") == "1"

func (c *VideoCompositor) canUseHardwareCompositorLocked() bool {
	if c.hardwareDisabled || os.Getenv("IE_DISABLE_GPU_COMPOSITOR") != "" || c.output == nil {
		return false
	}
	if _, ok := c.output.(HardwareCompositingOutput); !ok {
		return false
	}
	return true
}

func (c *VideoCompositor) updateHardwareOutput(out VideoOutput, update CompositorFrameUpdate) bool {
	if out == nil || !out.IsStarted() {
		return true
	}
	hw, ok := out.(HardwareCompositingOutput)
	if !ok {
		return false
	}
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	if err := hw.UpdateHardwareCompositorFrame(update); err != nil {
		fmt.Printf("Compositor: Error updating hardware frame: %v\n", err)
		return false
	}
	return true
}

func (c *VideoCompositor) clearHardwareSnapshotLocked() {
	releaseFrameLayerLeases(c.lastHardwareLayers)
	c.lastHardwareFrame = 0
	c.lastHardwareLayers = nil
	c.lastSnapshotFrame = 0
	c.lastSnapshot = nil
}

func (c *VideoCompositor) clearSoftwareFrameLeaseLocked() {
	if c.finalFrameLease != nil {
		c.finalFrameLease.Release()
		c.finalFrameLease = nil
	}
}

func (c *VideoCompositor) storeHardwareSnapshotLayersLocked(frameID uint64, layers []CompositorFrameLayer) {
	releaseFrameLayerLeases(c.lastHardwareLayers)
	c.lastHardwareFrame = frameID
	c.lastHardwareLayers = retainCompositorLayers(layers)
	c.lastSnapshotFrame = 0
	c.lastSnapshot = nil
}

func retainCompositorLayers(layers []CompositorFrameLayer) []CompositorFrameLayer {
	if len(layers) == 0 {
		return nil
	}
	out := make([]CompositorFrameLayer, len(layers))
	copy(out, layers)
	for i := range out {
		if out[i].Lease != nil && !out[i].Lease.Retain() {
			out[i].Lease = nil
		}
		if data := out[i].Indexed; data != nil {
			// The indices are per-source scratch that the next composite
			// overwrites, so a retained layer takes its own copy. A lease
			// protects RGBA buffers the same way; indices have no lease.
			out[i].Indexed = &IndexedLayerData{
				Indices: append([]byte(nil), data.Indices...),
				Palette: data.Palette,
			}
		}
	}
	return out
}

func releaseFrameLayerLeases(layers []CompositorFrameLayer) {
	for i := range layers {
		if layers[i].Lease != nil {
			layers[i].Lease.Release()
		}
	}
}

func cloneCompositorLayers(layers []CompositorFrameLayer) []CompositorFrameLayer {
	if len(layers) == 0 {
		return nil
	}
	out := make([]CompositorFrameLayer, len(layers))
	for i, layer := range layers {
		out[i] = layer
		if layer.Buffer != nil {
			out[i].Buffer = append([]byte(nil), layer.Buffer...)
		}
		out[i].Lease = nil
	}
	return out
}

func (c *VideoCompositor) renderLayersSnapshotLocked(layers []CompositorFrameLayer) []byte {
	frame := make([]byte, c.frameWidth*c.frameHeight*BYTES_PER_PIXEL)
	saved := c.finalFrame
	savedRects := c.softwareDirtyRects
	c.finalFrame = frame
	c.renderLayersIntoCurrentFrameLocked(layers)
	out := append([]byte(nil), c.finalFrame...)
	c.finalFrame = saved
	c.softwareDirtyRects = savedRects
	return out
}

// scanlineSourceEntry pairs a VideoSource with its ScanlineAware implementation
type scanlineSourceEntry struct {
	id     uint64
	source VideoSource
	sa     ScanlineAware
	layer  int
	height int
}

// sourceContentGen returns a content generation for a layer from this source.
// A FrameGenerationSource reports its own generation (bumped only on visible
// change), so an unchanged layer keeps a stable value. Any other source gets a
// per-composite unique value (the next frame id), which differs every frame and
// so forces the backend to re-upload: the conservative fallback that never
// claims unchanged pixels it cannot prove. The high bit tags fallback values so
// they can never collide with a real generation.
func (c *VideoCompositor) sourceContentGen(source VideoSource) uint64 {
	if gen, ok := source.(FrameGenerationSource); ok {
		return gen.FrameGeneration()
	}
	return (uint64(1) << 63) | (c.frameCounter + 1)
}

func (c *VideoCompositor) collectCompositeLayers(copyBuffers bool) ([]CompositorFrameLayer, bool) {
	layers, hasContent, usedScanline := c.collectScanlineAwareLayers(copyBuffers)
	if usedScanline {
		return layers, hasContent
	}
	return c.collectFullFrameLayers(copyBuffers)
}

func (c *VideoCompositor) appendCopiedCompositeLayer(layers []CompositorFrameLayer, registered registeredSource, copier CompositorFrameCopySource) ([]CompositorFrameLayer, bool) {
	source := registered.source
	srcW, srcH := source.GetDimensions()
	if srcW <= 0 || srcH <= 0 {
		return layers, false
	}
	rect := c.scaleRect(srcW, srcH, c.frameWidth, c.frameHeight)
	if rect.w <= 0 || rect.h <= 0 {
		return layers, false
	}
	bufLen := srcW * srcH * BYTES_PER_PIXEL

	// A source that holds palette indices can hand those to a backend that
	// expands them on the GPU, which skips the CPU expansion and uploads a
	// quarter of the bytes. Everything else, including the software fallback
	// below and any CPU consumer, goes through the RGBA path unchanged.
	if indexed, ok := c.collectIndexedLayerLocked(registered, srcW, srcH); ok {
		opaque := false
		if sourceOpaque, isOpaque := source.(OpaqueFrameSource); isOpaque {
			opaque = sourceOpaque.IsOpaqueFrame()
		}
		return append(layers, CompositorFrameLayer{
			SourceID:     registered.id,
			SourceWidth:  srcW,
			SourceHeight: srcH,
			DestX:        rect.x,
			DestY:        rect.y,
			DestWidth:    rect.w,
			DestHeight:   rect.h,
			Opaque:       opaque,
			ContentGen:   c.sourceContentGen(source),
			Indexed:      indexed,
		}), true
	}

	// An opaque layer needs no alpha normalisation: every consumer of one
	// forces alpha itself. The software kernels OR 0xFF000000 into each pixel
	// they copy (compositorOpaqueCopySpan, used by the whole-frame, scaled and
	// tiled paths alike), and the hardware path does the same in its shader.
	// Normalising anyway costs a full pass over every frame, which a browser
	// profile showed as the largest single function in the wasm build.
	opaque := false
	if sourceOpaque, ok := source.(OpaqueFrameSource); ok {
		opaque = sourceOpaque.IsOpaqueFrame()
	}

	var buf []byte
	var lease *VideoFrameLease
	if videoFrameLeasesEnabled() {
		if acquired, ok := c.acquireFrameLeaseLocked(registered.id, bufLen); ok {
			copied, ok := copier.CopyFrameForCompositor(acquired.Pixels()[:bufLen])
			if !ok || len(copied) < bufLen {
				acquired.Release()
				return layers, false
			}
			if !opaque {
				acquired.NormaliseAlpha()
			}
			lease = acquired
			buf = acquired.Pixels()[:bufLen]
		}
	}
	if buf == nil {
		buf = make([]byte, bufLen)
		copied, ok := copier.CopyFrameForCompositor(buf)
		if !ok || len(copied) < bufLen {
			return layers, false
		}
		if !opaque {
			normaliseFrameLeaseAlphaRGBA(buf)
		}
	}
	var dirtyRects []FrameDirtyRect
	if dirtySource, ok := source.(DirtyFrameSource); ok {
		dirtyRects = dirtySource.TakeDirtyRects()
		if len(dirtyRects) > 0 {
			dirtyRects = append([]FrameDirtyRect(nil), dirtyRects...)
		}
	}
	layers = append(layers, CompositorFrameLayer{
		SourceID:     registered.id,
		SourceWidth:  srcW,
		SourceHeight: srcH,
		DestX:        rect.x,
		DestY:        rect.y,
		DestWidth:    rect.w,
		DestHeight:   rect.h,
		Buffer:       buf,
		Lease:        lease,
		Opaque:       opaque,
		ContentGen:   c.sourceContentGen(source),
		DirtyRects:   dirtyRects,
	})
	return layers, true
}

func (c *VideoCompositor) appendCompositeLayer(layers []CompositorFrameLayer, registered registeredSource, frame []byte, copyBuffer bool) ([]CompositorFrameLayer, bool) {
	source := registered.source
	srcW, srcH := source.GetDimensions()
	if srcW <= 0 || srcH <= 0 || len(frame) < srcW*srcH*BYTES_PER_PIXEL {
		return layers, false
	}
	rect := c.scaleRect(srcW, srcH, c.frameWidth, c.frameHeight)
	if rect.w <= 0 || rect.h <= 0 {
		return layers, false
	}
	bufLen := srcW * srcH * BYTES_PER_PIXEL
	buf := frame[:bufLen]
	var lease *VideoFrameLease
	if copyBuffer {
		if videoFrameLeasesEnabled() {
			if acquired, ok := c.acquireFrameLeaseLocked(registered.id, bufLen); ok {
				copy(acquired.Pixels(), buf)
				acquired.NormaliseAlpha()
				lease = acquired
				buf = acquired.Pixels()[:bufLen]
			} else {
				buf = append([]byte(nil), buf...)
			}
		} else {
			buf = append([]byte(nil), buf...)
		}
	}
	opaque := false
	if sourceOpaque, ok := source.(OpaqueFrameSource); ok {
		opaque = sourceOpaque.IsOpaqueFrame()
	}
	var dirtyRects []FrameDirtyRect
	if dirtySource, ok := source.(DirtyFrameSource); ok {
		dirtyRects = dirtySource.TakeDirtyRects()
		if len(dirtyRects) > 0 {
			dirtyRects = append([]FrameDirtyRect(nil), dirtyRects...)
		}
	}
	layers = append(layers, CompositorFrameLayer{
		SourceID:     registered.id,
		SourceWidth:  srcW,
		SourceHeight: srcH,
		DestX:        rect.x,
		DestY:        rect.y,
		DestWidth:    rect.w,
		DestHeight:   rect.h,
		Buffer:       buf,
		Lease:        lease,
		Opaque:       opaque,
		ContentGen:   c.sourceContentGen(source),
		DirtyRects:   dirtyRects,
	})
	return layers, true
}

func videoFrameLeasesEnabled() bool {
	return os.Getenv("IE_VIDEO_FRAME_LEASES") != "0"
}

// compositeSkipEnabled reports whether the unchanged-frame composite skip is
// permitted. Default-on; IE_VIDEO_COMPOSITE_SKIP=0 forces every tick to
// composite. Read at use time so tests can toggle it with t.Setenv.
func compositeSkipEnabled() bool {
	return os.Getenv("IE_VIDEO_COMPOSITE_SKIP") != "0"
}

// collectIndexedLayerLocked returns the source's frame as palette indices when
// every condition for GPU expansion holds: the conversion is wanted, the output
// can expand indices itself, and the source is currently in an indexed mode.
// Any of those failing leaves the caller on the RGBA path.
func (c *VideoCompositor) collectIndexedLayerLocked(registered registeredSource, srcW, srcH int) (*IndexedLayerData, bool) {
	if selectGPUConversion(videoGPUConvertRequested(), c.outputAcceptsIndexedLayersLocked()) != gpuConvertShader {
		return nil, false
	}
	indexedSource, ok := registered.source.(IndexedFrameSource)
	if !ok {
		return nil, false
	}
	pixels := srcW * srcH
	if pixels <= 0 {
		return nil, false
	}
	data := c.indexedLayerScratch(registered.id, pixels)
	pal, ok := indexedSource.IndexedFrameForCompositor(data.Indices)
	if !ok {
		return nil, false
	}
	data.Palette = pal
	return data, true
}

// indexedLayerScratch returns per-source indexed storage, reused across frames
// so a steady state allocates nothing. A layer is consumed before the next
// composite for that source, so one buffer per source is enough.
func (c *VideoCompositor) indexedLayerScratch(sourceID uint64, pixels int) *IndexedLayerData {
	if c.indexedScratch == nil {
		c.indexedScratch = make(map[uint64]*IndexedLayerData)
	}
	data := c.indexedScratch[sourceID]
	if data == nil {
		data = &IndexedLayerData{}
		c.indexedScratch[sourceID] = data
	}
	if cap(data.Indices) < pixels {
		data.Indices = make([]byte, pixels)
	}
	data.Indices = data.Indices[:pixels]
	return data
}

// outputAcceptsIndexedLayersLocked reports whether the current output can
// expand palette indices itself. Headless and software outputs cannot, so they
// keep receiving RGBA.
func (c *VideoCompositor) outputAcceptsIndexedLayersLocked() bool {
	if c.output == nil || c.hardwareDisabled {
		return false
	}
	if _, ok := c.output.(HardwareCompositingOutput); !ok {
		return false
	}
	indexedOut, ok := c.output.(IndexedLayerOutput)
	return ok && indexedOut.AcceptsIndexedLayers()
}

// materialiseIndexedLayers expands any indexed layer into RGBA in place. It is
// the single choke point for CPU consumers: software rendering, the hardware
// fallback and snapshots all go through it, so nothing downstream has to know
// that a layer ever arrived as indices.
func (c *VideoCompositor) materialiseIndexedLayers(layers []CompositorFrameLayer) {
	for i := range layers {
		data := layers[i].Indexed
		if data == nil || layers[i].Buffer != nil {
			continue
		}
		buf := make([]byte, len(data.Indices)*BYTES_PER_PIXEL)
		if !data.ExpandInto(buf) {
			continue
		}
		layers[i].Buffer = buf
	}
}

func (c *VideoCompositor) acquireFrameLeaseLocked(sourceID uint64, frameBytes int) (*VideoFrameLease, bool) {
	if sourceID == 0 || frameBytes <= 0 {
		return nil, false
	}
	if c.frameLeaseRings == nil {
		c.frameLeaseRings = make(map[uint64]*VideoFrameLeaseRing)
		c.frameLeaseBytes = make(map[uint64]int)
	}
	ring := c.frameLeaseRings[sourceID]
	if ring == nil || c.frameLeaseBytes[sourceID] != frameBytes {
		ring = NewVideoFrameLeaseRing(3, frameBytes)
		c.frameLeaseRings[sourceID] = ring
		c.frameLeaseBytes[sourceID] = frameBytes
	}
	return ring.Acquire()
}

func (c *VideoCompositor) acquireSoftwareFrameLeaseLocked(frameBytes int) (*VideoFrameLease, bool) {
	if frameBytes <= 0 {
		return nil, false
	}
	if c.softwareFrameRing == nil || c.softwareFrameBytes != frameBytes {
		c.clearSoftwareFrameLeaseLocked()
		c.softwareFrameRing = NewVideoFrameLeaseRing(3, frameBytes)
		c.softwareFrameBytes = frameBytes
		// New slot buffers hold nothing, so no tile may be retained.
		c.invalidateTileStateLocked()
	}
	return c.softwareFrameRing.Acquire()
}

// compositeScanlineAware performs per-scanline rendering for copper-style effects
// Returns whether content was produced and whether the scanline path was used.
func (c *VideoCompositor) compositeScanlineAware() (bool, bool) {
	layers, hasContent, usedScanline := c.collectScanlineAwareLayers(false)
	if !usedScanline {
		return false, false
	}
	c.renderLayersSoftwareLocked(layers, c.frameCounter+1)
	return hasContent, true
}

func (c *VideoCompositor) collectScanlineAwareLayers(copyBuffers bool) ([]CompositorFrameLayer, bool, bool) {
	// Collect enabled scanline sources. Opaque sources are still blended later
	// in their sorted layer slots.
	var entries []scanlineSourceEntry
	maxSourceHeight := 0

	for _, registered := range c.sources {
		source := registered.source
		if !source.IsEnabled() {
			continue
		}

		sa, ok := source.(ScanlineAware)
		if !ok {
			continue
		}
		if selector, ok := source.(ScanlineCompositingSource); ok && !selector.NeedsScanlineCompositing() {
			continue
		}

		_, srcH := source.GetDimensions()
		if srcH > maxSourceHeight {
			maxSourceHeight = srcH
		}

		entries = append(entries, scanlineSourceEntry{
			id:     registered.id,
			source: source,
			sa:     sa,
			layer:  source.GetLayer(),
			height: srcH,
		})
	}

	if len(entries) == 0 {
		return nil, false, false
	}

	// Signal render goroutines to yield, then wait for any in-flight
	// render tick to finish before entering scanline-aware compositing.
	for _, e := range entries {
		if cm, ok := e.source.(CompositorManageable); ok {
			cm.SetCompositorManaged(true)
			defer cm.SetCompositorManaged(false)
		}
	}
	for _, e := range entries {
		if cm, ok := e.source.(CompositorManageable); ok {
			cm.WaitRenderIdle()
		}
	}

	// Start frame on all sources
	for _, e := range entries {
		safeCall("StartFrame", e.sa.StartFrame)
	}

	if len(entries) == 1 {
		if batch, ok := entries[0].source.(ScanlineBatchAware); ok {
			safeCall("ProcessScanlineRange", func() {
				batch.ProcessScanlineRange(0, maxSourceHeight)
			})
		} else {
			// Bind the interface method once per frame rather than rebinding it
			// (a heap allocation) on every scanline.
			proc := entries[0].sa.ProcessScanline
			for y := 0; y < maxSourceHeight; y++ {
				safeCallY("ProcessScanline", y, proc)
			}
		}
	} else {
		// Bind each source's ProcessScanline once per frame; the y loop below
		// runs maxSourceHeight times over every entry, so rebinding the method
		// value per scanline would allocate on each iteration.
		procs := make([]func(int), len(entries))
		for i := range entries {
			procs[i] = entries[i].sa.ProcessScanline
		}
		// Lower layer sources process first to update state, then higher layers
		// render using the updated palette. This interleaving is guest-visible.
		for y := 0; y < maxSourceHeight; y++ {
			for i, e := range entries {
				sourceY := y
				if e.height > 0 && sourceY >= e.height {
					sourceY = e.height - 1
				}
				safeCallY("ProcessScanline", sourceY, procs[i])
			}
		}
	}

	// Finish frame and collect results
	scanlineFrames := make(map[uint64][]byte, len(entries))
	for _, e := range entries {
		if frame, ok := safeCallR("FinishFrame", e.sa.FinishFrame); ok {
			scanlineFrames[e.id] = frame
		}
	}

	var layers []CompositorFrameLayer
	hasContent := false
	for _, registered := range c.sources {
		source := registered.source
		if !source.IsEnabled() {
			continue
		}
		frame, isScanline := scanlineFrames[registered.id]
		layerCopyBuffers := copyBuffers || isScanline
		if !isScanline {
			frame, _ = safeCallR("GetFrame", source.GetFrame)
		}
		safeCall("SignalVSync", source.SignalVSync)

		if frame != nil {
			var added bool
			layers, added = c.appendCompositeLayer(layers, registered, frame, layerCopyBuffers)
			hasContent = hasContent || added
		}
	}

	return layers, hasContent, true
}

// compositeFullFrame performs full-frame compositing with sequential frame collection
func (c *VideoCompositor) compositeFullFrame() bool {
	layers, hasContent := c.collectFullFrameLayers(false)
	c.renderLayersSoftwareLocked(layers, c.frameCounter+1)
	return hasContent
}

func (c *VideoCompositor) collectFullFrameLayers(copyBuffers bool) ([]CompositorFrameLayer, bool) {
	// Collect enabled sources and fetch frames sequentially
	// (GetFrame is a single atomic swap - goroutine overhead far exceeds the work)
	var layers []CompositorFrameLayer
	hasContent := false
	for _, registered := range c.sources {
		source := registered.source
		if !source.IsEnabled() {
			continue
		}
		if copyBuffers {
			if copier, ok := source.(CompositorFrameCopySource); ok {
				var added bool
				layers, added = c.appendCopiedCompositeLayer(layers, registered, copier)
				hasContent = hasContent || added
				safeCall("SignalVSync", source.SignalVSync)
				continue
			}
		}
		frame, _ := safeCallR("GetFrame", source.GetFrame)
		safeCall("SignalVSync", source.SignalVSync)
		if frame != nil {
			var added bool
			layers, added = c.appendCompositeLayer(layers, registered, frame, copyBuffers)
			hasContent = hasContent || added
		}
	}
	return layers, hasContent
}

// renderLayersSoftwareLocked composes layers into the software frame buffer.
// frameID identifies the frame being composed and must be the same value the
// caller publishes as the frame counter, because retained tile bookkeeping is
// keyed on it.
func (c *VideoCompositor) renderLayersSoftwareLocked(layers []CompositorFrameLayer, frameID uint64) {
	// Indexed layers only exist for backends that expand them on the GPU. Any
	// path that reaches the CPU rasteriser, including the hardware fallback,
	// needs pixels, so expand them here rather than at each blend site.
	c.materialiseIndexedLayers(layers)
	frameBytes := c.frameWidth * c.frameHeight * BYTES_PER_PIXEL
	if frameBytes <= 0 {
		c.clearSoftwareFrameLeaseLocked()
		c.finalFrame = nil
		c.softwareDirtyRects = nil
		return
	}
	if videoFrameLeasesEnabled() {
		if lease, ok := c.acquireSoftwareFrameLeaseLocked(frameBytes); ok {
			oldLease := c.finalFrameLease
			c.finalFrame = lease.Pixels()[:frameBytes]
			c.finalFrameLease = lease
			if oldLease != nil {
				oldLease.Release()
			}
			if c.tryRenderLayersTiledLocked(layers, lease.Slot(), frameID) {
				return
			}
			c.renderLayersIntoCurrentFrameLocked(layers)
			return
		}
	}
	if c.finalFrameLease != nil {
		c.clearSoftwareFrameLeaseLocked()
		c.finalFrame = nil
	}
	if c.finalFrame == nil || len(c.finalFrame) != frameBytes {
		c.finalFrame = make([]byte, frameBytes)
	}
	c.renderLayersIntoCurrentFrameLocked(layers)
}

func (c *VideoCompositor) renderLayersIntoCurrentFrameLocked(layers []CompositorFrameLayer) {
	for i := range c.finalFrame {
		c.finalFrame[i] = 0
	}
	for _, layer := range layers {
		c.blendLayer(layer)
	}
	c.softwareDirtyRects = c.collectOutputDirtyRectsLocked(layers)
}

func (c *VideoCompositor) prepareSoftwareOutputFrameLocked() ([]byte, *VideoFrameLease) {
	if len(c.finalFrame) == 0 {
		return nil, nil
	}
	if c.finalFrameLease != nil && c.finalFrameLease.Retain() {
		return c.finalFrame, c.finalFrameLease
	}
	if len(c.outputBuf) != len(c.finalFrame) {
		c.outputBuf = make([]byte, len(c.finalFrame))
	}
	copy(c.outputBuf, c.finalFrame)
	return c.outputBuf, nil
}

func (c *VideoCompositor) ensureBlendWorkersLocked() {
	if c.blendJobs != nil {
		return
	}
	count := runtime.GOMAXPROCS(0)
	if count < 1 {
		count = 1
	}
	c.blendJobs = make(chan blendStripJob, count*2)
	c.blendWorkerStop = make(chan struct{})
	for range count {
		c.blendWorkerWG.Add(1)
		go func() {
			defer c.blendWorkerWG.Done()
			for {
				select {
				case job := <-c.blendJobs:
					c.blendStrip(job.srcFrame, job.width, job.startY, job.endY)
					job.wg.Done()
				case <-c.blendWorkerStop:
					return
				}
			}
		}()
	}
}

func (c *VideoCompositor) stopBlendWorkersLocked() {
	if c.blendWorkerStop == nil {
		return
	}
	close(c.blendWorkerStop)
	c.blendWorkerWG.Wait()
	c.blendWorkerStop = nil
	c.blendJobs = nil
}

func (c *VideoCompositor) updateOutput(out VideoOutput, frame []byte, regions []FrameDirtyRect) {
	if frame == nil || out == nil {
		return
	}
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	if out.IsStarted() {
		if len(regions) > 0 {
			if regional, ok := out.(RegionUpdatingOutput); ok {
				planned := c.uploadPlan.plan(c.frameWidth, c.frameHeight, regions)
				if len(planned) > 0 {
					for _, rect := range planned {
						pixels := c.uploadPlan.regionPixels(frame, rect)
						if len(pixels) == 0 {
							continue
						}
						if err := regional.UpdateRegion(rect.X, rect.Y, rect.Width, rect.Height, pixels); err != nil {
							fmt.Printf("Compositor: Error updating region: %v\n", err)
							return
						}
					}
					return
				}
			}
		}
		if err := out.UpdateFrame(frame); err != nil {
			fmt.Printf("Compositor: Error updating frame: %v\n", err)
		}
	}
}

func (c *VideoCompositor) collectOutputDirtyRectsLocked(layers []CompositorFrameLayer) []FrameDirtyRect {
	if len(layers) == 0 {
		return nil
	}
	rects := make([]FrameDirtyRect, 0)
	for _, layer := range layers {
		if layer.DirtyRects == nil {
			return nil
		}
		for _, rect := range layer.DirtyRects {
			scaled := scaleLayerDirtyRect(layer, rect)
			if scaled.Width > 0 && scaled.Height > 0 {
				rects = append(rects, scaled)
			}
		}
	}
	if len(rects) == 0 {
		return nil
	}
	return rects
}

func scaleLayerDirtyRect(layer CompositorFrameLayer, rect FrameDirtyRect) FrameDirtyRect {
	if layer.SourceWidth <= 0 || layer.SourceHeight <= 0 || layer.DestWidth <= 0 || layer.DestHeight <= 0 {
		return FrameDirtyRect{}
	}
	x0 := layer.DestX + rect.X*layer.DestWidth/layer.SourceWidth
	y0 := layer.DestY + rect.Y*layer.DestHeight/layer.SourceHeight
	x1 := layer.DestX + ceilDivInt((rect.X+rect.Width)*layer.DestWidth, layer.SourceWidth)
	y1 := layer.DestY + ceilDivInt((rect.Y+rect.Height)*layer.DestHeight, layer.SourceHeight)
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	return FrameDirtyRect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

func ceilDivInt(n, d int) int {
	if d <= 0 {
		return 0
	}
	if n <= 0 {
		return n / d
	}
	return (n + d - 1) / d
}

func cloneFrameDirtyRects(rects []FrameDirtyRect) []FrameDirtyRect {
	if len(rects) == 0 {
		return nil
	}
	return append([]FrameDirtyRect(nil), rects...)
}

var compositorSoftwarePresentationHook func()
var compositorSoftwareScaleHook func()

// blendFrame blends a source frame into the final frame with scaling.
// All-zero pixels are transparent; any nonzero alpha or RGB value is opaque.
func (c *VideoCompositor) blendFrame(srcFrame []byte, srcW, srcH int) {
	dstW := c.frameWidth
	dstH := c.frameHeight

	// Early bounds check
	if srcW <= 0 || srcH <= 0 || len(srcFrame) < srcW*srcH*BYTES_PER_PIXEL {
		return
	}
	if dstW <= 0 || dstH <= 0 || len(c.finalFrame) < dstW*dstH*BYTES_PER_PIXEL {
		return
	}

	rect := c.scaleRect(srcW, srcH, dstW, dstH)
	if rect.w <= 0 || rect.h <= 0 {
		return
	}

	// Fast path: 1:1 scaling (most common case)
	if rect.x == 0 && rect.y == 0 && rect.w == dstW && rect.h == dstH && srcW == dstW && srcH == dstH {
		c.blendFrame1to1(srcFrame, srcW, srcH)
		return
	}

	// Scaled path using Bresenham-style integer arithmetic
	c.blendFrameScaled(srcFrame, srcW, srcH, rect)
}

func (c *VideoCompositor) blendLayer(layer CompositorFrameLayer) {
	if compositorSoftwarePresentationHook != nil {
		compositorSoftwarePresentationHook()
	}
	if layer.SourceWidth <= 0 || layer.SourceHeight <= 0 || len(layer.Buffer) < layer.SourceWidth*layer.SourceHeight*BYTES_PER_PIXEL {
		return
	}
	rect := scaleRect{x: layer.DestX, y: layer.DestY, w: layer.DestWidth, h: layer.DestHeight}
	if rect.w <= 0 || rect.h <= 0 {
		return
	}
	if rect.x == 0 && rect.y == 0 && rect.w == c.frameWidth && rect.h == c.frameHeight &&
		layer.SourceWidth == c.frameWidth && layer.SourceHeight == c.frameHeight {
		if layer.Opaque {
			c.copyOpaqueFrame1to1(layer.Buffer, layer.SourceWidth, layer.SourceHeight)
			return
		}
		c.blendFrame1to1(layer.Buffer, layer.SourceWidth, layer.SourceHeight)
		return
	}
	if layer.Opaque {
		c.copyOpaqueFrameScaled(layer.Buffer, layer.SourceWidth, layer.SourceHeight, rect)
		return
	}
	c.blendFrameScaled(layer.Buffer, layer.SourceWidth, layer.SourceHeight, rect)
}

// cachedResamplePlanLocked returns the resample plan for a scale, building it
// on first use. Plans are pure functions of the two widths, so caching them
// keeps the scaled paths free of per-frame allocation.
func (c *VideoCompositor) cachedResamplePlanLocked(srcW, rectW int) *resamplePlan {
	key := uint64(uint32(srcW))<<32 | uint64(uint32(rectW))
	if plan, ok := c.resamplePlans[key]; ok {
		return plan
	}
	if c.resamplePlans == nil {
		c.resamplePlans = make(map[uint64]*resamplePlan)
	}
	plan := newResamplePlan(srcW, rectW)
	c.resamplePlans[key] = plan
	return plan
}

// scaledRowScratchLocked returns a reusable row buffer of at least n bytes.
func (c *VideoCompositor) scaledRowScratchLocked(n int) []byte {
	if cap(c.scaledRowBuf) < n {
		c.scaledRowBuf = make([]byte, n)
	}
	return c.scaledRowBuf[:n]
}

type scaleRect struct {
	x, y int
	w, h int
}

func (c *VideoCompositor) scaleRect(srcW, srcH, dstW, dstH int) scaleRect {
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return scaleRect{}
	}
	if c.scaleMode == ScaleStretchFill || sameAspect(srcW, srcH, dstW, dstH) {
		return scaleRect{w: dstW, h: dstH}
	}
	drawW := dstW
	drawH := dstW * srcH / srcW
	if drawH > dstH {
		drawH = dstH
		drawW = dstH * srcW / srcH
	}
	return scaleRect{
		x: (dstW - drawW) / 2,
		y: (dstH - drawH) / 2,
		w: drawW,
		h: drawH,
	}
}

// blendFrame1to1 is the optimized fast path for same-size source and destination.
// For large frames, it splits into horizontal strips blended in parallel.
func (c *VideoCompositor) blendFrame1to1(srcFrame []byte, width, height int) {
	const stripHeight = 60
	if height <= stripHeight {
		c.blendStrip(srcFrame, width, 0, height)
		return
	}

	c.ensureBlendWorkersLocked()
	var wg sync.WaitGroup
	for y0 := 0; y0 < height; y0 += stripHeight {
		y1 := min(y0+stripHeight, height)
		wg.Add(1)
		c.blendJobs <- blendStripJob{
			srcFrame: srcFrame,
			width:    width,
			startY:   y0,
			endY:     y1,
			wg:       &wg,
		}
	}
	wg.Wait()
}

func (c *VideoCompositor) copyOpaqueFrame1to1(srcFrame []byte, width, height int) {
	// 1:1 copy: src and finalFrame share the same contiguous layout, so the
	// whole region is one span.
	n := width * height * BYTES_PER_PIXEL
	compositorOpaqueCopySpanImpl(c.finalFrame[:n], srcFrame[:n])
}

// Span-level compositor kernels. Scalar leaves are the canonical always-built
// implementation; the ...Impl vars default to them and are reassigned to SIMD
// variants in assignSIMDKernels on supported hosts (simd_dispatch_amd64.go).
// Differential tests call the scalar leaves directly, never the Impl vars.
var (
	compositorBlendSpanImpl      = compositorBlendSpanScalar
	compositorOpaqueCopySpanImpl = compositorOpaqueCopySpanScalar
)

// compositorBlendSpanScalar blends src into dst pixel by pixel: fully-zero
// source pixels skip the write (dst preserved), zero-alpha nonzero-rgb pixels
// promote to 0xFFRRGGBB, alpha-set pixels are written unchanged. dst and src
// are RGBA byte spans of equal length; extra tail bytes below one pixel are
// ignored.
func compositorBlendSpanScalar(dst, src []byte) {
	n := len(src)
	if len(dst) < n {
		n = len(dst)
	}
	x := 0
	// Two pixels per iteration: when both alpha bytes are already set the
	// pair is written through unchanged, which is the common case for
	// opaque content. Mixed pairs fall back to the per-pixel rules.
	for ; x+8 <= n; x += 8 {
		v := *(*uint64)(unsafe.Pointer(&src[x]))
		if v&0x00000000FF000000 != 0 && v&0xFF00000000000000 != 0 {
			*(*uint64)(unsafe.Pointer(&dst[x])) = v
			continue
		}
		if pixel, ok := compositorOpaquePixel(uint32(v)); ok {
			*(*uint32)(unsafe.Pointer(&dst[x])) = pixel
		}
		if pixel, ok := compositorOpaquePixel(uint32(v >> 32)); ok {
			*(*uint32)(unsafe.Pointer(&dst[x+4])) = pixel
		}
	}
	for ; x+BYTES_PER_PIXEL <= n; x += BYTES_PER_PIXEL {
		srcPixel := *(*uint32)(unsafe.Pointer(&src[x]))
		if pixel, ok := compositorOpaquePixel(srcPixel); ok {
			*(*uint32)(unsafe.Pointer(&dst[x])) = pixel
		}
	}
}

// compositorOpaqueCopySpanScalar copies src into dst, forcing every pixel opaque
// (src|0xFF000000). Unlike blend it always writes, including fully-zero pixels
// (which become 0xFF000000).
func compositorOpaqueCopySpanScalar(dst, src []byte) {
	n := len(src)
	if len(dst) < n {
		n = len(dst)
	}
	x := 0
	for ; x+8 <= n; x += 8 {
		v := *(*uint64)(unsafe.Pointer(&src[x]))
		*(*uint64)(unsafe.Pointer(&dst[x])) = v | 0xFF000000FF000000
	}
	for ; x+BYTES_PER_PIXEL <= n; x += BYTES_PER_PIXEL {
		srcPixel := *(*uint32)(unsafe.Pointer(&src[x]))
		*(*uint32)(unsafe.Pointer(&dst[x])) = srcPixel | 0xFF000000
	}
}

func (c *VideoCompositor) copyOpaqueFrameScaled(srcFrame []byte, srcW, srcH int, rect scaleRect) {
	dstW := c.frameWidth
	srcRowBytes := srcW * BYTES_PER_PIXEL
	dstRowBytes := dstW * BYTES_PER_PIXEL
	rectRowBytes := rect.w * BYTES_PER_PIXEL
	plan := c.cachedResamplePlanLocked(srcW, rect.w)
	rowBuf := c.scaledRowScratchLocked(rectRowBytes)
	// Opaque copy writes every pixel, so consecutive dst rows that map to the
	// same source row are byte-identical: copy the just-written row instead of
	// re-sampling.
	prevSrcY := -1
	prevDstOffset := 0
	for dy := range rect.h {
		srcY := dy * srcH / rect.h
		dstOffset := (rect.y+dy)*dstRowBytes + rect.x*BYTES_PER_PIXEL
		if srcY == prevSrcY {
			copy(c.finalFrame[dstOffset:dstOffset+rectRowBytes], c.finalFrame[prevDstOffset:prevDstOffset+rectRowBytes])
		} else {
			srcRowOffset := srcY * srcRowBytes
			compositorResampleRowImpl(rowBuf, srcFrame[srcRowOffset:], plan, 0, rect.w)
			compositorOpaqueCopySpanImpl(c.finalFrame[dstOffset:dstOffset+rectRowBytes], rowBuf)
		}
		prevSrcY = srcY
		prevDstOffset = dstOffset
	}
}

// blendStrip blends rows [startY, endY) from srcFrame into finalFrame.
// Partial alpha is intentionally treated opaque, and zero-alpha colour is
// promoted to opaque so BASIC-friendly 0x00RRGGBB pixels are visible.
func (c *VideoCompositor) blendStrip(srcFrame []byte, width, startY, endY int) {
	rowBytes := width * BYTES_PER_PIXEL
	// Rows [startY, endY) are contiguous and 1:1 in both buffers, so the strip
	// is a single span.
	start := startY * rowBytes
	end := endY * rowBytes
	compositorBlendSpanImpl(c.finalFrame[start:end], srcFrame[start:end])
}

// blendFrameScaled handles scaling using optimized integer arithmetic
// This matches the original dstX * srcW / dstW calculation exactly
func (c *VideoCompositor) blendFrameScaled(srcFrame []byte, srcW, srcH int, rect scaleRect) {
	if compositorSoftwareScaleHook != nil {
		compositorSoftwareScaleHook()
	}
	dstW := c.frameWidth

	srcRowBytes := srcW * BYTES_PER_PIXEL
	dstRowBytes := dstW * BYTES_PER_PIXEL

	// srcX depends only on dx; hoist it. The scaled source row is gathered
	// (scalar scatter-read) into a contiguous buffer once per distinct source
	// row, then blended into the destination row through the SIMD blend span.
	// Blend is skip-write (transparent source pixels leave the destination), so
	// unlike the opaque copy the destination is not identical for a repeated
	// source row and must be re-blended each row (only the gather is skipped).
	rectRowBytes := rect.w * BYTES_PER_PIXEL
	plan := c.cachedResamplePlanLocked(srcW, rect.w)
	rowBuf := c.scaledRowScratchLocked(rectRowBytes)
	prevSrcY := -1

	for dy := range rect.h {
		srcY := dy * srcH / rect.h
		if srcY != prevSrcY {
			srcRowOffset := srcY * srcRowBytes
			compositorResampleRowImpl(rowBuf, srcFrame[srcRowOffset:], plan, 0, rect.w)
			prevSrcY = srcY
		}
		dstOffset := (rect.y+dy)*dstRowBytes + rect.x*BYTES_PER_PIXEL
		compositorBlendSpanImpl(c.finalFrame[dstOffset:dstOffset+rectRowBytes], rowBuf)
	}
}

func compositorOpaquePixel(srcPixel uint32) (uint32, bool) {
	if srcPixel&0xFF000000 != 0 {
		return srcPixel, true
	}
	if srcPixel&0x00FFFFFF != 0 {
		return srcPixel | 0xFF000000, true
	}
	return 0, false
}

// SetFrameCallback installs the pixel-consumer callback, invoked only after a
// composed (non-skipped) composite() pass, when materialised pixels exist.
func (c *VideoCompositor) SetFrameCallback(cb func()) {
	c.mu.Lock()
	c.onFrameComplete = cb
	c.mu.Unlock()
}

// SetFrameTimingCallback installs the unconditional logical-frame/timing
// callback, invoked on every tick including skipped ones (no pixels required).
func (c *VideoCompositor) SetFrameTimingCallback(cb func()) {
	c.mu.Lock()
	c.onFrameTiming = cb
	c.mu.Unlock()
}

// SetPixelConsumerActiveFunc installs a predicate reporting whether a live
// pixel consumer (recording/capture) currently needs materialised pixels every
// tick. When it returns true the unchanged-frame skip is disabled.
func (c *VideoCompositor) SetPixelConsumerActiveFunc(fn func() bool) {
	c.mu.Lock()
	c.pixelConsumerActive = fn
	c.mu.Unlock()
}

// GetCurrentFrame returns a copy of the compositor's latest frame buffer.
func (c *VideoCompositor) GetCurrentFrame() []byte {
	buf, _, _ := c.GetFrameSnapshot()
	return buf
}

// GetFrameSnapshot returns a copy of the latest compositor frame with metadata.
func (c *VideoCompositor) GetFrameSnapshot() ([]byte, uint64, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastHardwareFrame == c.frameCounter && c.lastHardwareFrame != 0 {
		if c.lastSnapshotFrame == c.lastHardwareFrame && c.lastSnapshot != nil {
			out := append([]byte(nil), c.lastSnapshot...)
			return out, c.frameCounter, c.frameTimestamp
		}
		if hw, ok := c.output.(HardwareCompositingOutput); ok {
			if frame, ok := hw.HardwareCompositorSnapshot(c.lastHardwareFrame); ok {
				c.lastSnapshotFrame = c.lastHardwareFrame
				c.lastSnapshot = append(c.lastSnapshot[:0], frame...)
				out := append([]byte(nil), c.lastSnapshot...)
				return out, c.frameCounter, c.frameTimestamp
			}
		}
		c.lastSnapshotFrame = c.lastHardwareFrame
		c.lastSnapshot = c.renderLayersSnapshotLocked(c.lastHardwareLayers)
		out := append([]byte(nil), c.lastSnapshot...)
		return out, c.frameCounter, c.frameTimestamp
	}
	if len(c.finalFrame) == 0 {
		return nil, c.frameCounter, c.frameTimestamp
	}
	out := make([]byte, len(c.finalFrame))
	copy(out, c.finalFrame)
	return out, c.frameCounter, c.frameTimestamp
}

// GetDimensions returns the compositor's current output dimensions.
func (c *VideoCompositor) GetDimensions() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.frameWidth, c.frameHeight
}

// GetNativeSourceDimensions returns the first enabled video source's native
// resolution. This may differ from the compositor output when upscaling
// (e.g. VideoChip 640x480 → compositor 800x600). Falls back to compositor
// dimensions if no source is enabled.
func (c *VideoCompositor) GetNativeSourceDimensions() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.sources {
		if s.source.IsEnabled() {
			return s.source.GetDimensions()
		}
	}
	return c.frameWidth, c.frameHeight
}

func (c *VideoCompositor) MapPresentationPointToNative(x, y int) (int, int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	srcW, srcH := c.activeSourceDimensionsLocked()
	return c.mapPresentationPointToNativeLocked(x, y, srcW, srcH)
}

func (c *VideoCompositor) MapPresentationPointToNativeForSource(x, y, srcW, srcH int) (int, int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mapPresentationPointToNativeLocked(x, y, srcW, srcH)
}

func (c *VideoCompositor) mapPresentationPointToNativeLocked(x, y, srcW, srcH int) (int, int, int, int) {
	rect := c.scaleRect(srcW, srcH, c.frameWidth, c.frameHeight)
	if srcW <= 0 || srcH <= 0 || rect.w <= 0 || rect.h <= 0 {
		return x, y, c.frameWidth, c.frameHeight
	}

	nx := (x - rect.x) * srcW / rect.w
	ny := (y - rect.y) * srcH / rect.h
	nx = max(0, min(nx, srcW-1))
	ny = max(0, min(ny, srcH-1))
	return nx, ny, srcW, srcH
}

func (c *VideoCompositor) MapNativePointToPresentation(x, y int) (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	srcW, srcH := c.activeSourceDimensionsLocked()
	return c.mapNativePointToPresentationLocked(x, y, srcW, srcH)
}

func (c *VideoCompositor) MapNativePointToPresentationForSource(x, y, srcW, srcH int) (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mapNativePointToPresentationLocked(x, y, srcW, srcH)
}

func (c *VideoCompositor) mapNativePointToPresentationLocked(x, y, srcW, srcH int) (int, int) {
	rect := c.scaleRect(srcW, srcH, c.frameWidth, c.frameHeight)
	if srcW <= 0 || srcH <= 0 || rect.w <= 0 || rect.h <= 0 {
		return x, y
	}

	x = max(0, min(x, srcW-1))
	y = max(0, min(y, srcH-1))
	px := rect.x + x*rect.w/srcW
	py := rect.y + y*rect.h/srcH
	return px, py
}

func sameAspect(w1, h1, w2, h2 int) bool {
	if w1 <= 0 || h1 <= 0 || w2 <= 0 || h2 <= 0 {
		return false
	}
	return w1*h2 == w2*h1
}

func (c *VideoCompositor) GetScaleMode() PresentationScaleMode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scaleMode
}

func (c *VideoCompositor) SetScaleMode(mode PresentationScaleMode) {
	c.mu.Lock()
	if mode != ScaleAspectFit && mode != ScaleStretchFill {
		mode = ScaleAspectFit
	}
	c.scaleMode = mode
	c.mu.Unlock()
}

func (c *VideoCompositor) ActiveSourceNeedsScaleToggle() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, h := c.activeSourceDimensionsLocked()
	return !sameAspect(w, h, c.frameWidth, c.frameHeight)
}

func (c *VideoCompositor) ToggleScaleModeIfNonNative() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, h := c.activeSourceDimensionsLocked()
	if sameAspect(w, h, c.frameWidth, c.frameHeight) {
		return false
	}
	if c.scaleMode == ScaleStretchFill {
		c.scaleMode = ScaleAspectFit
	} else {
		c.scaleMode = ScaleStretchFill
	}
	return true
}

func (c *VideoCompositor) activeSourceDimensionsLocked() (int, int) {
	for i := len(c.sources) - 1; i >= 0; i-- {
		source := c.sources[i].source
		if source.IsEnabled() {
			return source.GetDimensions()
		}
	}
	return c.frameWidth, c.frameHeight
}

// GetTickRate returns the compositor's fixed scheduling tick in Hz.
func (c *VideoCompositor) GetTickRate() int {
	return COMPOSITOR_REFRESH_RATE
}

// GetRefreshRate returns the output device refresh rate in Hz, falling back to
// the compositor tick when no backend reports a usable value.
func (c *VideoCompositor) GetRefreshRate() int {
	c.mu.Lock()
	out := c.output
	c.mu.Unlock()
	if out == nil {
		return COMPOSITOR_REFRESH_RATE
	}
	rate := out.GetRefreshRate()
	if rate <= 0 {
		return COMPOSITOR_REFRESH_RATE
	}
	return rate
}

func safeCall(name string, fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Compositor: recovered panic in %s: %v\n", name, r)
			ok = false
		}
	}()
	fn()
	return true
}

func safeCallR[T any](name string, fn func() T) (out T, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Compositor: recovered panic in %s: %v\n", name, r)
			var zero T
			out = zero
			ok = false
		}
	}()
	return fn(), true
}

func safeCallY(name string, y int, fn func(int)) (ok bool) {
	return safeCall(name, func() { fn(y) })
}
