<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Feratel webcams data collector
Polls the Feratel XML webcam feed on a cron schedule, once per configured language, and posts all responses as one raw record to rabbitmq.

The raw data is a JSON object mapping each language to the unmodified XML response:
```json
{"de": "<?xml ...>", "it": "<?xml ...>", "en": "<?xml ...>"}
```
If any language request fails, nothing is published for that run.

For documentation on configuration, refer to the `.env.example` file
