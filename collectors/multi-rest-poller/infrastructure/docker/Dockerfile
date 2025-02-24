# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FROM golang:1.23-bookworm AS base

EXPOSE 8080

FROM base AS build-env
WORKDIR /app
COPY src/ .
COPY infrastructure/ ./infrastructure
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

# BUILD published image
FROM alpine:3 AS build
ARG CALLS_CONFIG_FILE

WORKDIR /app

# Dynamically create the directory before copying the file
RUN mkdir -p "$(dirname ${CALLS_CONFIG_FILE})"

COPY --from=build-env /app/main .
COPY --from=build-env ${CALLS_CONFIG_FILE} .
ENTRYPOINT [ "./main"]

# LOCAL DEVELOPMENT
FROM base AS dev
WORKDIR /code
CMD ["go", "run", "."]

# TESTS
FROM base AS test
WORKDIR /code
CMD ["go", "test", "."]
