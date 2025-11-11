#!/bin/bash

# SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: AGPL-3.0-or-later

# SSIM to GTFS Converter - Build and Run Script

echo "Building SSIM to GTFS Converter..."
go build -o ssim2gtfs main.go

if [ $? -eq 0 ]; then
    echo "✓ Build successful!"
    echo ""
    echo "Usage examples:"
    echo "  ./ssim2gtfs -input sample.ssim -generate-template enrichment_template.yaml"
    echo "  ./ssim2gtfs -input sample.ssim -output gtfs/ -enrich enrichment.yaml"
    echo "  ./ssim2gtfs -show-missing"
    echo ""
    
    if [ -f "sample.ssim" ]; then
        echo "Running with sample data (with enrichment)..."
        if [ -f "enrichment.yaml" ]; then
            ./ssim2gtfs -input sample.ssim -output gtfs_enriched/ -enrich enrichment.yaml
        else
            echo "Running without enrichment..."
            ./ssim2gtfs -input sample.ssim -output gtfs_basic/
        fi
    fi
else
    echo "✗ Build failed"
    exit 1
fi
