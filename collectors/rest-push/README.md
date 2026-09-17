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

## Query parameters

Query parameters are stored as **top-level fields of the raw document**, next to `rawdata`.

```
POST /push/enrichment/parking?key=skidata|0404467_0

  → { "provider": "enrichment/parking", "timestamp": "…",
      "key": "skidata|0404467_0",
      "content_type": "application/json",
      "rawdata": "<base64 of the body>" }
```

They go at the root, not nested, because that is the only place they can be indexed and grouped
on: the raw data bridge's compacted view (`GET /latest/{db}/{collection}?key=…`) groups on a
root-level field, and nothing inside `rawdata` is reachable once the body is stored as a string
or as binary. This is what lets a consumer keep a keyed reference table in sync.

- A parameter sent once is stored as a scalar. **Send a group key once** — a repeated parameter
  is stored as an array, which is faithful but not groupable.
- Names the pipeline owns are rejected with `400`: `provider`, `timestamp`, `bsontimestamp`,
  `rawdata`, `raw_ref`, `content_type`, `id`, `_id`. `provider` selects the target database and
  collection and `timestamp` becomes `bsontimestamp`, so a caller setting them would redirect
  or corrupt the write.

The request's `Content-Type` is stored as `content_type`. `rawdata` is unchanged — still the
body, base64-encoded — so existing consumers are unaffected.

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