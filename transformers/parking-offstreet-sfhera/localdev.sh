#!/bin/bash

# Setup port forwards to services in cluster
# They keep running in the background as long as the shell is alive
# use 'killall kubectl' to terminate them
kubectl port-forward -n core svc/bdp-core 8080 --address 0.0.0.0 &
kubectl port-forward -n core svc/rabbitmq-headless 5672 --address 0.0.0.0 &
kubectl port-forward -n core svc/rabbitmq-headless 15672 &
kubectl port-forward -n core svc/mongodb-headless 27017 --address 0.0.0.0 &