#!/bin/bash


# load .env, if needed
if [ -f .env ]; then
	set -a
	source .env
	set +a
else
	printf "create .env first and fill needed vars\n"
	printf "cp .env.example .env\n"
	exit 1
fi

# env vars
# VERSIONS_ENDPOINT=
# OPERATOR_ID=
# TOKEN_A=
# TOKEN_C=

###############
# version
###############
printf "get version...\n"
curl $VERSIONS_ENDPOINT -H "Authorization: Token $TOKEN_A" 

printf "\nget version done.\n"
