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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"os"
)

func boilerPlate() {
	fmt.Println("\n\033[38;2;255;20;147m ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████\033[0m\n\033[38;2;255;50;147m▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀\033[0m\n\033[38;2;255;80;147m▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███\033[0m\n\033[38;2;255;110;147m░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄\033[0m\n\033[38;2;255;140;147m░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒\033[0m\n\033[38;2;255;170;147m░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░\033[0m\n\033[38;2;255;200;147m ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░\033[0m\n\033[38;2;255;230;147m ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░\033[0m\n\033[38;2;255;255;147m ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░\033[0m")
	fmt.Println("\nA modern 32-bit reimagining of the Commodore, Atari and Sinclair 8-bit home computers.")
	fmt.Println("(c) 2024 - 2025 Zayn Otley")
	fmt.Println("https://github.com/IntuitionAmiga/IntuitionEngine")
}

func main() {
	boilerPlate()

	sysBus := NewSystemBus()
	cpu := NewCPU(sysBus)

	soundChip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		fmt.Printf("Failed to initialize sound: %v\n", err)
		os.Exit(1)
	}

	videoChip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		fmt.Printf("Failed to initialize video: %v\n", err)
		os.Exit(1)
	}

	// Map sound registers
	sysBus.MapIO(0xF900, 0xF97C,
		nil, // No read handler needed
		soundChip.HandleRegisterWrite)

	// Map video registers and VRAM
	sysBus.MapIO(0xF000, 0xF008,
		videoChip.HandleRead,
		videoChip.HandleWrite)
	sysBus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1,
		videoChip.HandleRead,
		videoChip.HandleWrite)

	var startExecution bool
	if len(os.Args) > 1 {
		if err := cpu.LoadProgram(os.Args[1]); err != nil {
			fmt.Printf("Error loading program: %v\n", err)
			os.Exit(1)
		}
		startExecution = true
	}

	// Initialize GUI
	gui, err := NewGUIFrontend(GUI_FRONTEND_GTK4, cpu, videoChip, soundChip)
	if err != nil {
		fmt.Printf("Failed to initialize GUI: %v\n", err)
		os.Exit(1)
	}

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

	// Start execution if we loaded a program
	if startExecution {
		videoChip.Start()
		soundChip.Start()
		go cpu.Execute()
	}

	// Show the GUI and run the main event loop
	err = gui.Show()
	if err != nil {
		return
	}
}
