#!/bin/bash
#docker login ghcr.io
RELEASETIME=`date +%s`
(cd ../lib/rabbit-mongo-listener; mvn clean install) \
&& mvn clean compile package \
&& docker build -t ghcr.io/noi-techpark/opendatahub-collectors/tr-parking-offstreet-meranobolzano:0.1.0 . -f infrastructure/docker/Dockerfile \
&& docker image push ghcr.io/noi-techpark/opendatahub-collectors/tr-parking-offstreet-meranobolzano:0.1.0 \
&& helm upgrade --namespace collector --install tr-parking-offstreet-meranobolzano ../../helm/generic-collector --values infrastructure/helm/values.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \