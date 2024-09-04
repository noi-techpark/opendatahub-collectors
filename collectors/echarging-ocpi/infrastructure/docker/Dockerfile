FROM golang:1.22.3-bookworm as base

# install if docker host doesn't has libc6 installed
RUN apt-get update && apt-get install -y libc6
RUN apt-get clean
EXPOSE 8080

FROM base as build
WORKDIR app
COPY src/main main
CMD [ "./main"]

FROM base as debug
WORKDIR /code
CMD [ "go", "run", "."]
