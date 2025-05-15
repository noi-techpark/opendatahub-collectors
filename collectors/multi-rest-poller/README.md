<!--
SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Multi-REST-Pollor
A versatile data collector that pulls data from multiple REST endpoints with support for pagination, nested API calls, and various authentication methods. The service is designed to collect data from RESTful APIs and forward it to the ODH processing pipeline (or a MongoDB waiting to be processed).

## Overview
Multi-REST-Poller is a Go-based service that enables the polling of RESTful APIs with the following features:
- Configurable HTTP requests via YAML configuration
- Support for multiple authentication methods
- Automatic pagination handling
- Nested API calls with parameter extraction
- JSONPath-based data selection
- Integration with ODH infrastructure

# Architecture Overview
The service utilizes a flexible YAML-based configuration system for defining API calls and data extraction rules.

## Core Components

### Main Package
The main package coordinates the overall application flow:
- Initializes configuration from environment variables
- Sets up cron-based scheduling
- Configures the data collector
- Processes API results and forwards them the ODH

### Call Configuration
The call configuration system supports:
- Single REST API calls
- Multiple parallel REST API calls
- Nested calls with parameter extraction
- Various authentication methods

### HTTP Request Processing
The service handles HTTP requests with:
- Method specification (GET, POST, etc.)
- Header customization
- Authentication injection
- Response parsing

### Data Selection
Data is extracted from responses using:
- JSONPath selectors
- Type-specific extraction (JSON, string)
- Custom field selection for nested results

### Pagination
The pagination system supports:
- Multiple pagination strategies (query, header, body)
- Customization pagination parameters
- Automatic offset calculations
- Conditional pagination termination

### Authentication 
Authentication support includes:
- OAuth2 (Password and Client Credentials flows)
- Basic Auth
- Bearer Token

## Flow Diagram
cron schedule -> configuration loader -> http client -> data extractor -> data collector -> odh ingest

## Configuration

### Environment Variables
Configure service using the following environment variables:

|Variable                     |Required      |Default|Description               |
|:---------------------------:|:------------:|:-----:|:------------------------:|
|MQ_URI                       |Yes           |-      |RabbitMQ connection URI   |
|MQ_CLIENT                    |Yes           |-      |RabbitMQ client identifier|
|MQ_EXCHANGE                  |Yes           |-      |RabbitMQ exchange name    |
|LOGLEVEL                     |No            |INFO   |Logging level             |
|PROVIDER                     |Yes           |-      |Provider identifier       |
|CRON                         |Yes           |-      |Cron scherdule            |
|HTTP_CONFIG_PATH             |Yes           |-      |Path to http config yaml  |
|SERVICE_NAME                 |Yes           |-      |service name for telemetry|
|TELEMETRY_TRACE_GRPC_ENDPOINT|No            |-      |OpenTelemetry endpoint    |
|AUTH_STRATEGY                |No            |-      |auth strategy             |
|BASIC_AUTH_USERNAME          |Basic Auth    |-      |basic auth username       |
|BASIC_AUTH_PASSWORD          |Basic Auth    |-      |basic auth password       |
|AUTH_BEARER_TOKEN            |Bearer Auth   |-      |bearer token              |
|OAUTH_METHOD                 |OAuth2        |-      |OAuth2 flow method        |
|OAUTH_TOKEN_URL              |OAuth2        |-      |OAuth2 token endpoint     |
|OAUTH_CLIENT_ID              |OAuth2        |-      |OAuth2 client ID          |
|OAUTH_CLIENT_SECRET          |OAuth2        |-      |OAuth2 client secret      |
|OAUTH_USERNAME               |OAuth password|-      |OAuth2 username           |
|OAUTH_PASSWORD               |OAuth password|-      |OAuth2 password           |

### Http Call Configuration (YAML)
The service uses YAML configuration files to define HTTP requests:

#### Root Configuration
```
# Option 1: Single HTTP call
http_call:
  url: "https://api.example.com/data"
  method: "GET"
  headers:
    Content-Type: "application/json"
  data_selector: "$.data"
  data_selector_type: "json"

# Option 2: Multiple HTTP calls
http_calls:
  data_selector_type: "json"
  nested_calls:
    - url: "https://api.example.com/data"
      method: "GET"
      headers:
        Content-Type: "application/json"
      data_selector: "$.data"
      data_selector_type: "json"
      data_destination_field: "mainData"
```

#### Call Configuration Properties

|Property              |Type               |Description                                     |
|:--------------------:|:-----------------:|:----------------------------------------------:|
|url                   |string             |The API endpoint URL                            |
|method                |string             |HTTP method (GET, POST, etc)                    |
|headers               |map[string][string]|HTTP request headers                            |
|data_selector         |string             |JSONPath to extract specific data               |
|data_selector_type    |string             |Type of data selection ("json", "string")       |
|nested_calls          |[]CallConfig       |Configuration for subsequent API calls          |
|param_selector_type   |string             |Type of parameter selection                     |
|param_selectors       |[]string           |JSONPaths to extract parameters for nested calls|
|data_destination_field|string             |Field name to store nested call results         |
|pagination            |Pagination         |Pagination configuration                        |

#### Pagination Configuration
```
pagination:
  request_strategy: "query"  # header | query | body
  response_strategy: "body"  # header | body
  request_key: "page"        # query parameter name
  offset_builder:
    current_start: 0
    next_field: "$.meta.next_page"
    increment: 1
    next_type: "int"
    break_on_next_empty: true
```

|Property         |Type         |Description                                                     |
|:---------------:|:-----------:|:--------------------------------------------------------------:|
|request_strategy |string       |How to include pagination parameters ("header", "query", "body")|
|response_strategy|string       |How pagination data is returned ("header", "body")              |
|offset_builder   |OffsetBuilder|Configuration for offset calculation                            |
|request_key      |string       |Parameter name for the pagination requests                      |

#### Offset Builder Configuration
|Property           |Type  |Description                                        |
|:-----------------:|:----:|:-------------------------------------------------:|
|current_start      |int   |Initial offset value                               |
|next               |string|JSONPath to extract next offset from response      |
|increment          |int   |Value to increment offset by                       |
|next_type          |string|Type of next offset value ("int", "string")        |
|break_on_next_empty|bool  |Whether to stop pagination when next field is empty|

#### Nested Calls Configuration
```
nested_calls:
  - url: "https://api.example.com/details/%s"
    method: "GET"
    headers:
      Content-Type: "application/json"
    data_selector: "$.details"
    data_selector_type: "json"
    param_selectors:
      - "$.id"
    data_destination_field: "details"
```

### Authentication Configuration
The servce supports multiple authentication methods:

#### OAuth2
```
AUTH_STRATEGY=oauth2
OAUTH_METHOD=password  # or client_credentials
OAUTH_TOKEN_URL=https://auth.example.com/oauth/token
OAUTH_CLIENT_ID=your-client-id
OAUTH_CLIENT_SECRET=your-client-secret
OAUTH_USERNAME=your-username  # Only for password method
OAUTH_PASSWORD=your-password  # Only for password method
```

#### Basic Auth
```
AUTH_STRATEGY=basic
BASIC_AUTH_USERNAME=username
BASIC_AUTH_PASSWORD=password
```

#### Bearer Token
```
AUTH_STRATEGY=bearer
AUTH_BEARER_TOKEN=your-token
```

### Examples

##### Basic API Call
```
http_call:
  url: "https://api.example.com/data"
  method: "GET"
  headers:
    Accept: "application/json"
  data_selector: "$.items"
  data_selector_type: "json"
```

#### Paginated API Call
```
http_call:
  url: "https://api.example.com/data"
  method: "GET"
  headers:
    Accept: "application/json"
  data_selector: "$.items"
  data_selector_type: "json"
  pagination:
    request_strategy: "query"
    response_strategy: "body"
    request_key: "page"
    offset_builder:
      current_start: 1
      next_field: "$.meta.next_page"
      increment: 1
      next_type: "int"
      break_on_next_empty: true
```

#### Nested API Calls
```
http_call:
  url: "https://api.example.com/users"
  method: "GET"
  headers:
    Accept: "application/json"
  data_selector: "$.users"
  data_selector_type: "json"
  nested_calls:
    - url: "https://api.example.com/users/%s/details"
      method: "GET"
      headers:
        Accept: "application/json"
      data_selector: "$.details"
      data_selector_type: "json"
      param_selectors:
        - "$.id"
      data_destination_field: "user_details"
```

## Development

### Requirements
- Go version 1.23.7
- RabbitMQ
- Docker (optional)

### Running with Docker
The project includes Docker configuration for easy deployment:
```
# Start with Docker Compose
docker-compose up -d

# Build and run
docker build -f infrastructure/docker/Dockerfile -t multi-rest-poller .
docker run -d --env-file .env multi-rest-poller
```

### Setup
1. Clone repo
2. Copy .env.example to .env and configure accordingly
3. Create and/or modify YAML configuration
4. Run service

```
go run src/main.go
```

### Troubleshooting
#### Common Issues
1. #### Authentication Failures
    - Verify credentials are correctly set in environment variables
    - Check token expiration for OAuth2 and Bearer tokens
    - Confirm OAuh2 token URL is accessible

2. #### Data Selection Issues
    - Validate JSONPath expressions against actual API responses
    - Ensure data_selector_type matches response format
    - Check for structural changes in the API response

3. #### Pagination Problems
    - Confirm pagination parameters match API requiremens
    - Verify offset calculation logic works with the API
    - Test pagination limits and boundary conditions

### Logging
The service uses structural logging with configurable levels. Set LOGLEVEL=DEBUG for detailed diagnostics during troubleshooting.

### Monitoring
The service exposes telemetry data through OpenTelemetry, which can be configured using the TELEMETRY_TRACE_GRPC_ENDPOINT environment variable.

### License
SPDX-License-Identifier:AGPL-30-or-later
Copyright: 2025 [NOI Techpark](https://noi.bz.it/en)