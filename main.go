// main.go - Main entry point for the IntuitionEngine Virtual Machine

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

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// emutosSentinel is a non-filesystem path passed to runProgramWithFullReset
// to trigger EmuTOS boot without a filename (ROM resolved via loadEmuTOSImage).
const emutosSentinel = "\x00emutos\x00"
const intuitionOSSentinel = "\x00intuitionos\x00"

// Version metadata injected at build time via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type optionalStringFlag struct {
	value string
	set   bool
}

func (f *optionalStringFlag) String() string {
	return f.value
}

func (f *optionalStringFlag) Set(value string) error {
	f.value = value
	f.set = true
	return nil
}

func validateResolutionOverride(w, h int) (int, int, bool) {
	if w > 0 && h > 0 {
		return w, h, true
	}
	if w == 0 && h == 0 {
		return 0, 0, false
	}
	fmt.Println("Warning: -width and -height must be set together; ignoring partial resolution override")
	return 0, 0, false
}

func boilerPlate() {
	fmt.Println("\n\033[38;2;255;20;147m ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████\033[0m\n\033[38;2;255;50;147m▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀\033[0m\n\033[38;2;255;80;147m▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███\033[0m\n\033[38;2;255;110;147m░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄\033[0m\n\033[38;2;255;140;147m░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒\033[0m\n\033[38;2;255;170;147m░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░\033[0m\n\033[38;2;255;200;147m ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░\033[0m\n\033[38;2;255;230;147m ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░\033[0m\n\033[38;2;255;255;147m ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░\033[0m")
	fmt.Println("\nA modern 64-bit RISC re-imagining of Commodore/Atari/Sinclair/BBC/Amstrad/IBM 8/16/32-bit home computers.")
	fmt.Println("Default core: IE64. Also supports IE32, M68K, x86, Z80, and 6502 CPU modes.")
	fmt.Println("Video: IEVideoChip, VGA, ZX Spectrum ULA, Commodore TED video, Atari ANTIC/GTIA, 3DFX Voodoo.")
	fmt.Println("Audio: IESoundChip, AY/YM/PSG, SID, POKEY, TED audio, ProTracker MOD, PCM WAV, Amiga AHX Resynth, TI SN76489, Amiga Paula DMA.")
	fmt.Println("(c) 2024 - 2026 Zayn Otley")
	fmt.Println("https://github.com/IntuitionAmiga/IntuitionEngine")
	fmt.Println("Buy me a coffee: https://ko-fi.com/intuition/tip")
	fmt.Println("License: GPLv3 or later")
}

//func main() {
//	boilerPlate()
//
//	sysBus := NewMachineBus()
//	cpu := NewCPU(sysBus)
//
//	soundChip, err := NewSoundChip(AUDIO_BACKEND_OTO)
//	if err != nil {
//		fmt.Printf("Failed to initialize sound: %v\n", err)
//		os.Exit(1)
//	}
//
//	videoChip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
//	if err != nil {
//		fmt.Printf("Failed to initialize video: %v\n", err)
//		os.Exit(1)
//	}
//
//	// Map sound registers
//	sysBus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
//		nil, // No read handler needed
//		soundChip.HandleRegisterWrite)
//
//	// Map video registers and VRAM
//	sysBus.MapIO(VIDEO_CTRL, VIDEO_STATUS,
//		videoChip.HandleRead,
//		videoChip.HandleWrite)
//	sysBus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
//		videoChip.HandleRead,
//		videoChip.HandleWrite)
//
//	var startExecution bool
//	if len(os.Args) > 1 {
//		if err := cpu.LoadProgram(os.Args[1]); err != nil {
//			fmt.Printf("Error loading program: %v\n", err)
//			os.Exit(1)
//		}
//		startExecution = true
//	}
//
//	// Initialize GUI
//	gui, err := NewGUIFrontend(cpu, videoChip, soundChip)
//	if err != nil {
//		fmt.Printf("Failed to initialize GUI: %v\n", err)
//		os.Exit(1)
//	}
//
//	config := GUIConfig{
//		Width:     800,
//		Height:    600,
//		Title:     "Intuition Engine",
//		Resizable: true,
//	}
//
//	if err := gui.Initialize(config); err != nil {
//		fmt.Printf("Failed to configure GUI: %v\n", err)
//		os.Exit(1)
//	}
//
//	// Start execution if we loaded a program
//	if startExecution {
//		videoChip.Start()
//		soundChip.Start()
//		go cpu.ExecuteInstruction()
//	}
//
//	// Show the GUI and run the main event loop
//	err = gui.Show()
//	if err != nil {
//		return
//	}
//}

func main() {
	// Handle -version and -features before boilerplate so output is clean and script-friendly
	for _, arg := range os.Args[1:] {
		if arg == "-version" || arg == "--version" {
			fmt.Printf("Intuition Engine %s\n", Version)
			fmt.Printf("  Commit:     %s\n", Commit)
			fmt.Printf("  Built:      %s\n", BuildDate)
			fmt.Printf("  Go version: %s\n", runtime.Version())
			fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
			os.Exit(0)
		}
		if arg == "-features" || arg == "--features" {
			printFeatures()
			os.Exit(0)
		}
	}

	boilerPlate()

	var (
		modeIE32    bool
		modeIE64    bool
		modeBasic   bool
		modeTerm    bool
		basicImage  string
		modeM68K    bool
		modeEmuTOS  bool
		emutosImage string
		modeAROS    bool
		arosImage   string
		modeM6502   bool
		modeZ80     bool
		modeX86     bool
		modePSG     bool
		modeSID     bool
		psgPlus     bool
		sidPlus     bool
		modePOKEY   bool
		pokeyPlus   bool
		modeTED     bool
		tedPlus     bool
		modeAHX     bool
		ahxPlus     bool
		modeMOD     bool
		modeWAV     bool
		perfMode    bool
		sidFile     string
		sidDebug    int
		sidPAL      bool
		sidNTSC     bool
		loadAddr    optionalStringFlag
		entryAddr   optionalStringFlag
		resWidth    int
		resHeight   int
		scale       int
		fullscreen  bool
		scriptFile  string
		noJIT       bool
	)

	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.BoolVar(&modeIE32, "ie32", false, "Run IE32 CPU mode")
	flagSet.BoolVar(&modeIE64, "ie64", false, "Run IE64 CPU mode (64-bit RISC)")
	flagSet.BoolVar(&modeBasic, "basic", false, "Run EhBASIC IE64 interpreter (embedded image)")
	flagSet.BoolVar(&modeTerm, "term", false, "Use console terminal with -basic")
	flagSet.StringVar(&basicImage, "basic-image", "", "Run EhBASIC IE64 from custom binary path")
	flagSet.BoolVar(&modeM68K, "m68k", false, "Run M68K CPU mode")
	flagSet.BoolVar(&modeEmuTOS, "emutos", false, "Run EmuTOS (M68K ROM)")
	flagSet.StringVar(&emutosImage, "emutos-image", "", "Run EmuTOS from custom ROM image path")
	flagSet.BoolVar(&modeAROS, "aros", false, "Run AROS (M68K Workbench)")
	flagSet.StringVar(&arosImage, "aros-image", "", "Run AROS from custom ROM image path")
	flagSet.BoolVar(&modeM6502, "m6502", false, "Run 6502 CPU mode")
	flagSet.BoolVar(&modeZ80, "z80", false, "Run Z80 CPU mode")
	flagSet.BoolVar(&modeX86, "x86", false, "Run x86 CPU mode (32-bit flat model)")
	flagSet.BoolVar(&modePSG, "psg", false, "Play PSG file")
	flagSet.StringVar(&sidFile, "sid", "", "Play SID file")
	flagSet.IntVar(&sidDebug, "sid-debug", 0, "Log SID timing and ADSR changes for N seconds")
	flagSet.BoolVar(&sidPAL, "sid-pal", false, "Force PAL timing for SID playback")
	flagSet.BoolVar(&sidNTSC, "sid-ntsc", false, "Force NTSC timing for SID playback")
	flagSet.BoolVar(&psgPlus, "psg+", false, "Enable PSG+ enhancements")
	flagSet.BoolVar(&sidPlus, "sid+", false, "Enable SID+ enhancements")
	flagSet.BoolVar(&modePOKEY, "pokey", false, "Play SAP file (POKEY emulation)")
	flagSet.BoolVar(&pokeyPlus, "pokey+", false, "Enable POKEY+ enhancements")
	flagSet.BoolVar(&modeTED, "ted", false, "Play TED file (Plus/4 TED emulation)")
	flagSet.BoolVar(&tedPlus, "ted+", false, "Enable TED+ enhancements")
	flagSet.BoolVar(&modeAHX, "ahx", false, "Play AHX file (Amiga AHX module)")
	flagSet.BoolVar(&ahxPlus, "ahx+", false, "Enable AHX+ enhanced mode")
	flagSet.BoolVar(&modeMOD, "mod", false, "Play ProTracker MOD file (Amiga 4-channel)")
	flagSet.BoolVar(&modeWAV, "wav", false, "Play WAV file (PCM audio)")
	flagSet.BoolVar(&perfMode, "perf", false, "Enable performance measurement (MIPS reporting)")
	flagSet.IntVar(&resWidth, "width", 0, "Override output width (0 = auto)")
	flagSet.IntVar(&resHeight, "height", 0, "Override output height (0 = auto)")
	flagSet.IntVar(&scale, "scale", 1, "Integer window scale factor (1-4)")
	flagSet.BoolVar(&fullscreen, "fullscreen", false, "Start in fullscreen mode")
	flagSet.StringVar(&scriptFile, "script", "", "Run IES Lua script file after startup")
	flagSet.BoolVar(&noJIT, "nojit", false, "Disable JIT compilation, use interpreter only")
	var emutosDrive string
	flagSet.StringVar(&emutosDrive, "emutos-drive", "", "Host directory to map as GEMDOS drive U: (default: ~/)")
	var arosDrive string
	flagSet.StringVar(&arosDrive, "aros-drive", "", "Host directory for AROS DOS volume (default: ~/)")
	flagSet.Bool("version", false, "Print version information and exit")
	loadAddr.value = "0x0600"
	flagSet.Var(&loadAddr, "load-addr", "6502/Z80 load address (hex or decimal, defaults: 6502=0x0600, Z80=0x0000)")
	flagSet.Var(&entryAddr, "entry", "6502/Z80 entry address (hex or decimal, defaults to load address)")

	flagSet.Usage = func() {
		flagSet.SetOutput(os.Stdout)
		fmt.Println("Usage: ./intuition_engine [mode] [options] [filename]")
		fmt.Println("Default (no mode/filename): start EhBASIC IE64.")
		fmt.Println("Default core: IE64. Also supports IE32, M68K, x86, Z80, and 6502 CPU modes.")
		fmt.Println("Video: IEVideoChip, VGA, ZX Spectrum ULA, Commodore TED video, Atari ANTIC/GTIA, 3DFX Voodoo.")
		fmt.Println("Audio: IESoundChip, AY/YM/PSG, SID, POKEY, TED audio, ProTracker MOD, PCM WAV, Amiga AHX Resynth, TI SN76489, Amiga Paula DMA.")
		flagSet.PrintDefaults()
	}

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	filename := flagSet.Arg(0)

	// Single-instance IPC handoff: if another instance is already running and we have
	// a file to open, send it via IPC and exit. This must happen before hardware init.
	if filename != "" {
		absPath, absErr := filepath.Abs(filename)
		if absErr == nil {
			if _, extErr := modeFromExtension(absPath); extErr == nil {
				if err := SendIPCOpen(absPath); err == nil {
					fmt.Printf("Sent %s to running instance\n", filepath.Base(filename))
					os.Exit(0)
				}
				// SendIPCOpen failed - no running instance, continue as primary
			}
		}
	}

	validWidth, validHeight, useResolutionOverride := validateResolutionOverride(resWidth, resHeight)

	if sidFile != "" {
		modeSID = true
	}
	if psgPlus && !modePSG {
		modePSG = true
	}
	if sidPlus && !modeSID {
		modeSID = true
	}
	if pokeyPlus && !modePOKEY {
		modePOKEY = true
	}
	if tedPlus && !modeTED {
		modeTED = true
	}
	if ahxPlus && !modeAHX {
		modeAHX = true
	}
	// -basic-image implies -basic mode
	if basicImage != "" {
		modeBasic = true
	}
	// -emutos-image implies -emutos mode
	if emutosImage != "" {
		modeEmuTOS = true
	}
	// -aros-image implies -aros mode
	if arosImage != "" {
		modeAROS = true
	}

	// Resolve AROS drive config (host filesystem mapping).
	exePath, _ := os.Executable()
	arosHostRoot := resolveAROSDrivePath(arosDrive, exePath)

	// Resolve GEMDOS drive config for EmuTOS (always, since EmuTOS can be
	// launched dynamically from BASIC or the program executor)
	gemdosHostRoot := emutosDrive
	gemdosDriveNum := uint16(20) // U: = drive 20
	if gemdosHostRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gemdosHostRoot = home
		}
	}

	// -basic is IE64 mode with embedded/custom BASIC image
	if modeBasic {
		modeIE64 = true
	}
	useGraphicalTerm := false

	modeCount := 0
	if modeIE32 {
		modeCount++
	}
	if modeIE64 {
		modeCount++
	}
	if modeM68K {
		modeCount++
	}
	if modeEmuTOS {
		modeCount++
	}
	if modeAROS {
		modeCount++
	}
	if modeM6502 {
		modeCount++
	}
	if modeZ80 {
		modeCount++
	}
	if modeX86 {
		modeCount++
	}
	if modePSG {
		modeCount++
	}
	if modeSID {
		modeCount++
	}
	if modePOKEY {
		modeCount++
	}
	if modeTED {
		modeCount++
	}
	if modeAHX {
		modeCount++
	}
	if modeMOD {
		modeCount++
	}
	if modeWAV {
		modeCount++
	}
	if modeCount == 0 && filename == "" {
		modeBasic = true
		modeIE64 = true
		modeCount = 1
	}
	useGraphicalTerm = modeBasic && !modeTerm
	if modeCount != 1 {
		fmt.Println("Error: select exactly one mode flag: -ie32, -ie64, -m68k, -emutos, -m6502, -z80, -x86, -basic, -psg, -psg+, -sid, -sid+, -pokey, -pokey+, -ted, -ted+, -ahx, -ahx+, -mod, or -wav")
		os.Exit(1)
	}
	if modeBasic && filename != "" {
		fmt.Println("Error: -basic and -basic-image do not accept a positional filename")
		os.Exit(1)
	}
	if modeTerm && !modeBasic {
		fmt.Println("Error: -term is only valid with -basic")
		os.Exit(1)
	}
	if filename == "" && modePSG {
		fmt.Println("Error: PSG mode requires a filename")
		os.Exit(1)
	}
	if sidPAL && sidNTSC {
		fmt.Println("Error: choose only one of -sid-pal or -sid-ntsc")
		os.Exit(1)
	}
	if modeSID && sidFile == "" {
		sidFile = filename
	}
	if modeSID && sidFile == "" {
		fmt.Println("Error: SID mode requires a filename")
		os.Exit(1)
	}
	if filename == "" && !modeBasic {
		switch {
		case modeIE32:
			fmt.Println("Error: IE32 mode requires a filename")
			os.Exit(1)
		case modeIE64:
			fmt.Println("Error: IE64 mode requires a filename (or use -basic)")
			os.Exit(1)
		case modeM68K:
			fmt.Println("Error: M68K mode requires a filename")
			os.Exit(1)
		case modeEmuTOS:
			if emutosImage == "" && filename == "" && len(embeddedEmuTOSImage) == 0 && resolveDefaultEmuTOSImagePath() == "" {
				fmt.Println("Error: EmuTOS mode requires -emutos-image <path> or an embedded EmuTOS ROM")
				os.Exit(1)
			}
		case modeM6502:
			fmt.Println("Error: 6502 mode requires a filename")
			os.Exit(1)
		case modeZ80:
			fmt.Println("Error: Z80 mode requires a filename")
			os.Exit(1)
		case modeX86:
			fmt.Println("Error: x86 mode requires a filename")
			os.Exit(1)
		}
	}

	// Initialize sound first (used by all modes)
	soundChip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		fmt.Printf("Failed to initialize sound: %v\n", err)
		os.Exit(1)
	}

	psgEngine := NewPSGEngine(soundChip, SAMPLE_RATE)
	psgPlayer := NewPSGPlayer(psgEngine)
	if psgPlus {
		psgEngine.SetPSGPlusEnabled(true)
	}

	sidEngine := NewSIDEngine(soundChip, SAMPLE_RATE)
	sid2Engine := NewSIDEngineMulti(soundChip, SAMPLE_RATE, 4, SID2_BASE, SID2_END)
	sid3Engine := NewSIDEngineMulti(soundChip, SAMPLE_RATE, 7, SID3_BASE, SID3_END)
	sidEngine.sid2 = sid2Engine
	sidEngine.sid3 = sid3Engine
	sidPlayer := NewSIDPlayer(sidEngine)
	if sidPlus {
		sidEngine.SetSIDPlusEnabled(true)
	}

	if modePSG {
		if filename == "" {
			fmt.Println("Error: PSG mode requires a filename")
			os.Exit(1)
		}
		if err := psgPlayer.Load(filename); err != nil {
			fmt.Printf("Error loading PSG file: %v\n", err)
			os.Exit(1)
		}
		meta := psgPlayer.Metadata()
		if meta.Title != "" || meta.Author != "" {
			fmt.Printf("Playing: %s - %s", meta.Title, meta.Author)
		} else {
			fmt.Printf("Playing: %s", filename)
		}
		if dur := psgPlayer.DurationText(); dur != "" {
			fmt.Printf(" (%s)", dur)
		}
		fmt.Println()
		if perfMode {
			instrCount, cpuName, execNanos := psgPlayer.RenderPerf()
			if cpuName != "" {
				if execNanos > 0 {
					secs := float64(execNanos) / 1e9
					mips := float64(instrCount) / secs / 1e6
					fmt.Printf("PSG (%s): %.2f MIPS (%d instructions in %.3fs)\n",
						cpuName, mips, instrCount, secs)
				} else {
					fmt.Printf("PSG (%s): %d instructions (too fast to measure)\n",
						cpuName, instrCount)
				}
			} else {
				fmt.Printf("PSG: register dump - no CPU to measure\n")
			}
		}
		soundChip.Start()
		psgPlayer.Play()
		// Wait for playback to complete, then exit
		for psgEngine.IsPlaying() {
			time.Sleep(100 * time.Millisecond)
		}
		soundChip.Stop()
		os.Exit(0)
	}

	if modeSID {
		if err := sidPlayer.LoadWithOptions(sidFile, 0, sidPAL, sidNTSC); err != nil {
			fmt.Printf("Error loading SID file: %v\n", err)
			os.Exit(1)
		}
		if sidDebug > 0 {
			sidEngine.EnableDebugLogging(sidDebug)
		}
		meta := sidPlayer.Metadata()
		if meta.Title != "" || meta.Author != "" {
			fmt.Printf("Playing: %s - %s", meta.Title, meta.Author)
		} else {
			fmt.Printf("Playing: %s", sidFile)
		}
		if meta.Released != "" {
			fmt.Printf(" (%s)", meta.Released)
		}
		if dur := sidPlayer.DurationText(); dur != "" {
			fmt.Printf(" [%s]", dur)
		}
		fmt.Println()
		if perfMode {
			instrCount, cpuName, execNanos := sidPlayer.RenderPerf()
			if cpuName != "" {
				if execNanos > 0 {
					secs := float64(execNanos) / 1e9
					mips := float64(instrCount) / secs / 1e6
					fmt.Printf("SID (%s): %.2f MIPS (%d instructions in %.3fs)\n",
						cpuName, mips, instrCount, secs)
				} else {
					fmt.Printf("SID (%s): %d instructions (too fast to measure)\n",
						cpuName, instrCount)
				}
			}
		}
		soundChip.Start()
		sidPlayer.Play()
		for sidPlayer.IsPlaying() {
			time.Sleep(100 * time.Millisecond)
		}
		soundChip.Stop()
		os.Exit(0)
	}

	// POKEY/SAP playback mode
	if modePOKEY {
		if filename == "" {
			fmt.Println("Error: POKEY mode requires a SAP filename")
			os.Exit(1)
		}
		pokeyEngine := NewPOKEYEngine(soundChip, SAMPLE_RATE)
		soundChip.SetSampleTicker(pokeyEngine) // Register for sample-accurate event processing
		pokeyPlayer := NewPOKEYPlayer(pokeyEngine)
		if pokeyPlus {
			pokeyEngine.SetPOKEYPlusEnabled(true)
		}
		if err := pokeyPlayer.Load(filename); err != nil {
			fmt.Printf("Error loading SAP file: %v\n", err)
			os.Exit(1)
		}
		meta := pokeyPlayer.Metadata()
		if meta.Title != "" || meta.Author != "" {
			fmt.Printf("Playing: %s - %s", meta.Title, meta.Author)
		} else {
			fmt.Printf("Playing: %s", filename)
		}
		if dur := pokeyPlayer.DurationText(); dur != "" {
			fmt.Printf(" (%s)", dur)
		}
		fmt.Println()
		if perfMode {
			instrCount, cpuName, execNanos := pokeyPlayer.RenderPerf()
			if cpuName != "" {
				if execNanos > 0 {
					secs := float64(execNanos) / 1e9
					mips := float64(instrCount) / secs / 1e6
					fmt.Printf("SAP (%s): %.2f MIPS (%d instructions in %.3fs)\n",
						cpuName, mips, instrCount, secs)
				} else {
					fmt.Printf("SAP (%s): %d instructions (too fast to measure)\n",
						cpuName, instrCount)
				}
			}
		}
		soundChip.Start()
		pokeyPlayer.Play()
		// Wait for playback to complete
		for pokeyPlayer.IsPlaying() {
			time.Sleep(100 * time.Millisecond)
		}
		soundChip.Stop()
		os.Exit(0)
	}

	// TED playback mode
	if modeTED {
		if filename == "" {
			fmt.Println("Error: TED mode requires a .ted filename")
			os.Exit(1)
		}
		tedEngine := NewTEDEngine(soundChip, SAMPLE_RATE)
		soundChip.SetSampleTicker(tedEngine) // Register for sample-accurate event processing
		tedPlayer := NewTEDPlayer(tedEngine)
		if tedPlus {
			tedEngine.SetTEDPlusEnabled(true)
		}
		if err := tedPlayer.Load(filename); err != nil {
			fmt.Printf("Error loading TED file: %v\n", err)
			os.Exit(1)
		}
		meta := tedPlayer.Metadata()
		if meta.Title != "" || meta.Author != "" {
			fmt.Printf("Playing: %s - %s", meta.Title, meta.Author)
		} else {
			fmt.Printf("Playing: %s", filename)
		}
		if meta.Date != "" {
			fmt.Printf(" (%s)", meta.Date)
		}
		if dur := tedPlayer.DurationText(); dur != "" {
			fmt.Printf(" [%s]", dur)
		}
		fmt.Println()
		if perfMode {
			instrCount, cpuName, execNanos := tedPlayer.RenderPerf()
			if cpuName != "" {
				if execNanos > 0 {
					secs := float64(execNanos) / 1e9
					mips := float64(instrCount) / secs / 1e6
					fmt.Printf("TED (%s): %.2f MIPS (%d instructions in %.3fs)\n",
						cpuName, mips, instrCount, secs)
				} else {
					fmt.Printf("TED (%s): %d instructions (too fast to measure)\n",
						cpuName, instrCount)
				}
			}
		}
		soundChip.Start()
		tedPlayer.Play()
		// Wait for playback to complete
		for tedPlayer.IsPlaying() {
			time.Sleep(100 * time.Millisecond)
		}
		soundChip.Stop()
		os.Exit(0)
	}

	// AHX playback mode
	if modeAHX {
		if filename == "" {
			fmt.Println("Error: AHX mode requires an AHX filename")
			os.Exit(1)
		}
		ahxPlayer := NewAHXPlayer(soundChip, SAMPLE_RATE)
		// Register the player's internal engine with SoundChip for sample-accurate ticking
		soundChip.SetSampleTicker(ahxPlayer.engine)
		if ahxPlus {
			ahxPlayer.engine.SetAHXPlusEnabled(true)
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("Error reading AHX file: %v\n", err)
			os.Exit(1)
		}
		if err := ahxPlayer.Load(data); err != nil {
			fmt.Printf("Error loading AHX file: %v\n", err)
			os.Exit(1)
		}
		meta := ahxPlayer.Metadata()
		if meta.Name != "" {
			fmt.Printf("Playing: %s", meta.Name)
		} else {
			fmt.Printf("Playing: %s", filename)
		}
		if ahxPlus {
			fmt.Print(" [AHX+]")
		}
		fmt.Println()
		if perfMode {
			instrCount, cpuName, execNanos := ahxPlayer.RenderPerf()
			if cpuName != "" {
				if execNanos > 0 {
					secs := float64(execNanos) / 1e9
					mips := float64(instrCount) / secs / 1e6
					fmt.Printf("AHX (%s): %.2f MIPS (%d instructions in %.3fs)\n",
						cpuName, mips, instrCount, secs)
				} else {
					fmt.Printf("AHX (%s): %d instructions (too fast to measure)\n",
						cpuName, instrCount)
				}
			} else {
				fmt.Printf("AHX: software module replay - no CPU to measure\n")
			}
		}
		soundChip.Start()
		ahxPlayer.Play()
		// Wait for playback to complete
		for ahxPlayer.IsPlaying() {
			time.Sleep(100 * time.Millisecond)
		}
		soundChip.Stop()
		os.Exit(0)
	}

	// MOD playback mode
	if modeMOD {
		if filename == "" {
			fmt.Println("Error: MOD mode requires a MOD filename")
			os.Exit(1)
		}
		modPlayerStandalone := NewMODPlayer(soundChip, SAMPLE_RATE)
		soundChip.SetSampleTicker(modPlayerStandalone.engine)
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("Error reading MOD file: %v\n", err)
			os.Exit(1)
		}
		if err := modPlayerStandalone.Load(data); err != nil {
			fmt.Printf("Error loading MOD file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Playing: %s\n", filename)
		soundChip.Start()
		modPlayerStandalone.Play()
		for modPlayerStandalone.IsPlaying() {
			time.Sleep(100 * time.Millisecond)
		}
		soundChip.Stop()
		os.Exit(0)
	}

	// WAV playback mode
	if modeWAV {
		if filename == "" {
			fmt.Println("Error: WAV mode requires a WAV filename")
			os.Exit(1)
		}
		wavPlayerStandalone := NewWAVPlayer(soundChip, SAMPLE_RATE)
		soundChip.SetSampleTicker(wavPlayerStandalone.engine)
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("Error reading WAV file: %v\n", err)
			os.Exit(1)
		}
		if err := wavPlayerStandalone.Load(data); err != nil {
			fmt.Printf("Error loading WAV file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Playing: %s\n", filename)
		soundChip.Start()
		wavPlayerStandalone.Play()
		for wavPlayerStandalone.IsPlaying() {
			time.Sleep(100 * time.Millisecond)
		}
		soundChip.Stop()
		os.Exit(0)
	}

	// Create system bus
	sysBus := NewMachineBus()

	// PLAN_MAX_RAM.md slice 1+2: detect host RAM and publish guest RAM
	// sizing on the bus so SYSINFO MMIO (and the GetSysInfo syscall) can
	// report meaningful values. Active visible RAM is clamped to the
	// legacy bus.memory[] window (DEFAULT_MEMORY_SIZE) since the IE32
	// surface still uses the 32 MB ABI; total guest RAM reflects the
	// host autodetection result so AVAIL "Phys" prints actual host RAM.
	// On detection failure (tests, non-Linux, missing /proc/meminfo),
	// fall back to a sizing where total = active = DEFAULT_MEMORY_SIZE
	// so SYSINFO is at least non-zero.
	{
		// First try autodetection. If platform classification or
		// /proc/meminfo parsing fails, fall back to publishing the
		// legacy 32 MB sizing so SYSINFO is at least non-zero.
		ms, err := ComputeMemorySizing(uint64(DEFAULT_MEMORY_SIZE), SizingOverrides{})
		if err != nil {
			ms = MemorySizing{
				DetectedUsableRAM: uint64(DEFAULT_MEMORY_SIZE),
				TotalGuestRAM:     uint64(DEFAULT_MEMORY_SIZE),
				ActiveVisibleRAM:  uint64(DEFAULT_MEMORY_SIZE),
				VisibleCeiling:    uint64(DEFAULT_MEMORY_SIZE),
			}
		}
		sysBus.SetSizing(ms)
		RegisterSysInfoMMIOFromBus(sysBus)
	}
	psgPlayer.AttachBus(sysBus)
	sidPlayer.AttachBus(sysBus)

	videoChip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		fmt.Printf("Failed to initialize video: %v\n", err)
		os.Exit(1)
	}
	videoChip.AttachBus(sysBus)

	// Setup terminal MMIO device (host adapter started later, just before GUI loop)
	termMMIO := NewTerminalMMIO()
	if setter, ok := videoChip.GetOutput().(TerminalMMIOSetter); ok {
		setter.SetTerminalMMIO(termMMIO)
	}
	var termHost *TerminalHost
	var videoTerm *VideoTerminal
	var outputTicker *time.Ticker
	var outputStop chan struct{}
	if useGraphicalTerm {
		videoTerm = NewVideoTerminal(videoChip, termMMIO)
		initTerminalClipboard(videoTerm)
		termMMIO.SetForceEchoOff(true)
		videoTerm.Start()
		if ki, ok := videoChip.GetOutput().(KeyboardInput); ok {
			ki.SetKeyHandler(videoTerm.HandleKeyInput)
		}
		if si, ok := videoChip.GetOutput().(ScrollInput); ok {
			si.SetScrollHandler(videoTerm.HandleScroll)
		}
		if ci, ok := videoChip.GetOutput().(ClipboardInput); ok {
			ci.SetCopyHandler(videoTerm.CopySelection)
			ci.SetCutHandler(videoTerm.CutSelection)
			ci.SetMiddleMouseHandler(videoTerm.MiddleMousePaste)
		}
	} else if !modeEmuTOS {
		termHost = NewTerminalHost(termMMIO)
	}

	// Map I/O regions for peripherals — all devices registered unconditionally
	// so that every CPU mode (including EmuTOS) can access the full hardware.
	sysBus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
		soundChip.HandleRegisterRead,
		soundChip.HandleRegisterWrite)
	sysBus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, soundChip.HandleRegisterWrite8)

	sysBus.MapIO(VIDEO_CTRL, VIDEO_REG_END,
		videoChip.HandleRead,
		videoChip.HandleWrite)
	sysBus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, videoChip.HandleWrite8)

	// Register lock-free VIDEO_STATUS reader for fast VBlank polling
	sysBus.SetVideoStatusReader(videoChip.HandleRead)

	sysBus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
		videoChip.HandleRead,
		videoChip.HandleWrite)
	sysBus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1, videoChip.HandleWrite8)

	sysBus.MapIO(TERM_OUT, TERMINAL_REGION_END,
		termMMIO.HandleRead,
		termMMIO.HandleWrite)

	// Map PSG registers
	sysBus.MapIO(PSG_BASE, PSG_END,
		psgEngine.HandleRead,
		psgEngine.HandleWrite)
	sysBus.MapIO(PSG_PLUS_CTRL, PSG_PLUS_CTRL,
		psgEngine.HandlePSGPlusRead,
		psgEngine.HandlePSGPlusWrite)
	sysBus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3,
		psgPlayer.HandlePlayRead,
		psgPlayer.HandlePlayWrite)

	// Map SID registers
	sysBus.MapIO(SID_BASE, SID_END,
		sidEngine.HandleRead,
		sidEngine.HandleWrite)
	sysBus.MapIO(SID_PLAY_PTR, SID_SUBSONG,
		sidPlayer.HandlePlayRead,
		sidPlayer.HandlePlayWrite)

	// Map SID2/SID3 registers for multi-SID playback
	sysBus.MapIO(SID2_BASE, SID2_END,
		sid2Engine.HandleRead,
		sid2Engine.HandleWrite)
	sysBus.MapIO(SID3_BASE, SID3_END,
		sid3Engine.HandleRead,
		sid3Engine.HandleWrite)

	// Map TED audio registers
	tedEngine := NewTEDEngine(soundChip, SAMPLE_RATE)
	tedPlayer := NewTEDPlayer(tedEngine)
	tedPlayer.AttachBus(sysBus)
	sysBus.MapIO(TED_BASE, TED_END,
		tedEngine.HandleRead,
		tedEngine.HandleWrite)
	sysBus.MapIO(TED_PLAY_PTR, TED_PLAY_STATUS+3,
		tedPlayer.HandlePlayRead,
		tedPlayer.HandlePlayWrite)

	// Map AHX registers (Amiga AHX module player)
	ahxPlayerCPU := NewAHXPlayer(soundChip, SAMPLE_RATE)
	ahxPlayerCPU.AttachBus(sysBus)
	sysBus.MapIO(AHX_BASE, AHX_SUBSONG,
		ahxPlayerCPU.HandlePlayRead,
		ahxPlayerCPU.HandlePlayWrite)

	// Map MOD registers (ProTracker MOD player)
	modPlayer := NewMODPlayer(soundChip, SAMPLE_RATE)
	modPlayer.AttachBus(sysBus)
	sysBus.MapIO(MOD_PLAY_PTR, MOD_END,
		modPlayer.HandlePlayRead,
		modPlayer.HandlePlayWrite)

	// Map WAV registers (PCM WAV player)
	wavPlayer := NewWAVPlayer(soundChip, SAMPLE_RATE)
	wavPlayer.AttachBus(sysBus)
	sysBus.MapIO(WAV_PLAY_PTR, WAV_END,
		wavPlayer.HandlePlayRead,
		wavPlayer.HandlePlayWrite)

	// Map POKEY registers (Atari POKEY chip for SAP playback)
	pokeyEngine := NewPOKEYEngine(soundChip, SAMPLE_RATE)
	pokeyPlayer := NewPOKEYPlayer(pokeyEngine)
	pokeyPlayer.AttachBus(sysBus)
	sysBus.MapIO(POKEY_BASE, POKEY_END,
		pokeyEngine.HandleRead,
		pokeyEngine.HandleWrite)
	sysBus.MapIO(SAP_PLAY_PTR, SAP_SUBSONG,
		pokeyPlayer.HandlePlayRead,
		pokeyPlayer.HandlePlayWrite)

	// Map VGA registers
	var vgaEngine *VGAEngine
	var ulaEngine *ULAEngine
	var tedVideoEngine *TEDVideoEngine
	var anticEngine *ANTICEngine
	var voodooEngine *VoodooEngine

	vgaEngine = NewVGAEngine(sysBus)
	sysBus.MapIO(VGA_BASE, VGA_REG_END,
		vgaEngine.HandleRead,
		vgaEngine.HandleWrite)
	sysBus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1,
		vgaEngine.HandleVRAMRead,
		vgaEngine.HandleVRAMWrite)
	sysBus.MapIO(VGA_TEXT_WINDOW, VGA_TEXT_WINDOW+VGA_TEXT_SIZE-1,
		vgaEngine.HandleTextRead,
		vgaEngine.HandleTextWrite)

	// Map ULA registers (ZX Spectrum video chip)
	ulaEngine = NewULAEngine(sysBus)
	sysBus.MapIO(ULA_BASE, ULA_REG_END,
		ulaEngine.HandleRead,
		ulaEngine.HandleWrite)
	sysBus.MapIO(ULA_VRAM_BASE, ULA_VRAM_BASE+ULA_VRAM_SIZE-1,
		ulaEngine.HandleBusVRAMRead,
		ulaEngine.HandleBusVRAMWrite)

	// Map TED video registers (Commodore Plus/4 video chip)
	tedVideoEngine = NewTEDVideoEngine(sysBus)
	sysBus.MapIO(TED_VIDEO_BASE, TED_VIDEO_END,
		tedVideoEngine.HandleRead,
		tedVideoEngine.HandleWrite)
	sysBus.MapIO(TED_V_VRAM_BASE, TED_V_VRAM_BASE+TED_V_VRAM_SIZE-1,
		tedVideoEngine.HandleBusVRAMRead,
		tedVideoEngine.HandleBusVRAMWrite)

	// Map ANTIC video registers (Atari 8-bit video chip)
	anticEngine = NewANTICEngine(sysBus)
	sysBus.MapIO(ANTIC_BASE, ANTIC_END,
		anticEngine.HandleRead,
		anticEngine.HandleWrite)
	// Map GTIA color registers (ANTIC's companion chip)
	sysBus.MapIO(GTIA_BASE, GTIA_END,
		anticEngine.HandleRead,
		anticEngine.HandleWrite)

	// Map Voodoo 3D graphics registers (3DFX SST-1 with Vulkan HLE)
	voodooEngine, err = NewVoodooEngine(sysBus)
	if err != nil {
		fmt.Printf("Warning: Voodoo initialization failed: %v\n", err)
	} else {
		sysBus.MapIO(VOODOO_BASE, VOODOO_END,
			voodooEngine.HandleRead,
			voodooEngine.HandleWrite)
	}

	// Create video compositor - owns the display output and blends video sources
	compositor := NewVideoCompositor(videoChip.GetOutput())
	compositor.RegisterSource(videoChip) // Layer 0 - background
	if vgaEngine != nil {
		compositor.RegisterSource(vgaEngine) // Layer 10 - VGA renders on top
	}
	if tedVideoEngine != nil {
		compositor.RegisterSource(tedVideoEngine) // Layer 12 - TED video between VGA and ULA
	}
	if anticEngine != nil {
		compositor.RegisterSource(anticEngine) // Layer 13 - ANTIC (Atari 8-bit)
	}
	if ulaEngine != nil {
		compositor.RegisterSource(ulaEngine) // Layer 15 - ULA renders on top of TED
	}
	if voodooEngine != nil {
		compositor.RegisterSource(voodooEngine) // Layer 20 - Voodoo 3D on top
	}
	videoChip.SetResolutionChangeCallback(func(w, h int) {
		compositor.NotifyResolutionChange(w, h)
		termMMIO.SetMouseNativeResolution(w, h)
	})
	if voodooEngine != nil {
		voodooEngine.SetResolutionChangeCallback(func(w, h int) {
			compositor.NotifyResolutionChange(w, h)
		})
	}
	compositor.LockResolution(DefaultScreenWidth, DefaultScreenHeight)
	if useResolutionOverride {
		compositor.LockResolution(validWidth, validHeight)
	}

	runtimeStatus.setChips(
		videoChip,
		vgaEngine,
		ulaEngine,
		tedVideoEngine,
		anticEngine,
		voodooEngine,
		soundChip,
		psgEngine,
		sidEngine,
		pokeyEngine,
		tedEngine,
		ahxPlayerCPU.engine,
		modPlayer.engine,
		wavPlayer.engine,
	)
	runtimeStatus.setPlayers(psgPlayer, sidPlayer, pokeyPlayer, tedPlayer)

	output := videoChip.GetOutput()
	outputConfig := output.GetDisplayConfig()
	outputConfig.Scale = ClampScale(scale)
	outputConfig.Fullscreen = fullscreen
	if err := output.SetDisplayConfig(outputConfig); err != nil {
		fmt.Printf("Failed to configure video output: %v\n", err)
		os.Exit(1)
	}

	// Initialize File I/O
	fileIO := NewFileIODevice(sysBus, ".") // Current directory as base
	sysBus.MapIO(FILE_IO_BASE, FILE_IO_END, fileIO.HandleRead, fileIO.HandleWrite)
	sysBus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fileIO.HandleWrite8)
	bootHostFS := NewBootstrapHostFSDevice(sysBus, defaultBootstrapHostFSRoot())
	sysBus.MapIO(BOOT_HOSTFS_BASE, BOOT_HOSTFS_END, bootHostFS.HandleRead, bootHostFS.HandleWrite)

	// Attach bus memory to sound engines and SoundChip so register writes
	// during file playback are mirrored to raw memory, making them visible
	// in the Machine Monitor's IO view.
	busMem := sysBus.GetMemory()
	soundChip.AttachBusMemory(busMem)
	psgEngine.AttachBusMemory(busMem)
	sidEngine.AttachBusMemory(busMem)
	sid2Engine.AttachBusMemory(busMem)
	sid3Engine.AttachBusMemory(busMem)
	pokeyEngine.AttachBusMemory(busMem)
	tedEngine.AttachBusMemory(busMem)

	// Initialize unified SOUND PLAY media loader
	mediaLoader := NewMediaLoader(sysBus, soundChip, ".", psgPlayer, sidPlayer, tedPlayer, ahxPlayerCPU, pokeyPlayer, modPlayer, wavPlayer)
	sysBus.MapIO(MEDIA_LOADER_BASE, MEDIA_LOADER_END, mediaLoader.HandleRead, mediaLoader.HandleWrite)

	// Initialize coprocessor subsystem MMIO (available to all CPU modes)
	coprocMgr := NewCoprocessorManager(sysBus, ".")
	sysBus.MapIO(COPROC_BASE, COPROC_END, coprocMgr.HandleRead, coprocMgr.HandleWrite)
	sysBus.MapIO(COPROC_EXT_BASE, COPROC_EXT_END, coprocMgr.HandleRead, coprocMgr.HandleWrite)
	defer coprocMgr.StopAll()
	runtimeStatus.setCoprocManager(coprocMgr)

	// Initialize Machine Monitor (debugger)
	monitor := NewMachineMonitor(sysBus)
	monitor.coprocMgr = coprocMgr
	monitor.soundChip = soundChip
	coprocMgr.monitor = monitor
	monitor.StartBreakpointListener()

	// Initialize the selected CPU and optionally load program
	var cpuRunner EmulatorCPU
	var startExecution bool
	var ie64CPU *CPU64
	var emuTOSLoader *EmuTOSLoader
	var arosLoader *AROSLoader
	var z80LoadAddr, z80Entry uint16
	var cpu6502LoadAddr, cpu6502Entry uint16
	var x86LoadAddr, x86Entry uint32

	// ProgramExecutor is created unconditionally so EXEC MMIO is always mapped.
	// Its CPU pointer is set/updated when entering IE64 mode (initial or mode-switch).
	progExec := NewProgramExecutor(sysBus, nil, videoChip, vgaEngine, voodooEngine, ".")
	if gemdosHostRoot != "" {
		progExec.SetGemdosConfig(gemdosHostRoot, gemdosDriveNum)
	}
	sysBus.MapIO(EXEC_BASE, EXEC_END, progExec.HandleRead, progExec.HandleWrite)

	// State for runProgramWithFullReset
	var programBytes []byte
	var reloadProgram func()
	var currentMode string
	var currentPath string
	var scriptEngine *ScriptEngine

	createRunnerForMode := func(mode string) (EmulatorCPU, error) {
		switch mode {
		case "ie32":
			videoChip.SetBigEndianMode(false)
			cpu := NewCPU(sysBus)
			cpu.PerfEnabled = perfMode
			return cpu, nil
		case "ie64":
			videoChip.SetBigEndianMode(false)
			cpu := NewCPU64(sysBus)
			cpu.PerfEnabled = perfMode
			return cpu, nil
		case "m68k":
			videoChip.SetBigEndianMode(true)
			m68k := NewM68KCPU(sysBus)
			if noJIT {
				m68k.m68kJitEnabled = false
			}
			runner := NewM68KRunner(m68k)
			runner.PerfEnabled = perfMode
			if noJIT {
				m68k.m68kJitEnabled = false
			}
			return runner, nil
		case "emutos", "aros":
			videoChip.SetBigEndianMode(true)
			m68k := NewM68KCPU(sysBus)
			if noJIT {
				m68k.m68kJitEnabled = false
			}
			runner := NewM68KRunner(m68k)
			runner.PerfEnabled = perfMode
			if noJIT {
				m68k.m68kJitEnabled = false
			}
			return runner, nil
		case "z80":
			videoChip.SetBigEndianMode(false)
			runner := NewCPUZ80Runner(sysBus, CPUZ80Config{
				LoadAddr:     z80LoadAddr,
				Entry:        z80Entry,
				JITEnabled:   !noJIT,
				VGAEngine:    vgaEngine,
				VoodooEngine: voodooEngine,
			})
			runner.PerfEnabled = perfMode
			return runner, nil
		case "x86":
			videoChip.SetBigEndianMode(false)
			runner := NewCPUX86Runner(sysBus, &CPUX86Config{
				LoadAddr:     x86LoadAddr,
				Entry:        x86Entry,
				JITEnabled:   !noJIT,
				VGAEngine:    vgaEngine,
				VoodooEngine: voodooEngine,
			})
			runner.PerfEnabled = perfMode
			return runner, nil
		case "6502":
			videoChip.SetBigEndianMode(false)
			runner := NewCPU6502Runner(sysBus, CPU6502Config{
				LoadAddr: cpu6502LoadAddr,
				Entry:    cpu6502Entry,
			})
			runner.PerfEnabled = perfMode
			return runner, nil
		default:
			return nil, fmt.Errorf("unsupported CPU mode: %s", mode)
		}
	}

	loadBasicBootImage := func() ([]byte, string, error) {
		if basicImage != "" {
			b, err := os.ReadFile(basicImage)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read BASIC image %s: %w", basicImage, err)
			}
			return b, basicImage, nil
		}
		if len(embeddedBasicImage) > 0 {
			return append([]byte(nil), embeddedBasicImage...), "", nil
		}
		autoPath := resolveDefaultBasicImagePath()
		if autoPath == "" {
			return nil, "", fmt.Errorf("BASIC not embedded and no local BASIC image found")
		}
		b, err := os.ReadFile(autoPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read fallback BASIC image %s: %w", autoPath, err)
		}
		return b, autoPath, nil
	}

	loadEmuTOSImage := func() ([]byte, string, error) {
		if emutosImage != "" {
			b, err := os.ReadFile(emutosImage)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read EmuTOS image %s: %w", emutosImage, err)
			}
			return b, emutosImage, nil
		}
		if filename != "" {
			b, err := os.ReadFile(filename)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read EmuTOS image %s: %w", filename, err)
			}
			return b, filename, nil
		}
		if len(embeddedEmuTOSImage) > 0 {
			return append([]byte(nil), embeddedEmuTOSImage...), "", nil
		}
		autoPath := resolveDefaultEmuTOSImagePath()
		if autoPath == "" {
			return nil, "", fmt.Errorf("EmuTOS not embedded and no local ROM image found")
		}
		b, err := os.ReadFile(autoPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read fallback EmuTOS image %s: %w", autoPath, err)
		}
		return b, autoPath, nil
	}

	loadAROSImage := func() ([]byte, string, error) {
		if arosImage != "" {
			b, err := os.ReadFile(arosImage)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read AROS image %s: %w", arosImage, err)
			}
			return b, arosImage, nil
		}
		if filename != "" {
			b, err := os.ReadFile(filename)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read AROS image %s: %w", filename, err)
			}
			return b, filename, nil
		}
		if len(embeddedAROSImage) > 0 {
			return append([]byte(nil), embeddedAROSImage...), "", nil
		}
		autoPath := resolveDefaultAROSImagePath()
		if autoPath == "" {
			return nil, "", fmt.Errorf("AROS not embedded and no ROM image specified")
		}
		b, err := os.ReadFile(autoPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read AROS image %s: %w", autoPath, err)
		}
		return b, autoPath, nil
	}

	loadIntuitionOSImage := func() ([]byte, string, error) {
		// Search candidate paths for the assembled IExec kernel binary
		candidates := []string{
			"sdk/intuitionos/iexec/iexec.ie64", // repo root
			"iexec.ie64",                       // current directory
			"bin/iexec.ie64",                   // bin directory
			"sdk/examples/prebuilt/iexec.ie64", // prebuilt location
		}
		// Also try relative to the executable location
		if exePath, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exePath)
			candidates = append(candidates,
				filepath.Join(exeDir, "iexec.ie64"),
				filepath.Join(exeDir, "..", "sdk", "intuitionos", "iexec", "iexec.ie64"),
			)
		}
		for _, p := range candidates {
			if b, err := os.ReadFile(p); err == nil {
				return b, p, nil
			}
		}
		return nil, "", fmt.Errorf("IntuitionOS kernel not found (run 'make intuitionos' to build)")
	}

	if modeIE32 {
		ie32CPU := NewCPU(sysBus)
		ie32CPU.PerfEnabled = perfMode
		runtimeStatus.setCPUs(runtimeCPUIE32, ie32CPU, nil, nil, nil, nil, nil)

		if filename != "" {
			if err := ie32CPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading IE32 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		cpuRunner = ie32CPU
		currentMode = "ie32"
		monitor.RegisterCPU("IE32", NewDebugIE32(ie32CPU))

		if startExecution {
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()
			fmt.Printf("Starting IE32 CPU with program: %s\n", filename)
			ie32CPU.StartExecution()
		}

	} else if modeIE64 {
		sysBus.SetLegacyMMIO64Policy(MMIO64PolicySplit)
		ie64CPU = NewCPU64(sysBus)
		ie64CPU.PerfEnabled = perfMode
		ie64CPU.jitEnabled = jitAvailable && !noJIT
		runtimeStatus.setCPUs(runtimeCPUIE64, nil, ie64CPU, nil, nil, nil, nil)
		progExec.SetCPU(ie64CPU)

		if modeBasic {
			if basicImage != "" {
				if err := ie64CPU.LoadProgram(basicImage); err != nil {
					fmt.Printf("Error loading BASIC image %s: %v\n", basicImage, err)
					os.Exit(1)
				}
				programBytes, _ = os.ReadFile(basicImage)
				currentPath = basicImage
				fmt.Printf("Starting EhBASIC IE64 (custom image: %s)\n", basicImage)
			} else if len(embeddedBasicImage) > 0 {
				ie64CPU.LoadProgramBytes(embeddedBasicImage)
				programBytes = append([]byte(nil), embeddedBasicImage...)
				currentPath = ""
				fmt.Println("Starting EhBASIC IE64 (embedded image)")
			} else {
				autoPath := resolveDefaultBasicImagePath()
				if autoPath == "" {
					fmt.Println("Error: BASIC not embedded and no local BASIC image found.")
					fmt.Println("Use -basic-image <path>, run 'make basic', or place ehbasic_ie64.ie64 in sdk/examples/asm/ or bin/.")
					os.Exit(1)
				}
				if err := ie64CPU.LoadProgram(autoPath); err != nil {
					fmt.Printf("Error loading fallback BASIC image %s: %v\n", autoPath, err)
					os.Exit(1)
				}
				programBytes, _ = os.ReadFile(autoPath)
				currentPath = autoPath
				fmt.Printf("Starting EhBASIC IE64 (auto image: %s)\n", autoPath)
			}
			startExecution = true
		} else if filename != "" {
			if err := ie64CPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading IE64 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		cpuRunner = ie64CPU
		currentMode = "ie64"
		monitor.RegisterCPU("IE64", NewDebugIE64(ie64CPU))

		if startExecution {
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()
			if !modeBasic {
				fmt.Printf("Starting IE64 CPU with program: %s\n", filename)
			}
			ie64CPU.StartExecution()
		}

	} else if modeM68K {
		m68kCPU := NewM68KCPU(sysBus)
		videoChip.SetBigEndianMode(true)

		if filename != "" {
			if err := m68kCPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading M68K program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		m68kRunner := NewM68KRunner(m68kCPU)
		m68kRunner.PerfEnabled = perfMode
		if noJIT {
			m68kCPU.m68kJitEnabled = false
		}
		runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, m68kRunner, nil, nil, nil)

		cpuRunner = m68kRunner
		currentMode = "m68k"
		monitor.RegisterCPU("M68K", NewDebugM68K(m68kCPU, m68kRunner))

		if startExecution {
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()
			fmt.Printf("Starting M68K CPU with program: %s\n\n", filename)
			m68kRunner.StartExecution()
		}
	} else if modeEmuTOS {
		romBytes, romPath, err := loadEmuTOSImage()
		if err != nil {
			fmt.Printf("Error loading EmuTOS image: %v\n", err)
			os.Exit(1)
		}

		// EmuTOS uses addresses 0x100000-0x3FFFFF as normal RAM (heap, stack).
		// Remove the VRAM I/O mapping so writes go to bus memory, not VideoChip.
		// The VideoChip reads from bus memory directly for display in EmuTOS mode.
		sysBus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
		videoChip.SetBusMemory(sysBus.memory)
		videoChip.SetBigEndianMode(true)
		// Point VideoChip's GetFrame at full VRAM so CLUT8 bitmaps at any offset work.
		videoChip.SetDirectVRAM(sysBus.memory[VRAM_START : VRAM_START+VRAM_SIZE])
		m68kCPU := NewM68KCPU(sysBus)
		m68kRunner := NewM68KRunner(m68kCPU)
		m68kRunner.PerfEnabled = perfMode
		if noJIT {
			m68kCPU.m68kJitEnabled = false
		}
		loader := NewEmuTOSLoader(sysBus, m68kCPU, videoChip)
		if err := loader.LoadROM(romBytes); err != nil {
			fmt.Printf("Error loading EmuTOS ROM: %v\n", err)
			os.Exit(1)
		}
		if gemdosHostRoot != "" {
			if err := loader.SetupGemdos(gemdosHostRoot, gemdosDriveNum); err != nil {
				fmt.Printf("Warning: GEMDOS drive U: disabled: %v\n", err)
			}
		}

		coprocMgr.SetIRQTarget(m68kCPU)
		coprocMgr.StartCompletionWatcher()

		emuTOSLoader = loader
		runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, m68kRunner, nil, nil, nil)
		cpuRunner = m68kRunner
		currentMode = "emutos"
		monitor.RegisterCPU("M68K", NewDebugM68K(m68kCPU, m68kRunner))
		startExecution = true
		if romPath != "" {
			currentPath = romPath
		}
		programBytes = append([]byte(nil), romBytes...)

		// Hide system cursor — EmuTOS draws its own VDI cursor in VRAM
		if hider, ok := videoChip.GetOutput().(SystemCursorHider); ok {
			hider.HideSystemCursor()
		}

		videoChip.Start()
		compositor.Start()
		soundChip.Start()
		// All MMIO peripherals are registered unconditionally,
		// so EmuTOS .PRG programs have full hardware access.
		loader.StartTimer()
		fmt.Println("Starting EmuTOS on M68K")
		m68kRunner.StartExecution()
	} else if modeAROS {
		romBytes, romPath, err := loadAROSImage()
		if err != nil {
			fmt.Printf("Error loading AROS image: %v\n", err)
			os.Exit(1)
		}

		// AROS uses bus-backed VRAM at the top of the 32MB address space.
		configureArosVRAM(sysBus, videoChip)
		m68kCPU := NewM68KCPU(sysBus)
		m68kRunner := NewM68KRunner(m68kCPU)
		m68kRunner.PerfEnabled = perfMode
		if noJIT {
			m68kCPU.m68kJitEnabled = false
		}
		loader := NewAROSLoader(sysBus, m68kCPU, videoChip)
		if err := loader.LoadROM(romBytes); err != nil {
			fmt.Printf("Error loading AROS ROM: %v\n", err)
			os.Exit(1)
		}
		// Initialize AROS DOS interceptor for host filesystem access
		arosDOS, dosErr := NewArosDOSDevice(sysBus, arosHostRoot)
		if dosErr != nil {
			fmt.Printf("Warning: AROS DOS device init failed: %v\n", dosErr)
		} else {
			sysBus.MapIO(AROS_DOS_REGION_BASE, AROS_DOS_REGION_END, arosDOS.HandleRead, arosDOS.HandleWrite)
			fmt.Printf("AROS DOS: IE: → %s\r\n", arosHostRoot)
		}

		// Initialize AROS Audio DMA engine (Paula-compatible DMA → flex channel DAC)
		arosDMA := NewArosAudioDMA(sysBus, soundChip, m68kCPU)
		sysBus.MapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END, arosDMA.HandleRead, arosDMA.HandleWrite)
		soundChip.SetSampleTicker(arosDMA)
		runtimeStatus.setPaulaDMA(arosDMA)

		// Initialize clipboard bridge (host ↔ guest clipboard exchange)
		clipBridge := NewClipboardBridge(sysBus)
		sysBus.MapIO(CLIP_REGION_BASE, CLIP_REGION_END, clipBridge.HandleRead, clipBridge.HandleWrite)

		// IRQ diagnostic registers for freeze investigation scripts
		loader.MapIRQDiagnostics()

		// Wire up IE64 coprocessor completion interrupts to M68K CPU
		coprocMgr.SetIRQTarget(m68kCPU)
		coprocMgr.StartCompletionWatcher()

		arosLoader = loader
		runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, m68kRunner, nil, nil, nil)
		cpuRunner = m68kRunner
		currentMode = "aros"
		monitor.RegisterCPU("M68K", NewDebugM68K(m68kCPU, m68kRunner))
		startExecution = true
		if romPath != "" {
			currentPath = romPath
		}
		programBytes = append([]byte(nil), romBytes...)

		// Disable the emulator's software cursor overlay — AROS draws its own
		if disabler, ok := videoChip.GetOutput().(SoftwareCursorDisabler); ok {
			disabler.DisableSoftwareCursor()
		}
		// Hide system cursor — AROS draws its own Intuition cursor in VRAM
		if hider, ok := videoChip.GetOutput().(SystemCursorHider); ok {
			hider.HideSystemCursor()
		}

		// AROS HIDDs expect Amiga rawkey scancodes, not PC/AT scancodes
		termMMIO.amigaScancodeMode.Store(true)

		// Unlock compositor resolution so the iegfx HIDD's IE_VideoSetMode(640x480)
		// propagates correctly during boot (default lock is 800x600).
		compositor.UnlockResolution()

		videoChip.Start()
		compositor.Start()
		soundChip.Start()
		loader.StartTimer()
		fmt.Println("Starting AROS on M68K")
		m68kRunner.StartExecution()
	} else if modeZ80 {
		var parsedLoadAddr uint16
		if loadAddr.set {
			parsed, err := parseUint16Flag(loadAddr.value)
			if err != nil {
				fmt.Printf("Invalid --load-addr: %v\n", err)
				os.Exit(1)
			}
			parsedLoadAddr = parsed
		}
		z80LoadAddr = parsedLoadAddr
		var parsedEntry uint16
		if entryAddr.set {
			parsed, err := parseUint16Flag(entryAddr.value)
			if err != nil {
				fmt.Printf("Invalid --entry: %v\n", err)
				os.Exit(1)
			}
			parsedEntry = parsed
		}
		z80Entry = parsedEntry

		z80CPU := NewCPUZ80Runner(sysBus, CPUZ80Config{
			LoadAddr:     parsedLoadAddr,
			Entry:        parsedEntry,
			JITEnabled:   !noJIT,
			VGAEngine:    vgaEngine,
			VoodooEngine: voodooEngine,
		})
		z80CPU.PerfEnabled = perfMode
		runtimeStatus.setCPUs(runtimeCPUZ80, nil, nil, nil, z80CPU, nil, nil)

		if filename != "" {
			if err := z80CPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading Z80 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		cpuRunner = z80CPU
		currentMode = "z80"
		monitor.RegisterCPU("Z80", NewDebugZ80(z80CPU.cpu, z80CPU))

		if startExecution {
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()
			fmt.Printf("Starting Z80 CPU with program: %s\n\n", filename)
			z80CPU.StartExecution()
		}
	} else if modeX86 {
		x86Config := &CPUX86Config{
			LoadAddr:     0,
			Entry:        0,
			JITEnabled:   !noJIT,
			VGAEngine:    vgaEngine,
			VoodooEngine: voodooEngine,
		}
		x86LoadAddr = x86Config.LoadAddr
		x86Entry = x86Config.Entry

		x86CPU := NewCPUX86Runner(sysBus, x86Config)
		x86CPU.PerfEnabled = perfMode
		runtimeStatus.setCPUs(runtimeCPUX86, nil, nil, nil, nil, x86CPU, nil)

		if filename != "" {
			if err := x86CPU.LoadProgramFromFile(filename); err != nil {
				fmt.Printf("Error loading x86 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		cpuRunner = x86CPU
		currentMode = "x86"
		monitor.RegisterCPU("X86", NewDebugX86(x86CPU.cpu, x86CPU))

		if startExecution {
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()
			fmt.Printf("Starting x86 CPU with program: %s\n\n", filename)
			x86CPU.StartExecution()
		}
	} else {
		var parsedLoadAddr uint16
		if loadAddr.set {
			parsed, err := parseUint16Flag(loadAddr.value)
			if err != nil {
				fmt.Printf("Invalid --load-addr: %v\n", err)
				os.Exit(1)
			}
			parsedLoadAddr = parsed
		} else if strings.HasSuffix(strings.ToLower(filename), ".ie65") {
			parsedLoadAddr = 0x0800
		}
		cpu6502LoadAddr = parsedLoadAddr
		var parsedEntry uint16
		if entryAddr.set {
			parsed, err := parseUint16Flag(entryAddr.value)
			if err != nil {
				fmt.Printf("Invalid --entry: %v\n", err)
				os.Exit(1)
			}
			parsedEntry = parsed
		}
		cpu6502Entry = parsedEntry

		cpu6502 := NewCPU6502Runner(sysBus, CPU6502Config{
			LoadAddr: parsedLoadAddr,
			Entry:    parsedEntry,
		})
		cpu6502.PerfEnabled = perfMode
		runtimeStatus.setCPUs(runtimeCPU6502, nil, nil, nil, nil, nil, cpu6502)

		if filename != "" {
			if err := cpu6502.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading 6502 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		cpuRunner = cpu6502
		currentMode = "6502"
		monitor.RegisterCPU("6502", NewDebug6502(cpu6502.cpu, cpu6502))

		if startExecution {
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()
			fmt.Printf("Starting 6502 CPU with program: %s\n\n", filename)
			cpu6502.StartExecution()
		}
	}

	// Set global state for cross-module access
	activeCPU = cpuRunner
	activeVideoChip = videoChip

	// Wire monitor overlay to the video output
	if ma, ok := videoChip.GetOutput().(MonitorAttachable); ok {
		ma.AttachMonitor(monitor)
	}

	// Cache initial program bytes for F10 reload
	if filename != "" && len(programBytes) == 0 {
		programBytes, _ = os.ReadFile(filename)
		currentPath = filename
	}
	reloadProgram = buildReloadClosure(currentMode, cpuRunner, programBytes, sysBus)

	// runProgramWithFullReset is the ONLY mutating entry point for reset+load.
	// path == "" means "hard reset to BASIC cold boot" (F10 case).
	// path != "" means "load new program from disk" (IPC OPEN case).
	runProgramWithFullReset := func(path string) error {
		resetMu.Lock()
		defer resetMu.Unlock()

		if scriptEngine != nil && !scriptEngine.IsLoadingProgram() {
			scriptEngine.Cancel()
		}

		// Quiesce all live video producers before hot-reloading a new program.
		// Manual demo runs start from a fresh process; showreel/scripted loads do not.
		// Stopping the compositor and standalone render loops here prevents stale
		// frames from being composed while devices are being reset in-place.
		compositor.Stop()
		if vgaEngine != nil {
			vgaEngine.StopRenderLoop()
		}
		if ulaEngine != nil {
			ulaEngine.StopRenderLoop()
		}
		if tedVideoEngine != nil {
			tedVideoEngine.StopRenderLoop()
		}
		if anticEngine != nil {
			anticEngine.StopRenderLoop()
		}

		var bytes []byte
		var mode string
		forceBasicBoot := false

		if path == "" {
			// F10 hard reset: reload the original CLI boot mode, not the
			// current mode. EmuTOS launched via BASIC's EMUTOS command
			// should reset back to BASIC, not EmuTOS.
			if modeEmuTOS {
				var err error
				bytes, path, err = loadEmuTOSImage()
				if err != nil {
					return err
				}
				mode = "emutos"
			} else {
				forceBasicBoot = true
				var err error
				bytes, path, err = loadBasicBootImage()
				if err != nil {
					return err
				}
				mode = "ie64"
			}
		} else if path == emutosSentinel {
			// BASIC EMUTOS command: boot EmuTOS from embedded/flag/local ROM.
			var err error
			bytes, path, err = loadEmuTOSImage()
			if err != nil {
				return err
			}
			mode = "emutos"
		} else if path == arosSentinel {
			var err error
			bytes, path, err = loadAROSImage()
			if err != nil {
				return err
			}
			mode = "aros"
		} else if path == intuitionOSSentinel {
			var err error
			bytes, path, err = loadIntuitionOSImage()
			if err != nil {
				return err
			}
			mode = "ie64"
		} else {
			var err error
			bytes, err = os.ReadFile(path)
			if err != nil {
				return err
			}
			mode, err = modeFromExtension(path)
			if err != nil {
				return err
			}
			if mode == "script" {
				if scriptEngine == nil {
					return fmt.Errorf("script engine unavailable")
				}
				return scriptEngine.RunFile(path)
			}
			// Preserve CLI/default 6502 semantics for .ie65 reload/launch paths.
			// In BASIC/IPC mode we don't parse --load-addr, so apply the standard
			// .ie65 default load address when no explicit load address was provided.
			if mode == "6502" && !loadAddr.set && cpu6502LoadAddr == 0 {
				cpu6502LoadAddr = 0x0800
			}
		}

		// 0. Deactivate monitor if active (prevents freeze/resume interference)
		if monitor.IsActive() {
			monitor.Deactivate()
		}

		// 1. Stop CPU
		cpuRunner.Stop()
		if emuTOSLoader != nil {
			emuTOSLoader.Stop()
			emuTOSLoader = nil
		}
		if arosLoader != nil {
			arosLoader.Stop()
			arosLoader = nil
		}

		// 2. Stop compositor
		compositor.Stop()

		// 3. Stop render loops (engines are optional in -emutos profile)
		if vgaEngine != nil {
			vgaEngine.StopRenderLoop()
		}
		if ulaEngine != nil {
			ulaEngine.StopRenderLoop()
		}
		if tedVideoEngine != nil {
			tedVideoEngine.StopRenderLoop()
		}
		if anticEngine != nil {
			anticEngine.StopRenderLoop()
		}

		// Preserve explicit JIT choices when reloading the same active CPU family.
		snap := runtimeStatus.snapshot()
		var preserveM68KJIT bool
		var haveM68KJIT bool
		var preserveZ80JIT bool
		var haveZ80JIT bool
		var preserve6502JIT bool
		var have6502JIT bool
		var preserveIE64JIT bool
		var haveIE64JIT bool
		switch snap.selectedCPU {
		case runtimeCPUM68K:
			if snap.m68k != nil && snap.m68k.cpu != nil {
				preserveM68KJIT = snap.m68k.cpu.m68kJitEnabled
				haveM68KJIT = true
			}
		case runtimeCPUZ80:
			if snap.z80 != nil && snap.z80.cpu != nil {
				preserveZ80JIT = snap.z80.cpu.jitEnabled
				haveZ80JIT = true
			}
		case runtimeCPU6502:
			if snap.cpu65 != nil && snap.cpu65.cpu != nil {
				preserve6502JIT = snap.cpu65.JITEnabled
				have6502JIT = true
			}
		case runtimeCPUIE64:
			if snap.ie64 != nil {
				preserveIE64JIT = snap.ie64.jitEnabled
				haveIE64JIT = true
			}
		}

		// 4. Recreate CPU runner for a true cold boot, then update runtime status/progExec.
		newRunner, err := createRunnerForMode(mode)
		if err != nil {
			return err
		}
		switch mode {
		case "ie64":
			if haveIE64JIT {
				newRunner.(*CPU64).jitEnabled = preserveIE64JIT
			}
		case "m68k", "emutos", "aros":
			if haveM68KJIT {
				newRunner.(*M68KRunner).cpu.m68kJitEnabled = preserveM68KJIT
			}
		case "z80":
			if haveZ80JIT {
				newRunner.(*CPUZ80Runner).cpu.jitEnabled = preserveZ80JIT
			}
		case "6502":
			if have6502JIT {
				newRunner.(*CPU6502Runner).JITEnabled = preserve6502JIT
				newRunner.(*CPU6502Runner).cpu.jitEnabled = preserve6502JIT
			}
		}
		cpuRunner = newRunner
		activeCPU = newRunner

		switch mode {
		case "ie32":
			runtimeStatus.setCPUs(runtimeCPUIE32, newRunner.(*CPU), nil, nil, nil, nil, nil)
			progExec.SetCPU(nil)
		case "ie64":
			cpu64 := newRunner.(*CPU64)
			runtimeStatus.setCPUs(runtimeCPUIE64, nil, cpu64, nil, nil, nil, nil)
			progExec.SetCPU(cpu64)
		case "m68k":
			runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, newRunner.(*M68KRunner), nil, nil, nil)
			progExec.SetCPU(nil)
		case "emutos", "aros":
			runtimeStatus.setCPUs(runtimeCPUM68K, nil, nil, newRunner.(*M68KRunner), nil, nil, nil)
			progExec.SetCPU(nil)
		case "z80":
			runtimeStatus.setCPUs(runtimeCPUZ80, nil, nil, nil, newRunner.(*CPUZ80Runner), nil, nil)
			progExec.SetCPU(nil)
		case "x86":
			runtimeStatus.setCPUs(runtimeCPUX86, nil, nil, nil, nil, newRunner.(*CPUX86Runner), nil)
			progExec.SetCPU(nil)
		case "6502":
			runtimeStatus.setCPUs(runtimeCPU6502, nil, nil, nil, nil, nil, newRunner.(*CPU6502Runner))
			progExec.SetCPU(nil)
		}

		// Re-register CPU with the monitor (old adapters are stale after recreate)
		monitor.ResetCPUs()
		switch mode {
		case "ie32":
			monitor.RegisterCPU("IE32", NewDebugIE32(newRunner.(*CPU)))
		case "ie64":
			monitor.RegisterCPU("IE64", NewDebugIE64(newRunner.(*CPU64)))
		case "m68k":
			r := newRunner.(*M68KRunner)
			monitor.RegisterCPU("M68K", NewDebugM68K(r.cpu, r))
		case "emutos":
			r := newRunner.(*M68KRunner)
			monitor.RegisterCPU("M68K", NewDebugM68K(r.cpu, r))
		case "z80":
			r := newRunner.(*CPUZ80Runner)
			monitor.RegisterCPU("Z80", NewDebugZ80(r.cpu, r))
		case "x86":
			r := newRunner.(*CPUX86Runner)
			monitor.RegisterCPU("X86", NewDebugX86(r.cpu, r))
		case "6502":
			r := newRunner.(*CPU6502Runner)
			monitor.RegisterCPU("6502", NewDebug6502(r.cpu, r))
		}

		// 5-6. Reset audio engines + sound chip
		psgEngine.Reset()
		sidEngine.Reset()
		tedEngine.Reset()
		pokeyEngine.Reset()
		ahxPlayerCPU.engine.Reset()
		psgPlayer.Reset()
		sidPlayer.Reset()
		tedPlayer.Reset()
		pokeyPlayer.Reset()
		soundChip.Reset()

		// 7. Reset memory
		sysBus.Reset()

		// 7b. VRAM I/O mapping — must happen after sysBus.Reset().
		if mode == "emutos" || mode == "aros" {
			// M68K ROM mode: unmap VRAM so M68K writes go to bus memory directly.
			if mode == "aros" {
				configureArosVRAM(sysBus, videoChip)
				// AROS draws its own Intuition cursor — disable Go-side software cursor
				if disabler, ok := videoChip.GetOutput().(SoftwareCursorDisabler); ok {
					disabler.DisableSoftwareCursor()
				}
			} else {
				sysBus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
				videoChip.SetBusMemory(sysBus.memory)
				videoChip.SetBigEndianMode(true)
				videoChip.SetDirectVRAM(sysBus.memory[VRAM_START : VRAM_START+VRAM_SIZE])
			}
			if hider, ok := videoChip.GetOutput().(SystemCursorHider); ok {
				hider.HideSystemCursor()
			}
		} else if currentMode == "emutos" || currentMode == "aros" {
			// Leaving M68K ROM mode: restore VRAM I/O mapping and VideoChip defaults.
			sysBus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
				videoChip.HandleRead, videoChip.HandleWrite)
			sysBus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1, videoChip.HandleWrite8)
			videoChip.SetBigEndianMode(false)
			videoChip.SetDirectVRAM(nil)
		}

		// 8. Reset video chips
		videoChip.Reset()
		if vgaEngine != nil {
			vgaEngine.Reset()
		}
		if ulaEngine != nil {
			ulaEngine.Reset()
		}
		if tedVideoEngine != nil {
			tedVideoEngine.Reset()
		}
		if anticEngine != nil {
			anticEngine.Reset()
		}
		if voodooEngine != nil {
			voodooEngine.Reset()
		}

		// 9. Reset terminal/coproc
		termMMIO.Reset()
		if forceBasicBoot {
			// F10 is a power-on reset to BASIC: switch terminal plumbing to
			// the in-window BASIC terminal path.
			if outputTicker != nil {
				outputTicker.Stop()
				outputTicker = nil
			}
			if outputStop != nil {
				close(outputStop)
				outputStop = nil
			}
			if termHost != nil {
				termHost.Stop()
				termHost = nil
			}
			if videoTerm == nil {
				videoTerm = NewVideoTerminal(videoChip, termMMIO)
				initTerminalClipboard(videoTerm)
				videoTerm.Start()
			}
			termMMIO.SetForceEchoOff(true)
			if ki, ok := videoChip.GetOutput().(KeyboardInput); ok {
				ki.SetKeyHandler(videoTerm.HandleKeyInput)
			}
			if si, ok := videoChip.GetOutput().(ScrollInput); ok {
				si.SetScrollHandler(videoTerm.HandleScroll)
			}
			if ci, ok := videoChip.GetOutput().(ClipboardInput); ok {
				ci.SetCopyHandler(videoTerm.CopySelection)
				ci.SetCutHandler(videoTerm.CutSelection)
				ci.SetMiddleMouseHandler(videoTerm.MiddleMousePaste)
			}
		}
		if videoTerm != nil {
			videoTerm.Reset()
		}
		coprocMgr.Reset()

		// 10. Update cached state
		programBytes = bytes
		currentPath = path
		currentMode = mode
		if mode != "emutos" {
			reloadProgram = buildReloadClosure(mode, cpuRunner, bytes, sysBus)
		}

		// 11. Load program
		if mode == "emutos" {
			r := cpuRunner.(*M68KRunner)
			loader := NewEmuTOSLoader(sysBus, r.cpu, videoChip)
			if err := loader.LoadROM(bytes); err != nil {
				return fmt.Errorf("failed to load EmuTOS ROM: %w", err)
			}
			if gemdosHostRoot != "" {
				if err := loader.SetupGemdos(gemdosHostRoot, gemdosDriveNum); err != nil {
					fmt.Printf("Warning: GEMDOS drive U: disabled: %v\n", err)
				}
			}
			loader.StartTimer()
			emuTOSLoader = loader
			runtime.GC() // sweep BASIC/IE64 garbage before EmuTOS starts
		} else if mode == "aros" {
			r := cpuRunner.(*M68KRunner)
			loader := NewAROSLoader(sysBus, r.cpu, videoChip)
			if err := loader.LoadROM(bytes); err != nil {
				return fmt.Errorf("failed to load AROS ROM: %w", err)
			}
			// Wire up DOS device for host filesystem access
			if arosDOS, dosErr := NewArosDOSDevice(sysBus, arosHostRoot); dosErr == nil {
				sysBus.MapIO(AROS_DOS_REGION_BASE, AROS_DOS_REGION_END, arosDOS.HandleRead, arosDOS.HandleWrite)
				fmt.Printf("AROS DOS: IE: → %s\r\n", arosHostRoot)
			}
			// Wire up audio DMA engine
			arosDMA := NewArosAudioDMA(sysBus, soundChip, r.cpu)
			sysBus.MapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END, arosDMA.HandleRead, arosDMA.HandleWrite)
			soundChip.SetSampleTicker(arosDMA)
			runtimeStatus.setPaulaDMA(arosDMA)
			// Wire up clipboard bridge
			clipBridge := NewClipboardBridge(sysBus)
			sysBus.MapIO(CLIP_REGION_BASE, CLIP_REGION_END, clipBridge.HandleRead, clipBridge.HandleWrite)
			// IRQ diagnostic registers for freeze investigation scripts
			loader.MapIRQDiagnostics()
			// AROS HIDDs expect Amiga rawkey scancodes
			termMMIO.amigaScancodeMode.Store(true)
			loader.StartTimer()
			arosLoader = loader
			runtime.GC()
		} else {
			reloadProgram()
		}
		if forceBasicBoot {
			// Optional cleanup point: BASIC image is now loaded into reset state.
			runtime.GC()
		}

		// 12. Start peripherals
		videoChip.Start()
		soundChip.Start()

		// 13. Start compositor + render loops
		compositor.Start()
		if vgaEngine != nil {
			vgaEngine.StartRenderLoop()
		}
		if ulaEngine != nil {
			ulaEngine.StartRenderLoop()
		}
		if tedVideoEngine != nil {
			tedVideoEngine.StartRenderLoop()
		}
		if anticEngine != nil {
			anticEngine.StartRenderLoop()
		}

		// 14. Start CPU
		cpuRunner.StartExecution()
		return nil
	}

	scriptEngine = NewScriptEngine(sysBus, compositor, termMMIO)
	scriptEngine.SetProgramLoader(runProgramWithFullReset)
	scriptEngine.SetEmutosSentinel(emutosSentinel)
	scriptEngine.SetArosSentinel(arosSentinel)
	scriptEngine.SetHardReset(func() error {
		return runProgramWithFullReset("")
	})
	scriptEngine.SetMonitor(monitor)
	scriptEngine.SetQuitFunc(func() {
		os.Exit(0)
	})
	scriptEngine.SetExitFunc(func(code int) {
		os.Exit(code)
	})
	if videoTerm != nil {
		scriptEngine.SetVideoTerminal(videoTerm)
	}
	if sa, ok := videoChip.GetOutput().(interface{ SetScriptEngine(*ScriptEngine) }); ok {
		sa.SetScriptEngine(scriptEngine)
	}
	scriptEngine.SetEmutosDriveFunc(func(path string, driveNum uint16) {
		resetMu.Lock()
		gemdosHostRoot = path
		gemdosDriveNum = driveNum
		resetMu.Unlock()
		progExec.SetGemdosConfig(path, driveNum)
	})

	launchProgramOrScript := func(path string) error {
		if strings.EqualFold(filepath.Ext(path), ".ies") {
			return scriptEngine.RunFile(path)
		}
		return runProgramWithFullReset(path)
	}

	// Ensure RUN "file" (ProgramExecutor) uses the same launch path as IPC/F10
	// so monitor/runtime state stays consistent across all entry points.
	progExec.SetExternalLauncher(launchProgramOrScript)
	progExec.SetEmuTOSBootLoader(func() error {
		return runProgramWithFullReset(emutosSentinel)
	})
	progExec.SetAROSBootLoader(func() error {
		return runProgramWithFullReset(arosSentinel)
	})
	progExec.SetIExecBootLoader(func() error {
		return runProgramWithFullReset(intuitionOSSentinel)
	})

	// Wire F10 hard reset handler
	if hr, ok := videoChip.GetOutput().(HardResettable); ok {
		hr.SetHardResetHandler(func() {
			if scriptEngine != nil {
				scriptEngine.Cancel()
			}
			if err := runProgramWithFullReset(""); err != nil {
				fmt.Printf("F10 reset failed: %v\n", err)
			}
		})
	}

	// Start IPC server for single-instance file opening
	ipcServer, err := NewIPCServer(func(path string) error {
		return launchProgramOrScript(path)
	})
	if err != nil {
		fmt.Printf("Warning: IPC server failed to start: %v\n", err)
	} else {
		ipcServer.Start()
		defer ipcServer.Stop()
	}

	// Suppress unused warnings for variables used by the closure
	_ = ie64CPU
	_ = programBytes
	_ = reloadProgram
	_ = currentMode
	_ = currentPath
	if modeBasic {
		// Initial BASIC startup: collect transient allocations after image setup.
		runtime.GC()
	}

	if scriptFile != "" {
		if err := scriptEngine.RunFile(scriptFile); err != nil {
			fmt.Printf("Error starting script %s: %v\n", scriptFile, err)
			os.Exit(1)
		}
	}

	// Start console terminal host only when not using graphical BASIC terminal.
	if termHost != nil {
		termHost.Start()
		outputTicker = time.NewTicker(10 * time.Millisecond)
		outputStop = make(chan struct{})
		go func() {
			for {
				select {
				case <-outputStop:
					return
				case <-outputTicker.C:
					termHost.PrintOutput()
				}
			}
		}()
	}

	// Wait for window close (graphical mode), script completion, or CPU halt.
	waited := false
	if waiter, ok := videoChip.GetOutput().(interface{ Done() <-chan struct{} }); ok {
		<-waiter.Done()
		waited = true
	}
	if !waited && scriptEngine != nil {
		if ch := scriptEngine.Done(); ch != nil {
			<-ch
			waited = true
		}
	}
	if !waited && emuTOSLoader != nil {
		// Headless EmuTOS: block until the CPU stops running.
		for emuTOSLoader.cpu.Running() {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !waited && arosLoader != nil {
		// Headless AROS: block until the CPU stops running.
		for arosLoader.cpu.Running() {
			time.Sleep(100 * time.Millisecond)
		}
		waited = true
	}
	if !waited {
		if runner, ok := cpuRunner.(interface{ IsRunning() bool }); ok {
			for runner.IsRunning() {
				time.Sleep(100 * time.Millisecond)
			}
			waited = true
		}
	}

	// Shut down terminal host (restores stdin to blocking) and render goroutines.
	if scriptEngine != nil {
		scriptEngine.Cancel()
	}
	if emuTOSLoader != nil {
		emuTOSLoader.Stop()
	}
	if arosLoader != nil {
		arosLoader.Stop()
	}
	if outputTicker != nil {
		outputTicker.Stop()
	}
	if outputStop != nil {
		close(outputStop)
	}
	if termHost != nil {
		termHost.Stop()
	}
	if videoTerm != nil {
		videoTerm.Stop()
	}
	if vgaEngine != nil {
		vgaEngine.StopRenderLoop()
	}
	if ulaEngine != nil {
		ulaEngine.StopRenderLoop()
	}
	if tedVideoEngine != nil {
		tedVideoEngine.StopRenderLoop()
	}
	if anticEngine != nil {
		anticEngine.StopRenderLoop()
	}
}

func parseUint16Flag(value string) (uint16, error) {
	parsed, err := strconv.ParseUint(value, 0, 16)
	if err != nil {
		return 0, err
	}
	if parsed > 0xFFFF {
		return 0, fmt.Errorf("value out of range: 0x%X", parsed)
	}
	return uint16(parsed), nil
}

func resolveDefaultBasicImagePath() string {
	candidates := []string{
		"sdk/examples/prebuilt/ehbasic_ie64.ie64",
		"sdk/examples/asm/ehbasic_ie64.ie64",
		"bin/ehbasic_ie64.ie64",
		"ehbasic_ie64.ie64",
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		exeCandidates := []string{
			filepath.Join(exeDir, "ehbasic_ie64.ie64"),
			filepath.Join(exeDir, "bin", "ehbasic_ie64.ie64"),
			filepath.Join(exeDir, "..", "assembler", "ehbasic_ie64.ie64"),
		}
		for _, p := range exeCandidates {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}

	return ""
}

func resolveDefaultAROSImagePath() string {
	candidates := []string{
		"sdk/roms/aros-ie-m68k.rom",
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func resolveDefaultEmuTOSImagePath() string {
	candidates := []string{
		"etos256us.img",
		"emutos.img",
		"bin/etos256us.img",
		"bin/emutos.img",
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}
