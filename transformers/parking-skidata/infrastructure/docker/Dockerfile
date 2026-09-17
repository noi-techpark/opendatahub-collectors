# SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

# There is no resources/ directory any more: the counting categories that used
# to be baked in as a CSV are collected into the raw data lake and read from
# there at boot, so the image carries no data of its own.

FROM golang:1.25-bookworm AS base

FROM base AS build-env
WORKDIR /app
COPY src/. .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

# BUILD published image
FROM alpine:latest AS build
WORKDIR /app
COPY --from=build-env /app/main .
ENTRYPOINT [ "./main"]

# LOCAL DEVELOPMENT
FROM base AS dev
WORKDIR /code
CMD ["go", "run", "."]

# TESTS
FROM base AS test
WORKDIR /code
COPY src/. .
RUN go mod download
CMD ["go", "test", "./..."]
