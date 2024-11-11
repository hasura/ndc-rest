# NDC REST Connector

NDC REST Connector allows you to quickly convert REST APIs to NDC schema and proxy requests from GraphQL Engine v3 to remote services.
The connector can automatically transform OpenAPI 2.0 and 3.0 definitions to NDC schema.

![REST connector](./assets/rest_connector.png)

> [!NOTE]
> REST connector is configuration-based and doesn't limit to the OpenAPI specs only. Use [OpenAPI Connector](https://hasura.io/docs/3.0/connectors/external-apis/open-api) if you want to take more controls of OpenAPI via code generation.

## Quick start

Start the connector server at http://localhost:8080 using the [JSON Placeholder](https://jsonplaceholder.typicode.com/) APIs.

```go
go run ./server serve --configuration ./connector/testdata/jsonplaceholder
```

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

The config of each element follows the [config schema](https://github.com/hasura/ndc-rest/ndc-rest-schema/blob/main/config.example.yaml) of `ndc-rest-schema`.

You can add many API documentation files into the same connector.

> [!IMPORTANT]
> Conflicted object and scalar types will be ignored. Only the type of the first file is kept in the schema.

### Supported specs

#### OpenAPI

REST connector supports both OpenAPI 2 and 3 specifications.

- `oas3`: OpenAPI 3.0/3.1.
- `oas2`: OpenAPI 2.0.

**Supported request types**

| Request Type | Query | Path | Body | Headers |
| ------------ | ----- | ---- | ---- | ------- |
| GET          | ✅    | ✅   | NA   | ✅      |
| POST         | ✅    | ✅   | ✅   | ✅      |
| DELETE       | ✅    | ✅   | ✅   | ✅      |
| PUT          | ✅    | ✅   | ✅   | ✅      |
| PATCH        | ✅    | ✅   | ✅   | ✅      |

#### Supported content types

- `application/json`
- `application/x-www-form-urlencoded`
- `application/octet-stream`
- `multipart/form-data`
- `text/*`
- Upload file content types, e.g.`image/*` from `base64` arguments.

#### REST schema

Enum: `ndc`

REST schema is the native configuration schema which other specs will be converted to behind the scene. The schema extends the NDC Specification with REST configuration and can be converted from other specs by the [NDC REST schema CLI](./ndc-rest-schema).

### Authentication

The current version supports API key and Auth token authentication schemes. The configuration is inspired from `securitySchemes` [with env variables](https://github.com/hasura/ndc-rest/ndc-rest-schema#authentication). The connector supports the following authentication strategies:

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

You can add JSON patches to extend API documentation files. REST connector supports `merge` and `json6902` strategies. JSON patches can be applied before or after the conversion from OpenAPI to REST schema configuration. It will be useful if you need to extend or fix some fields in the API documentation such as server URL.

See [the example](./ndc-rest-schema/command/testdata/patch) for more context.

## Distributed execution

Imagine that your backend have many server replications, or multiple applications with different credentials. You want to:

- Specify the server where the request will be executed to.
- Execute an operation to all servers.

For example, with below server settings, the connector will replicate existing operations with `Distributed` suffixes:

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
        "restOptions": {
          "type": {
            "type": "nullable",
            "underlying_type": {
              "name": "RestSingleOptions",
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
        "restOptions": {
          "type": {
            "type": "nullable",
            "underlying_type": {
              "name": "RestDistributedOptions",
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
    "RestDistributedOptions": {
      "description": "Distributed execution options for REST requests to multiple servers",
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
                "name": "RestServerId",
                "type": "named"
              },
              "type": "array"
            }
          }
        }
      }
    },
    "RestSingleOptions": {
      "description": "Execution options for REST requests to a single server",
      "fields": {
        "servers": {
          "description": "Specify remote servers to receive the request. If there are many server IDs the server is selected randomly",
          "type": {
            "type": "nullable",
            "underlying_type": {
              "element_type": {
                "name": "RestServerId",
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

`RestSingleOptions` object type is added to existing operations (findPets). API consumers can specify the server to be executed. If you want to execute all remote servers in sequence or parallel, `findPetsDistributed` function should be used.

## License

REST Connector is available under the [Apache License 2.0](./LICENSE).
