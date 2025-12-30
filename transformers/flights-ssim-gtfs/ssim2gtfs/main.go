// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package ssim2gtfs

import (
	"flag"
	"fmt"
	"log"
	"os"

	ssim "opendatahub.com/ssimparser"
)

func main() {
	inputFile := flag.String("input", "", "Input SSIM file path")
	outputDir := flag.String("output", "gtfs_output", "Output directory for GTFS files")
	agencyName := flag.String("agency", "", "Agency name (optional, uses airline code if not provided)")
	agencyURL := flag.String("url", "http://example.com", "Agency URL")
	agencyTimezone := flag.String("timezone", "UTC", "Agency timezone")

	flag.Parse()

	if *inputFile == "" {
		log.Fatal("Input file is required. Use -input flag")
	}

	// Parse SSIM file
	file, err := os.Open(*inputFile)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer file.Close()

	parser := ssim.NewParser()
	ssimData, err := parser.Parse(file)
	if err != nil {
		log.Fatalf("Error parsing SSIM: %v", err)
	}

	// Convert to GTFS
	converter := NewSSIMToGTFSConverter(*agencyName, *agencyURL, *agencyTimezone)
	err = converter.Convert(ssimData, *outputDir)
	if err != nil {
		log.Fatalf("Error converting to GTFS: %v", err)
	}

	fmt.Printf("Successfully converted SSIM to GTFS in directory: %s\n", *outputDir)
}
