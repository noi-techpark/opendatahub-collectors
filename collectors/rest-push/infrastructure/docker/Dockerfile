FROM golang:1.22.2-bookworm as base

EXPOSE 8080

FROM base as build-env
WORKDIR /app
COPY src/* .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

# BUILD published image
FROM alpine:latest as build
WORKDIR /app
COPY --from=build-env /app/main .
COPY --from=build-env /app/openapi3.yaml.tmpl .
ENTRYPOINT [ "./main"]

# LOCAL DEVELOPMENT
FROM base as dev
WORKDIR /code
# Air for auto rebuild on changes
RUN go install github.com/cosmtrek/air@latest
RUN air init
CMD ["air"]

# TESTS
FROM base as test
WORKDIR /code
CMD ["go", "test", "."]
