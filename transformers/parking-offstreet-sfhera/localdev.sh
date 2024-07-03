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
# kubectl port-forward -n core svc/mongodb-headless 27017 --address 0.0.0.0 &

# Extract connection strings from secrets
RABBIT_URI=`kubectl get secret -n core rabbitmq-svcbind -o jsonpath='{.data.uri}' | base64 -d`
MONGO_URI=`kubectl get secret -n core mongodb-collector-svcbind -o jsonpath='{.data.uri}' | base64 -d`
# The +srv type connection string requires a TXT DNS record
# Since we don't have access to the cluster DNS here, use a regular direct connection string
MONGO_URI="${MONGO_URI/mongodb+srv/mongodb}"

# Write connection string to .env file
echo >> .env
sed -i '/MQ_LISTEN_URI=/d' .env
echo "MQ_LISTEN_URI=$RABBIT_URI" >> .env
sed -i '/MONGO_URI=/d' .env
echo "MONGO_URI=$MONGO_URI" >> .env