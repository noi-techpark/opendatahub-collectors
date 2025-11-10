<!--
SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# sftp-server data collector
This data collector provides a sftp server for a single user.  

The collector application watches the file system to detect when new files are transferred to the server.  

There is no persistance, the files are deleted as soon as they have been transferred to rabbitmq.

The SFTP setup is based on https://github.com/atmoz/sftp

## Instructions for providers
Write files to `~/upload/

## generate host keys
```sh
ssh-keygen -t ed25519 -fssh_host_ed25519_key < /dev/null
ssh-keygen -t rsa -b 4096 -f ssh_host_rsa_key < /dev/nul
```