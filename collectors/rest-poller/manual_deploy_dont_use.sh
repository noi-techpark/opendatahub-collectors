#!/bin/bash
#docker login
RELEASETIME=`date +%s`
(cd ../lib/ingress-mq; mvn clean install) \
&& mvn clean compile package \
&& docker build -t ghcr.io/noi-techpark/opendatahub-collectors/rest-poller:0.2.0 . \
-f infrastructure/docker/Dockerfile  \
--label "org.opencontainers.image.source=https://github.com/noi-techpark/opendatahub-collectors"  \
&& docker image push ghcr.io/noi-techpark/opendatahub-collectors/rest-poller:0.2.0 \
&& helm upgrade --namespace collector --install dc-echarging-alperia ../../helm/generic-collector --values infrastructure/helm/alperia.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install dc-echarging-driwe ../../helm/generic-collector --values infrastructure/helm/driwe.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install dc-echarging-route220 ../../helm/generic-collector --values infrastructure/helm/route220.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install dc-parking-offstreet-merano ../../helm/generic-collector --values infrastructure/helm/parking-offstreet-merano.yaml --set-string podAnnotations.releaseTime=$RELEASETIME