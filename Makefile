.PHONY: build build-frontend build-backend clean dev all

# 默认目标
all: build

# 构建前端
build-frontend:
	cd web && npm install && npm run build

# 复制前端到 cmd/cloud 目录以便 embed
copy-frontend:
	rm -rf cmd/cloud/dist
	cp -r web/dist cmd/cloud/dist

# 构建后端 (包含前端)
build-backend:
	CGO_ENABLED=1 go build -o bin/cloud ./cmd/cloud
	CGO_ENABLED=1 go build -o bin/agent ./cmd/agent

# 完整构建
build: build-frontend copy-frontend build-backend

# 仅构建后端 (开发用，不包含前端)
build-dev:
	@mkdir -p cmd/cloud/dist
	@echo '<!DOCTYPE html><html><body><h1>Development Mode</h1><p>Run make build for full build</p></body></html>' > cmd/cloud/dist/index.html
	CGO_ENABLED=1 go build -o bin/cloud ./cmd/cloud
	CGO_ENABLED=1 go build -o bin/agent ./cmd/agent

# 清理
clean:
	rm -rf bin/
	rm -rf web/dist/
	rm -rf cmd/cloud/dist/

# 开发模式 - 前端
dev-frontend:
	cd web && npm run dev

# 开发模式 - Cloud
dev-cloud:
	@mkdir -p cmd/cloud/dist
	@echo '<!DOCTYPE html><html><body><h1>Dev</h1></body></html>' > cmd/cloud/dist/index.html
	go run ./cmd/cloud -addr :8080 -token dev-token

# 开发模式 - Agent
dev-agent:
	go run ./cmd/agent -server ws://localhost:8080/ws -token dev-token -name dev-agent

# 运行测试
test:
	go test -v ./...

# 格式化代码
fmt:
	go fmt ./...
	cd web && npm run format 2>/dev/null || true

# 下载依赖
deps:
	go mod tidy
	cd web && npm install

# 构建 Linux 版本 (交叉编译)
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/cloud-linux-amd64 ./cmd/cloud
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/agent-linux-amd64 ./cmd/agent

# 构建 Windows 版本
build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/cloud-windows-amd64.exe ./cmd/cloud
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/agent-windows-amd64.exe ./cmd/agent
