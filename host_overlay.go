//go:build !headless

package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const hostOverlayBG = 0x0055AAFF
const hostOverlayAutoCloseDelay = 5 * time.Second

type HostOverlay struct {
	mu          sync.Mutex
	active      bool
	done        bool
	completedAt time.Time
	cmd         HostCommand
	status      string
	lines       []string
	partial     string
	scroll      int
	glyphs      [256][16]byte
	image       *ebiten.Image
	pixels      []byte
}

func NewHostOverlay() *HostOverlay {
	return &HostOverlay{
		glyphs: loadTopazFont(),
		pixels: make([]byte, overlayWidth*overlayHeight*4),
	}
}

func (o *HostOverlay) HostCommandStarted(cmd HostCommand) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.active = true
	o.done = false
	o.completedAt = time.Time{}
	o.cmd = cmd
	o.status = "running"
	o.lines = o.lines[:0]
	o.partial = ""
	o.scroll = 0
	o.appendLineLocked(fmt.Sprintf("%s started", hostCommandTitle(cmd)))
}

func (o *HostOverlay) HostCommandOutput(cmd HostCommand, text string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.active || o.cmd != cmd {
		o.active = true
		o.cmd = cmd
	}
	o.appendTextLocked(text)
}

func (o *HostOverlay) HostCommandCompleted(cmd HostCommand, result HostCommandResult) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.active || o.cmd != cmd {
		o.active = true
		o.cmd = cmd
	}
	o.flushPartialLocked()
	o.done = true
	o.completedAt = time.Now()
	switch result.Status {
	case HostStatusOK:
		o.status = "complete"
		switch cmd {
		case HostCommandNet:
			if len(o.lines) <= 1 {
				o.appendLineLocked("No wireless devices or networks reported.")
			}
			o.appendLineLocked("HOST NET complete. Returning to BASIC in 5 seconds.")
		case HostCommandUpdate:
			o.appendLineLocked("HOST UPDATE complete. Returning to BASIC in 5 seconds.")
		default:
			o.appendLineLocked("Command complete. Returning to BASIC in 5 seconds.")
		}
	case HostStatusUserCancel:
		o.status = "cancelled"
		o.appendLineLocked("Command cancelled. Returning to BASIC in 5 seconds.")
	default:
		o.status = "failed"
		o.appendLineLocked(fmt.Sprintf("Command failed: %s", HostHelperExitStatusText(result.ExitCode)))
		o.appendLineLocked("Details are also written to host-helper.log on the shared disk.")
		o.appendLineLocked("Returning to BASIC in 5 seconds.")
	}
	o.scroll = 0
}

func (o *HostOverlay) IsActive() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.active
}

func (o *HostOverlay) HandleInput() {
	o.mu.Lock()
	done := o.done
	completedAt := o.completedAt
	lineCount := len(o.lines)
	o.mu.Unlock()

	if done && !completedAt.IsZero() && time.Since(completedAt) >= hostOverlayAutoCloseDelay {
		o.mu.Lock()
		o.active = false
		o.mu.Unlock()
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) || hostOverlayShouldRepeat(ebiten.KeyPageUp) {
		o.scrollBy(5, lineCount)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) || hostOverlayShouldRepeat(ebiten.KeyPageDown) {
		o.scrollBy(-5, lineCount)
	}
	_, wy := ebiten.Wheel()
	if wy != 0 {
		delta := int(wy)
		if delta == 0 && wy > 0 {
			delta = 1
		}
		if delta == 0 && wy < 0 {
			delta = -1
		}
		o.scrollBy(delta, lineCount)
	}
	if done && (inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
		inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
		inpututil.IsKeyJustPressed(ebiten.KeySpace)) {
		o.mu.Lock()
		o.active = false
		o.mu.Unlock()
	}
}

func (o *HostOverlay) Draw(screen *ebiten.Image) {
	o.mu.Lock()
	active := o.active
	cmd := o.cmd
	status := o.status
	lines := append([]string(nil), o.lines...)
	if o.partial != "" {
		lines = append(lines, o.partial)
	}
	scroll := o.scroll
	done := o.done
	completedAt := o.completedAt
	o.mu.Unlock()

	if !active {
		return
	}
	if o.image == nil {
		o.image = ebiten.NewImage(overlayWidth, overlayHeight)
	}
	for row := range overlayRows {
		for col := range overlayCols {
			o.drawGlyph(' ', col, row, colorWhite, hostOverlayBG)
		}
	}

	header := fmt.Sprintf("%s - %s", hostCommandTitle(cmd), status)
	if done {
		header += " (Esc closes)"
	}
	o.drawString(header, 0, 0, colorCyan)

	visibleRows := overlayRows - 3
	start := 0
	if len(lines) > visibleRows {
		start = max(len(lines)-visibleRows-scroll, 0)
	}
	for i := 0; i < visibleRows && start+i < len(lines); i++ {
		o.drawString(lines[start+i], 0, 1+i, colorWhite)
	}

	footer := "PageUp/PageDown scroll"
	if !done {
		footer = "Running... PageUp/PageDown scroll"
	} else {
		remaining := 1
		if !completedAt.IsZero() {
			left := hostOverlayAutoCloseDelay - time.Since(completedAt)
			remaining = int((left + time.Second - 1) / time.Second)
			if remaining < 1 {
				remaining = 1
			}
		}
		footer = fmt.Sprintf("Returning to BASIC in %d... Esc/Enter/Space now    PageUp/PageDown/Wheel scroll", remaining)
	}
	o.drawString(footer, 0, overlayRows-1, colorDim)

	o.image.WritePixels(o.pixels)
	drawOverlayImage(screen, o.image)
}

func (o *HostOverlay) appendTextLocked(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	parts := strings.Split(text, "\n")
	for i, part := range parts {
		if i == 0 {
			o.partial += part
			continue
		}
		o.flushPartialLocked()
		o.partial = part
	}
	if strings.HasSuffix(text, "\n") {
		o.flushPartialLocked()
	}
}

func (o *HostOverlay) flushPartialLocked() {
	if o.partial == "" {
		return
	}
	o.appendLineLocked(o.partial)
	o.partial = ""
}

func (o *HostOverlay) appendLineLocked(line string) {
	line = strings.TrimRight(line, "\t ")
	if line == "" {
		line = " "
	}
	for len(line) > overlayCols {
		o.lines = append(o.lines, line[:overlayCols])
		line = line[overlayCols:]
	}
	o.lines = append(o.lines, line)
	const maxLines = 2000
	if len(o.lines) > maxLines {
		o.lines = append([]string(nil), o.lines[len(o.lines)-maxLines:]...)
	}
}

func (o *HostOverlay) scrollBy(delta int, lineCount int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	maxScroll := max(lineCount-(overlayRows-3), 0)
	o.scroll += delta
	if o.scroll < 0 {
		o.scroll = 0
	}
	if o.scroll > maxScroll {
		o.scroll = maxScroll
	}
}

func (o *HostOverlay) drawGlyph(ch byte, col, row int, fg, bg uint32) {
	x := col * glyphW
	y := row * glyphH
	if x+glyphW > overlayWidth || y+glyphH > overlayHeight {
		return
	}
	fgR, fgG, fgB, fgA := colorFromPacked(fg)
	bgR, bgG, bgB, bgA := colorFromPacked(bg)
	glyph := &o.glyphs[ch]
	for dy := range glyphH {
		rowBits := glyph[dy]
		pixY := (y + dy) * overlayWidth * 4
		for dx := range glyphW {
			pixIdx := pixY + (x+dx)*4
			if rowBits&(0x80>>dx) != 0 {
				o.pixels[pixIdx] = fgR
				o.pixels[pixIdx+1] = fgG
				o.pixels[pixIdx+2] = fgB
				o.pixels[pixIdx+3] = fgA
			} else {
				o.pixels[pixIdx] = bgR
				o.pixels[pixIdx+1] = bgG
				o.pixels[pixIdx+2] = bgB
				o.pixels[pixIdx+3] = bgA
			}
		}
	}
}

func (o *HostOverlay) drawString(s string, col, row int, fg uint32) {
	for i := 0; i < len(s) && col+i < overlayCols; i++ {
		o.drawGlyph(s[i], col+i, row, fg, hostOverlayBG)
	}
}

func hostOverlayShouldRepeat(key ebiten.Key) bool {
	dur := inpututil.KeyPressDuration(key)
	if dur < monitorKeyRepeatDelay {
		return false
	}
	return (dur-monitorKeyRepeatDelay)%monitorKeyRepeatInterval == 0
}

func hostCommandTitle(cmd HostCommand) string {
	switch cmd {
	case HostCommandNet:
		return "HOST NET"
	case HostCommandUpdate:
		return "HOST UPDATE"
	case HostCommandReboot:
		return "HOST REBOOT"
	case HostCommandPoweroff:
		return "HOST POWEROFF"
	default:
		return "HOST"
	}
}
