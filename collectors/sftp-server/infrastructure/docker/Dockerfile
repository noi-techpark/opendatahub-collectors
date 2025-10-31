# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FROM alpine:latest AS sftp
# based on https://github.com/atmoz/sftp
# Steps done in one RUN layer:
# - Install upgrades and new packages
# - OpenSSH needs /var/run/sshd to run
# - Remove generic host keys, entrypoint generates unique keys
RUN apk update && apk upgrade && \
    apk add bash shadow openssh-server-pam openssh-sftp-server && \
    rm -rf /var/cache/apk/* && \
    ln -s /usr/sbin/sshd.pam /usr/sbin/sshd && \
    mkdir -p /var/run/sshd && \
    rm -f /etc/ssh/ssh_host_*key*
COPY infrastructure/sftp.sh /
RUN chmod u+x /sftp.sh
EXPOSE 22
ENTRYPOINT ["/bin/sh", "-c", "/sftp.sh"]

FROM sftp AS golang
RUN apk add --no-cache go

# LOCAL DEVELOPMENT
FROM golang AS dev
WORKDIR /code
ENTRYPOINT ["/bin/sh", "-c", "/sftp.sh && go run ."]

# Build published executable
FROM golang AS build-env
WORKDIR /app
COPY src/* .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o main

# BUILD published image
FROM sftp AS build
WORKDIR /app
COPY --from=build-env /app/main .
ENTRYPOINT ["/bin/sh", "-c", "/sftp.sh && /app/main"]
