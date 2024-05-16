#!/bin/bash
#docker login ghcr.io
RELEASETIME=`date +%s`
(cd ../lib/rabbit-mongo-listener; mvn clean install) \
&& mvn clean compile package \
&& docker build -t ghcr.io/noi-techpark/opendatahub-collectors/tr-google-spreadsheet:0.1.0 . -f infrastructure/docker/Dockerfile \
&& docker image push ghcr.io/noi-techpark/opendatahub-collectors/tr-google-spreadsheet:0.1.0 \
&& helm upgrade --namespace collector --install tr-spreadsheet-google-sta-echarging ../../helm/generic-collector --values infrastructure/helm/sta_echarging.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install tr-spreadsheet-google-centro-trevi ../../helm/generic-collector --values infrastructure/helm/centro_trevi.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install tr-spreadsheet-google-creative-industries ../../helm/generic-collector --values infrastructure/helm/creative_industries.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install tr-spreadsheet-google-umadumm ../../helm/generic-collector --values infrastructure/helm/umadumm.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \