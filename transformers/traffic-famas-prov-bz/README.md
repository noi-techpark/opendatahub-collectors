<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# traffic-famas-prov-bz

Transformer for FAMAS traffic sensor data (Province of Bolzano). Consumes
the raw payload published by the `dc-api-crawler-traffic-famas-prov-bz` collector
(config: `collectors/api-crawler/infrastructure/crawler-config/traffic-famas-prov-bz.silky.yaml`)
and writes it to BDP as `TrafficSensor` stations, one per lane, with
16 data types per lane (total transits, per-vehicle-class counts, average
speed, headway, gap - see `trafficDataTypes` in `src/main.go`).

## Testing

`cd src && go test ./...` runs the snapshot tests in `src/main_test.go`
against fixtures in `src/testdata/` (real captured FAMAS payloads). Like
CI: `docker run --rm $(docker build -q . -f infrastructure/docker/Dockerfile --target test)`
from this directory.
