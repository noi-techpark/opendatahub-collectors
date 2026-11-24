#!/bin/bash

# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

# Setup port forwards to services in cluster
# They keep running in the background as long as the shell is alive
# use 'killall kubectl' to terminate them
# kubectl port-forward -n core svc/bdp-core 8080 --address 0.0.0.0 &
# kubectl port-forward -n core svc/rabbitmq-headless 5672 --address 0.0.0.0 &
# kubectl port-forward -n core svc/rabbitmq-headless 15672 &
# kubectl port-forward -n core svc/raw-data-bridge 2000 &

# Extract the connection string from the secret
RABBIT_URI=`kubectl get secret -n core rabbitmq-svcbind -o jsonpath='{.data.uri}' | base64 -d`

# Write connection string to .env file
echo >> .env
sed -i '/MQ_URI=/d' .env
echo "MQ_URI=$RABBIT_URI" >> .env
