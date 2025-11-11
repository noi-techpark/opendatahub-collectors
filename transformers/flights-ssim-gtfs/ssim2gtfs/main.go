// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"flag"
	"fmt"
	"os"
	"ssim_parser/converter"
	"ssim_parser/enrichment"
	"ssim_parser/gtfs"
	"ssim_parser/ssim"
)

func main() {
	inputFile := flag.String("input", "", "Input SSIM file")
	outputDir := flag.String("output", "gtfs_output", "Output directory for GTFS files")
	enrichmentFile := flag.String("enrich", "", "YAML file with airport coordinates and agency info")
	generateTemplate := flag.String("generate-template", "", "Generate enrichment template YAML file")
	showMissing := flag.Bool("show-missing", false, "Show fields that cannot be converted")
	flag.Parse()

	if *showMissing {
		printMissingFields()
		return
	}

	if *inputFile == "" {
		fmt.Println("Usage:")
		fmt.Println("  Convert:  ssim2gtfs -input <ssim_file> -output <output_dir> [-enrich <yaml_file>]")
		fmt.Println("  Template: ssim2gtfs -input <ssim_file> -generate-template <output_yaml>")
		fmt.Println("  Info:     ssim2gtfs -show-missing")
		fmt.Println()
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Parse SSIM file
	fmt.Printf("Reading SSIM file: %s\n", *inputFile)
	file, err := os.Open(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	parser := ssim.NewParser(file)
	records, err := parser.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing SSIM: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parsed %d SSIM records\n", len(records))

	// Extract codes from SSIM
	airlineCodes, airportCodes := converter.ExtractCodes(records)
	fmt.Printf("Found %d airlines and %d airports\n", len(airlineCodes), len(airportCodes))

	// Generate template if requested
	if *generateTemplate != "" {
		fmt.Printf("Generating enrichment template: %s\n", *generateTemplate)
		if err := enrichment.GenerateTemplate(*generateTemplate, airlineCodes, airportCodes); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating template: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Template generated successfully!")
		fmt.Println("Please edit the file to add actual coordinates and agency information.")
		return
	}

	// Load enrichment data if provided
	var enrichmentData *enrichment.EnrichmentData
	if *enrichmentFile != "" {
		fmt.Printf("Loading enrichment data: %s\n", *enrichmentFile)
		enrichmentData, err = enrichment.LoadFromFile(*enrichmentFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading enrichment data: %v\n", err)
			os.Exit(1)
		}

		// Validate enrichment data
		missing := enrichmentData.Validate(airlineCodes, airportCodes)
		if len(missing) > 0 {
			fmt.Println("⚠ Warning: Missing enrichment data for:")
			for _, item := range missing {
				fmt.Printf("  - %s\n", item)
			}
			fmt.Println()
		} else {
			fmt.Println("✓ All codes have enrichment data")
		}
	} else {
		fmt.Println("⚠ No enrichment file provided - using placeholder values")
		fmt.Println("  Run with -generate-template to create a template file")
	}

	// Convert to GTFS
	fmt.Println("Converting to GTFS format...")
	conv := converter.NewConverter(records)
	if enrichmentData != nil {
		conv.SetEnrichmentData(enrichmentData)
	}
	
	gtfsData, err := conv.Convert()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error converting to GTFS: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated GTFS data:\n")
	fmt.Printf("  - %d agencies\n", len(gtfsData.Agency))
	fmt.Printf("  - %d stops\n", len(gtfsData.Stops))
	fmt.Printf("  - %d routes\n", len(gtfsData.Routes))
	fmt.Printf("  - %d trips\n", len(gtfsData.Trips))
	fmt.Printf("  - %d stop times\n", len(gtfsData.StopTimes))
	fmt.Printf("  - %d calendar entries\n", len(gtfsData.Calendar))

	// Write GTFS files
	fmt.Printf("Writing GTFS files to: %s\n", *outputDir)
	writer := gtfs.NewWriter(*outputDir)
	if err := writer.Write(gtfsData); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing GTFS files: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Conversion complete!")
	
	if enrichmentData == nil {
		fmt.Println("\n⚠ Note: Airport coordinates and agency info are placeholders.")
		fmt.Println("Run with -generate-template to create an enrichment file.")
	}
}

func printMissingFields() {
	fmt.Println("=== SSIM Fields That Cannot Be Mapped to GTFS ===\n")
	for i, field := range converter.GetMissingFields() {
		fmt.Printf("%d. %s\n", i+1, field)
	}

	fmt.Println("\n=== GTFS Fields That Cannot Be Populated from SSIM ===\n")
	for i, field := range converter.GetGTFSFieldsMissingInSSIM() {
		fmt.Printf("%d. %s\n", i+1, field)
	}
	
	fmt.Println("\n💡 Use -enrich flag with a YAML file to provide missing data")
	fmt.Println("💡 Use -generate-template to create a template enrichment file")
}
