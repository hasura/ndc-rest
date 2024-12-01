# Distributed Execution

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
