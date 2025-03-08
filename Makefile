projectname?=go-translator

default: help

.PHONY: help
help: ## list makefile targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## build golang binary
	@go build -ldflags "-X main.Version=dev -X main.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.BuildDate=$(shell date +%Y-%m-%d)" -o $(projectname) ./cmd/translator

.PHONY: install
install: ## install golang binary
	@go install -ldflags "-X main.Version=dev -X main.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.BuildDate=$(shell date +%Y-%m-%d)" ./cmd/translator

.PHONY: run
run: ## run the app
	@go run -ldflags "-X main.Version=dev -X main.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.BuildDate=$(shell date +%Y-%m-%d)"  ./cmd/translator/main.go

.PHONY: bootstrap
bootstrap: ## install build deps
	go generate -tags tools tools/tools.go
	go mod tidy

.PHONY: test
test: clean ## display test coverage
	go test --cover -parallel=1 -v -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | sort -rnk3
	
.PHONY: clean
clean: ## clean up environment
	@rm -rf coverage.out dist/ $(projectname)

.PHONY: cover
cover: ## display test coverage
	go test -v -race $(shell go list ./... | grep -v /vendor/) -v -coverprofile=coverage.out
	go tool cover -func=coverage.out

.PHONY: fmt
fmt: ## format go files
	gofumpt -w .
	gci write .

.PHONY: lint
lint: ## lint go files
	golangci-lint run -c .golangci.yml

.PHONY: release-test
release-test: ## test release
	goreleaser release --rm-dist --snapshot --clean --skip-publish

.PHONY: release
release: ## release new version
	goreleaser release --clean

# .PHONY: pre-commit
# pre-commit:	## run pre-commit hooks
# 	pre-commit run --all-files
