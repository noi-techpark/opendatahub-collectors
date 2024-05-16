#!/bin/bash
echo Rabbit URL: `kubectl get secret -n collector rabbitmq-svcbind -o jsonpath='{.data.uri}' | base64 -d`
echo Mongo URL: `kubectl get secret -n collector mongodb-collector-svcbind -o jsonpath='{.data.uri}' | base64 -d`
echo Oauth client secret: `kubectl get secret -n collector oauth-collector -o jsonpath='{.data.clientSecret}' | base64 -d`