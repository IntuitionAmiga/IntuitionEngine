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
)

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
//	sysBus := NewSystemBus()
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
		modeIE32  bool
		modeM68K  bool
		modeM6502 bool
		modePSG   bool
		psgPlus   bool
		loadAddr  string
		entryAddr string
	)

	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.BoolVar(&modeIE32, "ie32", false, "Run IE32 CPU mode")
	flagSet.BoolVar(&modeM68K, "m68k", false, "Run M68K CPU mode")
	flagSet.BoolVar(&modeM6502, "m6502", false, "Run 6502 CPU mode")
	flagSet.BoolVar(&modePSG, "psg", false, "Play PSG file")
	flagSet.BoolVar(&psgPlus, "psg+", false, "Enable PSG+ enhancements")
	flagSet.StringVar(&loadAddr, "load-addr", "0x0600", "6502 load address (hex or decimal)")
	flagSet.StringVar(&entryAddr, "entry", "", "6502 entry address (hex or decimal)")

	flagSet.Usage = func() {
		flagSet.SetOutput(os.Stdout)
		fmt.Println("Usage: ./intuition_engine -ie32|-m68k|-m6502|-psg|-psg+ [--load-addr 0x0600] [--entry 0x0600] filename")
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

	if psgPlus && !modePSG {
		modePSG = true
	}

	modeCount := 0
	if modeIE32 {
		modeCount++
	}
	if modeM68K {
		modeCount++
	}
	if modeM6502 {
		modeCount++
	}
	if modePSG {
		modeCount++
	}
	if modeCount == 0 && filename == "" {
		modeIE32 = true
		modeCount = 1
	}
	if modeCount != 1 {
		fmt.Println("Error: select exactly one mode flag: -ie32, -m68k, -m6502, -psg, or -psg+")
		os.Exit(1)
	}
	if filename == "" && modePSG {
		fmt.Println("Error: PSG mode requires a filename")
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
			fmt.Printf("Playing PSG: %s - %s\n", meta.Title, meta.Author)
		} else {
			fmt.Printf("Playing PSG file: %s\n", filename)
		}
		soundChip.Start()
		psgPlayer.Play()
		select {}
	}

	// Create system bus
	sysBus := NewSystemBus()

	videoChip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		fmt.Printf("Failed to initialize video: %v\n", err)
		os.Exit(1)
	}

	// Setup terminal output
	termOut := NewTerminalOutput()

	// Map I/O regions for peripherals
	sysBus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
		nil,
		soundChip.HandleRegisterWrite)

	sysBus.MapIO(VIDEO_CTRL, VIDEO_STATUS,
		videoChip.HandleRead,
		videoChip.HandleWrite)

	sysBus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
		videoChip.HandleRead,
		videoChip.HandleWrite)

	sysBus.MapIO(TERM_OUT, TERM_OUT,
		nil,
		termOut.HandleWrite)

	// Map PSG registers (CPU modes only)
	sysBus.MapIO(PSG_BASE, PSG_END,
		psgEngine.HandleRead,
		psgEngine.HandleWrite)
	sysBus.MapIO(PSG_PLUS_CTRL, PSG_PLUS_CTRL,
		psgEngine.HandlePSGPlusRead,
		psgEngine.HandlePSGPlusWrite)

	// Initialize the selected CPU and optionally load program
	var gui GUIFrontend
	var startExecution bool

	if modeIE32 {
		// Initialize IE32 CPU
		ie32CPU := NewCPU(sysBus)

		// Load program
		if filename != "" {
			if err := ie32CPU.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading IE32 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		// Initialize GUI with IE32 CPU
		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, ie32CPU, videoChip, soundChip, psgPlayer)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
			soundChip.Start()

			// Start CPU execution
			fmt.Printf("Starting IE32 CPU with program: %s\n", filename)
			go ie32CPU.Execute()
		}

	} else if modeM68K {
		// Initialize M68K CPU
		m68kCPU := NewM68KCPU(sysBus)
		// debug defaults to false (atomic.Bool), no need to set

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
		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, m68kRunner, videoChip, soundChip, psgPlayer)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
			soundChip.Start()

			// Start CPU execution
			fmt.Printf("Starting M68K CPU with program: %s\n\n", filename)
			go m68kRunner.Execute()
		}
	} else {
		parsedLoadAddr, err := parseUint16Flag(loadAddr)
		if err != nil {
			fmt.Printf("Invalid --load-addr: %v\n", err)
			os.Exit(1)
		}
		var parsedEntry uint16
		if entryAddr != "" {
			parsedEntry, err = parseUint16Flag(entryAddr)
			if err != nil {
				fmt.Printf("Invalid --entry: %v\n", err)
				os.Exit(1)
			}
		}

		// Initialize 6502 CPU
		cpu6502 := NewCPU6502Runner(sysBus, CPU6502Config{
			LoadAddr: parsedLoadAddr,
			Entry:    parsedEntry,
		})

		// Load program
		if filename != "" {
			if err := cpu6502.LoadProgram(filename); err != nil {
				fmt.Printf("Error loading 6502 program: %v\n", err)
				os.Exit(1)
			}
			startExecution = true
		}

		gui, err = NewGUIFrontend(GUI_FRONTEND_GTK4, cpu6502, videoChip, soundChip, psgPlayer)
		if err != nil {
			fmt.Printf("Failed to initialize GUI: %v\n", err)
			os.Exit(1)
		}

		if startExecution {
			// Start peripherals
			videoChip.Start()
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

	// Show the GUI and run the main event loop
	err = gui.Show()
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
