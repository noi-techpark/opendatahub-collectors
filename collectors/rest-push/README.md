<!--
SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Inbound REST API
Generic API to push data to the Open Data Hub via REST

## Endpoint spec

| HTTP method | path | |
|--|--|--|
| POST | `/push/<provider>/<dataset>` | Push data to the Open Data Hub|
| GET | `/health` | Health check |
| GET | `/apispec` | Openapi3 spec (yaml format) |

Refer to the [openapi spec](src/openapi3.yaml) for more details

In practice, you will be given credentials and URL path by the Open Data Hub team, and just push your data as the request body

## Authentication and Authorization
Authentication is done via Keycloak Oauth2  
Authorization on a path level is done via Keycloak UMA

Get an access token from Keycloak and pass it as `Authorization: Bearer` header

In practice this means using the client_credentials flow with client_id and client_secret

To log in an actual user (not a client in the OAuth sense), you will need an intermediate webclient like the Open Data Hub databrowser, as this API does not implement the Authorization flow

## Setting up a Keycloak client:
Create a new client with authorization enabled.  
Disable all authentication mechanisms, as users will not login on this client directly

Go to the Authorization tab  
Delete the default resources/policies/permissions 

Create a resource with the URL and name format `/provider/dataset`  (plug in your own provider and dataset IDs)
Create a policy to some user, client or role you have credentials to  
Create a permission linking the scope, resource and policy

# Testing
`test/run-tests.sh` runs the tests in a container, together with local keycloak and rabbitmq instances