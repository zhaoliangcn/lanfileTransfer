# lanfileTransfer-c Makefile
# 兼容 linfileTransfer-go 协议的命令行文件传输工具

# 编译器设置
CC = gcc
CFLAGS = -Wall -Wextra -std=gnu99 -pedantic
CFLAGS += -D_POSIX_C_SOURCE=200809L -D_BSD_SOURCE -D_DEFAULT_SOURCE
CFLAGS += -pthread

# 优化级别
ifeq ($(RELEASE),1)
    CFLAGS += -O2 -DNDEBUG
else
    CFLAGS += -g -DDEBUG
endif

# 目录
SRC_DIR = src
INC_DIR = include
OBJ_DIR = obj
BIN_DIR = bin

# 目标程序
TARGET = $(BIN_DIR)/lanfileTransfer-c

# 源文件
# 注意：status_sync.c / auto_receive.c / cli_interface.c 曾在此列表中，但
# main.c 从未调用它们的入口（cli_init / status_sync_init / auto_receive_init），
# 三个模块仅互相引用，属于约 1800 行死代码，已移除。其中 status_sync.c 还存在
# 非递归 mutex 自死锁，一旦接线即会让 CLI 永久卡住。
SRCS = $(SRC_DIR)/main.c \
       $(SRC_DIR)/protocol.c \
       $(SRC_DIR)/discovery.c \
       $(SRC_DIR)/transfer.c

# 对象文件
OBJS = $(SRCS:$(SRC_DIR)/%.c=$(OBJ_DIR)/%.o)

# 头文件依赖
DEPS = $(SRCS:$(SRC_DIR)/%.c=$(OBJ_DIR)/%.d)

# 库
LIBS = -lpthread -lm

# 安装目录
PREFIX = /usr/local
BINDIR = $(PREFIX)/bin
INCLUDEDIR = $(PREFIX)/include

# 默认目标
all: dirs $(TARGET)

# 创建目录
dirs:
	@mkdir -p $(OBJ_DIR) $(BIN_DIR)

# 主程序链接
$(TARGET): $(OBJS)
	@echo "链接: $@"
	$(CC) $(CFLAGS) -o $@ $^ $(LIBS)
	@echo "✓ 构建完成: $@"

# 编译规则
$(OBJ_DIR)/%.o: $(SRC_DIR)/%.c
	@echo "编译: $< -> $@"
	$(CC) $(CFLAGS) -I$(INC_DIR) -MMD -c $< -o $@

# 包含依赖
-include $(DEPS)

# 调试版本
debug:
	$(MAKE) RELEASE=0

# 发布版本
release:
	$(MAKE) RELEASE=1

# 清理
clean:
	@echo "清理构建文件..."
	rm -rf $(OBJ_DIR) $(BIN_DIR)
	@echo "✓ 清理完成"

# 重新构建
rebuild: clean all

# 安装
install: all
	@echo "安装到 $(PREFIX)..."
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(TARGET) $(DESTDIR)$(BINDIR)/
	@echo "✓ 安装完成"
	@echo "运行: lanfileTransfer-c --help"

# 卸载
uninstall:
	@echo "从 $(PREFIX) 卸载..."
	rm -f $(DESTDIR)$(BINDIR)/lanfileTransfer-c
	@echo "✓ 卸载完成"

# 运行测试
test: all
	@echo "运行测试..."
	$(TARGET) --version
	$(TARGET) --help

# 显示帮助
help:
	@echo "lanfileTransfer-c 构建系统"
	@echo ""
	@echo "可用目标:"
	@echo "  all       - 构建默认版本 (debug)"
	@echo "  debug     - 构建调试版本"
	@echo "  release   - 构建发布版本"
	@echo "  clean     - 清理构建文件"
	@echo "  rebuild   - 重新构建"
	@echo "  install   - 安装到系统"
	@echo "  uninstall - 卸载"
	@echo "  test      - 运行测试"
	@echo "  help      - 显示此帮助"
	@echo ""
	@echo "选项:"
	@echo "  RELEASE=1 - 构建发布版本"
	@echo "  PREFIX    - 安装前缀 (默认: /usr/local)"
	@echo ""
	@echo "示例:"
	@echo "  make                 # 构建调试版本"
	@echo "  make release         # 构建发布版本"
	@echo "  make install         # 安装"
	@echo "  make clean all       # 清理并重新构建"

# 显示版本
version:
	@echo "lanfileTransfer-c v1.0.0"

# 检查依赖
check-deps:
	@echo "检查依赖..."
	@which $(CC) > /dev/null && echo "  ✓ $(CC) 可用" || echo "  ✗ $(CC) 未找到"
	@pkg-config --exists pthread && echo "  ✓ pthread 可用" || echo "  ✗ pthread 未找到"
	@echo "检查完成"

.PHONY: all dirs debug release clean rebuild install uninstall test help version check-deps