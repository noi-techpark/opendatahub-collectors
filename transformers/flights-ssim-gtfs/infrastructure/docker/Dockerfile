# SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FROM golang:1.25-bookworm AS base

FROM base AS build-env
WORKDIR /app
COPY src/. .
COPY ssimparser /ssimparser 
COPY ssim2gtfs /ssim2gtfs
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

# BUILD published image
FROM alpine:latest AS build
WORKDIR /app
RUN apk add --no-cache tzdata
COPY --from=build-env /app/main .
COPY ssim2gtfs/airports.csv .
ENTRYPOINT [ "./main"]

# LOCAL DEVELOPMENT
FROM base AS dev
WORKDIR /code
CMD ["go", "run", "main.go"]

# TESTS
FROM base AS test
WORKDIR /code
CMD find . -name go.mod -execdir go test ./... \;