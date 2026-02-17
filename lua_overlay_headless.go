//go:build headless

package main

// LuaOverlay is a no-op in headless mode.
type LuaOverlay struct{}

func NewLuaOverlay(_ *ScriptEngine) *LuaOverlay { return &LuaOverlay{} }
func (o *LuaOverlay) SetScriptEngine(_ *ScriptEngine) {
}
func (o *LuaOverlay) Toggle()        {}
func (o *LuaOverlay) IsActive() bool { return false }
func (o *LuaOverlay) HandleInput()   {}
func (o *LuaOverlay) Draw(_ any)     {}
func (o *LuaOverlay) Close() error   { return nil }
