#!/bin/bash
# SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

PROJECT=boilerplate-manifest
PROVIDER1=boilerplate
PROVIDER2=manifest
ORIGIN=origin

BP_DIR="$(dirname "${BASH_SOURCE[0]}")"
TARGET_DIR=$BP_DIR/../$PROJECT

# Copy basic structure
mkdir $TARGET_DIR
for f in infrastructure src .env.example docker-compose.yml; do
    cp -r $BP_DIR/$f $TARGET_DIR/
done

# Setup golang module
(cd $TARGET_DIR/src; 
    go mod init opendatahub.com/tr-$PROJECT;
	go get github.com/noi-techpark/go-bdp-client;
	go get github.com/noi-techpark/opendatahub-go-sdk/ingest;
	go get github.com/noi-techpark/opendatahub-go-sdk/tel;
    go mod tidy;
)