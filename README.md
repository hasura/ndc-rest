# NDC REST Connector

NDC REST Connector allows you to quickly convert REST APIs to NDC schema and proxy requests from GraphQL Engine v3 to remote services.
The connector can automatically transform OpenAPI 2.0 and 3.0 definitions to NDC schema.

## Quick start

Start the connector server at http://localhost:8080 using the [JSON Placeholder](https://jsonplaceholder.typicode.com/) APIs.

```go
go run ./server serve --configuration ./connector/testdata/jsonplaceholder
```

## How it works

![REST connector](./assets/rest_connector.png)

REST connector uses the [NDC REST extension](https://github.com/hasura/ndc-rest/ndc-rest-schema#ndc-rest-schema-extension) that includes request information.
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

The config of each element follows the [config schema](https://github.com/hasura/ndc-rest/ndc-rest-schema/blob/main/config.example.yaml) of `ndc-rest-schema`.

You can add many OpenAPI files into the same connector.

> [!IMPORTANT]
> Conflicted object and scalar types will be ignored. Only the type of the first file is kept in the schema.

#### Supported specs

- `openapi3`: OpenAPI 3.0/3.1.
- `openapi2`: OpenAPI 2.0.
- `ndc`: NDC REST schema.

The connector can convert OpenAPI to REST NDC schema in runtime. However, it's more flexible and performance-wise to pre-convert them, for example, change better function or procedure names.

```sh
go install github.com/hasura/ndc-rest/ndc-rest-schema@latest
ndc-rest-schema convert -f ./rest/testdata/jsonplaceholder/swagger.json -o ./rest/testdata/jsonplaceholder/schema.json --spec openapi2
```

#### Supported content types

- `application/json`
- `application/x-www-form-urlencoded`
- `application/octet-stream`
- `multipart/form-data`
- `text/*`
- Upload file content types, e.g.`image/*` from `base64` arguments.

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

The generated schema will be:

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
