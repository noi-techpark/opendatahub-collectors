<!--
SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

### Local development and the IP filter
There is an IP filter on the a22 mqtt server.

To develop locally, you can establish an SSH tunnel to a server that has access like so:
```sh
ssh -L '*:61616:123.213.123.213:61616' docker01-test
```
test the connection with:
```sh
nc -zv 127.0.0.1 61616
```
Now `127.0.0.1:61616` port forwards to the remote address `123.213.123.213:61616`

When connecting, make sure to use a unique `CLIENTID`, because MQTT per spec disconnects any existing session with the same ID



