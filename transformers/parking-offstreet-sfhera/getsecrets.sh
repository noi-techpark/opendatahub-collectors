#!/bin/bash

# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

echo Rabbit URL: `kubectl get secret -n collector rabbitmq-svcbind -o jsonpath='{.data.uri}' | base64 -d`
echo Mongo URL: `kubectl get secret -n collector mongodb-collector-svcbind -o jsonpath='{.data.uri}' | base64 -d`
echo Oauth client secret: `kubectl get secret -n collector oauth-collector -o jsonpath='{.data.clientSecret}' | base64 -d`
echo Oauth client id: `kubectl get secret -n collector oauth-collector -o jsonpath='{.data.clientId}' | base64 -d`
echo Oauth auth url: `kubectl get secret -n collector oauth-collector -o jsonpath='{.data.authorizationUri}' | base64 -d`
echo Oauth token url: `kubectl get secret -n collector oauth-collector -o jsonpath='{.data.tokenUri}' | base64 -d`