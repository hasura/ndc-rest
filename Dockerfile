# build context at repo root: docker build -f Dockerfile .
FROM golang:1.23 AS builder

WORKDIR /app
COPY ndc-rest-schema ./ndc-rest-schema
COPY go.mod go.sum go.work ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -v -o ndc-cli ./server

# stage 2: production image
FROM gcr.io/distroless/static-debian12:nonroot

# Copy the binary to the production image from the builder stage.
COPY --from=builder /app/ndc-cli /ndc-cli

ENV HASURA_CONFIGURATION_DIRECTORY=/etc/connector

ENTRYPOINT ["/ndc-cli"]

# Run the web service on container startup.
CMD ["serve"]