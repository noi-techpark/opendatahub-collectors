<!--
SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

## OCPI endpoint
Data collector partly implementing a OCPI eMSP (mobility service provider) server.

Echarging providers like Neogy can push status updates to this endpoint

See [the OCPI spec document](./documentation/OCPI-2.2.pdf) for details.  
Up to date documents here: https://github.com/ocpi/ocpi

### Supported methods
At time of writing, only the `locations/evse` path is implemented, but additional endpoints should be fairly trivial to add once needed.

### Authentication
The supplier will give you a Token A and a versions URL 

If the token A is not already base64 encoded, do so with
```sh
echo -n '<Token A>' | base64
```
Query the versions URL to get the credentials endpoint:
```sh
curl -H 'Authorization: Token <Token A>' https://demo.eu-neogy.charge.ampeco.tech/ocpi/versions

# responds with url to actual version endpoint
{"status_code":1000,"status_message":"Success","timestamp":"2026-04-17T08:41:30Z","data":[{"version":"2.2","url":"https://demo.eu-neogy.charge.ampeco.tech/ocpi/2.2"}]}

# now query the version
curl -H 'Authorization: Token <Token A>' https://demo.eu-neogy.charge.ampeco.tech/ocpi/2.2

# you get the endpoints, most importantly credentials
{"status_code":1000,"status_message":"Success","timestamp":"2026-04-17T08:45:16Z","data":{"version":"2.2","endpoints":[{"identifier":"sessions","role":"SENDER","url":"https://demo.eu-neogy.charge.ampeco.tech/ocpi/2.2/sender/sessions"},{"identifier":"cdrs","role":"SENDER","url":"https://demo.eu-neogy.charge.ampeco.tech/ocpi/2.2/sender/cdrs"},{"identifier":"locations","role":"SENDER","url":"https://demo.eu-neogy.charge.ampeco.tech/ocpi/2.2/sender/locations"},{"identifier":"credentials","role":"RECEIVER","url":"https://demo.eu-neogy.charge.ampeco.tech/ocpi/2.2/credentials"},{"identifier":"tariffs","role":"SENDER","url":"https://demo.eu-neogy.charge.ampeco.tech/ocpi/2.2/sender/tariffs"}]}}

```

Once obtained the credentials endpoint, you must send it your `Token B`, a randomly generated base64 encoded string that you will use to authenticate incoming (from our perspective) requests.  
This will go into env variable `OCPI_TOKENS`  
You also send them the URL your service responds under. They will likely verify this is reachable in the loop so make sure it exists.  

```sh
curl -X POST https://demo.eu-neogy.charge.ampeco.tech/ocpi/2.2/credentials \
  -H "Authorization: Token <Token A>" \
  -H "Content-Type: application/json" \
  -d '{
    "token": "<Token B>",
    "url": "https://neogy-ampeco.ocpi.io.dev.testingmachine.eu/ocpi/emsp/versions",
    "roles": [{"role": "EMSP", "party_id": "OpenDataHub", "country_code": "IT", "business_details": {"name": "Open Data Hub"}}]
  }'

```
The request will return with a `Token C`, which is to be used as env variable `PULL_TOKEN`


