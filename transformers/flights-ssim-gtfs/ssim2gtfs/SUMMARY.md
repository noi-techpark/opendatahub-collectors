<!--
SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# SSIM to GTFS Converter - Complete Project

## ✨ New Feature: YAML Enrichment

The converter now accepts a YAML file to provide missing data not available in SSIM format.

## Quick Start

### 1. Generate Enrichment Template
```bash
./ssim2gtfs -input sample.ssim -generate-template my_enrichment.yaml
```

### 2. Edit Template with Real Data
Open `my_enrichment.yaml` and fill in actual coordinates and agency info.

### 3. Convert with Enrichment
```bash
./ssim2gtfs -input sample.ssim -output gtfs/ -enrich my_enrichment.yaml
```

## Architecture

```
SSIM File ──→ Parser ──→ Converter ──→ GTFS Writer ──→ GTFS Files
                            ↑
                            │
                      Enrichment YAML
                   (airports & agencies)
```

## Project Structure

```
├── main.go                      # CLI with enrichment support
├── ssim/ssim_parser.go         # SSIM parsing
├── converter/converter.go      # SSIM→GTFS + enrichment
├── gtfs/gtfs.go                # GTFS writer
├── enrichment/enrichment.go    # YAML enrichment handler
├── sample.ssim                 # Example SSIM file
├── enrichment.yaml             # Example enrichment data
├── build.sh                    # Build script
├── README.md                   # Main documentation
├── ENRICHMENT_GUIDE.md         # Enrichment how-to
└── CONVERSION_GUIDE.md         # Field mapping details
```

## What Gets Enriched

### From Enrichment YAML:

✅ **Airport Coordinates**
- Precise latitude/longitude instead of 0.0, 0.0

✅ **Airport Names**
- Real names instead of "XXX Airport"

✅ **Airport Timezones**
- Proper IANA timezones for accurate local times

✅ **Agency Information**
- Official airline names
- Real website URLs
- Headquarters timezones
- Language codes
- Phone numbers

## Usage Examples

### Basic (No Enrichment)
```bash
./ssim2gtfs -input schedule.ssim -output gtfs/
# Uses placeholder values
```

### With Enrichment
```bash
./ssim2gtfs -input schedule.ssim -output gtfs/ -enrich enrichment.yaml
# Uses real coordinates and agency data
```

### Generate Template
```bash
./ssim2gtfs -input schedule.ssim -generate-template template.yaml
# Creates YAML template with all codes from SSIM
```

### Show Field Info
```bash
./ssim2gtfs -show-missing
# Lists fields that can't be converted
```

## Enrichment YAML Format

```yaml
airports:
  JFK:
    name: John F. Kennedy International Airport
    latitude: 40.6413
    longitude: -73.7781
    timezone: America/New_York
    city: New York        # optional
    country: USA          # optional

agencies:
  AA:
    name: American Airlines
    url: https://www.aa.com
    timezone: America/Chicago
    lang: en              # optional
    phone: +1-800-433-7300 # optional
```

## Data Sources for Enrichment

**Airport Data:**
- OurAirports: https://ourairports.com/data/
- OpenFlights: https://openflights.org/data.html

**Timezones:**
- IANA List: https://en.wikipedia.org/wiki/List_of_tz_database_time_zones

**Airline Info:**
- Official airline websites
- Wikipedia airline pages

## What Still Can't Be Converted

Even with enrichment, these SSIM fields have no GTFS equivalent:

❌ Aircraft type (no GTFS field)
❌ Service type - passenger/cargo (no equivalent)
❌ Meal service codes (not in core GTFS)
❌ Aircraft configuration (not in GTFS)
❌ Flight paths/shapes (not in SSIM)
❌ Detailed codeshare info (limited GTFS support)

## Validation

The converter automatically validates enrichment data:

```
✓ All codes have enrichment data
```

Or warns about missing entries:

```
⚠ Warning: Missing enrichment data for:
  - Airport: XYZ
  - Agency: AB
```

## Output Quality

**Without Enrichment:**
- Valid GTFS structure ✓
- Placeholder data (0,0 coordinates) ⚠️
- Not suitable for production ❌

**With Enrichment:**
- Valid GTFS structure ✓
- Real coordinates and data ✓
- Production-ready* ✓

*Still missing flight paths (shapes.txt) and some optional fields

## Files Generated

All GTFS files with enriched data:
- `agency.txt` - Real airline info
- `stops.txt` - Airports with coordinates
- `routes.txt` - Flight numbers
- `trips.txt` - Individual flights
- `stop_times.txt` - Times
- `calendar.txt` - Operating schedules

## Build & Run

```bash
chmod +x build.sh
./build.sh
```

Or manually:

```bash
go build -o ssim2gtfs
./ssim2gtfs -input sample.ssim -generate-template enrichment.yaml
# Edit enrichment.yaml
./ssim2gtfs -input sample.ssim -output gtfs/ -enrich enrichment.yaml
```

## Documentation

- **README.md** - Usage and features
- **ENRICHMENT_GUIDE.md** - How to create enrichment files
- **CONVERSION_GUIDE.md** - Detailed field mapping
- **SUMMARY.md** - This file

## Dependencies

- Go 1.21+
- gopkg.in/yaml.v3

Install with: `go mod download`
