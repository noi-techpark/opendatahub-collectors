Validation rules:

## Toplevel
- rootContext: required. [] or {}
- auth: optional. AuthenticationStruct
- headers: optional. map[string]string
- stream: optional. true|false. true requires rootContext = []
- steps: required. Array<ForeachStep|RequestStep>

## AuthenticationStruct
- type: required. basic | bearer | oauth
- token: optional. string. required if type == bearer
- method: optional. password | client_credentials. required if type == oauth
- tokenUrl: optional. string. required if type == oauth
- clientId: optional. string. required if type == oauth && method == client_credentials
- clientSecret: optional. string. required if type == oauth && method == client_credentials
- username: optional. string. required if (type == oauth && method == password) || type == basic
- password: optional. string. required if (type == oauth && method == password) || type == basic

## ForeachStep
- type: required. foreach
- name: optional. string
- path: required. jq expression
- as: required. string
- values: optional. array<any>
- steps: optional. Array<ForeachStep|RequestStep>
- mergeWithParentOn: optional. jq expression
- mergeOn: optional. jq expression
- mergeWithContext: optional. MergeWithContextRule

## MergeWithContextRule
- name: required. string
- rule: required. string

## RequestStep
- type: required. request
- name: optional. string
- request: required. RequestStruct
- resultTransformer: optional. jq expression

## RequestStruct
- url: required. go-templat string
- method: required. GET|POST
- headers: optional. map[string]string
- body: optional. yaml struct
- pagination: optional. PagniationStruct
- auth: optional. AuthenticationStruct

## PagniationStruct
- params: required. Array<PaginationParamsStruct>
- stopOn: required. Array<PaginationStopsStruct>

## PaginationParamsStruct
name: required. string
location: rquired. "query", "body", "header"
type: required. "int", "float", "datetime", "dynamic"
format: optional. go date template. required if type == datetime
default: optional. value of type type. if type == datetime defaults must follow "format"
increment: optional. string
source: optional. "body:qj-selector" or "header:q-jselector". requried if type == dynamic

## PaginationStopsStruct
type: required. "responseBody", "requestParam"
expression: optional. jq expression. required if type == responseBody
param: optional. string. required if type == requestParam
compare: optional. "lt", "lte", "eq", "gt", "gte". required if type == requestParam
value: optional. any  required if type == requestParam