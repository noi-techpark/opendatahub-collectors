# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FROM golang:1.23.7-bookworm AS base

EXPOSE 8080

FROM base AS build-env
WORKDIR /app
COPY src/ .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

# BUILD published image
FROM alpine:3 AS build
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
CMD ["go", "test", "."]
