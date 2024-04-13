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
  - path: swagger.json
    spec: openapi2
  - path: openapi.yaml
    spec: openapi3
    trimPrefix: /v1
    envPrefix: PET_STORE
    methodAlias:
      post: create
      put: update
  - path: schema.json
    spec: ndc
```

`trimPrefix`, `envPrefix` and `methodAlias` options are used to convert OpenAPI by [ndc-rest-schema](https://github.com/hasura/ndc-rest-schema#openapi).

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

### Authentication

The current version supports API key and Auth token authentication schemes. The configuration is inspired from `securitySchemes` [with env variables](https://github.com/hasura/ndc-rest-schema#authentication)

See [this example](rest/testdata/auth/schema.yaml) for more context.
