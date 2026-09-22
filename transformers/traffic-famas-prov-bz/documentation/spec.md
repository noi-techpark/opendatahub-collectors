<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# FAMAS traffic sensor data - integration spec

Source: FAMAS traffic API, `https://webservices.trafficopab.famassystem.it/api/v1`.
Auth: HTTP Basic. Two endpoints integrated; a third exists but is unused.

## Endpoints

### `GET /AnagrafichePostazioni` - station metadata

Returns the full list of measurement stations, called once per collector
run (no pagination, no parameters).

| Field | Type | Use |
|---|---|---|
| `id` | int | Not used as station code (see below); used to scope the aggregated-data request per station |
| `nome` | string | Station name/code, e.g. `"3"`, `"65"` |
| `geoInfo.latitudine`, `geoInfo.longitudine` | float | Station coordinates |
| `geoInfo.regione` | string | -> `region` metadata |
| `geoInfo.comune` | string | -> `municipality` metadata |
| `stradaInfo.nome` | string | -> `street_name` metadata |
| `stradaInfo.chilometrica` | float | -> `kilometric` metadata |
| `numeroCorsie` | int | -> `total_lanes` metadata |
| `corsieInfo[]` | array | One entry per lane: `id` (1-indexed), `descrizione` (free-text lane label), `sensoDiMarcia` (direction, e.g. `ascendente`/`discendente`) |
| `schemaDiClassificazione` | int | Not used (a `schemiDiClassificazione`-keyed field exists in the legacy client but never matched the actual response field name - dead code, not carried forward) |
| `direzioni[]` | array | Not used |

A station's lane count is not fixed - `corsieInfo` is read fresh every
collector run and drives how many BDP stations are derived from it (see
Mapping below).

### `POST /DatiAggregatiSuPostazioni` - aggregated traffic counts

Body: `{"IdPostazioni": ["<nome>"], "InizioPeriodo": "<ISO8601, UTC, literal Z>", "FinePeriodo": "<same>"}`.
One row per lane, per direction, per 5-minute bucket within the requested
window. A single call covers every lane of the station - the response is
not scoped to one lane.

| Field | Type | Use |
|---|---|---|
| `data` | string | Bucket timestamp, `yyyy-MM-ddTHH:mm:ss`, sometimes suffixed `Z`; always UTC regardless |
| `corsia` | int | 0-indexed lane number (`corsieInfo[].id - 1`) - matches a lane, not a fixed physical meaning |
| `direzione` | string | Must match the lane's `sensoDiMarcia` for this row to belong to that lane |
| `totaleVeicoli` | float | Total vehicle count in the bucket |
| `totaliPerClasseVeicolare` | map[string -> float] | Per-vehicle-class counts, keyed by a small numeric class code (`"1"`..`"10"` observed) |
| `mediaArmonicaVelocita` | float | Harmonic mean vehicle speed |
| `headwayMedioSecondi`, `varianzaHeadwayMedioSecondi` | float | Mean headway and its variance |
| `gapMedioSecondi`, `varianzaGapMedioSecondi` | float | Mean gap and its variance |

A row only produces class-count/speed/headway/gap records when
`totaliPerClasseVeicolare` is present; `totaleVeicoli` is written
regardless.

### `GET /SchemiDiClassificazione` - not integrated

Returns the vehicle classification schema (human-readable names for the
class codes used in `totaliPerClasseVeicolare`). Not called - the class
code is used directly as a fixed index into a hardcoded data type table
(see below) rather than resolved dynamically, matching the legacy
collector.

### `POST /DatiPassaggiSuPostazioni` - out of scope

Bluetooth device passage detections per station. Not integrated in this
pipeline - see `README.md`.

## Mapping to Open Data Hub

**Station type**: `TrafficSensor`, one station per **lane**, not per
physical FAMAS station.

- **Station code**: `<nome>:<corsieInfo[].descrizione>`, e.g.
  `"3:Marcia (verso Bolzano)"`. Not a synthetic/hashed id - carried over
  verbatim from the legacy collector to preserve existing BDP station
  identities.
- **Name**: same as the code.
- **Coordinates**: the parent FAMAS station's `geoInfo` (lanes of the same
  station share one location).
- **Metadata**:
  | Key | Source | Type |
  |---|---|---|
  | `municipality` | `geoInfo.comune` | string |
  | `region` | `geoInfo.regione` | string |
  | `street_name` | `stradaInfo.nome` | string |
  | `kilometric` | `stradaInfo.chilometrica` | float |
  | `total_lanes` | `numeroCorsie` | int |
  | `direction` | `corsieInfo[].descrizione` | `[]string`, single element (array-shaped for compatibility with existing production metadata) |
  | `sensor_type` | static CSV lookup by station code (`resources/sensor-type-mapping.csv`), default `"induction_loop"` | string |
- **Origin**: `FAMAS-traffic-provinceBZ` (fixed, matches the legacy
  collector's provenance for continuity of existing records).

**Records**: one `DataMap` entry per data type per bucket, timestamp =
bucket time in Unix ms, period = 300s (fixed, matches the FAMAS bucket
size). A row is attributed to a lane's station only if
`corsia == lane.id - 1 AND direzione == lane.sensoDiMarcia`.

**Data types** (all registered at startup via `SyncDataTypes`; unit/
description/rtype below are the values already present in production BDP,
preserved rather than reinvented):

| Data type | Source | Unit | rtype |
|---|---|---|---|
| `total-transits` | `totaleVeicoli` | nr | - |
| `number-of-motorcycles` | class `1` | nr | - |
| `number-of-cars` | class `2` | nr | - |
| `number-of-cars-and-minivans-with-trailer` | class `3` | nr | - |
| `number-of-small-trucks-and-vans` | class `4` | nr | - |
| `number-of-medium-sized-trucks` | class `5` | nr | - |
| `number-of-big-trucks` | class `6` | nr | - |
| `number-of-articulated-trucks` | class `7` | nr | - |
| `number-of-articulated-lorries` | class `8` | nr | - |
| `number-of-busses` | class `9` | nr | - |
| `number-of-unclassified-vehicles` | class `10` | nr | - |
| `average-vehicle-speed` | `mediaArmonicaVelocita` | km/h | - |
| `headway` | `headwayMedioSecondi` | sec | - |
| `headway-variance` | `varianzaHeadwayMedioSecondi` | (none) | Average |
| `gap` | `gapMedioSecondi` | sec | - |
| `gap-variance` | `varianzaGapMedioSecondi` | (none) | Average |

The class code -> data type mapping is positional (`totaliPerClasseVeicolare`
key `N` -> the Nth entry in this table, 1-indexed, `0` being
`total-transits`), not name-based - FAMAS does not document class codes
beyond the (unintegrated) `SchemiDiClassificazione` endpoint.

## Polling model

No incremental cursor. Each collector run requests a fixed
`WINDOW_MINUTES` (default 30) ending "now", on a cron shorter than the
window (default every 15 min) so consecutive runs overlap. Correctness
relies on BDP write idempotency (station + data type + timestamp), not on
the collector tracking what it already sent.
