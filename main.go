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
	"strconv"
	"strings"
	"time"
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
	fmt.Println("\nA modern 32-bit reimagining of the Commodore, Atari and Sinclair 8-bit home computers.")
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
//	gui, err := NewGUIFrontend(GUI_FRONTEND_GTK4, cpu, videoChip, soundChip)
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
	boilerPlate()

	var (
		modeIE32   bool
		modeIE64   bool
		modeBasic  bool
		basicImage string
		modeM68K   bool
		modeM6502  bool
		modeZ80    bool
		modeX86    bool
		modePSG    bool
		modeSID    bool
		psgPlus    bool
		sidPlus    bool
		modePOKEY  bool
		pokeyPlus  bool
		modeTED    bool
		tedPlus    bool
		modeAHX    bool
		ahxPlus    bool
		perfMode   bool
		sidFile    string
		sidDebug   int
		sidPAL     bool
		sidNTSC    bool
		loadAddr   optionalStringFlag
		entryAddr  optionalStringFlag
		resWidth   int
		resHeight  int
		scale      int
		fullscreen bool
	)

	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.BoolVar(&modeIE32, "ie32", false, "Run IE32 CPU mode")
	flagSet.BoolVar(&modeIE64, "ie64", false, "Run IE64 CPU mode (64-bit RISC)")
	flagSet.BoolVar(&modeBasic, "basic", false, "Run EhBASIC IE64 interpreter (embedded image)")
	flagSet.StringVar(&basicImage, "basic-image", "", "Run EhBASIC IE64 from custom binary path")
	flagSet.BoolVar(&modeM68K, "m68k", false, "Run M68K CPU mode")
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
	flagSet.BoolVar(&perfMode, "perf", false, "Enable performance measurement (MIPS reporting)")
	flagSet.IntVar(&resWidth, "width", 0, "Override output width (0 = auto)")
	flagSet.IntVar(&resHeight, "height", 0, "Override output height (0 = auto)")
	flagSet.IntVar(&scale, "scale", 1, "Integer window scale factor (1-4)")
	flagSet.BoolVar(&fullscreen, "fullscreen", false, "Start in fullscreen mode")
	loadAddr.value = "0x0600"
	flagSet.Var(&loadAddr, "load-addr", "6502/Z80 load address (hex or decimal, defaults: 6502=0x0600, Z80=0x0000)")
	flagSet.Var(&entryAddr, "entry", "6502/Z80 entry address (hex or decimal, defaults to load address)")

	flagSet.Usage = func() {
		flagSet.SetOutput(os.Stdout)
		fmt.Println("Usage: ./intuition_engine -ie32|-ie64|-basic|-m68k|-m6502|-z80|-x86|-psg|-psg+|-sid|-sid+|-pokey|-pokey+|-ted|-ted+|-ahx|-ahx+ [--basic-image path] [--load-addr addr] [--entry addr] filename")
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
	// -basic is IE64 mode with embedded/custom BASIC image
	if modeBasic {
		modeIE64 = true
	}

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
	if modeCount == 0 && filename == "" {
		modeIE32 = true
		modeCount = 1
	}
	if modeCount != 1 {
		fmt.Println("Error: select exactly one mode flag: -ie32, -ie64, -m68k, -m6502, -z80, -x86, -basic, -psg, -psg+, -sid, -sid+, -pokey, -pokey+, -ted, -ted+, -ahx, or -ahx+")
		os.Exit(1)
	}
	if modeBasic && filename != "" {
		fmt.Println("Error: -basic and -basic-image do not accept a positional filename")
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
				fmt.Printf("PSG: register dump — no CPU to measure\n")
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
		soundChip.SetSampleTicker(sidEngine)
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
				fmt.Printf("AHX: software module replay — no CPU to measure\n")
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

	// Create system bus
	sysBus := NewMachineBus()
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
	termHost := NewTerminalHost(termMMIO)

	// Map I/O regions for peripherals
	sysBus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
		soundChip.HandleRegisterRead,
		soundChip.HandleRegisterWrite)

	sysBus.MapIO(VIDEO_CTRL, VIDEO_REG_END,
		videoChip.HandleRead,
		videoChip.HandleWrite)

	// Register lock-free VIDEO_STATUS reader for fast VBlank polling
	sysBus.SetVideoStatusReader(videoChip.HandleRead)

	sysBus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
		videoChip.HandleRead,
		videoChip.HandleWrite)

	sysBus.MapIO(TERM_OUT, TERMINAL_REGION_END,
		termMMIO.HandleRead,
		termMMIO.HandleWrite)

	// Map PSG registers (CPU modes only)
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

	// Map TED registers
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

	// Map POKEY registers (Atari POKEY chip for SAP playback in CPU modes)
	pokeyEngine := NewPOKEYEngine(soundChip, SAMPLE_RATE)
	pokeyPlayer := NewPOKEYPlayer(pokeyEngine)
	pokeyPlayer.AttachBus(sysBus)
	sysBus.MapIO(POKEY_BASE, POKEY_END,
		pokeyEngine.HandleRead,
		pokeyEngine.HandleWrite)
	sysBus.MapIO(SAP_PLAY_PTR, SAP_SUBSONG,
		pokeyPlayer.HandlePlayRead,
		pokeyPlayer.HandlePlayWrite)

	// Map VGA registers (VGA is a standalone video device)
	vgaEngine := NewVGAEngine(sysBus)
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
	ulaEngine := NewULAEngine(sysBus)
	sysBus.MapIO(ULA_BASE, ULA_REG_END,
		ulaEngine.HandleRead,
		ulaEngine.HandleWrite)
	sysBus.MapIO(ULA_VRAM_BASE, ULA_VRAM_BASE+ULA_VRAM_SIZE-1,
		ulaEngine.HandleBusVRAMRead,
		ulaEngine.HandleBusVRAMWrite)

	// Map TED video registers (Commodore Plus/4 video chip)
	tedVideoEngine := NewTEDVideoEngine(sysBus)
	sysBus.MapIO(TED_VIDEO_BASE, TED_VIDEO_END,
		tedVideoEngine.HandleRead,
		tedVideoEngine.HandleWrite)
	sysBus.MapIO(TED_V_VRAM_BASE, TED_V_VRAM_BASE+TED_V_VRAM_SIZE-1,
		tedVideoEngine.HandleBusVRAMRead,
		tedVideoEngine.HandleBusVRAMWrite)

	// Map ANTIC video registers (Atari 8-bit video chip)
	anticEngine := NewANTICEngine(sysBus)
	sysBus.MapIO(ANTIC_BASE, ANTIC_END,
		anticEngine.HandleRead,
		anticEngine.HandleWrite)
	// Map GTIA color registers (ANTIC's companion chip)
	sysBus.MapIO(GTIA_BASE, GTIA_END,
		anticEngine.HandleRead,
		anticEngine.HandleWrite)

	// Map Voodoo 3D graphics registers (3DFX SST-1 with Vulkan HLE)
	voodooEngine, err := NewVoodooEngine(sysBus)
	if err != nil {
		fmt.Printf("Warning: Voodoo initialization failed: %v\n", err)
	} else {
		sysBus.MapIO(VOODOO_BASE, VOODOO_END,
			voodooEngine.HandleRead,
			voodooEngine.HandleWrite)
	}

	// Create video compositor - owns the display output and blends video sources
	compositor := NewVideoCompositor(videoChip.GetOutput())
	compositor.RegisterSource(videoChip)      // Layer 0 - background
	compositor.RegisterSource(vgaEngine)      // Layer 10 - VGA renders on top
	compositor.RegisterSource(tedVideoEngine) // Layer 12 - TED video between VGA and ULA
	compositor.RegisterSource(anticEngine)    // Layer 13 - ANTIC (Atari 8-bit)
	compositor.RegisterSource(ulaEngine)      // Layer 15 - ULA renders on top of TED
	if voodooEngine != nil {
		compositor.RegisterSource(voodooEngine) // Layer 20 - Voodoo 3D on top
	}
	videoChip.SetResolutionChangeCallback(func(w, h int) {
		compositor.NotifyResolutionChange(w, h)
	})
	if voodooEngine != nil {
		voodooEngine.SetResolutionChangeCallback(func(w, h int) {
			compositor.NotifyResolutionChange(w, h)
		})
	}
	if useResolutionOverride {
		compositor.LockResolution(validWidth, validHeight)
	}
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

	// Initialize the selected CPU and optionally load program
	var gui GUIFrontend
	var startExecution bool

	if modeIE32 {
		// Initialize IE32 CPU
		ie32CPU := NewCPU(sysBus)
		ie32CPU.PerfEnabled = perfMode

		// Load program
		if filename != "" {
			if err := ie32CPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading IE32 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		// Initialize GUI with IE32 CPU
		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, ie32CPU, videoChip, soundChip, psgPlayer, sidPlayer, ahxPlayerCPU)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()

			// Start CPU execution
			fmt.Printf("Starting IE32 CPU with program: %s\n", filename)
			go ie32CPU.Execute()
		}

	} else if modeIE64 {
		// Initialize IE64 CPU (64-bit RISC)
		ie64CPU := NewCPU64(sysBus)
		ie64CPU.PerfEnabled = perfMode

		// Load program — three paths: -basic, -basic-image, or explicit file
		if modeBasic {
			if basicImage != "" {
				// -basic-image: load custom binary
				if err := ie64CPU.LoadProgram(basicImage); err != nil {
					fmt.Printf("Error loading BASIC image %s: %v\n", basicImage, err)
					os.Exit(1)
				}
				fmt.Printf("Starting EhBASIC IE64 (custom image: %s)\n", basicImage)
			} else if len(embeddedBasicImage) > 0 {
				// -basic: load embedded image
				ie64CPU.LoadProgramBytes(embeddedBasicImage)
				fmt.Println("Starting EhBASIC IE64 (embedded image)")
			} else {
				fmt.Println("Error: BASIC not embedded in this build; use -basic-image or rebuild with 'make basic'")
				os.Exit(1)
			}
			startExecution = true
		} else if filename != "" {
			if err := ie64CPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading IE64 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		// Initialize GUI with IE64 CPU
		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, ie64CPU, videoChip, soundChip, psgPlayer, sidPlayer, ahxPlayerCPU)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()

			// Start CPU execution
			if !modeBasic {
				fmt.Printf("Starting IE64 CPU with program: %s\n", filename)
			}
			go ie64CPU.Execute()
		}

	} else if modeM68K {
		// Initialize M68K CPU
		m68kCPU := NewM68KCPU(sysBus)
		// debug defaults to false (atomic.Bool), no need to set

		// Configure video chip for M68K (big-endian data format)
		videoChip.SetBigEndianMode(true)

		// Load program
		if filename != "" {
			if err := m68kCPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading M68K program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}
		//fmt.Println("Memory after program load:")
		//for i := 0x1000; i < 0x1020; i += 2 {
		//	value := m68kCPU.Read16(uint32(i))
		//	fmt.Printf("Addr 0x%08X: 0x%04X\n", i, value)
		//}

		// Initialize GUI with M68K CPU
		// Note: The GUI might need modifications to properly support M68K CPU
		m68kRunner := NewM68KRunner(m68kCPU)
		m68kRunner.PerfEnabled = perfMode
		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, m68kRunner, videoChip, soundChip, psgPlayer, sidPlayer, ahxPlayerCPU)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.SetSampleTicker(sidEngine)
			soundChip.Start()

			// Start CPU execution
			fmt.Printf("Starting M68K CPU with program: %s\n\n", filename)
			go m68kRunner.Execute()
		}
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
		var parsedEntry uint16
		if entryAddr.set {
			parsed, err := parseUint16Flag(entryAddr.value)
			if err != nil {
				fmt.Printf("Invalid --entry: %v\n", err)
				os.Exit(1)
			}
			parsedEntry = parsed
		}

		z80CPU := NewCPUZ80Runner(sysBus, CPUZ80Config{
			LoadAddr:     parsedLoadAddr,
			Entry:        parsedEntry,
			VGAEngine:    vgaEngine,
			VoodooEngine: voodooEngine,
		})
		z80CPU.PerfEnabled = perfMode

		// Load program
		if filename != "" {
			if err := z80CPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading Z80 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, z80CPU, videoChip, soundChip, psgPlayer, sidPlayer, ahxPlayerCPU)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()

			// Start CPU execution
			fmt.Printf("Starting Z80 CPU with program: %s\n\n", filename)
			go z80CPU.Execute()
		}
	} else if modeX86 {
		// x86 32-bit flat memory model
		x86Config := &CPUX86Config{
			LoadAddr:     0,
			Entry:        0,
			VGAEngine:    vgaEngine,
			VoodooEngine: voodooEngine,
		}

		x86CPU := NewCPUX86Runner(sysBus, x86Config)
		x86CPU.PerfEnabled = perfMode

		// Load program
		if filename != "" {
			if err := x86CPU.LoadProgramFromFile(filename); err != nil {
				fmt.Printf("Error loading x86 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, x86CPU, videoChip, soundChip, psgPlayer, sidPlayer, ahxPlayerCPU)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()

			// Start CPU execution
			fmt.Printf("Starting x86 CPU with program: %s\n\n", filename)
			go x86CPU.Execute()
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
			// Auto-detect IE65 files and default to $0800 load address
			parsedLoadAddr = 0x0800
		}
		var parsedEntry uint16
		if entryAddr.set {
			parsed, err := parseUint16Flag(entryAddr.value)
			if err != nil {
				fmt.Printf("Invalid --entry: %v\n", err)
				os.Exit(1)
			}
			parsedEntry = parsed
		}

		// Initialize 6502 CPU
		cpu6502 := NewCPU6502Runner(sysBus, CPU6502Config{
			LoadAddr: parsedLoadAddr,
			Entry:    parsedEntry,
		})
		cpu6502.PerfEnabled = perfMode

		// Load program
		if filename != "" {
			if err := cpu6502.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading 6502 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, cpu6502, videoChip, soundChip, psgPlayer, sidPlayer, ahxPlayerCPU)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
			compositor.Start()
			vgaEngine.StartRenderLoop()
			ulaEngine.StartRenderLoop()
			tedVideoEngine.StartRenderLoop()
			anticEngine.StartRenderLoop()
			soundChip.Start()

			// Start CPU execution
			fmt.Printf("Starting 6502 CPU with program: %s\n\n", filename)
			go cpu6502.Execute()
		}
	}

	// Configure and show GUI
	config := GUIConfig{
		Width:     800,
		Height:    600,
		Title:     "Intuition Engine",
		Resizable: true,
	}

	if err := gui.Initialize(config); err != nil {
		fmt.Printf("Failed to configure GUI: %v\n", err)
		os.Exit(1)
	}

	// If running with a file argument, minimize control window so it doesn't
	// obscure the display (Wayland doesn't allow apps to position windows)
	if startExecution {
		if minimizable, ok := gui.(interface{ SetStartMinimized(bool) }); ok {
			minimizable.SetStartMinimized(true)
		}
	}

	// Start terminal host (stdin non-blocking) just before the GUI loop.
	// Placed here so no os.Exit() path sits between Start and Stop.
	termHost.Start()
	outputTicker := time.NewTicker(10 * time.Millisecond)
	outputStop := make(chan struct{})
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

	// Show the GUI and run the main event loop
	err = gui.Show()

	// Shut down terminal host (restores stdin to blocking) and render goroutines.
	outputTicker.Stop()
	close(outputStop)
	termHost.Stop()
	vgaEngine.StopRenderLoop()
	ulaEngine.StopRenderLoop()
	tedVideoEngine.StopRenderLoop()
	anticEngine.StopRenderLoop()

	if err != nil {
		return
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
