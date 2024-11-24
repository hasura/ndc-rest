# HTTP Connector

HTTP Connector allows you to quickly convert HTTP APIs to NDC schema and proxy requests from GraphQL Engine v3 to remote services.
The connector can automatically transform OpenAPI 2.0 and 3.0 definitions to NDC schema.

![HTTP connector](./assets/rest_connector.png)

> [!NOTE]
> HTTP connector is configuration-based HTTP engine and isn't limited to the OpenAPI specs only. Use [OpenAPI Connector](https://hasura.io/docs/3.0/connectors/external-apis/open-api) if you want to take more control of OpenAPI via code generation.

## Features

- [No code. Configuration based](#configuration).
- Composable API collections.
- [Supported many API specifications](#supported-specs).
- [Supported authentication](#authentication).
- [Supported headers forwarding](#header-forwarding).
- [Supported timeout and retry](#timeout-and-retry).
- Supported concurrency and [sending distributed requests](#distributed-execution) to multiple servers.

**Supported request types**

| Request Type | Query | Path | Body | Headers |
| ------------ | ----- | ---- | ---- | ------- |
| GET          | ✅    | ✅   | NA   | ✅      |
| POST         | ✅    | ✅   | ✅   | ✅      |
| DELETE       | ✅    | ✅   | ✅   | ✅      |
| PUT          | ✅    | ✅   | ✅   | ✅      |
| PATCH        | ✅    | ✅   | ✅   | ✅      |

**Supported content types**

| Content Type                      | Supported |
| --------------------------------- | --------- |
| application/json                  | ✅        |
| application/xml                   | ✅        |
| application/x-www-form-urlencoded | ✅        |
| multipart/form-data               | ✅        |
| application/octet-stream          | ✅ (\*)   |
| text/\*                           | ✅        |
| application/x-ndjson              | ✅        |
| image/\*                          | ✅ (\*)   |

\*: Upload file content types are converted to `base64` encoding.

## Quick start

Start the connector server at http://localhost:8080 using the [JSON Placeholder](https://jsonplaceholder.typicode.com/) APIs.

```go
go run ./server serve --configuration ./connector/testdata/jsonplaceholder
```

## Documentation

- [NDC HTTP schema](./ndc-http-schema)
- [Recipes](https://github.com/hasura/ndc-http-recipes/tree/main): You can find or request pre-built configuration recipes of popular API services here.

## Configuration

The connector reads `config.{json,yaml}` file in the configuration folder. The file contains information about the schema file path and its specification:

```yaml
files:
  - file: swagger.json
    spec: openapi2
  - file: openapi.yaml
    spec: openapi3
    trimPrefix: /v1
    envPrefix: PET_STORE
  - file: schema.json
    spec: ndc
```

The config of each element follows the [config schema](https://github.com/hasura/ndc-http/ndc-http-schema/blob/main/config.example.yaml) of `ndc-http-schema`.

You can add many API documentation files into the same connector.

> [!IMPORTANT]
> Conflicted object and scalar types will be ignored. Only the type of the first file is kept in the schema.

### Supported specs

#### OpenAPI

HTTP connector supports both OpenAPI 2 and 3 specifications.

- `oas3`: OpenAPI 3.0/3.1.
- `oas2`: OpenAPI 2.0.

#### HTTP Connector schema

Enum: `ndc`

HTTP schema is the native configuration schema which other specs will be converted to behind the scene. The schema extends the NDC Specification with HTTP configuration and can be converted from other specs by the [NDC HTTP schema CLI](./ndc-http-schema).

### Authentication

The current version supports API key and Auth token authentication schemes. The configuration is inspired from `securitySchemes` [with env variables](https://github.com/hasura/ndc-http/ndc-http-schema#authentication). The connector supports the following authentication strategies:

- API Key
- Bearer Auth
- Cookies
- OAuth 2.0

The configuration automatically generates environment variables for the api key and Bearer token.

For Cookie authentication and OAuth 2.0, you need to enable headers forwarding from the Hasura engine to the connector.

### Header Forwarding

Enable `forwardHeaders` in the configuration file.

```yaml
# ...
forwardHeaders:
  enabled: true
  argumentField: headers
```

And configure in the connector link metadata.

```yaml
kind: DataConnectorLink
version: v1
definition:
  name: my_api
  # ...
  argumentPresets:
    - argument: headers
      value:
        httpHeaders:
          forward:
            - Cookie
          additional: {}
```

See the configuration example in [Hasura docs](https://hasura.io/docs/3.0/recipes/business-logic/http-header-forwarding/#step-2-update-the-metadata-1).

### Timeout and retry

The global timeout and retry strategy can be configured in each file:

```yaml
files:
  - file: swagger.json
    spec: oas2
    timeout:
      value: 30
    retry:
      times:
        value: 1
      delay:
        # delay between each retry in milliseconds
        value: 500
      httpStatus: [429, 500, 502, 503]
```

### JSON Patch

You can add JSON patches to extend API documentation files. HTTP connector supports `merge` and `json6902` strategies. JSON patches can be applied before or after the conversion from OpenAPI to HTTP schema configuration. It will be useful if you need to extend or fix some fields in the API documentation such as server URL.

```yaml
files:
  - file: openapi.yaml
    spec: oas3
    patchBefore:
      - path: patch-before.yaml
        strategy: merge
    patchAfter:
      - path: patch-after.yaml
        strategy: json6902
```

See [the example](./ndc-http-schema/command/testdata/patch) for more context.

## Distributed execution

Imagine that your backend has many server replications or multiple applications with different credentials. You want to:

- Specify the server where the request will be executed.
- Execute an operation to all servers.

For example, with the following server settings, the connector will replicate existing operations with `Distributed` suffixes:

```yaml
settings:
  servers:
    - id: dog
      url: "http://localhost:3000"
      securitySchemes:
        api_key:
          type: apiKey
          value: "dog-secret"
          in: header
          name: api_key
    - id: cat
      url: "http://localhost:3001"
      securitySchemes:
        api_key:
          type: apiKey
          value: "cat-secret"
          in: header
          name: api_key
```

Add `distributed: true` to the config file:

```yaml
files:
  - file: schema.yaml
    spec: oas3
    distributed: true
```

<details>
<summary>The generated schema will be:</summary>

```json
{
  "functions": [
    {
      "arguments": {
        "httpOptions": {
          "type": {
            "type": "nullable",
            "underlying_type": {
              "name": "HttpSingleOptions",
              "type": "named"
            }
          }
        }
      },
      "name": "findPets",
      "result_type": {
        "element_type": {
          "name": "Pet",
          "type": "named"
        },
        "type": "array"
      }
    },
    {
      "arguments": {
        "httpOptions": {
          "type": {
            "type": "nullable",
            "underlying_type": {
              "name": "HttpDistributedOptions",
              "type": "named"
            }
          }
        }
      },
      "name": "findPetsDistributed",
      "result_type": {
        "name": "FindPetsDistributedResult",
        "type": "named"
      }
    }
  ],
  "object_types": {
    "HttpDistributedOptions": {
      "description": "Distributed execution options for HTTP requests to multiple servers",
      "fields": {
        "parallel": {
          "description": "Execute requests to remote servers in parallel",
          "type": {
            "type": "nullable",
            "underlying_type": {
              "name": "Boolean",
              "type": "named"
            }
          }
        },
        "servers": {
          "description": "Specify remote servers to receive the request",
          "type": {
            "type": "nullable",
            "underlying_type": {
              "element_type": {
                "name": "HttpServerId",
                "type": "named"
              },
              "type": "array"
            }
          }
        }
      }
    },
    "HttpSingleOptions": {
      "description": "Execution options for HTTP requests to a single server",
      "fields": {
        "servers": {
          "description": "Specify remote servers to receive the request. If there are many server IDs the server is selected randomly",
          "type": {
            "type": "nullable",
            "underlying_type": {
              "element_type": {
                "name": "HttpServerId",
                "type": "named"
              },
              "type": "array"
            }
          }
        }
      }
    }
  }
}
```

</details>

`HttpSingleOptions` object type is added to existing operations (findPets). API consumers can specify the server to be executed. If you want to execute all remote servers in sequence or parallel, `findPetsDistributed` function should be used.

## License

HTTP Connector is available under the [Apache License 2.0](./LICENSE).
