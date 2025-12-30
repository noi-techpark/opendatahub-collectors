<!--
SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

source for airport data:
https://github.com/davidmegginson/ourairports-data
Licensed under Unlicense (public domain)

## Some links for implementation details:

https://www.octallium.com/blog/2025/02/27/flight-schedules-from-ssim/
https://github.com/planarnetwork/ssim2gtfs/
https://pkg.go.dev/github.com/echa/code/iata
https://usermanual.wiki/Pdf/242320788SSIMManualMarch2011.1329255234.pdf

## Run converter:
``` cd ssim2gtfs
go run ./... --input testdata/sample.ssim --output gtfs.zip --agency SkyAlps --url 'https://skyalps.com' --timezone 'URC'
```

## Validate GTFS:
```
# first run converter as documented above
docker run --rm -v .:/work ghcr.io/mobilitydata/gtfs-validator:latest -i /work/gtfs.zip -o /work/validator_output
```