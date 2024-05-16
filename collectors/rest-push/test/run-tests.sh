#!/bin/sh

docker compose down
docker compose up --attach app
result=$?

docker compose down

exit $result