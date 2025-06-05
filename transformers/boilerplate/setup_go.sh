#!/bin/bash
# SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
# 

echo "This wizard will set up the boilerplate for a new golang transformer"
read -p "Project name. This will determine root folder, cicd and k8s service e.g. 'parking-valgardena': " PROJECT
read -p "First part of provider tuple: " PROVIDER1
read -p "Second part of provider tuple: " PROVIDER2
read -p "Origin: " ORIGIN

read -p "Are you sure these are correct (y/n)? " choice 
case "$choice" in 
  y|Y ) echo "ok, proceeding...";;
  * ) exit 1;;
esac

export PROJECT
export PROVIDER1
export PROVIDER2
export ORIGIN

BP_DIR="$(dirname "${BASH_SOURCE[0]}")/go"
TARGET_DIR=$BP_DIR/../../$PROJECT

if [ -d "$TARGET_DIR" ]; then
    echo Target folder already exists. aborting...
    exit 1
fi

function template {
    # copy file while substituting $ENV variables
    cat $1 | envsubst '$PROJECT $PROVIDER1 $PROVIDER2 $ORIGIN' > $2
}

mkdir -p $TARGET_DIR/infrastructure/docker
cp $BP_DIR/infrastructure/docker/Dockerfile $TARGET_DIR/infrastructure/docker/Dockerfile
cp $BP_DIR/infrastructure/docker-compose.build.yml $TARGET_DIR/infrastructure/docker-compose.build.yml
cp -r $BP_DIR/src $TARGET_DIR/
cp $BP_DIR/docker-compose.yml $TARGET_DIR/docker-compose.yml

mkdir -p $TARGET_DIR/infrastructure/helm
template $BP_DIR/infrastructure/helm/values.yaml $TARGET_DIR/infrastructure/helm/$ORIGIN.yaml

template $BP_DIR/cicd.yml $TARGET_DIR/../../.github/workflows/tr-$PROJECT.yml
template $BP_DIR/.env.example $TARGET_DIR/.env.example

# Setup golang module
(cd $TARGET_DIR/src; 
    go mod init opendatahub.com/tr-$PROJECT;
	go get github.com/noi-techpark/go-bdp-client;
	go get github.com/noi-techpark/opendatahub-go-sdk/ingest;
	go get github.com/noi-techpark/opendatahub-go-sdk/tel;
    go mod tidy;
)

echo "All setup!"