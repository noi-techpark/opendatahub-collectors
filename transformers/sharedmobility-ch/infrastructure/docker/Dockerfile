# SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FROM golang:1.23.7-bookworm as base

FROM base as build-env
WORKDIR /app
COPY src/. .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

# BUILD published image
FROM alpine:latest as build
WORKDIR /app
COPY --from=build-env /app/main .
ENTRYPOINT [ "./main"]

# LOCAL DEVELOPMENT
FROM base as dev
WORKDIR /code
CMD ["go", "run", "."]

