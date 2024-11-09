VERSION ?= $(shell date +"%Y%m%d")
OUTPUT_DIR := _output

.PHONY: format
format:
	gofmt -w -s .
	cd ndc-rest-schema && gofmt -w -s .

.PHONY: test
test:
	go test -v -race -timeout 3m ./...
	cd ndc-rest-schema && go test -v -race -timeout 3m ./...

# Install golangci-lint tool to run lint locally
# https://golangci-lint.run/usage/install
.PHONY: lint
lint:
	golangci-lint run --fix
	cd ndc-rest-schema && golangci-lint run --fix

# clean the output directory
.PHONY: clean
clean:
	rm -rf "$(OUTPUT_DIR)"

.PHONY: go-tidy
go-tidy:
	go mod tidy
	cd ndc-rest-schema && go mod tidy

.PHONY: build-jsonschema
build-jsonschema:
	cd ./ndc-rest-schema/jsonschema && go run .

# build the ndc-rest-schema for all given platform/arch
.PHONY: build-cli
build-cli:
	go build -o _output/ndc-rest-schema ./ndc-rest-schema

.PHONY: ci-build-cli
ci-build-cli: export CGO_ENABLED=0
ci-build-cli: clean
	cd ./ndc-rest-schema && \
	go get github.com/mitchellh/gox && \
	go run github.com/mitchellh/gox -ldflags '-X github.com/hasura/ndc-rest/ndc-rest-schema/version.BuildVersion=$(VERSION) -s -w -extldflags "-static"' \
		-osarch="linux/amd64 darwin/amd64 windows/amd64 darwin/arm64 linux/arm64" \
		-output="../$(OUTPUT_DIR)/ndc-rest-schema-{{.OS}}-{{.Arch}}" \
		.