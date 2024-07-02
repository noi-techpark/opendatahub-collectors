#!/bin/bash

# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

#docker login
RELEASETIME=`date +%s`
(cd ../lib/ingress-mq; mvn clean install) \
&& mvn clean compile package \
&& docker build -t ghcr.io/noi-techpark/opendatahub-collectors/google-spreadsheet:0.0.0 . -f infrastructure/docker/Dockerfile \
&& docker image push ghcr.io/noi-techpark/opendatahub-collectors/google-spreadsheet:0.0.0 \
&& helm upgrade --namespace collector --install dc-spreadsheets-google-sta-echarging ../../helm/generic-collector --values infrastructure/helm/sta_echarging.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install dc-spreadsheets-google-centro-trevi ../../helm/generic-collector --values infrastructure/helm/centro_trevi.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install dc-spreadsheets-google-creative-industries ../../helm/generic-collector --values infrastructure/helm/creative_industries.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install dc-spreadsheets-google-umadumm ../../helm/generic-collector --values infrastructure/helm/umadumm.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install dc-spreadsheets-google-traffic-bluetooth ../../helm/generic-collector --values infrastructure/helm/traffic_bluetooth.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install dc-spreadsheets-google-parking-offstreet-mebo ../../helm/generic-collector --values infrastructure/helm/parking_offstreet_meranobolzano.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \