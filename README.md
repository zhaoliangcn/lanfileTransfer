# LanFileTransfer

局域网文件传输工具，由两个互通组件构成：

| 组件 | 目录 | 形态 | 平台 |
|---|---|---|---|
| **LanFileTransfer-Go** | `lanfileTransfer-go/` | Wails 桌面应用（发送端 + 图形界面） | Windows / macOS / Linux |
| **lanfileTransfer-c** | `lanfileTransfer-c/` | 无头守护进程 + 内置 CLI（接收端） | Linux / macOS |

两端通过自定义 TCP 分块协议 + UDP 广播发现通信，可互为发送方或接收方。

## 功能

- 局域网设备自动发现（UDP IPv4 广播，5s 心跳 / 30s 判离线）
- **手动添加对端** —— 广播不跨路由器，跨网段设备可按 IP + 端口直接接入
- 单文件 / 整目录传输，目录结构通过 `relativePath` 保持
- 实时进度、速度、剩余时间
- 断点续传（发送端 checkpoint 文件 + 接收端按偏移写入）
- 分块 MD5 校验，损坏立即报错而非静默产出坏文件
- 传输历史持久化
- 设备间局域网聊天
- 远程关机 / 重启（**需共享认证令牌**）

## 安全说明

⚠️ 传输本身**无加密、无签名**。协议只做分块校验和，不提供机密性或对端身份认证 —— 请仅在可信网络中使用。

远程关机 / 重启需要双方持有相同的共享认证令牌：

- Go 端：`%AppData%/<vendor>/LanFileTransfer/auth_token`（或 `~/.config/LanFileTransfer/auth_token`）
- C 端：`~/.lanfiletransfer.token`

令牌首次启动时自动生成。若需要对端执行远程开关机，需手动将令牌文件复制到另一台机器。

## 协议

传输帧格式（TCP 流）：

```
┌──────────────────┬─────────────────────┬──────────────────┐
│ uint32 BE (4B)   │ JSON header (≤4096B)│ payload           │
│ header 字节数    │                      │ 长度 = fileSize   │
└──────────────────┴─────────────────────┴──────────────────┘
```

header 字段：

| 字段 | 含义 |
|---|---|
| `transferId` | 传输唯一标识 |
| `fileName` | 文件名 |
| `fileSize` | **本块**字节数（易误解） |
| `totalFileSize` | 整个文件大小 |
| `chunkIndex` | 块序号，从 0 开始 |
| `chunkOffset` | 本块在文件中的**绝对字节偏移** |
| `totalChunks` | 总块数 |
| `checksum` | 本块 MD5（hex） |
| `relativePath` | 目录传输时的相对路径（单文件传输时省略） |

控制消息（UDP，裸 JSON，无长度前缀）类型：`discovery`、`transfer_request`、`transfer_control`、`chat_message`、`system_command`。

`chunkOffset` 与 `authToken` 为较新版本新增字段。旧版本接收端会忽略未知字段，可单向兼容；但**要启用远程开关机认证，两端都需升级**。

## 构建

### Go 桌面端

前置：[Go 1.23+](https://go.dev/dl/)、[Node.js 18+](https://nodejs.org/)、[Wails CLI](https://wails.io)

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd lanfileTransfer-go
npm --prefix frontend install     # 或先进入 frontend 执行 npm install
wails build                        # 产物: build/bin/LanFileTransfer-Go[.exe]
```

开发模式（热重载）：

```bash
wails dev
```

其他命令：

```bash
go test ./...          # 单元测试
gofmt -l .             # 格式检查
```

### C 守护进程

```bash
cd lanfileTransfer-c
make                   # 产物: bin/lanfileTransfer-c
```

以 systemd 服务安装（Linux）：

```bash
sudo ./install_service.sh
```

常用参数：

```
-n, --name NAME          设备名称（默认取 hostname）
-d, --device-id ID       设备 ID（默认 "<hostname>-<时间戳>"）
-p, --port PORT          TCP 传输端口（默认 9876）
-D, --discovery-port     UDP 发现端口（默认 9876）
-s, --save-dir DIR       接收文件保存目录
-a, --auto-receive       自动接收
-c, --concurrent NUM     最大并发传输数
-b, --chunk-size KB      分块大小（默认 64）
-i, --interval SEC       发现广播间隔（默认 5）
-f, --foreground        前台运行（默认后台守护）
```

## 跨网段使用

UDP 广播不会被路由器转发，因此**不同网段的设备无法自动发现**。三种应对：

1. **同网段** —— 最简单，让两台设备接入同一台路由器。
2. **路由器改桥接** —— 副路由/第二台路由器改 AP 或桥接模式并与主路由同网段，关闭其 DHCP。
3. **端口映射 + 手动添加** —— 在对端出口路由器映射 TCP 9876，然后在桌面端设备面板点「+」按地址添加。

跨网段场景下，**必须使用第 3 种**，因为自动发现始终不可用。

## 防火墙

两端都需要放行 **TCP 9876 + UDP 9876**：

```powershell
# Windows（管理员）
New-NetFirewallRule -DisplayName "LanFileTransfer TCP" -Direction Inbound -Action Allow -Protocol TCP -LocalPort 9876 -Profile Any
New-NetFirewallRule -DisplayName "LanFileTransfer UDP" -Direction Inbound -Action Allow -Protocol UDP -LocalPort 9876 -Profile Any
```

```bash
# Linux
ufw allow 9876/tcp && ufw allow 9876/udp
```

## 文档

- [架构设计 · Architecture](lanfileTransfer-go/ARCHITECTURE.md)
- [代码审查报告 · Code Review](lanfileTransfer-go/CODE_REVIEW.md)
- [接收端已知问题时序图 · Known Receiver Issues](lanfileTransfer-c/SEQUENCE_DIAGRAM.md)

## 许可

见 [LICENSE](LICENSE)。
