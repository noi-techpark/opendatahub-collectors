#!/bin/bash

# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

#docker login
RELEASETIME=`date +%s`
(cd ../lib/ingress-mq; mvn clean install) \
&& mvn clean install \
&& docker build -t ghcr.io/noi-techpark/opendatahub-collectors/parking-offstreet-bolzano:0.0.0 . -f infrastructure/docker/Dockerfile \
&& docker image push ghcr.io/noi-techpark/opendatahub-collectors/parking-offstreet-bolzano:0.0.0 \
&& helm upgrade --namespace collector --install dc-parking-offstreet-bolzano ../../helm/generic-collector --values infrastructure/helm/values.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \