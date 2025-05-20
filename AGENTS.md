# Contributors Guide for Go Translator Agent

## Project Overview

This repository implements a CLI-based translator agent in Go. It provides translation features for various formats, supports multiple translation engines, and exposes commands under the `cmd/` directory.

## Directory Structure

- `cmd/`      : CLI entrypoints
- `configs/`  : Configuration templates
- `internal/` : Internal packages (not for public use)
- `pkg/`      : Public library packages
- `docs/`     : Architecture and usage documentation
- `tests/`    : Integration and end-to-end tests
- `formats.md`: Supported file formats and specifications
- `Makefile`  : Common build/test/lint targets
- `justfile`  : Alternative task runner

## Prerequisites

- Go 1.20+ installed
- GNU Make or `just` for task automation
- `golangci-lint` installed (see `.golangci.yml` for config)
- (Optional) Docker for containerized builds

## Building

```sh
make build         # build the CLI binary
# or
go build ./cmd/... # build commands directly
```

## Testing

```sh
make test          # run all test suites
# or
go test -v -race -coverprofile=coverage.out -covermode=atomic ./tests/...     # run all unit tests
go tool cover -func=coverage.out  # view coverage report
```

## Linting

```sh
make lint          # run linters via Makefile
# or
golangci-lint run # directly run all configured linters
```

## Documentation

- `docs/` contains design docs and usage guides
- `formats.md` describes supported file formats
- `README.md` covers installation and quick-start examples

## Contribution Guidelines

- Fork the repository and create feature branches: `feat/<description>` or `fix/<description>`
- Write clear commit messages using the format:

  ```
  [feat/fix/docs/chore]: Brief description of the change

  - Detailed description (if necessary)
  ```

- PR title should match commit message subject: `[feat]: Add translation for YAML files`
- Submit PRs against the `master` branch
- Ensure all tests pass and lint issues are resolved before merging

## Release Process

- Use [semantic versioning](https://semver.org/): `vMAJOR.MINOR.PATCH`
- Update `CHANGELOG.md` with notable changes
- Tag the commit and run `make release` to build and publish artifacts

## Validation

- All CI checks (GitHub Actions) must pass
- Locally run `make test` and `make lint`
- Perform manual smoke tests if making behavioral changes
