//go:build !headless

package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.design/x/clipboard"
)

// LuaOverlay provides an in-window Lua REPL (F8).
type LuaOverlay struct {
	scriptEngine *ScriptEngine
	L            *lua.LState

	mu sync.Mutex

	active bool
	glyphs [256][16]byte
	image  *ebiten.Image
	pixels []byte

	input   []rune
	cursor  int
	history []string
	histIdx int
	output  []string
	scroll  int

	pendingLines []string
	multiline    bool
}

var (
	luaClipboardOnce sync.Once
	luaClipboardOK   bool
)

const luaOverlayBG = uint32(0x0055AAFF)

func NewLuaOverlay(scriptEngine *ScriptEngine) *LuaOverlay {
	o := &LuaOverlay{
		scriptEngine: scriptEngine,
		glyphs:       loadTopazFont(),
		pixels:       make([]byte, overlayWidth*overlayHeight*4),
		histIdx:      -1,
	}
	o.resetState()
	return o
}

func (o *LuaOverlay) SetScriptEngine(se *ScriptEngine) {
	o.scriptEngine = se
	o.resetState()
}

func (o *LuaOverlay) Toggle() {
	o.mu.Lock()
	o.active = !o.active
	o.mu.Unlock()
}

func (o *LuaOverlay) IsActive() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.active
}

func (o *LuaOverlay) Show() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.active = true
}

func (o *LuaOverlay) Hide() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.active = false
}

func (o *LuaOverlay) AppendLine(s string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.output = append(o.output, s)
	const max = 1000
	if len(o.output) > max {
		o.output = o.output[len(o.output)-max:]
	}
}

func (o *LuaOverlay) Clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.output = o.output[:0]
	o.scroll = 0
}

func (o *LuaOverlay) ScrollUp(n int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.scroll += n
	if max := len(o.output) - (overlayRows - 3); o.scroll > max {
		o.scroll = max
	}
	if o.scroll < 0 {
		o.scroll = 0
	}
}

func (o *LuaOverlay) ScrollDown(n int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.scroll -= n
	if o.scroll < 0 {
		o.scroll = 0
	}
}

func (o *LuaOverlay) LineCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.output)
}

func (o *LuaOverlay) resetState() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.L != nil {
		o.L.Close()
	}
	L := lua.NewState()
	if o.scriptEngine != nil {
		o.scriptEngine.registerModules(L, context.Background())
	}
	L.SetGlobal("print", L.NewFunction(func(L *lua.LState) int {
		args := make([]string, 0, L.GetTop())
		for i := 1; i <= L.GetTop(); i++ {
			args = append(args, L.Get(i).String())
		}
		// Called from L.DoString inside HandleInput which already holds o.mu.
		o.appendOutputLocked(strings.Join(args, " "))
		return 0
	}))
	if sysTbl, ok := L.GetGlobal("sys").(*lua.LTable); ok {
		fn := L.NewFunction(func(L *lua.LState) int {
			args := make([]string, 0, L.GetTop())
			for i := 1; i <= L.GetTop(); i++ {
				args = append(args, L.Get(i).String())
			}
			o.appendOutputLocked(strings.Join(args, " "))
			return 0
		})
		sysTbl.RawSetString("print", fn)
		sysTbl.RawSetString("log", fn)
	}
	o.L = L
	o.output = nil
	o.input = nil
	o.cursor = 0
	o.history = nil
	o.histIdx = -1
	o.scroll = 0
	o.pendingLines = nil
	o.multiline = false
}

// appendOutputLocked appends text to the output buffer. Caller must hold o.mu.
func (o *LuaOverlay) appendOutputLocked(s string) {
	for line := range strings.SplitSeq(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		o.output = append(o.output, line)
	}
	if len(o.output) > 1000 {
		o.output = o.output[len(o.output)-1000:]
	}
}

// appendOutput acquires o.mu and appends text. Safe to call without holding the lock.
func (o *LuaOverlay) appendOutput(s string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.appendOutputLocked(s)
}

// eval executes a Lua line in the REPL. Caller must hold o.mu.
func (o *LuaOverlay) eval(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if after, ok := strings.CutPrefix(line, "return "); ok {
		expr := strings.TrimSpace(after)
		err := o.L.DoString("__repl_result = (" + expr + ")")
		if err != nil {
			o.appendOutputLocked("error: " + err.Error())
			return
		}
		v := o.L.GetGlobal("__repl_result")
		o.appendOutputLocked(v.String())
		o.L.SetGlobal("__repl_result", lua.LNil)
		return
	}
	if after, ok := strings.CutPrefix(line, "="); ok {
		expr := strings.TrimSpace(after)
		err := o.L.DoString("__repl_result = (" + expr + ")")
		if err != nil {
			o.appendOutputLocked("error: " + err.Error())
			return
		}
		v := o.L.GetGlobal("__repl_result")
		o.appendOutputLocked(v.String())
		o.L.SetGlobal("__repl_result", lua.LNil)
		return
	}
	if err := o.L.DoString(line); err != nil {
		o.appendOutputLocked("error: " + err.Error())
	}
}

func isMultilineIncomplete(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "<eof>") || strings.Contains(msg, "unexpected EOF") || strings.Contains(msg, " at EOF:")
}

// submitInputLine processes a REPL input line. Caller must hold o.mu.
func (o *LuaOverlay) submitInputLine(line string) {
	if strings.TrimSpace(line) == "" && !o.multiline {
		return
	}
	if strings.TrimSpace(line) != "" {
		o.history = append(o.history, line)
	}
	o.histIdx = -1
	prefix := "lua> "
	if o.multiline {
		prefix = "...> "
	}
	o.appendOutputLocked(prefix + line)
	if o.multiline {
		o.pendingLines = append(o.pendingLines, line)
		combined := strings.Join(o.pendingLines, "\n")
		if _, err := o.L.LoadString(combined); err != nil {
			if isMultilineIncomplete(err) {
				return
			}
			o.appendOutputLocked("error: " + err.Error())
			o.pendingLines = nil
			o.multiline = false
			return
		}
		o.eval(combined)
		o.pendingLines = nil
		o.multiline = false
		return
	}
	if _, err := o.L.LoadString(line); err != nil {
		if isMultilineIncomplete(err) {
			o.pendingLines = []string{line}
			o.multiline = true
			return
		}
		o.appendOutputLocked("error: " + err.Error())
		return
	}
	o.eval(line)
}

func (o *LuaOverlay) pasteClipboard() {
	luaClipboardOnce.Do(func() {
		luaClipboardOK = clipboard.Init() == nil
	})
	if !luaClipboardOK {
		return
	}
	data := clipboard.Read(clipboard.FmtText)
	for _, b := range data {
		if b < 0x20 || b > 0x7E {
			continue
		}
		if len(o.input) >= overlayCols-6 {
			break
		}
		r := rune(b)
		o.input = append(o.input[:o.cursor], append([]rune{r}, o.input[o.cursor:]...)...)
		o.cursor++
	}
}

func (o *LuaOverlay) HandleInput() {
	o.mu.Lock()
	defer o.mu.Unlock()

	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		o.active = false
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		o.scroll += 5
		maxScroll := len(o.output) - (overlayRows - 3)
		if o.scroll > maxScroll {
			o.scroll = max(maxScroll, 0)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		o.scroll -= 5
		if o.scroll < 0 {
			o.scroll = 0
		}
	}
	if ctrl && shift && inpututil.IsKeyJustPressed(ebiten.KeyV) {
		o.pasteClipboard()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || shouldRepeat(ebiten.KeyEnter) {
		line := string(o.input)
		o.submitInputLine(line)
		o.input = o.input[:0]
		o.cursor = 0
		o.scroll = 0
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || shouldRepeat(ebiten.KeyBackspace) {
		if o.cursor > 0 {
			o.input = append(o.input[:o.cursor-1], o.input[o.cursor:]...)
			o.cursor--
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) || shouldRepeat(ebiten.KeyDelete) {
		if o.cursor < len(o.input) {
			o.input = append(o.input[:o.cursor], o.input[o.cursor+1:]...)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) || shouldRepeat(ebiten.KeyHome) {
		o.cursor = 0
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) || shouldRepeat(ebiten.KeyEnd) {
		o.cursor = len(o.input)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || shouldRepeat(ebiten.KeyArrowLeft) {
		if o.cursor > 0 {
			o.cursor--
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || shouldRepeat(ebiten.KeyArrowRight) {
		if o.cursor < len(o.input) {
			o.cursor++
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) || shouldRepeat(ebiten.KeyArrowUp) {
		if len(o.history) > 0 {
			if o.histIdx < 0 {
				o.histIdx = len(o.history) - 1
			} else if o.histIdx > 0 {
				o.histIdx--
			}
			o.input = []rune(o.history[o.histIdx])
			o.cursor = len(o.input)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) || shouldRepeat(ebiten.KeyArrowDown) {
		if len(o.history) > 0 && o.histIdx >= 0 {
			if o.histIdx < len(o.history)-1 {
				o.histIdx++
				o.input = []rune(o.history[o.histIdx])
				o.cursor = len(o.input)
			} else {
				o.histIdx = -1
				o.input = o.input[:0]
				o.cursor = 0
			}
		}
	}
	if ctrl && !shift {
		if inpututil.IsKeyJustPressed(ebiten.KeyA) {
			o.cursor = 0
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyE) {
			o.cursor = len(o.input)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyK) {
			o.input = o.input[:o.cursor]
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyU) {
			o.input = o.input[o.cursor:]
			o.cursor = 0
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || shouldRepeat(ebiten.KeyArrowLeft) {
			for o.cursor > 0 && o.input[o.cursor-1] == ' ' {
				o.cursor--
			}
			for o.cursor > 0 && o.input[o.cursor-1] != ' ' {
				o.cursor--
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || shouldRepeat(ebiten.KeyArrowRight) {
			for o.cursor < len(o.input) && o.input[o.cursor] != ' ' {
				o.cursor++
			}
			for o.cursor < len(o.input) && o.input[o.cursor] == ' ' {
				o.cursor++
			}
		}
		ebiten.AppendInputChars(nil)
		return
	}

	for _, r := range ebiten.AppendInputChars(nil) {
		if r < 0x20 || r > 0x7E {
			continue
		}
		if len(o.input) >= overlayCols-6 {
			break
		}
		o.input = append(o.input[:o.cursor], append([]rune{r}, o.input[o.cursor:]...)...)
		o.cursor++
	}
}

func (o *LuaOverlay) Draw(screen *ebiten.Image) {
	// Snapshot all mutable fields under lock, then release lock before drawing
	o.mu.Lock()
	active := o.active
	outputLines := append([]string(nil), o.output...)
	scroll := o.scroll
	multiline := o.multiline
	inputRunes := append([]rune(nil), o.input...)
	cursor := o.cursor
	o.mu.Unlock()

	if !active {
		return
	}

	if o.image == nil {
		o.image = ebiten.NewImage(overlayWidth, overlayHeight)
	}
	bg := luaOverlayBG
	for row := range overlayRows {
		for col := range overlayCols {
			o.drawGlyph(' ', col, row, colorWhite, bg)
		}
	}
	o.drawString("LUA REPL (F8) - Esc to close", 0, 0, colorCyan)

	lines := overlayRows - 3
	start := 0
	if len(outputLines) > lines {
		start = max(len(outputLines)-lines-scroll, 0)
	}
	for i := 0; i < lines && start+i < len(outputLines); i++ {
		o.drawString(outputLines[start+i], 0, 1+i, colorWhite)
	}

	promptRow := overlayRows - 1
	prefix := "lua> "
	if multiline {
		prefix = "...> "
	}
	prompt := prefix + string(inputRunes)
	o.drawString(prompt, 0, promptRow, colorWhite)
	cursorCol := len(prefix) + cursor
	if cursorCol < overlayCols {
		o.drawGlyph('_', cursorCol, promptRow, colorWhite, bg)
	}

	o.image.WritePixels(o.pixels)
	screen.DrawImage(o.image, nil)
}

func (o *LuaOverlay) drawGlyph(ch byte, col, row int, fg, bg uint32) {
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

func (o *LuaOverlay) drawString(s string, col, row int, fg uint32) {
	bg := luaOverlayBG
	for i := 0; i < len(s) && col+i < overlayCols; i++ {
		o.drawGlyph(s[i], col+i, row, fg, bg)
	}
}

func (o *LuaOverlay) Close() error {
	if o.L != nil {
		o.L.Close()
		o.L = nil
	}
	return nil
}

func (o *LuaOverlay) String() string {
	return fmt.Sprintf("LuaOverlay(active=%v)", o.active)
}
