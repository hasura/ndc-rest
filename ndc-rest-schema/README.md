# NDC REST Schema

This module includes libraries and tools to convert other API schemas to Native Data Connector (NDC) schema, as well as extend the NDC spec with REST request information.

## Features

- Convert API documentation to NDC schema
  - OpenAPI [2.0](https://swagger.io/specification/v2/) (`oas2`)
  - OpenAPI [3.0](https://swagger.io/specification/v3)/[3.1](https://swagger.io/specification/) (`oas3`)
- Convert JSON to YAML. It's helpful to convert JSON schema

## Installation

## Build from source

**Prerequisites**

- Go 1.23+

**Install**

```go
go install github.com/hasura/ndc-rest/ndc-rest-schema@latest
```

## Quick start

```sh
Usage: ndc-rest-schema <command>

Flags:
  -h, --help                Show context-sensitive help.
      --log-level="info"    Log level.

Commands:
  convert --file=STRING
    Convert API spec to NDC schema. For example:

        ndc-rest-schema convert -f petstore.yaml --spec oas2 -o petstore.json

  json2yaml --file=STRING
    Convert JSON file to YAML. For example:

        ndc-rest-schema json2yaml -f petstore.json -o petstore.yaml

  version
    Print the CLI version.
```

Convert an OpenAPI v3 file to NDC schema with the `convert` command. The tool can accept either file path or URL. The output format can be in JSON or YAML, depending on the file extension:

```sh
ndc-rest-schema convert -f https://raw.githubusercontent.com/OAI/OpenAPI-Specification/main/examples/v3.0/petstore.yaml -o petstore.json --spec oas3
```

The `--spec` flag represents the input specification:

- `oas3` (`openapi3`): OpenAPI 3.0 and 3.1 (default)
- `oas2` (`openapi2`): OpenAPI 2.0

The output schema can extend from the NDC schema with REST information that will be used for the NDC REST connector. You can convert the pure NDC schema with `--pure` flag.

You also can use a config file to convert ([example](./config.example.yaml)). Arguments will override the config file if you use both the config file and other arguments.

```sh
ndc-rest-schema convert -c ./config.yaml
```

> [!NOTE]
> The tool will consider the path of the config file as the root directory. For example, if the config path is `./foo/bar/config.yaml`, the tool will look for relative patch files from `./foo/bar` folder. Extra arguments will take the execution location as the root directory.

## NDC REST configuration

### Request

The NDC REST configuration adds `request` information into `functions` and `procedures` so the connector can have more context to initiate HTTP requests to the remote REST service. The request schema is inspired by [OpenAPI 3 paths and operations](https://swagger.io/docs/specification/paths-and-operations/).

```yaml
- request:
    url: /pets/{petId}
    method: get
    type: rest
    headers:
      Foo: bar
    timeout: 30 # seconds, default 30s
    security:
      - api_key: []
```

The URL can be a relative path or an absolute URL. If the URL is relative, there must be a base URL in `settings`:

```yaml
settings:
  servers:
    - url: http://petstore.swagger.io/v1
```

`parameters` include the list of URL and query parameters so the connector can replace values from request arguments.

For procedures, the `body` argument is always treated as the request body. If there is a parameter that has the same name, the tool will rename it to `paramBody`.

### Settings

The `settings` object contains global configuration about servers, authentication, and other information.

- `servers`: list of servers that serve the API service.
  - `url`: the base URL of the API server.
  - `id`: the unique identity for the server. The array index will be used if empty. If the server ID is present, the variable name of the server URL will be `[prefix]_[server-id]_SERVER_URL`. This value can be parsed from `x-server-id` extension field (OAS 3.0).
  - `headers`, `timeout`, `securitySchemes`, `security`: same as below but take effect to the current server only.
- `headers`: default headers will be injected into all requests.
- `timeout`: default timeout for all requests
- `securitySchemes`: global configurations for authentication, follow the [security scheme](https://swagger.io/docs/specification/authentication/) of OpenAPI 3.
- `security`: default [authentication requirements](https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md#security-requirement-object) will be applied to all requests.

### Environment variable template

Environment variable template which is in `{{CONSTANT_CASE}}` or `{{CONSTANT_CASE:-some_default_value}}` format can be replaced with value in the runtime. The wrapper should be double-brackets to avoid mistaking an OpenAPI variable template.

### Full example

```yaml
settings:
  servers:
    - url: "{{PET_STORE_SERVER_URL:-https://petstore3.swagger.io/api/v3}}"
  timeout: 30
  headers:
    foo: bar
  securitySchemes:
    api_key:
      type: apiKey
      value: "{{PET_STORE_API_KEY}}"
      in: header
      name: api_key
    petstore_auth:
      type: oauth2
      flows:
        implicit:
          authorizationUrl: https://petstore3.swagger.io/oauth/authorize
          scopes:
            read:pets: read your pets
            write:pets: modify pets in your account
  security:
    - {}
    - petstore_auth:
        - write:pets
        - read:pets
  version: 1.0.18
collections: []
functions:
  findPetsByStatus:
    request:
      url: "/pet/findByStatus"
      method: get
      security:
        - petstore_auth:
            - write:pets
            - read:pets
    arguments:
      status:
        description: Status values that need to be considered for filter
        type:
          type: nullable
          underlying_type:
            name: String
            type: named
        rest:
          name: status
          in: query
          schema:
            type: string
            nullable: true
            enum:
              - available
              - pending
              - sold
    description: Finds Pets by status
    result_type:
      element_type:
        name: Pet
        type: named
      type: array
procedures:
  addPet:
    request:
      url: "/pet"
      method: post
      headers:
        Content-Type: application/json
      security:
        - petstore_auth:
            - write:pets
            - read:pets
    arguments:
      body:
        description: Request body of /pet
        type:
          name: Pet
          type: named
        rest:
          in: body
          schema:
            type: object
    description: Add a new pet to the store
    result_type:
      name: Pet
      type: named
```

## Supported specs

### OpenAPI

The tool can parse and convert OpenAPI documentation to NDC functions and procedures via HTTP methods:

- `GET` -> `Function`
- `POST`, `PUT`, `PATCH`, `DELETE` -> `Procedure`

#### Type conversion

- `boolean` -> `Boolean`
- `string` -> `String`
- `integer` -> `Int`
- `number` -> `Float`
- `object` -> Object types
- `anyOf`, `additionalProperties` and others -> `JSON`

> Because NDC schema doesn't support union types it's impossible to convert dynamic schema to a static type. The `JSON` scalar represents as a dynamic JSON field and doesn't support nested selection.

#### Naming convention

Schema type names are usually matched with the referenced name. Anonymous type names will be generated from the URL path in PascalCase.

If the `operationId` field exists in API operation, it will be used for functions or procedure names. Otherwise, the operation name will be generated from the URL path with camelCase format:

```sh
{http_method}{url_path_without_slash}

# GET     /users/{id} => getUsersId
# POST    /users      => postUsers
# DELETE  /users      => deleteUsers
```

You can also change the method alias with `--method-alias=KEY=VALUE;...` flag, for example: `--method-alias=post=create;put=update`.

If the URL path has a prefix such as `/api/v1/users`, you can trim that prefix with `--trim-prefix` flag.

#### Authentication

If the OpenAPI definition has authentication (or security), the tool converts them to `settings` object. The schema is similar to [OpenAPI 3.0 authentication](https://swagger.io/docs/specification/authentication/) with extra configuration fields.

**API Keys**

There is an extra `value` field with the environment variable template to be replaced in runtime. The name of the variable is generated from the security scheme key. For example:

```json
{
  "securitySchemes": {
    "api_key": {
      "type": "apiKey",
      "value": "{{API_KEY}}", // the constant case of api_key
      "in": "header",
      "name": "api_key"
    }
  }
}
```

> You can set the prefix for environment variables with `--env-prefix` flag.

**Auth Token**

This is the general authentication for [Basic](https://swagger.io/docs/specification/authentication/basic-authentication), [Bearer](https://swagger.io/docs/specification/authentication/bearer-authentication/) or any token with the scheme. The output credential will be the combination of `scheme` and `value`.

```json
{
  "securitySchemes": {
    "bearer_auth": {
      "type": "http",
      "scheme": "bearer",
      "value": "{{BEARER_AUTH_TOKEN}}", // the constant case of bearer_auth + _TOKEN suffix
      "header": "Authentication"
    }
  }
}
```

```
Authentication: Bearer {{BEARER_AUTH_TOKEN}}
```

The environment variable name of `value` field is the constant case of the security scheme key with `_TOKEN` suffix.

> You can set the prefix for environment variables with `--env-prefix` flag.

**OAuth 2.0**

See [OAuth 2.0](https://swagger.io/docs/specification/authentication/oauth2) section of OpenAPI 3.

**OpenID Connect Discovery**

See [OpenID Connect Discovery](https://swagger.io/docs/specification/authentication/oauth2) section of OpenAPI 3.

## Patch

You may want to modify the API document but don't want to edit the original file. That's possible with patch files. This tool supports the following specifications:

- `merge`: [RFC7396](https://tools.ietf.org/html/rfc7396) JSON merge patch.
- `json6902`: [RFC6902](https://datatracker.ietf.org/doc/html/rfc6902) JSON patch.

Patches can be applied before (`--patch-before`) and after (`--patch-after`) the conversion. The value accepts a list of paths, separated by commas. Each path can be a file, folder, or URL.
The pre-hook is useful for applying against raw documents such as OpenAPI, and the post-hook patches are applied against the output schema.

> [!NOTE]
> You must convert slashes in paths to `~1` or the RFC6902 JSON patch will confuse them with [JSON Pointer reference tokens](https://datatracker.ietf.org/doc/html/rfc6901).
>
> ```json
> [
>   {
>     "op": "remove",
>     "path": "/paths/~1notifications~1{notification_id}~1history"
>   }
> ]
> ```
