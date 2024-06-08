# NDC REST Connector

NDC REST Connector allows you to quickly convert REST APIs to NDC schema and proxy requests from GraphQL Engine v3 to remote services.
The connector can automatically transform OpenAPI 2.0 and 3.0 definitions to NDC schema.

## Quick start

Start the connector server at http://localhost:8080 using the [JSON Placeholder](https://jsonplaceholder.typicode.com/) APIs.

```go
go run . serve --configuration ./rest/testdata/jsonplaceholder
```

## How it works

![REST connector](./assets/rest_connector.png)

REST connector uses the [NDC REST extension](https://github.com/hasura/ndc-rest-schema#ndc-rest-schema-extension) that includes request information.
The connector has request context to transform the NDC request body to REST request and versa.

### Configuration

The connector reads `config.{json,yaml}` file in the configuration folder. The file contains information about the schema file path and its specification:

```yaml
files:
  - file: swagger.json
    spec: openapi2
  - file: openapi.yaml
    spec: openapi3
    trimPrefix: /v1
    envPrefix: PET_STORE
    methodAlias:
      post: create
      put: update
  - file: schema.json
    spec: ndc
```

The config of each element follows the [config schema](https://github.com/hasura/ndc-rest-schema/blob/main/config.example.yaml) of `ndc-rest-schema`.

> [!IMPORTANT]
> Conflicted object and scalar types will be ignored. Only the type of the first file is kept in the schema.

#### Supported specs

- `openapi3`: OpenAPI 3.0/3.1.
- `openapi2`: OpenAPI 2.0.
- `ndc`: NDC REST schema.

The connector can convert OpenAPI to REST NDC schema in runtime. However, it's more flexible and performance-wise to pre-convert them, for example, change better function or procedure names. The connector supports a convert command to do it.

```sh
ndc-rest convert -f ./rest/testdata/jsonplaceholder/swagger.json -o ./rest/testdata/jsonplaceholder/schema.json --spec openapi2
```

> The `convert` command is imported from the [NDC REST Schema](https://github.com/hasura/ndc-rest-schema#quick-start) CLI tool.

#### Supported content types

- `application/json`
- `application/x-www-form-urlencoded`
- `application/octet-stream`
- `multipart/form-data`
- `text/*`
- Upload file content types, e.g.`image/*` from `base64` arguments.

### Environment variable template

The connector can replaces `{{xxx}}` templates with environment variables. The converter automatically renders variables for API keys and tokens when converting OpenAPI documents. However, you can add your custom variables as well.

### Timeout and retry

The global timeout and retry strategy can be configured in the `settings` object.

```yaml
settings:
  timeout: 30
  retry:
    times: 2
    # delay between each retry in milliseconds
    delay: 1000
    httpStatus: [429, 500, 502, 503]
```

### Authentication

The current version supports API key and Auth token authentication schemes. The configuration is inspired from `securitySchemes` [with env variables](https://github.com/hasura/ndc-rest-schema#authentication)

See [this example](rest/testdata/auth/schema.yaml) for more context.

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

`RestSingleOptions` object type is added to existing operations (findPets). API consumers can specify the server to be executed. If you want to execute all remote servers in sequence or parallel, `findPetsDistributed` function should be used.
