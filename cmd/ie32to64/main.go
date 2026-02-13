package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	outFile := flag.String("o", "", "Output file (default: input_ie64.asm)")
	sizeSuffix := flag.String("size", ".l", "Default size suffix (.l or .q)")
	noHeader := flag.Bool("no-header", false, "Omit header comment")
	stats := flag.Bool("stats", false, "Print conversion statistics")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ie32to64 [options] input.asm\n\nConverts IE32 assembly source to IE64 assembly.\n\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  ie32to64 assembler/rotozoomer.asm\n")
		fmt.Fprintf(os.Stderr, "  ie32to64 -o assembler/rotozoomer_ie64.asm assembler/rotozoomer.asm\n")
		fmt.Fprintf(os.Stderr, "  ie32to64 -size .q assembler/program.asm\n")
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputPath := flag.Arg(0)

	if *sizeSuffix != ".l" && *sizeSuffix != ".q" {
		fmt.Fprintf(os.Stderr, "error: -size must be .l or .q\n")
		os.Exit(1)
	}

	conv := NewConverter()
	conv.sizeSuffix = *sizeSuffix
	conv.noHeader = *noHeader

	output, err := conv.ConvertFileFromPath(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	outputPath := *outFile
	if outputPath == "" {
		outputPath = strings.TrimSuffix(inputPath, ".asm") + "_ie64.asm"
	}

	if err := os.WriteFile(outputPath, []byte(output), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	if *stats {
		inputLines := strings.Count(string(mustReadFile(inputPath)), "\n") + 1
		outputLines := strings.Count(output, "\n") + 1
		fmt.Printf("Input:  %s (%d lines)\n", inputPath, inputLines)
		fmt.Printf("Output: %s (%d lines)\n", outputPath, outputLines)
		if conv.errors > 0 {
			fmt.Printf("Errors: %d (search for '; ERROR:' in output)\n", conv.errors)
		}
	}

	if conv.errors > 0 {
		fmt.Fprintf(os.Stderr, "%d conversion error(s) — search for '; ERROR:' in %s\n", conv.errors, outputPath)
		os.Exit(1)
	}
}

func mustReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}
