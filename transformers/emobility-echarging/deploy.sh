#!/bin/bash
#docker login ghcr.io
RELEASETIME=`date +%s`
(cd ../lib/rabbit-mongo-listener; mvn clean install) \
&& mvn clean compile package \
&& docker build -t ghcr.io/noi-techpark/opendatahub-collectors/emobility-echarging:0.1.0 . -f infrastructure/docker/Dockerfile \
&& docker image push ghcr.io/noi-techpark/opendatahub-collectors/emobility-echarging:0.1.0 \
&& helm upgrade --namespace collector --install tr-echarging-alperia ../../helm/generic-collector --values infrastructure/helm/alperia.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
&& helm upgrade --namespace collector --install tr-echarging-driwe ../../helm/generic-collector --values infrastructure/helm/driwe.yaml --set-string podAnnotations.releaseTime=$RELEASETIME \
#&& helm upgrade --namespace collector --install tr-echarging-route220 ../../helm/generic-collector --values infrastructure/helm/route220_values.yaml --set-string podAnnotations.releaseTime=$RELEASETIME