//go:build headless

package main

import (
	"os"
	"runtime"
	"strings"
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
	if strings.Contains(text, "&HF0084") || strings.Contains(text, "&H000F0084") {
		t.Fatal("resonance.bas must leave FB_BASE unset")
	}
	for _, want := range []string{
		"BLIT MODE7",
		"BLIT MEMCOPY BB,FB,1228800",
		"VSYNC",
		"SOUND PLAY",
		"COPPER",
		"PEEK(&H000F0BB0)",
		"PEEK(&H000F075C)",
		"PM=0:RT0=PEEK(&H000F075C)/1000000",
		"IF MP>PM THEN TM=MP/44100:PM=MP:GOTO 560",
		"TM=RT-RT0",
		"IF TM>=45 THEN GOSUB 2800",
		"IF TM>=90 THEN GOSUB 3100",
		"IF TM>=120 THEN GOSUB 3300",
		"IF TM>=150 THEN GOSUB 3400",
		"SA=SIN(A):SZ=SIN(Z):CZ=COS(A):AB=INT(A*40.743665) AND 255:AB2=INT(A*2*40.743665) AND 255",
		"GOSUB 3500",
		"IF TM>=218 THEN GOTO 3700",
		"IF PM>0 THEN MS=PEEK(&H000F0BAC) AND 1:IF MS=0 THEN GOTO 3700",
		`SOUND PLAY "sdk/examples/assets/music/adagioforstrings.mid"`,
		"GOSUB 1500",
		"970 GOTO 510",
		"RETRO HARDWARE ENGINE",
		"INTUITION ENGINE SOURCE LOGO",
		"MIDI CHORD PULSE ENVELOPE FOR BARS",
		"IF TM>=2.36 AND TM<3.08 THEN PU=28:DR=-1",
		"IF TM>=12.30 AND TM<12.80 THEN PU=18:DR=1",
		"IF TM>=16.85 AND TM<17.40 THEN PU=20:DR=1",
		"IF TM>=104.75 AND TM<105.55 THEN PU=30:DR=-1",
		"IF TM>=184.30 AND TM<185.10 THEN PU=34:DR=1",
		"IF TM>=202.10 AND TM<202.90 THEN PU=34:DR=1",
		"PHRASE-BASED BRIGHTNESS ENVELOPE",
		"PB=0:PD=0",
		"IF TM>=11.70 AND TM<29.00 THEN PB=INT(4+(TM-11.70)*0.5):PD=1",
		"IF TM>=185 AND TM<218 THEN PB=INT(14-(TM-185)*0.3):PD=-1:IF PB<0 THEN PB=0",
		"IF TM>=42.50 AND TM<72.25 THEN PB=INT(8+5*SIN((TM-42.50)*0.45))",
		"IF TM>=149.50 AND TM<185.10 THEN PB=INT(18+8*SIN((TM-149.50)*0.28))",
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
		"POKE &HF0024,SB+SX*4:POKE &HF0028,BB+Y*ST+X*4:POKE &HF002C,16:POKE &HF0030,33",
		"POKE &HF0034,SS:POKE &HF0038,ST:POKE &HF0020,4:POKE &HF001C,1",
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
		"POKE &HF0004,7",
		"POKE &HF0000,1",
		`PRINT "INTUITION ENGINE"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("resonance.bas missing %q", want)
		}
	}
	if strings.Contains(text, "TE=F") || strings.Contains(text, "TE=FR") {
		t.Fatal("timeline must not be driven only by the frame counter")
	}
	for _, forbidden := range []string{"CATHEDRAL", "GOTHIC", "ROSE WINDOW", "STAINED GLASS", "CRUCIFIX", "ADAGIO COMPLETE", "MONOLITH", "DATA WINDOWS", "RAVE LASER"} {
		if strings.Contains(strings.ToUpper(text), forbidden) {
			t.Fatalf("demo must not contain religious/incorrect outro marker %q", forbidden)
		}
	}
	if strings.Contains(text, "POKE &HF0488,48") || strings.Contains(text, "GOSUB 3610") {
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
		"POKE &HF0024,SR+108*ST+SX*4",
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

func TestResonanceSpansStayInVRAM(t *testing.T) {
	const (
		frontBase = 0x100000
		backBase  = 0x230000
		workBase  = 0x360000
		copper    = 0x5E0000
		vramBase  = 0x100000
		vramEnd   = 0x600000
		screen    = 640 * 480 * 4
		work      = 1024 * 512 * 4
		copperLen = 4096
	)

	for name, span := range map[string][2]int{
		"front":  {frontBase, frontBase + screen},
		"back":   {backBase, backBase + screen},
		"work":   {workBase, workBase + work},
		"copper": {copper, copper + copperLen},
	} {
		if span[0] < vramBase || span[1] > vramEnd {
			t.Fatalf("%s span [%#x,%#x) outside VRAM [%#x,%#x)", name, span[0], span[1], vramBase, vramEnd)
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
	out, _ := execStmtTestCore(t, asmBin, text, func(h *ehbasicTestHarness) {
		sound := newTestSoundChip()
		midiPlayer = NewMIDIPlayer(sound, SAMPLE_RATE)
		midiPlayer.AttachBus(h.bus)
		h.bus.MapIO(MIDI_PLAY_PTR, MIDI_TEMPO_BPM+3, midiPlayer.HandlePlayRead, midiPlayer.HandlePlayWrite)
		loader = NewMediaLoader(h.bus, sound, ".", nil, nil, nil, nil, nil, nil, nil, midiPlayer)
		h.bus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, loader.HandleRead, loader.HandleWrite)
	})
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
}
