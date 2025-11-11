<!--
SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# SSIM to GTFS Converter

Converts IATA Standard Schedules Information Manual (SSIM) format to GTFS (General Transit Feed Specification).

## Features

- Parses SSIM records (Header, Carrier, Flight, Trailer)
- Converts flight schedules to GTFS format
- **NEW: YAML-based enrichment for missing data**
- Automatic template generation
- Validates enrichment data completeness
- Generates all required GTFS files
- Modular architecture: Parser → Converter → Writer

## Installation

```bash
go build -o ssim2gtfs
```

## Usage

### 1. Generate Enrichment Template

First, analyze your SSIM file and generate a template:

```bash
./ssim2gtfs -input schedule.ssim -generate-template enrichment.yaml
```

This creates a YAML file with all airports and airlines found in your SSIM file.

### 2. Edit the Template

Open `enrichment.yaml` and fill in the actual values:

```yaml
airports:
  JFK:
    name: John F. Kennedy International Airport
    latitude: 40.6413
    longitude: -73.7781
    timezone: America/New_York
    city: New York
    country: USA

agencies:
  AA:
    name: American Airlines
    url: https://www.aa.com
    timezone: America/Chicago
    lang: en
    phone: +1-800-433-7300
```

### 3. Convert with Enrichment

```bash
./ssim2gtfs -input schedule.ssim -output gtfs/ -enrich enrichment.yaml
```

### 4. Convert without Enrichment (Basic)

```bash
./ssim2gtfs -input schedule.ssim -output gtfs/
```

This uses placeholder values (0,0 coordinates, example.com URLs).

### Show Field Mapping Info

```bash
./ssim2gtfs -show-missing
```

## Example Workflow

```bash
# Step 1: Generate template from your SSIM file
./ssim2gtfs -input sample.ssim -generate-template my_enrichment.yaml

# Step 2: Edit my_enrichment.yaml with real data
nano my_enrichment.yaml

# Step 3: Convert with enrichment
./ssim2gtfs -input sample.ssim -output gtfs/ -enrich my_enrichment.yaml
```

## Enrichment File Format

```yaml
airports:
  IATA_CODE:
    name: "Full Airport Name"
    latitude: 40.6413
    longitude: -73.7781
    timezone: "America/New_York"
    city: "City Name"        # optional
    country: "Country Name"  # optional

agencies:
  AIRLINE_CODE:
    name: "Airline Name"
    url: "https://www.airline.com"
    timezone: "America/New_York"
    lang: "en"               # optional
    phone: "+1-800-000-0000" # optional
```

## Project Structure

```
├── main.go                  # Main program
├── ssim/ssim_parser.go     # SSIM parsing
├── gtfs/gtfs.go            # GTFS structures & writer
├── converter/converter.go  # SSIM→GTFS conversion
├── enrichment/enrichment.go # YAML enrichment handling
├── enrichment.yaml         # Example enrichment data
└── sample.ssim             # Example SSIM file
```

## What Gets Enriched

✓ **Airports:**
- Real names (instead of "XXX Airport")
- Accurate coordinates (instead of 0.0, 0.0)
- Correct timezones
- City and country information

✓ **Airlines:**
- Official names (instead of "XX Airlines")
- Real websites (instead of example.com)
- Correct headquarters timezone
- Language and phone numbers

## SSIM Fields That CANNOT Be Mapped to GTFS

1. **Aircraft type** - No GTFS field (lost in conversion)
2. **Service type** (passenger/cargo) - No direct equivalent
3. **Flight codes** (meal service) - Not in core GTFS
4. **Aircraft configuration** - Not in GTFS
5. **Traffic restrictions** - Not in GTFS
6. **Codeshare details** - Limited GTFS support

## GTFS Fields Requiring Enrichment

1. **stop_lat, stop_lon** - ✓ From enrichment YAML
2. **agency_url** - ✓ From enrichment YAML
3. **agency_timezone** - ✓ From enrichment YAML
4. **stop_name** - ✓ From enrichment YAML
5. **route_long_name** - Not in SSIM
6. **shapes.txt** - Flight paths not in SSIM

## Output GTFS Files

- `agency.txt` - Airlines (enriched with real data)
- `stops.txt` - Airports (with coordinates from YAML)
- `routes.txt` - Flight numbers
- `trips.txt` - Individual flight instances
- `stop_times.txt` - Departure/arrival times
- `calendar.txt` - Days of operation

## Notes

- The enrichment file is validated automatically
- Missing codes are reported but don't stop conversion
- Without enrichment, placeholder values are used
- See `CONVERSION_GUIDE.md` for detailed field mapping
