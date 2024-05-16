#!/bin/bash
#docker login ghcr.io (create token with read/write package permissions in github developer settings)
RELEASETIME=`date +%s`
(cd ../lib/ingress-mq; mvn clean install) \
&& mvn clean install \
&& docker build -t ghcr.io/noi-techpark/opendatahub-collectors/dc-meteorology-eurac:0.0.0 . -f infrastructure/docker/Dockerfile \
&& docker image push ghcr.io/noi-techpark/opendatahub-collectors/dc-meteorology-eurac:0.0.0 \
&& helm upgrade --namespace collector --install dc-meteorology-eurac ../../helm/generic-collector --values infrastructure/helm/values.yaml --set-string podAnnotations.releaseTime=$RELEASETIME