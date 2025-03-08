projectname := "go-translator"

# 列出所有可用的命令
default:
    @just --list

# 构建 Golang 二进制文件
build:
    go build -ldflags "-X main.Version=dev -X main.Commit=$(git rev-parse --short HEAD || echo 'unknown') -X main.BuildDate=$(date +%Y-%m-%d)" -o {{projectname}} ./cmd/translator

# 安装 Golang 二进制文件
install:
    go install -ldflags "-X main.Version=dev -X main.Commit=$(git rev-parse --short HEAD || echo 'unknown') -X main.BuildDate=$(date +%Y-%m-%d)" ./cmd/translator

# 运行应用程序
run *args:
    go run -ldflags "-X main.Version=dev -X main.Commit=$(git rev-parse --short HEAD || echo 'unknown') -X main.BuildDate=$(date +%Y-%m-%d)" ./cmd/translator/main.go {{args}}

# 安装构建依赖
bootstrap:
    go generate -tags tools tools/tools.go
    go mod tidy

# 运行测试并显示覆盖率
test: clean
    go test --cover -parallel=1 -v -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out | sort -rnk3

# 清理环境
clean:
    rm -rf coverage.out dist {{projectname}} {{projectname}}.exe

# 显示测试覆盖率
cover:
    go test -v -race $(go list ./... | grep -v /vendor/) -v -coverprofile=coverage.out
    go tool cover -func=coverage.out

# 格式化 Go 文件
fmt:
    gofumpt -w .
    gci write .

# 运行 linter
lint:
    golangci-lint run -c .golangci.yml

# 测试发布
release-test:
    goreleaser release --snapshot --clean

# 正式发布
release:
    goreleaser release --clean
