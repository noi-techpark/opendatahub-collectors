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
Authentication uses a pre-shared "Token C", which is in effect just a static password that is exchanged via a separate channel.  
The OCPI credentials exchange is currently not implemented

