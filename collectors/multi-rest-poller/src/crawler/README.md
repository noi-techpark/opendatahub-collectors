# ApiGorowler Documentation

## Introduction

**ApiGorowler** is a declarative, YAML-configured API crawler designed for complex and dynamic API data extraction. It allows developers to describe multi-step API interactions with support for nested operations, data transformations, and context-based processing.

The core functionality of ApiGorowler revolves around two main step types:

* `request`: to perform API calls,
* `foreach`: to iterate over arrays and dynamically create nested contexts.

Each step operates in its own **context**, allowing for precise manipulation and isolation of data. Contexts are pushed onto a stack, especially by `foreach` steps, enabling fine-grained control of nested operations. After execution, contexts can be merged into parent or ancestor contexts using declarative **merge rules**.

ApiGorowler also supports:

* Static iteration using `values` in `foreach`
* Response transformation via `jq` expressions
* Request templating with Go templates
* Global and request-level authentication and headers
* Multiple authentication mechanisms: OAuth2 (with password and client\_credentials flows), Bearer tokens, and Basic auth
* Streaming of top-level entities when operating on array-based root contexts

To simplify development, ApiGorowler includes a **configuration builder CLI tool**, written in Go, that enables real-time execution and inspection of the configuration. This tool helps developers debug and refine their manifests by visualizing intermediate steps.

---

## Features

* Declarative configuration using YAML
* Supports nested data traversal and merging
* Powerful context stack system for scoped operations
* Built-in support for `jq` and Go templates
* Multiple authentication types (OAuth2, Basic, Bearer)
* Config builder with live evaluation and inspection
* Streaming support for root-level arrays

---

## Context Example

The configuration

```yaml
rootContext: []
  
steps:
  - type: request
    name: Fetch Facilities
    request:
      url: https://www.foo-bar/GetFacilities
      method: GET
      headers:
        Accept: application/json
    resultTransformer: |
      [.Facilities[]
        | select(.ReceiptMerchant == "STA – Strutture Trasporto Alto Adige SpA Via dei Conciapelli, 60 39100  Bolzano UID: 00586190217")
      ]
    steps:
      - type: forEach
        path: .
        as: facility
        steps:
          - type: request
            name: Get Facility Free Places
            request:
              url: https://www.foo-bar/FacilityFreePlaces?FacilityID={{ .facility.FacilityId }}
              method: GET
              headers:
                Accept: application/json
            resultTransformer: '[.FreePlaces]'
            mergeOn: .FacilityDetails = $res

          - type: forEach
            path: .subFacilities
            as: sub
            steps:
              - type: request
                name: Get SubFacility Free Places
                request:
                  url: https://www.foo-bar/FacilityFreePlaces?FacilityID={{ .sub.FacilityId }}
                  method: GET
                  headers:
                    Accept: application/json
                resultTransformer: '[.FreePlaces]'
                mergeOn: .SubFacilityDetails = $res

              - type: forEach
                path: .locations
                as: loc
                steps:
                  - type: request
                    name: Get Location Details
                    request:
                      url: https://www.foo-bar/Locations/{{ .loc }}
                      method: GET
                      headers:
                        Accept: application/json
                    mergeWithContext:
                      name: sub
                      rule: ".locationDetails = (.locationDetails // {}) + {($res.id): $res}"
```

Generates a Context tree like

```
rootContext: []
│
└── Request: Fetch Facilities
    (result is filtered list of Facilities)
    │
    └── Foreach: facility in [.]
        (new context per facility)
        │
        ├── Request: Get Facility Free Places
        │   (adds .FacilityDetails to ancestor context via mergeOn)
        │
        └── Foreach: sub in .subFacilities
            (new context per sub-facility)
            │
            ├── Request: Get SubFacility Free Places
            │   (adds .SubFacilityDetails to ancestor context via mergeOn)
            │
            └── Foreach: loc in .locations
                (new context per location ID)
                │
                └── Request: Get Location Details
                    (merges $res into sub context under .locationDetails via mergeWithContext)
```

-----

## Configuration Structure

### Top-Level Fields

| Field         | Type                   | Description                                                    |
| ------------- | ---------------------- | -------------------------------------------------------------- |
| `rootContext` | `[]` or `{}`           | **Required.** Initial context for the crawler.                 |
| `auth`        | `AuthenticationStruct` | Optional. Global authentication configuration.                 |
| `headers`     | `map[string]string`    | Optional. Global headers.                                      |
| `stream`      | `boolean`              | Optional. Enable streaming; requires `rootContext` to be `[]`. |
| `steps`       | `Array<ForeachStep\|RequestStep>` | **Required.** List of crawler steps. |

---

### AuthenticationStruct

| Field          | Type   | Required When                                                |
| -------------- | ------ | ------------------------------------------------------------ |
| `type`         | string | Always. One of: `basic`, `bearer`, `oauth`                   |
| `token`        | string | If `type == bearer`                                          |
| `method`       | string | If `type == oauth`. One of: `password`, `client_credentials` |
| `tokenUrl`     | string | If `type == oauth`                                           |
| `clientId`     | string | If `type == oauth && method == client_credentials`           |
| `clientSecret` | string | If `type == oauth && method == client_credentials`           |
| `username`     | string | If `type == basic` or `type == oauth && method == password`  |
| `password`     | string | If `type == basic` or `type == oauth && method == password`  |

---

### ForeachStep

| Field               | Type                 | Description                                          |
| ------------------- | -------------------- | ---------------------------------------------------- |
| `type`              | string               | **Required.** Must be `foreach`                      |
| `name`              | string               | Optional name for the step                           |
| `path`              | jq expression        | **Required.** Path to the array to iterate over      |
| `as`                | string               | **Required.** Variable name for each item in context |
| `values`            | array<any>           | Optional. Static values to iterate over              |
| `steps`             | Array<ForeachStep\|RequestStep> | Optional. Nested steps |
| `mergeWithParentOn` | jq expression        | Optional. Rule for merging with parent context       |
| `mergeOn`           | jq expression        | Optional. Rule for merging with ancestor context     |
| `mergeWithContext`  | MergeWithContextRule | Optional. Advanced merging rule                      |

---

### MergeWithContextRule

| Field  | Type   | Description                |
| ------ | ------ | -------------------------- |
| `name` | string | **Required.** Name of rule |
| `rule` | string | **Required.** Merge logic  |

---

### RequestStep

| Field               | Type          | Description                           |
| ------------------- | ------------- | ------------------------------------- |
| `type`              | string        | **Required.** Must be `request`       |
| `name`              | string        | Optional step name                    |
| `request`           | RequestStruct | **Required.** Request configuration   |
| `resultTransformer` | jq expression | Optional transformation of the result |

---

### RequestStruct

| Field        | Type                 | Description                      |                           |
| ------------ | -------------------- | -------------------------------- | ------------------------- |
| `url`        | go-template string   | **Required.** Request URL        |                           |
| `method`     | string (`GET`        | `POST`)                          | **Required.** HTTP method |
| `headers`    | map\<string, string> | Optional headers                 |                           |
| `body`       | yaml struct          | Optional request body            |                           |
| `pagination` | PaginationStruct     | Optional pagination config       |                           |
| `auth`       | AuthenticationStruct | Optional override authentication |                           |

---

### PaginationStruct

| Field    | Type                          | Description                         |
| -------- | ----------------------------- | ----------------------------------- |
| `params` | array<PaginationParamsStruct> | **Required.** Pagination parameters |
| `stopOn` | array<PaginationStopsStruct>  | **Required.** Stop conditions       |

---

### PaginationParamsStruct

| Field       | Type   | Description                                                 |
| ----------- | ------ | ----------------------------------------------------------- |
| `name`      | string | **Required.** Parameter name                                |
| `location`  | string | **Required.** One of: `query`, `body`, `header`             |
| `type`      | string | **Required.** One of: `int`, `float`, `datetime`, `dynamic` |
| `format`    | string | Optional. Required if `type == datetime` (Go time format)   |
| `default`   | any    | Optional. Must match the `type`                             |
| `increment` | string | Optional. Increment step                                    |
| `source`    | string | Required if `type == dynamic`. e.g., `body:<jq-selector>`   |

---

### PaginationStopsStruct

| Field        | Type          | Description                                                         |
| ------------ | ------------- | ------------------------------------------------------------------- |
| `type`       | string        | **Required.** One of: `responseBody`, `requestParam`                |
| `expression` | jq expression | Required if `type == responseBody`                                  |
| `param`      | string        | Required if `type == requestParam`                                  |
| `compare`    | string        | Required if `type == requestParam`. One of: `lt`, `lte`, `eq`, etc. |
| `value`      | any           | Required if `type == requestParam`                                  |

---

## Stream Mode

When `stream: true` is enabled at the top-level, the crawler emits entities incrementally as it processes them. In this mode:

* `rootContext` must be an empty array (`[]`)
* Each `forEach` or `request` result is pushed to the output stream

---

## Configuration Builder

The CLI utility enables real-time execution of your manifest with step-by-step inspection. It helps:

* Validate configuration
* Execute each step and inspect intermediate results
* Debug jq and template expressions interactively

---

## Summary

ApiGorowler is a versatile tool for API data extraction, offering control over structure, transformation, and authentication. Its YAML-driven configuration and real-time testing make it a powerful choice for building custom data pipelines.

For best practices:

* Modularize `steps` using nested `forEach`
* Keep `auth` and `headers` global unless overrides are needed
* Use `mergeWithParentOn` and `mergeOn` thoughtfully to preserve context integrity

