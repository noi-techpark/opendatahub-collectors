<!--
SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# SSIM to GTFS Conversion: Field Mapping Analysis

## Overview

This document details which fields can and cannot be converted between SSIM and GTFS formats.

## SSIM Fields That CANNOT Be Mapped to GTFS

### 1. Geographic Coordinates
**SSIM Data:** Airport IATA codes only (e.g., "JFK", "LAX")
**GTFS Requirement:** Precise latitude/longitude coordinates
**Impact:** Critical - GTFS requires coordinates for all stops
**Solution:** Must enrich with external airport database

### 2. Agency Information
**Missing in SSIM:**
- Agency website URL (required in GTFS)
- Agency timezone (SSIM has per-flight UTC variations)
- Agency phone number
- Agency language

**Solution:** Must be added manually or from external source

### 3. Aircraft-Specific Information
**SSIM Data:** Aircraft type code (e.g., "738", "777")
**GTFS:** No field for aircraft type
**Note:** This information is lost in conversion

### 4. Service Classifications
**SSIM Data:**
- Service type (passenger, cargo, mail)
- Itinerary variation identifier
- Leg sequence numbers
- Traffic restriction codes

**GTFS:** No equivalent fields
**Note:** Some can be approximated with route descriptions

### 5. Operational Details
**SSIM Data:**
- Creator reference codes
- Data set serial numbers
- UTC time variations per segment
- Operating vs. marketing carrier distinctions

**GTFS:** Limited or no support
**Note:** GTFS has basic codeshare support but not full detail

### 6. Onboard Services
**SSIM Data:**
- Meal service codes
- Smoking restrictions
- Passenger reservation classes
- Aircraft seat configuration

**GTFS:** Not in core specification
**Note:** Could be added as extended attributes

### 7. Flight Path Information
**SSIM:** Describes legs and segments
**GTFS:** Uses shapes.txt for geographic paths
**Gap:** SSIM doesn't include actual flight path coordinates

## GTFS Fields That CANNOT Be Populated from SSIM

### Required Fields Missing

1. **stop_lat, stop_lon**
   - Must be obtained from airport database
   - Critical for mapping and routing

2. **agency_url**
   - Required by GTFS specification
   - Must be provided manually

3. **agency_timezone**
   - Required by GTFS specification
   - Must be inferred from airline headquarters or route

### Optional But Important Fields

4. **stop_timezone**
   - Recommended for accurate time display
   - Should be derived from airport location

5. **route_long_name**
   - Not in SSIM
   - Could be constructed from origin-destination

6. **route_desc**
   - Not in SSIM
   - Useful for passengers

7. **trip_short_name**
   - Can be derived from flight number
   - Not explicitly in SSIM

8. **trip_headsign**
   - Can use destination airport
   - Convention not specified in SSIM

### Geographic Data

9. **shape_id and shapes.txt**
   - Flight paths not included in SSIM
   - Would require flight tracking data

10. **stop_code**
    - Could use IATA code
    - Not explicitly mapped

### Operational Features

11. **block_id**
    - Aircraft rotations not explicit in SSIM
    - Would require operational analysis

12. **wheelchair_accessible**
    - Not in SSIM
    - Not typically applicable for air service

13. **bikes_allowed**
    - Not applicable for air travel

14. **pickup_type, drop_off_type**
    - Always 0 (regular service) for flights

## Mapping Strategy

### Direct Mappings (1:1)

| SSIM Field | GTFS Field |
|------------|------------|
| Airline Designator | agency_id |
| Flight Number | route_short_name |
| Departure Station | stop_id (origin) |
| Arrival Station | stop_id (destination) |
| STD (Scheduled Time Departure) | departure_time |
| STA (Scheduled Time Arrival) | arrival_time |
| Days of Operation | monday-sunday in calendar.txt |
| Period of Operation | start_date, end_date |

### Derived Mappings (Requires Logic)

| SSIM Data | GTFS Field | Derivation |
|-----------|------------|------------|
| Flight Number + Airline | route_id | Concatenate with separator |
| Days + Period | service_id | Hash of schedule pattern |
| Destination | trip_headsign | Use arrival airport |
| Leg sequence | stop_sequence | Map to sequential numbers |

### Default Values (No SSIM Data)

| GTFS Field | Default Value | Reason |
|------------|---------------|--------|
| route_type | 1100 | GTFS extended: Air service |
| location_type | 1 | Station (airport) |
| stop_lat | 0.0 | Placeholder - must enrich |
| stop_lon | 0.0 | Placeholder - must enrich |
| agency_url | "http://example.com" | Placeholder |
| agency_timezone | "UTC" | Fallback |
| agency_lang | "en" | Assumption |

## Recommendations

### Critical Actions Required

1. **Enrich airport data** with coordinates from sources like:
   - OurAirports database
   - OpenFlights airport database
   - IATA official data

2. **Add agency information:**
   - Website URLs
   - Proper timezone (airline headquarters)
   - Contact information

3. **Consider timezone accuracy:**
   - Each airport should have correct timezone
   - Times should be in local time at each airport

### Optional Enhancements

1. Add route descriptions (city pairs)
2. Include aircraft type in route long name
3. Add fare information if available
4. Consider extensions for aviation-specific data

## Conclusion

**Can be converted:** Basic schedule information (routes, times, dates, frequencies)

**Cannot be converted:**
- Precise geographic data (requires external database)
- Agency metadata (requires manual input)
- Aviation-specific attributes (aircraft, service classes)
- Detailed operational data (codeshares, restrictions)

**Result:** A functional but minimal GTFS feed that requires enrichment for production use.
