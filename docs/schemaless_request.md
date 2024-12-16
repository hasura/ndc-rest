# Schemaless Requests

## How it work

The connector can be used as an HTTP proxy. You can send schemaless HTTP requests through the `sendHttpRequest` operation.

```graphql
mutation SendRawRequest($additionalHeaders: JSON) {
  sendHttpRequest(
    body: { id: 101, title: "Hello world", userId: 10, body: "A test post" }
    method: "post"
    url: "https://jsonplaceholder.typicode.com/posts"
    additionalHeaders: $additionalHeaders
  )
}

# Variables
# {
#   "additionalHeaders": {
#       "X-Test-Header": "bar"
#     }
# }

# Response
# {
#   "data": {
#     "sendHttpRequest": {
#       "body": "A test post",
#       "id": 101,
#       "title": "Hello world",
#       "userId": 10
#     }
#   }
# }
```

## Options

### Content-Type

The request body is encoded with `application/json` content type by default. If you add the `Content-Type` header in the `additionalHeaders` argument the connector will try to convert the request body object to the corresponding format:

- `application/json`
- `application/xml`
- `application/x-www-form-urlencoded`
- `multipart/form-data`

```graphql
mutation SendRawRequest($additionalHeaders: JSON) {
  sendHttpRequest(
    body: { id: 101, title: "Hello world", userId: 10, body: "A test post" }
    method: "post"
    url: "http://localhost:1234/posts"
    timeout: 30
    retry: { times: 2, delay: 1000 }
    additionalHeaders: $additionalHeaders
  )
}

# Variables
# {
#   "additionalHeaders": {
#     "Content-Type": "application/xml"
#   }
# }

# Request
# POST https://jsonplaceholder.typicode.com/posts
#
# <xml><id>101</id><title>Hello world</title><userId>10</userId><body>A test post</body></xml>
```

> [!NOTE]
> If you set `application/xml` or `application/x-www-form-urlencoded` content type header but the request body is a string the connector will forward the raw string without converting.

### Timeout and Retry

You can specify the timeout in seconds and retry policy in request arguments:

- `timeout`: The timeout limit in seconds.
- `retry.times`: The number of times to be retried.
- `retry.delay`: The delay duration between retries.
- `retry.httpStatus`: The list of HTTP statuses the connector will retry on.

```graphql
mutation SendRawRequest {
  sendHttpRequest(
    method: "get"
    url: "https://jsonplaceholder.typicode.com/posts"
    timeout: 30
    retry: { times: 2, delay: 1000, httpStatus: [429, 500, 502, 503] }
  )
}
```
