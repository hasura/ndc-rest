VERSION ?= $(shell date +"%Y%m%d")
OUTPUT_DIR := _output

.PHONY: format
format:
	gofmt -w -s .
	cd ndc-http-schema && gofmt -w -s .

.PHONY: test
test:
	go test -v -race -timeout 3m ./...
	cd ndc-http-schema && go test -v -race -timeout 3m ./...

# Install golangci-lint tool to run lint locally
# https://golangci-lint.run/usage/install
.PHONY: lint
lint:
	golangci-lint run --fix
	cd ndc-http-schema && golangci-lint run --fix

# clean the output directory
.PHONY: clean
clean:
	rm -rf "$(OUTPUT_DIR)"

.PHONY: go-tidy
go-tidy:
	go mod tidy
	cd ndc-http-schema && go mod tidy

.PHONY: build-jsonschema
build-jsonschema:
	cd ./ndc-http-schema/jsonschema && go run .

# build the ndc-http-schema for all given platform/arch
.PHONY: build-cli
build-cli:
	go build -o _output/ndc-http-schema ./ndc-http-schema

.PHONY: ci-build-cli
ci-build-cli: export CGO_ENABLED=0
ci-build-cli: clean
	cd ./ndc-http-schema && \
	go get github.com/mitchellh/gox && \
	go run github.com/mitchellh/gox -ldflags '-X github.com/hasura/ndc-http/ndc-http-schema/version.BuildVersion=$(VERSION) -s -w -extldflags "-static"' \
		-osarch="linux/amd64 darwin/amd64 windows/amd64 darwin/arm64 linux/arm64" \
		-output="../$(OUTPUT_DIR)/ndc-http-schema-{{.OS}}-{{.Arch}}" \
		.

.PHONY: generate-test-config
generate-test-config:
	go run ./ndc-http-schema update -d ./tests/configuration

.PHONY: start-ddn
start-ddn:
	HASURA_DDN_PAT=$$(ddn auth print-pat) docker compose --env-file tests/engine/.env up --build -d

.PHONY: stop-ddn
stop-ddn:
	docker compose down --remove-orphans

.PHONY: build-supergraph-test
build-supergraph-test:
	docker compose up -d --build ndc-http
	cd tests/engine && \
		ddn connector-link update myapi --add-all-resources --subgraph ./app/subgraph.yaml && \
		ddn supergraph build local
	make start-ddn