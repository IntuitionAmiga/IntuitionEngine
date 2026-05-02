package main

import "testing"

func TestANTICMissilePixelRender(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.gractl = GTIA_GRACTL_MISSILE
	antic.colpm[0] = 0x0E
	antic.writeBuffer = 0
	antic.missileGfx[1][0][0] = 1
	antic.missilePos[1][0][0] = 48

	frame := antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP); got != anticRGBA(0x0E) {
		t.Fatalf("missile pixel got %v", got)
	}
}

func TestANTICPriorityPlayerBelowPlayfield(t *testing.T) {
	bus, antic := newModeTestANTIC(DL_MODE15, 1)
	antic.gractl = GTIA_GRACTL_PLAYER
	antic.prior = GTIA_PRIOR_P23
	antic.colpm[0] = 0x0E
	antic.writeBuffer = 0
	antic.playerGfx[1][0][0] = 0x80
	antic.playerPos[1][0][0] = 48
	bus.Write8(0x3000, 0x80)

	frame := antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP); got != anticRGBA(0x04) {
		t.Fatalf("player0 should be below playfield with P23 priority, got %v", got)
	}
	if got := antic.playerPF[0]; got == 0 {
		t.Fatalf("player/playfield collision not latched")
	}
}

func TestANTICPlayfieldCollisionLatchIgnoresPMGDrawOrder(t *testing.T) {
	bus, antic := newModeTestANTIC(DL_MODE15, 1)
	antic.gractl = GTIA_GRACTL_PLAYER
	antic.writeBuffer = 0
	antic.playerGfx[1][0][0] = 0x80
	antic.playerGfx[1][1][0] = 0x80
	antic.playerPos[1][0][0] = 48
	antic.playerPos[1][1][0] = 48
	bus.Write8(0x3000, 0x80)

	antic.RenderFrame(nil)

	if got := antic.HandleRead(GTIA_P0PF); got&(1<<1) == 0 {
		t.Fatalf("P0PF should latch PF1, got 0x%02X", got)
	}
	if got := antic.HandleRead(GTIA_P1PF); got&(1<<1) == 0 {
		t.Fatalf("P1PF should latch PF1 despite P0 drawing first, got 0x%02X", got)
	}
}

func TestANTICBlankRasterColorDoesNotCreatePlayfieldCollision(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.colbk = 0
	antic.colpf[0] = 0
	antic.colpm[0] = 0x0E
	antic.gractl = GTIA_GRACTL_PLAYER
	antic.prior = GTIA_PRIOR_P23
	antic.writeBuffer = 0
	antic.playerGfx[1][0][0] = 0x80
	antic.playerPos[1][0][0] = 48

	frame := antic.RenderFrame(nil)

	if got := antic.HandleRead(GTIA_P0PF); got != 0 {
		t.Fatalf("blank raster matching COLPF0 should not latch P0PF, got 0x%02X", got)
	}
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP); got != anticRGBA(0x0E) {
		t.Fatalf("player over blank raster should not be hidden by nonexistent PF, got %v", got)
	}
}

func TestANTICHITCLRClearsCollisions(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.playerPF[0] = 0x03
	antic.missilePF[2] = 0x04
	antic.missilePL[1] = 0x01
	antic.playerPL[2] = 0x08
	antic.HandleWrite(GTIA_HITCLR, 0)
	if antic.playerPF[0] != 0 || antic.missilePF[2] != 0 || antic.missilePL[1] != 0 || antic.playerPL[2] != 0 {
		t.Fatalf("HITCLR did not clear collisions")
	}
}

func TestANTICHPOSPCapturesWithoutWSYNC(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.scanline = 5
	antic.writeBuffer = 0
	antic.HandleWrite(GTIA_HPOSP0, 0x55)
	if got := antic.playerPos[0][0][5]; got != 0x55 {
		t.Fatalf("HPOSP direct write did not capture current scanline: 0x%02X", got)
	}
	antic.HandleWrite(GTIA_HPOSM0, 0x66)
	if got := antic.missilePos[0][0][5]; got != 0x66 {
		t.Fatalf("HPOSM direct write did not capture current scanline: 0x%02X", got)
	}
}

func TestANTICPlayerPlayerCollisionLatch(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.gractl = GTIA_GRACTL_PLAYER
	antic.writeBuffer = 0
	antic.playerGfx[1][0][0] = 0x80
	antic.playerGfx[1][1][0] = 0x80
	antic.playerPos[1][0][0] = 48
	antic.playerPos[1][1][0] = 48

	antic.RenderFrame(nil)

	if got := antic.HandleRead(GTIA_P0PL); got&(1<<1) == 0 {
		t.Fatalf("P0PL should latch overlap with player 1, got 0x%02X", got)
	}
	if got := antic.HandleRead(GTIA_P1PL); got&(1<<0) == 0 {
		t.Fatalf("P1PL should latch overlap with player 0, got 0x%02X", got)
	}
}

func TestANTICMissilePlayerCollisionLatch(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.gractl = GTIA_GRACTL_PLAYER | GTIA_GRACTL_MISSILE
	antic.writeBuffer = 0
	antic.playerGfx[1][2][0] = 0x80
	antic.playerPos[1][2][0] = 48
	antic.missileGfx[1][0][0] = 1
	antic.missilePos[1][0][0] = 48

	antic.RenderFrame(nil)

	if got := antic.HandleRead(GTIA_M0PL); got&(1<<2) == 0 {
		t.Fatalf("M0PL should latch overlap with player 2, got 0x%02X", got)
	}
}
