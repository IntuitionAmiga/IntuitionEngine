//go:build headless

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const resonanceDemoPath = "sdk/examples/basic/resonance.bas"
const adagioMIDIPath = "sdk/examples/assets/music/adagioforstrings.mid"
const resonanceScrollPath = "sdk/examples/assets/resonance_scroll.rgba"

func TestResonanceAssetExists(t *testing.T) {
	info, err := os.Stat(adagioMIDIPath)
	if err != nil {
		t.Fatalf("Adagio MIDI asset missing: %v", err)
	}
	if info.Size() != 82522 {
		t.Fatalf("Adagio MIDI size = %d, want 82522", info.Size())
	}
	info, err = os.Stat(resonanceScrollPath)
	if err != nil {
		t.Fatalf("Resonance scrolltext asset missing: %v", err)
	}
	if info.Size() != 16384*33*4 {
		t.Fatalf("Resonance scrolltext size = %d, want %d", info.Size(), 16384*33*4)
	}
}

func TestResonanceProgramShape(t *testing.T) {
	program, err := os.ReadFile(resonanceDemoPath)
	if err != nil {
		t.Fatalf("read resonance.bas: %v", err)
	}
	text := string(program)

	if !strings.Contains(text, `SOUND PLAY "sdk/examples/assets/music/adagioforstrings.mid"`) {
		t.Fatal("resonance.bas must start the Adagio MIDI with SOUND PLAY after setup")
	}
	soundIdx := strings.Index(text, "SOUND PLAY")
	if loopIdx := strings.Index(text, "510 REM MAIN LOOP"); loopIdx >= 0 && soundIdx > loopIdx {
		t.Fatal("resonance.bas must start MIDI before entering the main loop")
	}
	for _, want := range []string{
		"FB=MEMALLOC(1228800,4096):BB=MEMALLOC(1228800,4096):TX=MEMALLOC(2097152,4096)",
		"CP=MEMALLOC(4096,4096):SR=MEMALLOC(235520,4096):SB=MEMALLOC(2162688,4096)",
		"POKE32 &HF0084,FB",
		"BLIT MODE7",
		"BLIT MEMCOPY BB,FB,1228800",
		"VSYNC",
		"SOUND PLAY",
		"COPPER",
		"PEEK32(&H000F0BB0)",
		"PEEK32(&H000F075C)",
		"PM=0:RT0=PEEK32(&H000F075C)/1000000",
		"IF MP>PM THEN TM=MP/44100:PM=MP:GOTO 560",
		"TM=RT-RT0",
		"IN=0.12:TI=INT(TM*100)",
		"IF TI>=4500 THEN GOSUB 2800",
		"IF TI>=9000 THEN GOSUB 3100",
		"IF TI>=12000 THEN GOSUB 3300",
		"IF TI>=15000 THEN GOSUB 3400",
		"SA=SIN(A):SZ=SIN(Z):CZ=COS(A):AB=INT(A*40.743665) AND 255:AB2=INT(A*2*40.743665) AND 255",
		"GOSUB 3500",
		"IF TI>=21800 THEN GOTO 3700",
		`SOUND PLAY "sdk/examples/assets/music/adagioforstrings.mid"`,
		"GOSUB 1500",
		"970 GOTO 510",
		"RETRO HARDWARE ENGINE",
		"INTUITION ENGINE SOURCE LOGO",
		"MIDI CHORD PULSE ENVELOPE FOR BARS",
		"IF TI>=236 THEN IF TI<308 THEN PU=28:DR=-1",
		"IF TI>=1230 THEN IF TI<1280 THEN PU=18:DR=1",
		"IF TI>=1685 THEN IF TI<1740 THEN PU=20:DR=1",
		"IF TI>=10475 THEN IF TI<10555 THEN PU=30:DR=-1",
		"IF TI>=18430 THEN IF TI<18510 THEN PU=34:DR=1",
		"IF TI>=20210 THEN IF TI<20290 THEN PU=34:DR=1",
		"PHRASE-BASED BRIGHTNESS ENVELOPE",
		"PB=0:PD=0",
		"IF TI>=1170 THEN IF TI<2900 THEN PB=INT(4+(TM-11.70)*0.5):PD=1",
		"IF TI>=18500 THEN IF TI<21800 THEN PB=INT(14-(TM-185)*0.3):PD=-1:IF PB<0 THEN PB=0",
		"IF TI>=4250 THEN IF TI<7225 THEN PB=INT(8+5*SIN((TM-42.50)*0.45))",
		"IF TI>=14950 THEN IF TI<18510 THEN PB=INT(18+8*SIN((TM-149.50)*0.28))",
		"SD=1:IF PD<0 THEN SD=-1",
		"N=18+INT(IN*86)+INT(PU*1.4)+INT(PB*0.5)",
		"X=(I*73+F*(1+(I AND 3))*SD+INT(PB*9*SD)) AND 639",
		"Y=(I*41+F+INT(28*SN((AB+I*41) AND 255)/1024)) AND 479",
		"IF PU>0 THEN S=S+1",
		"IF PB>12 THEN S=S+1",
		"IF PU>0 THEN BC1=&H00A8C8D0",
		"IF PU>0 THEN C=HC",
		"IF PB>10 THEN C2=&H00384850:C3=&H00505860",
		"IF PB>18 THEN C2=&H00808890:C3=&H00D0B868",
		"BLIT FILL TX,TW,TH,&H00000000,TS",
		"FOR Y=0 TO 448 STEP 64",
		"FOR X=0 TO 960 STEP 128",
		"IF PB>12 THEN C=&H00708088",
		"IF PB>18 THEN C=&H00D0B868",
		"CA=CZ/SC:MS=SA/SC",
		"MU=INT((CU-HW*CA+HH*MS)*FP)",
		"MV=INT((CV-HW*MS-HH*CA)*FP)",
		"BLIT MODE7 TX,BB,640,480,MU,MV,DC,DS,0-DS,DC,1023,511,TS,ST",
		"WD=1:IF PD<0 THEN WD=-1",
		"WA=5+INT(PB*.8)+INT(PU*.18)",
		"ZB=INT(Z*1.4*40.743665) AND 255:YS=5:IF WD<0 THEN YS=-5",
		"FOR YY=0 TO 91",
		"WX=INT(WA*SN((ZB+YY*YS) AND 255)/1024)",
		"BLIT COPY SR+YY*ST,TX+(LY+YY)*TS+(LX+WX)*4,640,1,ST,TS",
		"Y1=Y1-PD*INT(PB*1.2)",
		"Y2=Y2-PD*INT(PB*0.9)",
		"IF PD>=0 THEN X=(F*6+INT(PB*11)) AND 639",
		"IF PD<0 THEN X=639-((F*6+INT(PB*11)) AND 639)",
		"IF X>544 THEN X=544",
		"X2=544-X:IF X2<0 THEN X2=0",
		"IF PD<0 THEN X=600-((F*3+INT(PB*9)) AND 463)",
		"ENGINE LIGHT SWEEP ARRAY",
		"RISING MOTION",
		"PHRASE-DIRECTED PANEL STREAM",
		"CD=1:IF PD<0 THEN CD=-1",
		"X=(I*104+F*3*CD+INT(PB*7*CD)) AND 639",
		"PRE-CRESCENDO STEEL GLINTS",
		"GX=(F*4+INT(PB*13)) AND 447",
		"X=96+GX:IF PD<0 THEN X=543-GX",
		"STEEL/COPPER SWEEP OVERLAY",
		"CX=(F*(6+PD)+INT(PB*17)) AND 511",
		"X=64+CX:IF X>616 THEN X=616",
		"CX=(F*(6-PD)+INT(PB*13)) AND 511",
		"X=568-CX:IF X<0 THEN X=0",
		"PROTRACKER-STYLE MIDI VU METERS",
		"V0=0:V1=0:V2=0:V3=0:V4=0:V5=0",
		"T0=8+INT(PB*.9)+INT(PU*.8)+INT(IN*10)",
		"IF PD>0 THEN T0=T0+6:T1=T1+4:T2=T2+2",
		"IF PD<0 THEN T3=T3+2:T4=T4+4:T5=T5+6",
		"IF V0<T0 THEN V0=T0 ELSE V0=V0-2:IF V0<0 THEN V0=0",
		"IF V5>58 THEN V5=58",
		"BLIT FILL BB+386*ST+58*4,54,62,&H00101820,ST",
		"BLIT FILL BB+(Y-V0)*ST+64*4,8,V0,&H00A87058,ST",
		"BLIT FILL BB+(Y-V5)*ST+568*4,8,V5,&H00A87058,ST",
		`BLOAD "sdk/examples/assets/resonance_scroll.rgba",SB`,
		"LONG TRANSPARENT SINUS SCROLLTEXT BETWEEN VU METERS",
		"SC=(F*3) AND 16383",
		"FOR I=0 TO 24",
		"SX=(SC+I*16) AND 16383:IF SX>16368 THEN SX=0",
		"X=124+I*16:Y=405+INT(7*SN((AB2+I*14) AND 255)/1024)",
		"POKE32 &HF0024,SB+SX*4:POKE32 &HF0028,BB+Y*ST+X*4:POKE32 &HF002C,16:POKE32 &HF0030,33",
		"POKE32 &HF0034,SS:POKE32 &HF0038,ST:POKE32 &HF0020,4:POKE32 &HF001C,1",
		"WELCOME TO RESONANCE BY INTUITION",
		"IE64 BASIC JIT-COMPILED TO NATIVE CODE DRIVES A MIDI-SYNCHRONISED MODE7 ROTOZOOM",
		"BARBER'S ADAGIO FOR STRINGS BY WILLIAM ORBIT (1995)",
		"ALL EFFECTS ARE TIMED FROM THE LIVE MIDI PLAYBACK CLOCK",
		"VISIT WWW.INTUITIONSUBSYNTH.COM",
		"256-ENTRY SINE LOOKUP TABLE FOR LOOP-HEAVY EFFECTS",
		"DIM SN(255)",
		"SN(I)=INT(1024*SIN(I*6.28318/256))",
		"STOP MUSIC, FADE FULL SCREEN TO BLACK, RETURN TO BASIC",
		"FOR K=40 TO 0 STEP -1",
		"C=INT(K*1.6)",
		"BLIT FILL FB,640,480,CC,ST",
		"SOUND PLAY STOP",
		"COPPER OFF",
		"POKE32 &HF0004,7",
		"POKE32 &HF0000,1",
		`PRINT "INTUITION ENGINE"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("resonance.bas missing %q", want)
		}
	}
	if strings.Contains(text, "TE=F") || strings.Contains(text, "TE=FR") {
		t.Fatal("timeline must not be driven only by the frame counter")
	}
	if strings.Contains(text, "PEEK32(&H000F0BAC)") {
		t.Fatal("demo must not end on a transient raw MIDI busy-bit read")
	}
	for _, fixed := range []string{
		"FB=&H100000",
		"BB=&H230000",
		"TX=&H360000",
		"CP=&H5E0000",
		"SR=&H600000",
		"SB=&H668000",
	} {
		if strings.Contains(text, fixed) {
			t.Fatalf("resonance.bas must not use fixed buffer address %q", fixed)
		}
	}
	for _, forbidden := range []string{"CATHEDRAL", "GOTHIC", "ROSE WINDOW", "STAINED GLASS", "CRUCIFIX", "ADAGIO COMPLETE", "MONOLITH", "DATA WINDOWS", "RAVE LASER"} {
		if strings.Contains(strings.ToUpper(text), forbidden) {
			t.Fatalf("demo must not contain religious/incorrect outro marker %q", forbidden)
		}
	}
	if strings.Contains(text, "POKE32 &HF0488,48") || strings.Contains(text, "GOSUB 3610") {
		t.Fatal("demo must not reintroduce the arbitrary vector-line overlay")
	}
	if strings.Contains(text, "BLIT FILL BB+(Y+H)*ST+X*4,10,2,C,ST") {
		t.Fatal("MIDI event strip must not draw L-shaped note markers")
	}
	if strings.Contains(text, "BLIT FILL BB+Y*ST+X*4,3,H,C,ST") {
		t.Fatal("MIDI scope must draw a connected trace, not isolated square/tick markers")
	}
	if strings.Contains(text, "BLIT FILL BB+382*ST+54*4,532,66,&H00040810,ST") ||
		strings.Contains(text, "BLIT FILL BB+384*ST+X*4,1,60,&H00101820,ST") {
		t.Fatal("MIDI scope must not reintroduce the heavy boxed graticule")
	}
	for _, stale := range []string{
		"OSCILLOSCOPE-STYLE MIDI NOTE TRACE",
		"PRECALCULATED MIDI NOTE EVENT TABLE",
		"PRECALCULATED MIDI BAND ENERGY TABLE",
		"GOSUB 5000",
		"GOSUB 4900",
		"BT=&H",
		"POKE BT",
		"DIM ET(EC),EN(EC),EV(EC)",
		"READ ET(I),EN(I),EV(I)",
	} {
		if strings.Contains(text, stale) {
			t.Fatalf("demo must not reintroduce the removed scope/table path %q", stale)
		}
	}
	for _, stale := range []string{
		"GOSUB 2500",
		"RELEASE/OUTRO",
		"OP=0:OK=INT((TM-200)/0.445)",
		"IF OP>0 THEN OC1=&H00B8A070:OC2=&H00A8C8D0",
		"IF OP>0 THEN BLIT FILL BB+(222+OD*INT(OP*0.2))*ST+120*4,400,4,&H00D0C890,ST",
		"BLIT COPY SR,BB+Y*ST+X*4,640,92,ST,ST",
	} {
		if strings.Contains(text, stale) {
			t.Fatalf("demo must not reintroduce the removed outro path %q", stale)
		}
	}
	for _, stale := range []string{
		"BLIT COPY SR+108*ST+SX*4,BB+Y*ST+X*4,8,33,ST,ST",
		"POKE32 &HF0024,SR+108*ST+SX*4",
		"LC=&HFFA09078:MC=&HFF403828",
		"BLIT COPY SR,TX+LY*TS+LX*4,640,92,ST,TS",
		"BLIT MEMCOPY TB,TX,2097152",
		"STATIC MODE7 TUNNEL BASE",
		"GOSUB 1860",
		":SB=SA/SC",
		"CA=CZ/SC:SN=SA/SC",
		"U0=INT((CU-HW*CA+HH*SN)*FP)",
		"V0=INT((CV-HW*SN-HH*CA)*FP)",
		"Y=(I*41+F+INT(28*SIN(A+I))) AND 479",
		"WX=INT(WA*SIN(Z*1.4+YY*.12*WD))",
		"Y=405+INT(7*SIN(A*2+I*.35))",
	} {
		if strings.Contains(text, stale) {
			t.Fatalf("scrolltext must stay transparent and long-form, found stale source-strip path %q", stale)
		}
	}
	if strings.Contains(text, "2290 BLIT FILL BB+96*ST+48*4,42,330,&H00000000,ST") ||
		strings.Contains(text, "2300 BLIT FILL BB+96*ST+550*4,42,330,&H00000000,ST") {
		t.Fatal("demo must not reintroduce pointless black side masks")
	}
	if strings.Contains(text, "BLIT FILL BB+112*ST+318*4,6,46") ||
		strings.Contains(text, "BLIT FILL BB+132*ST+296*4,48,4") {
		t.Fatal("demo must not draw a cross-shaped central monolith highlight")
	}
	for _, stale := range []string{
		"BLIT FILL BB+158*ST+244*4,28,188",
		"BLIT FILL BB+158*ST+306*4,28,188",
		"BLIT FILL BB+158*ST+368*4,28,188",
		"BLIT FILL BB+110*ST+288*4,64,50",
		"BLIT FILL BB+90*ST+126*4,16,332",
		"BLIT FILL BB+90*ST+498*4,16,332",
	} {
		if strings.Contains(text, stale) {
			t.Fatalf("demo must not reintroduce static rectangle art %q", stale)
		}
	}
	if strings.Contains(text, "SR+10*ST+70*4") || strings.Contains(text, "SR+22*ST+520*4") {
		t.Fatal("demo must not reintroduce the old Adagio-shaped source logo")
	}
	flipIdx := strings.Index(text, "910 BLIT MEMCOPY BB,FB,1228800")
	if strings.Contains(text, "BLIT LINE") {
		t.Fatal("demo must not reintroduce arbitrary vector-line overlays")
	}
	mode7Idx := strings.Index(text, "630 GOSUB 1900")
	overlayIdx := strings.Index(text, "640 GOSUB 1000")
	if mode7Idx < 0 || overlayIdx < 0 || mode7Idx > overlayIdx {
		t.Fatal("Mode7 backdrop must render before abstract overlay")
	}
	if flipIdx < 0 {
		t.Fatal("framebuffer flip missing")
	}
}

func TestResonanceAllocationPlanStaysInLow32Memory(t *testing.T) {
	const (
		lowEnd    = 0x100000000
		screen    = 640 * 480 * 4
		work      = 1024 * 512 * 4
		copperLen = 4096
		sourceLen = 640 * 92 * 4
		scrollLen = 2162688
	)

	sizes := map[string]int{
		"front":  screen,
		"back":   screen,
		"work":   work,
		"copper": copperLen,
		"source": sourceLen,
		"scroll": scrollLen,
	}
	order := []string{"front", "back", "work", "copper", "source", "scroll"}
	cursor := 0x00820000
	spans := map[string][2]int{}
	for _, name := range order {
		size := sizes[name]
		if cursor%4096 != 0 {
			t.Fatalf("%s base %#x is not 4 KiB aligned", name, cursor)
		}
		spans[name] = [2]int{cursor, cursor + size}
		cursor = (cursor + size + 4095) &^ 4095
	}
	for name, span := range spans {
		if span[0] <= 0 || span[1] > lowEnd {
			t.Fatalf("%s span [%#x,%#x) outside low memory end %#x", name, span[0], span[1], lowEnd)
		}
		for otherName, other := range spans {
			if name >= otherName {
				continue
			}
			if span[0] < other[1] && other[0] < span[1] {
				t.Fatalf("%s span [%#x,%#x) overlaps %s span [%#x,%#x)", name, span[0], span[1], otherName, other[0], other[1])
			}
		}
	}
}

func TestResonanceMIDIThroughMediaLoader(t *testing.T) {
	if _, err := os.Stat(adagioMIDIPath); err != nil {
		t.Fatalf("Adagio MIDI asset missing: %v", err)
	}
	bus := NewMachineBus()
	sound := newTestSoundChip()
	midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
	loader := NewMediaLoader(bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)

	nameAddr := uint32(0x1000)
	writeFilenameToBus(bus, nameAddr, adagioMIDIPath)
	loader.HandleWrite(MEDIA_NAME_PTR, nameAddr)
	loader.HandleWrite(MEDIA_CTRL, MEDIA_OP_PLAY)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if loader.HandleRead(MEDIA_STATUS) != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}
	if status := loader.HandleRead(MEDIA_STATUS); status != MEDIA_STATUS_PLAYING {
		t.Fatalf("status=%d, want PLAYING=%d, err=%d, type=%d",
			status, MEDIA_STATUS_PLAYING, loader.HandleRead(MEDIA_ERROR), loader.HandleRead(MEDIA_TYPE))
	}
	if got := loader.HandleRead(MEDIA_TYPE); got != MEDIA_TYPE_MIDI {
		t.Fatalf("MEDIA_TYPE=%d, want %d", got, MEDIA_TYPE_MIDI)
	}

	before := midiPlayer.engine.PositionSamples()
	for i := 0; i < 4096; i++ {
		midiPlayer.engine.TickSample()
	}
	after := midiPlayer.engine.PositionSamples()
	if before != 0 {
		t.Fatalf("initial MIDI position = %d, want 0 before ticking", before)
	}
	if after == 0 {
		t.Fatal("MIDI_POSITION did not advance after driving the sample clock")
	}
}

func TestResonanceInitialEhBASICPath(t *testing.T) {
	program, err := os.ReadFile(resonanceDemoPath)
	if err != nil {
		t.Fatalf("read resonance.bas: %v", err)
	}
	text := strings.ReplaceAll(string(program), "940 VSYNC", "940 REM VSYNC")
	text = strings.ReplaceAll(text, "970 GOTO 510", "970 END")

	asmBin := buildAssembler(t)
	var loader *MediaLoader
	var midiPlayer *MIDIPlayer
	var video *VideoChip
	makeResonanceHarness := func(tb testing.TB) *ehbasicTestHarness {
		tb.Helper()
		bus, err := NewMachineBusSized(256 * 1024 * 1024)
		if err != nil {
			tb.Fatalf("NewMachineBusSized: %v", err)
		}
		bus.ApplyProfileVisibleCeiling(256 * 1024 * 1024)
		return newEhbasicHarnessOnBus(tb, bus)
	}
	out, _ := execStmtTestCoreWithHarness(t, asmBin, text, func(h *ehbasicTestHarness) {
		var err error
		video, err = NewVideoChip(VIDEO_BACKEND_EBITEN)
		if err != nil {
			t.Fatalf("NewVideoChip: %v", err)
		}
		video.AttachBus(h.bus)
		video.SetBigEndianMode(false)
		h.bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
		h.bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)

		sound := newTestSoundChip()
		midiPlayer = NewMIDIPlayer(sound, SAMPLE_RATE)
		midiPlayer.AttachBus(h.bus)
		h.bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
		loader = NewMediaLoader(h.bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)
		h.bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)
		fileIO := NewFileIODevice(h.bus, ".")
		h.bus.MapIO(FILE_IO_BASE, FILE_IO_END, fileIO.HandleRead, fileIO.HandleWrite)
		h.bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fileIO.HandleWrite8)
	}, makeResonanceHarness)
	if strings.Contains(out, "?") {
		t.Fatalf("initial Resonance EhBASIC path produced an error: %q", out)
	}
	if loader == nil || midiPlayer == nil {
		t.Fatal("media loader was not initialized")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if loader.HandleRead(MEDIA_STATUS) != MEDIA_STATUS_LOADING {
			break
		}
		runtime.Gosched()
	}
	if status := loader.HandleRead(MEDIA_STATUS); status != MEDIA_STATUS_PLAYING {
		t.Fatalf("EhBASIC SOUND PLAY status=%d, want PLAYING=%d, err=%d, type=%d",
			status, MEDIA_STATUS_PLAYING, loader.HandleRead(MEDIA_ERROR), loader.HandleRead(MEDIA_TYPE))
	}
	if video == nil {
		t.Fatal("video chip was not initialized")
	}
	if fbBase := video.HandleRead(VIDEO_FB_BASE); fbBase == 0 || fbBase%4096 != 0 {
		t.Fatalf("Resonance VIDEO_FB_BASE = %#x, want nonzero 4 KiB-aligned MEMALLOC address", fbBase)
	}
	frame := video.FinishFrame()
	if len(frame) == 0 {
		t.Fatal("Resonance visible framebuffer produced an empty frame")
	}
	nonzero := 0
	for off := uint32(0); off < 1228800; off += 4096 {
		if binary.LittleEndian.Uint32(frame[off:]) != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Fatal("Resonance visible framebuffer stayed black after initial BASIC path")
	}
	const scrollFirstColor = 0xFFA09078
	scrollPixels := 0
	for y := 380; y < 440; y++ {
		for x := 100; x < 540; x++ {
			off := uint32(y*640+x) * 4
			if binary.LittleEndian.Uint32(frame[off:]) == scrollFirstColor {
				scrollPixels++
			}
		}
	}
	if scrollPixels < 100 {
		t.Fatalf("Resonance scroll text absent: found %d expected-color pixels in scroll band", scrollPixels)
	}
}

func TestResonanceInitialRunAOTPath(t *testing.T) {
	program, err := os.ReadFile(resonanceDemoPath)
	if err != nil {
		t.Fatalf("read resonance.bas: %v", err)
	}
	text := strings.ReplaceAll(string(program), "940 VSYNC", "940 REM VSYNC")
	text = strings.ReplaceAll(text, "970 GOTO 510", "970 END")

	asmBin := buildAssembler(t)
	repo := repoRootDir(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repo)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)

	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(h.bus)
	video.SetBigEndianMode(false)
	var firstBlitError atomic.Value
	guardBlit := func(addr uint32, value uint32) {
		if addr == BLT_CTRL && value&1 != 0 {
			dst := video.HandleRead(BLT_DST)
			width := video.HandleRead(BLT_WIDTH)
			height := video.HandleRead(BLT_HEIGHT)
			if dst >= uint32(len(h.bus.GetMemory())) {
				vars := readAOTNativeVars(t, h, "FB", "BB", "TX", "CP", "SR", "SB", "ST", "TS", "SS", "F", "A", "Z", "TM", "I", "SX", "X", "Y", "AB2")
				firstBlitError.CompareAndSwap(nil, fmt.Sprintf("RUN AOT Resonance starts out-of-bounds blit: pc=%#x dst=%#x width=%#x height=%#x vars=%#v",
					h.cpu.PC, dst, width, height, vars))
				h.cpu.running.Store(false)
			}
		}
	}
	h.bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, func(addr uint32, value uint32) {
		guardBlit(addr, value)
		video.HandleWrite(addr, value)
		if status := video.HandleRead(BLT_STATUS); status&bltStatusErr != 0 {
			firstBlitError.CompareAndSwap(nil, fmt.Sprintf("RUN AOT Resonance blitter error after write: pc=%#x addr=%#x value=%#x status=%#x dst=%#x width=%#x height=%#x",
				h.cpu.PC, addr, value, status, video.HandleRead(BLT_DST), video.HandleRead(BLT_WIDTH), video.HandleRead(BLT_HEIGHT)))
			h.cpu.running.Store(false)
		}
	})
	h.bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, func(addr uint32, value uint8) {
		guardBlit(addr, uint32(value))
		video.HandleWrite8(addr, value)
		if status := video.HandleRead(BLT_STATUS); status&bltStatusErr != 0 {
			firstBlitError.CompareAndSwap(nil, fmt.Sprintf("RUN AOT Resonance blitter error after byte write: pc=%#x addr=%#x value=%#x status=%#x dst=%#x width=%#x height=%#x",
				h.cpu.PC, addr, value, status, video.HandleRead(BLT_DST), video.HandleRead(BLT_WIDTH), video.HandleRead(BLT_HEIGHT)))
			h.cpu.running.Store(false)
		}
	})

	sound := newTestSoundChip()
	midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
	midiPlayer.AttachBus(h.bus)
	h.bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
	loader := NewMediaLoader(h.bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)
	h.bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			storeLine(t, h, line)
		}
	}
	out := runCommandWithDeadline(h, "RUN AOT", 30*time.Second)
	if msg, ok := firstBlitError.Load().(string); ok && msg != "" {
		t.Fatal(msg)
	}
	if strings.Contains(out, "?") || strings.Contains(out, "ERROR") {
		t.Fatalf("initial Resonance RUN AOT path produced an error: %q\n%s\n%s", out, readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	if status := loader.HandleRead(MEDIA_STATUS); status != MEDIA_STATUS_PLAYING && status != MEDIA_STATUS_LOADING {
		t.Fatalf("RUN AOT SOUND PLAY status=%d, want PLAYING/LOADING, err=%d, type=%d",
			status, loader.HandleRead(MEDIA_ERROR), loader.HandleRead(MEDIA_TYPE))
	}
	if fbBase := video.HandleRead(VIDEO_FB_BASE); fbBase == 0 || fbBase%4096 != 0 {
		t.Fatalf("RUN AOT Resonance VIDEO_FB_BASE = %#x, want nonzero 4 KiB-aligned MEMALLOC address", fbBase)
	}
	frame := video.FinishFrame()
	if len(frame) == 0 {
		t.Fatal("RUN AOT Resonance visible framebuffer produced an empty frame")
	}
	nonzero := 0
	for off := uint32(0); off < 1228800; off += 4096 {
		if binary.LittleEndian.Uint32(frame[off:]) != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		const (
			basicState    = 0x042000
			stCurrentLine = 0x200
			stErrorFlag   = 0x208
			stErrorLine   = 0x228
		)
		vars := readAOTNativeVars(t, h, "FB", "BB", "TX", "CP", "SR", "SB", "ST", "TS", "SS", "F", "A", "Z", "TM", "I", "SX", "X", "Y", "AB2")
		fbNonzero := countNonZeroBusSamples(h, uint32(vars["FB"]), 1228800, 4096)
		bbNonzero := countNonZeroBusSamples(h, uint32(vars["BB"]), 1228800, 4096)
		txNonzero := countNonZeroBusSamples(h, uint32(vars["TX"]), 2097152, 4096)
		srNonzero := countNonZeroBusSamples(h, uint32(vars["SR"]), 235520, 1024)
		t.Fatalf("RUN AOT Resonance visible framebuffer stayed black; pc=%#x videoFB=%#x bltStatus=%#x videoStatus=%#x vars=%#v blt={op:%#x src:%#x dst:%#x width:%#x ctrl:%#x} sampled={FB:%d BB:%d TX:%d SR:%d} state={line:%d error:%d errorLine:%d}",
			h.cpu.PC,
			video.HandleRead(VIDEO_FB_BASE), video.HandleRead(BLT_STATUS), video.HandleRead(VIDEO_STATUS), vars,
			video.HandleRead(BLT_OP), video.HandleRead(BLT_SRC), video.HandleRead(BLT_DST), video.HandleRead(BLT_WIDTH), video.HandleRead(BLT_CTRL),
			fbNonzero, bbNonzero, txNonzero, srNonzero,
			h.bus.Read32(basicState+stCurrentLine),
			h.bus.Read32(basicState+stErrorFlag),
			h.bus.Read32(basicState+stErrorLine))
	}
	const scrollFirstColor = 0xFFA09078
	scrollPixels := 0
	for y := 380; y < 440; y++ {
		for x := 100; x < 540; x++ {
			off := uint32(y*640+x) * 4
			if binary.LittleEndian.Uint32(frame[off:]) == scrollFirstColor {
				scrollPixels++
			}
		}
	}
	if scrollPixels < 100 {
		t.Fatalf("RUN AOT Resonance scroll text absent: found %d expected-colour pixels in scroll band", scrollPixels)
	}
}

func TestResonanceRunAOTMultiFrameKeepsEffectDetail(t *testing.T) {
	program, err := os.ReadFile(resonanceDemoPath)
	if err != nil {
		t.Fatalf("read resonance.bas: %v", err)
	}
	text := strings.ReplaceAll(string(program), "940 VSYNC", "940 REM VSYNC")
	text = strings.ReplaceAll(text, "560 IN=0.12:TI=INT(TM*100)", "560 TM=35:IN=0.12:TI=INT(TM*100)")
	text = strings.ReplaceAll(text, "970 GOTO 510", "970 IF F<8 THEN GOTO 510 ELSE END")

	asmBin := buildAssembler(t)
	repo := repoRootDir(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repo)

	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(h.bus)
	video.SetBigEndianMode(false)
	h.bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	h.bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)

	sound := newTestSoundChip()
	midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
	midiPlayer.AttachBus(h.bus)
	h.bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
	loader := NewMediaLoader(h.bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)
	h.bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			storeLine(t, h, line)
		}
	}
	out := runCommandWithDeadline(h, "RUN AOT", 30*time.Second)
	if strings.Contains(out, "?") || strings.Contains(out, "ERROR") {
		t.Fatalf("multi-frame Resonance RUN AOT produced an error: %q\n%s\n%s", out, readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	frame := video.FinishFrame()
	if len(frame) == 0 {
		t.Fatal("RUN AOT Resonance multi-frame visible framebuffer produced an empty frame")
	}
	unique := map[uint32]int{}
	darkBand := 0
	texturePixels := 0
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			off := uint32(y*640+x) * 4
			px := binary.LittleEndian.Uint32(frame[off:])
			unique[px]++
			if y >= 80 && y < 360 && (px == 0x00060818 || px == 0x0010182A || px == 0x00201824 || px == 0x000A1020) {
				texturePixels++
			}
			if y >= 180 && y < 320 && (px == 0x00101820 || px == 0x00050860 || px == 0x00A8C8D0) {
				darkBand++
			}
		}
	}
	if len(unique) < 16 {
		vars := readAOTNativeVars(t, h, "FB", "BB", "TX", "SR", "SB", "F", "A", "Z", "TM", "IN", "PB", "PU")
		t.Fatalf("RUN AOT Resonance multi-frame has only %d unique colours, want effect detail preserved; top=%v vars=%#v samples={TX:%d BB:%d FB:%d}",
			len(unique), topFrameColours(unique, 8), vars,
			countUniqueBusSamples(h, uint32(vars["TX"]), 2097152, 4096),
			countUniqueBusSamples(h, uint32(vars["BB"]), 1228800, 4096),
			countUniqueBusSamples(h, uint32(vars["FB"]), 1228800, 4096))
	}
	if texturePixels < 1000 {
		t.Fatalf("RUN AOT Resonance multi-frame texture detail weak: %d expected texture pixels", texturePixels)
	}
	if darkBand < 500 {
		t.Fatalf("RUN AOT Resonance multi-frame band detail weak: %d expected band pixels", darkBand)
	}
}

func TestResonanceRunAOTAdvancingTimelineCompletesBoundedLoop(t *testing.T) {
	program, err := os.ReadFile(resonanceDemoPath)
	if err != nil {
		t.Fatalf("read resonance.bas: %v", err)
	}
	text := strings.ReplaceAll(string(program), "940 VSYNC", "940 REM VSYNC")
	text = strings.ReplaceAll(text, "560 IN=0.12:TI=INT(TM*100)", "560 TM=F*0.25:IN=0.12:TI=INT(TM*100)")
	text = strings.ReplaceAll(text, "970 GOTO 510", "970 IF F<80 THEN GOTO 510 ELSE END")

	asmBin := buildAssembler(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repoRootDir(t))

	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(h.bus)
	video.SetBigEndianMode(false)
	h.bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	h.bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)

	sound := newTestSoundChip()
	midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
	midiPlayer.AttachBus(h.bus)
	h.bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
	loader := NewMediaLoader(h.bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)
	h.bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)

	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			storeLine(t, h, line)
		}
	}
	out := runCommandWithDeadline(h, "RUN AOT", 60*time.Second)
	if strings.Contains(out, "?") || strings.Contains(out, "ERROR") {
		t.Fatalf("advancing-timeline Resonance RUN AOT produced an error: %q\n%s\n%s", out, readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	vars := readAOTNativeVars(t, h, "F", "TM", "IN", "PB", "PD", "TX", "BB", "FB")
	if got := vars["F"]; got < 80 {
		t.Fatalf("advancing-timeline Resonance RUN AOT did not complete bounded loop: F=%d vars=%#v\n%s\n%s",
			got, vars, readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	frame := video.FinishFrame()
	if len(frame) == 0 {
		t.Fatal("advancing-timeline Resonance visible framebuffer produced an empty frame")
	}
	unique := len(frameHistogram(frame))
	if unique < 16 {
		t.Fatalf("advancing-timeline Resonance lost effect detail: unique colours=%d top=%v vars=%#v samples={TX:%d BB:%d FB:%d}",
			unique, topFrameColours(frameHistogram(frame), 8), vars,
			countUniqueBusSamples(h, uint32(vars["TX"]), 2097152, 4096),
			countUniqueBusSamples(h, uint32(vars["BB"]), 1228800, 4096),
			countUniqueBusSamples(h, uint32(vars["FB"]), 1228800, 4096))
	}
}

func TestResonanceRunAOTLiveMIDIClockPathCompletesBoundedLoop(t *testing.T) {
	program, err := os.ReadFile(resonanceDemoPath)
	if err != nil {
		t.Fatalf("read resonance.bas: %v", err)
	}
	text := strings.ReplaceAll(string(program), "940 VSYNC", "940 REM VSYNC")
	text = strings.ReplaceAll(text, "970 GOTO 510", "970 IF F<80 THEN GOTO 510 ELSE END")

	asmBin := buildAssembler(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repoRootDir(t))

	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(h.bus)
	video.SetBigEndianMode(false)
	h.bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	h.bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)

	sound := newTestSoundChip()
	midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
	midiPlayer.AttachBus(h.bus)
	h.bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
	loader := NewMediaLoader(h.bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)
	h.bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)
	var midiPos atomic.Uint32
	h.bus.MapIO(MIDI_POSITION, MIDI_POSITION+3, func(addr uint32) uint32 {
		return midiPos.Add(11025)
	}, nil)

	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			storeLine(t, h, line)
		}
	}
	out := runCommandWithDeadline(h, "RUN AOT", 60*time.Second)
	if strings.Contains(out, "?") || strings.Contains(out, "ERROR") {
		t.Fatalf("live-MIDI-clock Resonance RUN AOT produced an error: %q\n%s\n%s", out, readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	vars := readAOTNativeVars(t, h, "F", "TM", "IN", "PB", "PD", "TX", "BB", "FB", "MP", "PM")
	if got := vars["F"]; got < 80 {
		t.Fatalf("live-MIDI-clock Resonance RUN AOT did not complete bounded loop: F=%d vars=%#v midiPos=%d\n%s\n%s",
			got, vars, midiPos.Load(), readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	frame := video.FinishFrame()
	if len(frame) == 0 {
		t.Fatal("live-MIDI-clock Resonance visible framebuffer produced an empty frame")
	}
	unique := len(frameHistogram(frame))
	if unique < 16 {
		t.Fatalf("live-MIDI-clock Resonance lost effect detail: unique colours=%d top=%v vars=%#v midiPos=%d",
			unique, topFrameColours(frameHistogram(frame), 8), vars, midiPos.Load())
	}
}

func TestResonanceRunAOTLiveLoopWithVsyncCompletes(t *testing.T) {
	program, err := os.ReadFile(resonanceDemoPath)
	if err != nil {
		t.Fatalf("read resonance.bas: %v", err)
	}
	text := strings.ReplaceAll(string(program), "970 GOTO 510", "970 IF F<6 THEN GOTO 510 ELSE END")

	asmBin := buildAssembler(t)
	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repoRootDir(t))

	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(h.bus)
	video.SetBigEndianMode(false)
	h.bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	h.bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	h.bus.SetVideoStatusReader(func(addr uint32) uint32 {
		return videoStatusVBlank
	})

	sound := newTestSoundChip()
	midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
	midiPlayer.AttachBus(h.bus)
	h.bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
	loader := NewMediaLoader(h.bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)
	h.bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)
	var midiPos atomic.Uint32
	h.bus.MapIO(MIDI_POSITION, MIDI_POSITION+3, func(addr uint32) uint32 {
		return midiPos.Add(11025)
	}, nil)

	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			storeLine(t, h, line)
		}
	}
	out := runCommandWithDeadline(h, "RUN AOT", 30*time.Second)
	if strings.Contains(out, "?") || strings.Contains(out, "ERROR") {
		t.Fatalf("live-loop Resonance RUN AOT with VSYNC produced an error: %q\n%s\n%s", out, readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	vars := readAOTNativeVars(t, h, "F", "TM", "IN", "PB", "PD", "TX", "BB", "FB", "MP", "PM")
	if got := vars["F"]; got < 6 {
		t.Fatalf("live-loop Resonance RUN AOT with VSYNC did not complete bounded loop: F=%d vars=%#v midiPos=%d\n%s\n%s",
			got, vars, midiPos.Load(), readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	frame := video.FinishFrame()
	if len(frame) == 0 {
		t.Fatal("live-loop Resonance RUN AOT with VSYNC produced an empty frame")
	}
	if unique := len(frameHistogram(frame)); unique < 16 {
		t.Fatalf("live-loop Resonance RUN AOT with VSYNC lost effect detail: unique colours=%d top=%v vars=%#v midiPos=%d",
			unique, topFrameColours(frameHistogram(frame), 8), vars, midiPos.Load())
	}
}

func TestResonanceForcedTimeRunAOTMatchesInterpreterFrame(t *testing.T) {
	program, err := os.ReadFile(resonanceDemoPath)
	if err != nil {
		t.Fatalf("read resonance.bas: %v", err)
	}
	for _, tm := range []string{"35", "50", "95", "160", "190"} {
		t.Run("tm_"+tm, func(t *testing.T) {
			text := strings.ReplaceAll(string(program), "940 VSYNC", "940 REM VSYNC")
			text = strings.ReplaceAll(text, "560 IN=0.12:TI=INT(TM*100)", "560 TM="+tm+":IN=0.12:TI=INT(TM*100)")
			text = strings.ReplaceAll(text, "970 GOTO 510", "970 END")

			runFrame, runMode7 := runResonanceFrameForTest(t, text, false)
			aotFrame, aotMode7 := runResonanceFrameForTest(t, text, true)
			if len(runFrame) != len(aotFrame) {
				t.Fatalf("frame lengths differ: RUN=%d RUN AOT=%d", len(runFrame), len(aotFrame))
			}
			mismatches := 0
			for y := 0; y < 480; y += 7 {
				for x := 0; x < 640; x += 11 {
					off := uint32(y*640+x) * 4
					if binary.LittleEndian.Uint32(runFrame[off:]) != binary.LittleEndian.Uint32(aotFrame[off:]) {
						mismatches++
					}
				}
			}
			if mismatches > 20 {
				t.Fatalf("forced-time Resonance frame mismatch at TM=%s: %d sampled pixels differ; RUN top=%v RUN AOT top=%v Mode7 RUN=%#v RUN AOT=%#v",
					tm, mismatches, topFrameColours(frameHistogram(runFrame), 8), topFrameColours(frameHistogram(aotFrame), 8), runMode7, aotMode7)
			}
		})
	}
}

func runResonanceFrameForTest(t *testing.T, text string, aot bool) ([]byte, [10]uint32) {
	t.Helper()
	asmBin := buildAssembler(t)
	var video *VideoChip
	var mode7 [10]uint32
	setup := func(h *ehbasicTestHarness) {
		var err error
		video, err = NewVideoChip(VIDEO_BACKEND_EBITEN)
		if err != nil {
			t.Fatalf("NewVideoChip: %v", err)
		}
		video.AttachBus(h.bus)
		video.SetBigEndianMode(false)
		h.bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, func(addr uint32, value uint32) {
			if addr == BLT_CTRL && value&1 != 0 && video.HandleRead(BLT_OP) == bltOpMode7 {
				mode7 = [10]uint32{
					video.HandleRead(BLT_SRC), video.HandleRead(BLT_DST),
					video.HandleRead(BLT_MODE7_U0), video.HandleRead(BLT_MODE7_V0),
					video.HandleRead(BLT_MODE7_DU_COL), video.HandleRead(BLT_MODE7_DV_COL),
					video.HandleRead(BLT_MODE7_DU_ROW), video.HandleRead(BLT_MODE7_DV_ROW),
					video.HandleRead(BLT_MODE7_TEX_W), video.HandleRead(BLT_MODE7_TEX_H),
				}
			}
			video.HandleWrite(addr, value)
		})
		h.bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)

		sound := newTestSoundChip()
		midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
		midiPlayer.AttachBus(h.bus)
		h.bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
		loader := NewMediaLoader(h.bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)
		h.bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)
	}
	if !aot {
		makeHarness := func(tb testing.TB) *ehbasicTestHarness {
			tb.Helper()
			bus, err := NewMachineBusSized(256 * 1024 * 1024)
			if err != nil {
				tb.Fatalf("NewMachineBusSized: %v", err)
			}
			bus.ApplyProfileVisibleCeiling(256 * 1024 * 1024)
			return newEhbasicHarnessOnBus(tb, bus)
		}
		out, _ := execStmtTestCoreWithHarness(t, asmBin, text, func(h *ehbasicTestHarness) {
			setup(h)
			fileIO := NewFileIODevice(h.bus, ".")
			h.bus.MapIO(FILE_IO_BASE, FILE_IO_END, fileIO.HandleRead, fileIO.HandleWrite)
			h.bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fileIO.HandleWrite8)
		}, makeHarness)
		if strings.Contains(out, "?") || strings.Contains(out, "ERROR") {
			t.Fatalf("RUN Resonance frame produced an error: %q", out)
		}
		return video.FinishFrame(), mode7
	}

	h := newEhbasicAOTREPLHarnessWithFileIO(t, asmBin, repoRootDir(t))
	setup(h)
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			storeLine(t, h, line)
		}
	}
	out := runCommandWithDeadline(h, "RUN AOT", 30*time.Second)
	if strings.Contains(out, "?") || strings.Contains(out, "ERROR") {
		t.Fatalf("RUN AOT Resonance frame produced an error: %q\n%s\n%s", out, readAOTStateDebug(h), readAOTAsmDebug(h))
	}
	return video.FinishFrame(), mode7
}

func frameHistogram(frame []byte) map[uint32]int {
	hist := map[uint32]int{}
	for off := 0; off+4 <= len(frame); off += 4 {
		hist[binary.LittleEndian.Uint32(frame[off:])]++
	}
	return hist
}

func topFrameColours(hist map[uint32]int, n int) []string {
	type entry struct {
		colour uint32
		count  int
	}
	entries := make([]entry, 0, len(hist))
	for colour, count := range hist {
		entries = append(entries, entry{colour: colour, count: count})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return b.count - a.count
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, fmt.Sprintf("%#08x:%d", e.colour, e.count))
	}
	return out
}

func countUniqueBusSamples(h *ehbasicTestHarness, base uint32, length uint32, step uint32) int {
	if base == 0 {
		return 0
	}
	seen := map[uint32]struct{}{}
	for off := uint32(0); off < length; off += step {
		seen[h.bus.Read32(base+off)] = struct{}{}
	}
	return len(seen)
}

func runCommandWithDeadline(h *ehbasicTestHarness, cmd string, deadline time.Duration) string {
	h.sendInput(cmd + "\n")
	h.cpu.running.Store(true)
	var allOutput strings.Builder
	done := make(chan struct{})
	go func() {
		h.execCPU()
		close(done)
	}()
	timeout := time.After(deadline)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			allOutput.WriteString(h.readOutput())
			return allOutput.String()
		case <-timeout:
			h.cpu.running.Store(false)
			h.waitDone(done)
			allOutput.WriteString(h.readOutput())
			return allOutput.String()
		case <-ticker.C:
			out := h.readOutput()
			allOutput.WriteString(out)
			s := allOutput.String()
			if strings.Contains(s, "\nReady\n") || strings.Contains(s, "\nOk\n") ||
				strings.Contains(s, "\nReady\r\n") || strings.Contains(s, "\nOk\r\n") ||
				strings.HasSuffix(s, "Ready\n") || strings.HasSuffix(s, "Ok\n") ||
				strings.HasSuffix(s, "Ready\r\n") || strings.HasSuffix(s, "Ok\r\n") {
				h.cpu.running.Store(false)
				h.waitDone(done)
				return allOutput.String()
			}
		}
	}
}

func countNonZeroBusSamples(h *ehbasicTestHarness, base uint32, length uint32, step uint32) int {
	if base == 0 {
		return 0
	}
	count := 0
	for off := uint32(0); off < length; off += step {
		if h.bus.Read32(base+off) != 0 {
			count++
		}
	}
	return count
}

func readAOTNativeVars(t *testing.T, h *ehbasicTestHarness, names ...string) map[string]uint64 {
	t.Helper()
	const (
		aotNativeVarSeg = 0x00071000
		valOffset       = 16
		recSize         = 24
	)
	count := h.bus.Read32(aotNativeVarSeg + 8)
	out := make(map[string]uint64, len(names))
	for _, name := range names {
		want := basicAOTVarTag(name)
		for i := uint32(0); i < count; i++ {
			rec := uint32(aotNativeVarSeg + 16 + i*recSize)
			if h.bus.Read32(rec) == want {
				out[name] = h.bus.Read64(rec + valOffset)
				break
			}
		}
	}
	return out
}

func basicAOTVarTag(name string) uint32 {
	var tag uint32
	count := 0
	for _, ch := range strings.ToUpper(name) {
		if (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9' || count == 0) {
			break
		}
		c := uint32(byte(ch))
		if count < 4 {
			tag = (tag << 8) | c
			count++
			continue
		}
		tag = tag*33 + c
	}
	return tag
}
