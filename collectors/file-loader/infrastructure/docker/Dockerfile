# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FROM golang:1.23-bookworm as base

FROM base as build
WORKDIR app
COPY src/main main
CMD [ "./main"]

FROM base as dev
WORKDIR /code

CMD ["go", "run", "main.go"]
