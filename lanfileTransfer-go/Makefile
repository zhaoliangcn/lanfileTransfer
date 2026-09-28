SHELL := /bin/bash

APP_NAME     := LanFileTransfer-Go
MODULE       := LanFileTransfer-Go
BUILD_DIR    := build/bin
# 包含根包 "."：main.go 里有 Wails 绑定、信号处理与配置接线，
# 只写 ./internal/... ./pkg/... 会让 go vet / go test 完全跳过它。
SRC_DIRS     := . ./internal/... ./pkg/...

GO           ?= go
GOVERSION    := $(shell $(GO) version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+')

GOOS         ?= $(shell go env GOOS)
GOARCH       ?= $(shell go env GOARCH)

LDFLAGS      := -s -w -X main.buildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ) -X main.version=1.0.0

B_GREEN  := \033[32m
B_YELLOW := \033[33m
B_CYAN   := \033[36m
B_RED    := \033[31m
B_WHITE  := \033[1;37m
NC       := \033[0m

.PHONY: help all build dev test vet fmt fmt-check lint check clean tidy deps deps-frontend build-frontend ensure-dist run dev-run install-wails dist windows linux darwin

help:
	@echo -e "$(B_CYAN)========================================$(NC)"
	@echo -e "$(B_CYAN)  $(APP_NAME) Makefile$(NC)"
	@echo -e "$(B_CYAN)========================================$(NC)"
	@echo ""
	@echo -e "$(B_WHITE)目标 (Targets):$(NC)"
	@echo "  all              清理 -> 测试 -> 生成桌面应用"
	@echo "  build            生产模式构建 Wails 桌面应用"
	@echo "  dev              开发模式构建 Wails 桌面应用（带调试）"
	@echo "  test             运行 Go 单元测试"
	@echo "  vet              运行 go vet 静态检查"
	@echo "  fmt              格式化 Go 源码"
	@echo "  lint             vet + fmt 组合"
	@echo "  check            CI 同款校验 (fmt/vet/test 三合一)"
	@echo "  clean            清理构建产物"
	@echo "  tidy             整理 go.mod/go.sum 依赖"
	@echo "  deps             安装 Wails CLI 工具"
	@echo "  deps-frontend    安装前端 npm 依赖"
	@echo "  build-frontend   构建前端资源 (npm run build)"
	@echo "  ensure-dist      确保 frontend/dist 存在 (Go 编译前置条件)"
	@echo "  run              启动 Wails 桌面应用（开发模式）"
	@echo "  dev-run          等同于 make run"
	@echo ""
	@echo -e "$(B_WHITE)跨平台构建 (Cross-compile):$(NC)"
	@echo "  make windows     构建 Windows (amd64) .exe"
	@echo "  make linux       构建 Linux (amd64)"
	@echo "  make darwin      构建 macOS (universal)"
	@echo ""
	@echo -e "$(B_WHITE)示例:$(NC)"
	@echo "  make all"
	@echo "  make build"
	@echo "  make test"
	@echo "  GOOS=linux GOARCH=arm64 make build"

all: clean test build

# main.go 用 `//go:embed all:frontend/dist` 内嵌前端产物，而 frontend/dist 被
# .gitignore 排除。目录缺失时 go build / go vet / go test 全部会报
# "pattern all:frontend/dist: no matching files found"。放一个占位文件进去即可
# 让纯 Go 命令独立可用，无需先装 Node。
ensure-dist:
	@if [ ! -d frontend/dist ]; then \
		mkdir -p frontend/dist; \
		printf '' > frontend/dist/.gitkeep; \
		echo -e "$(B_YELLOW)frontend/dist 不存在，已创建占位目录（Go 编译需要）$(NC)"; \
	fi

check: ensure-dist
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory test
	@echo -e "$(B_GREEN)[OK]$(NC) 全部检查通过"

build: check-go build-frontend
	@echo -e "$(B_YELLOW)--- 构建桌面应用 (生产模式) ---$(NC)"
	@$(call check-wails)
	wails build -o $(APP_NAME)
	@echo -e "$(B_GREEN)[OK]$(NC) 构建完成: $(B_WHITE)$(BUILD_DIR)/$(APP_NAME)$(NC)"

dev: check-go build-frontend
	@echo -e "$(B_YELLOW)--- 构建桌面应用 (开发模式) ---$(NC)"
	@$(call check-wails)
	wails build -debug -o $(APP_NAME)
	@echo -e "$(B_GREEN)[OK]$(NC) 构建完成 (debug): $(B_WHITE)$(BUILD_DIR)/$(APP_NAME)$(NC)"

test: check-go ensure-dist
	@echo -e "$(B_YELLOW)--- 运行 Go 单元测试 ---$(NC)"
	$(GO) test $(SRC_DIRS) -count=1 -race
	@echo -e "$(B_GREEN)[OK]$(NC) 测试全部通过"

vet: check-go ensure-dist
	@echo -e "$(B_YELLOW)--- go vet ---$(NC)"
	$(GO) vet $(SRC_DIRS)
	@echo -e "$(B_GREEN)[OK]$(NC) go vet 无问题"

fmt: check-go
	@echo -e "$(B_YELLOW)--- gofmt ---$(NC)"
	@files=$$($(GO) fmt $(SRC_DIRS)); \
	if [ -n "$$files" ]; then \
		echo -e "$(B_GREEN)[OK]$(NC) 已格式化下列文件:"; \
		echo "$$files"; \
	else \
		echo -e "$(B_GREEN)[OK]$(NC) 无需格式化"; \
	fi

lint: fmt vet

fmt-check: check-go
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
		echo -e "$(B_RED)[FAIL]$(NC) 以下文件未格式化:"; \
		echo "$$out"; \
		exit 1; \
	fi
	@echo -e "$(B_GREEN)[OK]$(NC) gofmt 无问题"

clean:
	@echo -e "$(B_YELLOW)--- 清理构建产物 ---$(NC)"
	@mkdir -p $(BUILD_DIR)
	@rm -rf $(BUILD_DIR)/*
	@$(GO) clean -cache -testcache 2>/dev/null || true
	@echo -e "$(B_GREEN)[OK]$(NC) 清理完成: $(BUILD_DIR)"

tidy: check-go
	@echo -e "$(B_YELLOW)--- go mod tidy ---$(NC)"
	$(GO) mod tidy
	@echo -e "$(B_GREEN)[OK]$(NC) 依赖已整理"

deps: check-go install-wails

deps-frontend:
	@echo -e "$(B_YELLOW)--- 安装前端依赖 ---$(NC)"
	@if [ ! -d frontend/node_modules ]; then \
		cd frontend && npm install; \
		echo -e "$(B_GREEN)[OK]$(NC) 前端依赖已安装"; \
	else \
		echo -e "$(B_GREEN)[OK]$(NC) 前端依赖已安装 (跳过)"; \
	fi

build-frontend: deps-frontend
	@echo -e "$(B_YELLOW)--- 构建前端资源 ---$(NC)"
	@if [ ! -d frontend/dist ] || [ -n "$(find frontend/src -newer frontend/dist 2>/dev/null)" ]; then \
		cd frontend && npm run build; \
		echo -e "$(B_GREEN)[OK]$(NC) 前端资源已构建"; \
	else \
		echo -e "$(B_GREEN)[OK]$(NC) 前端资源已是最新 (跳过)"; \
	fi

run: check-go build-frontend
	@echo -e "$(B_YELLOW)--- 启动桌面应用 (开发模式) ---$(NC)"
	@$(call check-wails)
	wails dev

dev-run: run

install-wails: check-go
	@if ! command -v wails >/dev/null 2>&1; then \
		echo -e "$(B_YELLOW)--- 安装 Wails CLI ---$(NC)"; \
		$(GO) install github.com/wailsapp/wails/v2/cmd/wails@latest; \
		export PATH=$$($(GO) env GOPATH)/bin:$$PATH; \
		echo -e "$(B_GREEN)[OK]$(NC) Wails CLI 已安装到: $$($(GO) env GOPATH)/bin"; \
		echo -e "$(B_YELLOW)请确保 GOPATH/bin 在 PATH 中$(NC)"; \
	else \
		echo -e "$(B_GREEN)[OK]$(NC) Wails CLI 已存在: $$(wails version)"; \
	fi

# wails 的 -o 是相对 build/bin/ 的，写成 $(BUILD_DIR)/$(APP_NAME) 会产出
# build/bin/build/bin/<name>。
windows:
	@echo -e "$(B_YELLOW)--- 构建 Windows (amd64) ---$(NC)"
	@$(call check-wails)
	GOOS=windows GOARCH=amd64 wails build -o $(APP_NAME)-amd64.exe
	@echo -e "$(B_GREEN)[OK]$(NC) Windows 构建完成: $(BUILD_DIR)/$(APP_NAME)-amd64.exe"

linux:
	@echo -e "$(B_YELLOW)--- 构建 Linux (amd64) ---$(NC)"
	@$(call check-wails)
	GOOS=linux GOARCH=amd64 wails build -o $(APP_NAME)
	@echo -e "$(B_GREEN)[OK]$(NC) Linux 构建完成: $(BUILD_DIR)/$(APP_NAME)"

darwin:
	@echo -e "$(B_YELLOW)--- 构建 macOS (universal) ---$(NC)"
	@$(call check-wails)
	wails build -o $(APP_NAME)
	@echo -e "$(B_GREEN)[OK]$(NC) macOS 构建完成: $(BUILD_DIR)/$(APP_NAME).app"

dist: windows linux darwin

define check-wails
	@if ! command -v wails >/dev/null 2>&1; then \
		echo -e "$(B_RED)[ERROR]$(NC) Wails CLI 未安装"; \
		echo "请执行: go install github.com/wailsapp/wails/v2/cmd/wails@latest"; \
		echo "或: make install-wails"; \
		exit 1; \
	fi
endef

check-go:
	@if ! command -v $(GO) >/dev/null 2>&1; then \
		echo -e "$(B_RED)[ERROR]$(NC) Go 编译器未找到。请先安装 Go 1.23+"; \
		exit 1; \
	fi
