FROM golang:1.22.2-bookworm as base

FROM base as build
WORKDIR app
COPY src/main main
CMD [ "./main"]

FROM base as dev
WORKDIR /code

CMD ["go", "run", "main.go"]
