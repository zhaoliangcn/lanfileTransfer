# LanFileTransfer — Go Stack Architecture / 局域网文件传输工具 - Go 技术栈架构设计

> 文档语言：中文 | Document language: Chinese
> 状态：部分内容已落后于实现（例如目录结构、UI 功能清单、协议字段），以代码为准。

## 1. 项目概述

### 1.1 项目名称
LanFileTransfer-Go

### 1.2 项目目标
开发一款跨平台的局域网文件传输工具，支持文件/文件夹的高速传输、传输进度监控、断点续传等功能。

### 1.3 核心需求
- 局域网设备自动发现
- 文件/文件夹传输
- 实时进度显示
- 多任务并发传输
- 断点续传
- 跨平台支持（Windows/macOS/Linux）

---

## 2. 技术栈选型

### 2.1 核心技术栈

| 层级 | 技术 | 版本 | 说明 |
|------|------|------|------|
| 编程语言 | Go | 1.21+ | 主开发语言 |
| GUI 框架 | Wails | 2.x | 前端界面框架 |
| 前端框架 | Vue 3 + TypeScript | 3.4+ | 界面开发 |
| UI 组件库 | Naive UI / Element Plus | 最新 | 界面组件 |
| 网络通信 | net + net/udp | 标准库 | TCP/UDP 通信 |
| 服务发现 | mdns | hashicorp/mdns | mDNS 设备发现 |
| 异步处理 | goroutine + channel | 内置 | 并发模型 |
| 配置管理 | viper | 最新 | 配置文件管理 |
| 日志系统 | zap | 最新 | 高性能日志 |
| 打包工具 | wails build | 内置 | 应用打包 |

### 2.2 技术选型理由

**Go 语言优势：**
- goroutine 轻量级并发，适合网络 I/O 密集型应用
- 标准库强大，`net` 包开箱即用
- 编译速度快，交叉编译简单
- 垃圾回收，无需手动内存管理
- 学习曲线平缓，开发效率高

**Wails 优势：**
- 前端使用 Web 技术，界面美观度高
- 后端 Go 处理核心逻辑，性能好
- 打包体积小（相比 Electron）
- 支持热重载，开发体验好
- Go 与前端通信简单

---

## 3. 系统架构

### 3.1 整体架构

```
┌─────────────────────────────────────────────────────────┐
│                      用户界面层                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │  Vue 3 + TypeScript + Naive UI                  │   │
│  │  - 设备列表视图                                  │   │
│  │  - 文件选择器                                    │   │
│  │  - 传输进度面板                                  │   │
│  │  - 设置页面                                      │   │
│  └─────────────────────────────────────────────────┘   │
└────────────────────────┬────────────────────────────────┘
                         │ Wails Runtime Bridge
┌────────────────────────▼────────────────────────────────┐
│                      应用服务层                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │  FileService │  │ PeerService  │  │ ConfigService│  │
│  │  文件操作    │  │ 设备管理     │  │ 配置管理     │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                      核心业务层                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │TransferManager│ │ DiscoveryMgr │  │  ChunkMgr    │  │
│  │ 传输管理器   │  │ 发现管理器   │  │ 分块管理器   │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                      网络通信层                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ TCP Server   │  │ UDP Server   │  │  Protocol    │  │
│  │ 数据传输     │  │ 设备发现     │  │  协议编解码  │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 3.2 模块划分

| 模块 | 职责 | 关键接口 |
|------|------|---------|
| **UI 层** | 用户交互、状态展示 | 无 |
| **FileService** | 文件读写、路径处理 | `SelectFile()`, `GetFileInfo()` |
| **PeerService** | 设备发现、连接管理 | `DiscoverPeers()`, `Connect()` |
| **TransferManager** | 传输调度、进度追踪 | `SendFile()`, `CancelTransfer()` |
| **DiscoveryMgr** | mDNS 广播、监听 | `StartDiscovery()`, `StopDiscovery()` |
| **ChunkMgr** | 文件分块、校验 | `SplitFile()`, `VerifyChecksum()` |
| **TCP Server** | 数据流传输 | `Listen()`, `SendData()` |
| **UDP Server** | 控制消息、发现 | `Broadcast()`, `Listen()` |

---

## 4. 详细设计

### 4.1 项目结构

```
LanFileTransfer-Go/
├── frontend/                 # 前端代码
│   ├── src/
│   │   ├── components/      # Vue 组件
│   │   │   ├── DeviceList.vue
│   │   │   ├── TransferPanel.vue
│   │   │   ├── ProgressBar.vue
│   │   │   └── Settings.vue
│   │   ├── views/           # 页面视图
│   │   │   ├── HomeView.vue
│   │   │   └── HistoryView.vue
│   │   ├── stores/          # Pinia 状态管理
│   │   │   ├── device.ts
│   │   │   ├── transfer.ts
│   │   │   └── config.ts
│   │   ├── composables/     # 组合式函数
│   │   │   ├── useTransfer.ts
│   │   │   └── useDiscovery.ts
│   │   └── assets/          # 静态资源
│   ├── package.json
│   └── vite.config.ts
│
├── backend/                  # Go 后端代码
│   ├── cmd/
│   │   └── main.go          # 应用入口
│   ├── internal/
│   │   ├── config/          # 配置管理
│   │   │   └── config.go
│   │   ├── discovery/       # 设备发现
│   │   │   ├── mdns.go
│   │   │   └── peer.go
│   │   ├── transfer/        # 传输管理
│   │   │   ├── manager.go
│   │   │   ├── task.go
│   │   │   └── chunk.go
│   │   ├── network/         # 网络通信
│   │   │   ├── tcp.go
│   │   │   ├── udp.go
│   │   │   └── protocol.go
│   │   ├── file/            # 文件操作
│   │   │   ├── reader.go
│   │   │   └── writer.go
│   │   └── service/         # 业务服务
│   │       ├── file_service.go
│   │       ├── peer_service.go
│   │       └── transfer_service.go
│   └── pkg/                 # 公共库
│       └── utils/
│           ├── checksum.go
│           └── logger.go
│
├── wails.json               # Wails 配置
├── go.mod
└── README.md
```

### 4.2 核心数据结构

```go
// 设备信息
type Peer struct {
    ID       string    `json:"id"`
    Name     string    `json:"name"`
    IP       string    `json:"ip"`
    Port     int       `json:"port"`
    Online   bool      `json:"online"`
    LastSeen time.Time `json:"lastSeen"`
}

// 传输任务
type TransferTask struct {
    ID              string     `json:"id"`
    PeerID          string     `json:"peerId"`
    FileName        string     `json:"fileName"`
    FilePath        string     `json:"filePath"`
    FileSize        int64      `json:"fileSize"`
    BytesTransferred int64     `json:"bytesTransferred"`
    Type            string     `json:"type"` // "file" | "folder"
    Status          string     `json:"status"` // "pending" | "transferring" | "completed" | "failed" | "cancelled"
    IsSender        bool       `json:"isSender"`
    StartTime       time.Time  `json:"startTime"`
    EndTime         time.Time  `json:"endTime"`
    Speed           float64    `json:"speed"` // bytes/sec
}

// 传输协议头
type TransferHeader struct {
    TransferID  string `json:"transferId"`
    FileName    string `json:"fileName"`
    FileSize    int64  `json:"fileSize"`
    ChunkIndex  int64  `json:"chunkIndex"`
    TotalChunks int64  `json:"totalChunks"`
    Checksum    string `json:"checksum"`
}

// 配置
type Config struct {
    DefaultSavePath      string `mapstructure:"defaultSavePath"`
    AutoReceive          bool   `mapstructure:"autoReceive"`
    MaxConcurrentTransfers int  `mapstructure:"maxConcurrentTransfers"`
    ChunkSize            int    `mapstructure:"chunkSize"`
    ListenPort           int    `mapstructure:"listenPort"`
}
```

### 4.3 传输管理器设计

```go
// internal/transfer/manager.go

type TransferManager struct {
    mu              sync.RWMutex
    tasks           map[string]*TransferTask
    taskQueue       chan string
    maxConcurrent   int
    chunkSize       int
    tcpClient       *network.TCPClient
    eventBus        *EventBus
    ctx             context.Context
    cancel          context.CancelFunc
}

func NewTransferManager(cfg *config.Config) *TransferManager {
    ctx, cancel := context.WithCancel(context.Background())
    return &TransferManager{
        tasks:         make(map[string]*TransferTask),
        taskQueue:     make(chan string, 100),
        maxConcurrent: cfg.MaxConcurrentTransfers,
        chunkSize:     cfg.ChunkSize,
        ctx:           ctx,
        cancel:        cancel,
    }
}

// 发送文件
func (tm *TransferManager) SendFile(peerID, filePath string) (string, error) {
    taskID := generateTransferID()
    
    task := &TransferTask{
        ID:        taskID,
        PeerID:    peerID,
        FilePath:  filePath,
        FileName:  filepath.Base(filePath),
        FileSize:  getFileSize(filePath),
        Type:      "file",
        Status:    "pending",
        IsSender:  true,
        StartTime: time.Now(),
    }
    
    tm.mu.Lock()
    tm.tasks[taskID] = task
    tm.mu.Unlock()
    
    tm.taskQueue <- taskID
    return taskID, nil
}

// 处理传输队列
func (tm *TransferManager) processQueue() {
    activeCount := 0
    
    for {
        select {
        case taskID := <-tm.taskQueue:
            if activeCount >= tm.maxConcurrent {
                // 等待空闲
                continue
            }
            
            go func(id string) {
                activeCount++
                tm.executeTransfer(id)
                activeCount--
            }(taskID)
            
        case <-tm.ctx.Done():
            return
        }
    }
}

// 执行传输
func (tm *TransferManager) executeTransfer(taskID string) error {
    tm.mu.RLock()
    task := tm.tasks[taskID]
    tm.mu.RUnlock()
    
    task.Status = "transferring"
    tm.emitEvent("transferStarted", task)
    
    file, err := os.Open(task.FilePath)
    if err != nil {
        return tm.failTransfer(taskID, err)
    }
    defer file.Close()
    
    chunk := make([]byte, tm.chunkSize)
    chunkIndex := int64(0)
    totalChunks := (task.FileSize + int64(tm.chunkSize) - 1) / int64(tm.chunkSize)
    
    for {
        n, readErr := file.Read(chunk)
        if n > 0 {
            header := TransferHeader{
                TransferID:  taskID,
                FileName:    task.FileName,
                FileSize:    task.FileSize,
                ChunkIndex:  chunkIndex,
                TotalChunks: totalChunks,
            }
            
            data := buildTransferPacket(header, chunk[:n])
            if err := tm.tcpClient.Send(task.PeerID, data); err != nil {
                return tm.failTransfer(taskID, err)
            }
            
            task.BytesTransferred += int64(n)
            task.Speed = calculateSpeed(task)
            tm.emitEvent("transferProgress", task)
        }
        
        if readErr == io.EOF {
            break
        }
        if readErr != nil {
            return tm.failTransfer(taskID, readErr)
        }
        
        chunkIndex++
    }
    
    return tm.completeTransfer(taskID)
}
```

### 4.4 设备发现模块

```go
// internal/discovery/mdns.go

type DiscoveryManager struct {
    mu       sync.RWMutex
    peers    map[string]*Peer
    server   *mdns.Server
    ctx      context.Context
    cancel   context.CancelFunc
    onPeerFound func(*Peer)
}

func NewDiscoveryManager(port int) *DiscoveryManager {
    ctx, cancel := context.WithCancel(context.Background())
    return &DiscoveryManager{
        peers:  make(map[string]*Peer),
        ctx:    ctx,
        cancel: cancel,
    }
}

func (dm *DiscoveryManager) Start() error {
    // 启动 mDNS 广播
    info := &mdns.ServiceInfo{
        Name:   "LanFileTransfer",
        Type:   "_lanfiletransfer._tcp",
        Domain: "local.",
        Port:   9876,
        TXT:    []string{"version=1.0"},
    }
    
    server, err := mdns.New(info)
    if err != nil {
        return err
    }
    dm.server = server
    
    // 启动发现监听
    go dm.listenForPeers()
    
    return nil
}

func (dm *DiscoveryManager) listenForPeers() {
    entries := make(chan *mdns.ServiceEntry, 10)
    
    go func() {
        for entry := range entries {
            peer := &Peer{
                ID:       entry.Name,
                Name:     strings.TrimSuffix(entry.Name, "._lanfiletransfer._tcp"),
                IP:       entry.AddrV4.String(),
                Port:     entry.Port,
                Online:   true,
                LastSeen: time.Now(),
            }
            
            dm.mu.Lock()
            dm.peers[peer.ID] = peer
            dm.mu.Unlock()
            
            if dm.onPeerFound != nil {
                dm.onPeerFound(peer)
            }
        }
    }()
    
    mdns.Lookup("_lanfiletransfer._tcp", "local.", entries)
}

func (dm *DiscoveryManager) GetPeers() []*Peer {
    dm.mu.RLock()
    defer dm.mu.RUnlock()
    
    peers := make([]*Peer, 0, len(dm.peers))
    for _, p := range dm.peers {
        if time.Since(p.LastSeen) < 30*time.Second {
            peers = append(peers, p)
        }
    }
    return peers
}
```

### 4.5 Wails 后端接口

```go
// backend/cmd/main.go

type App struct {
    ctx             context.Context
    transferMgr     *transfer.TransferManager
    discoveryMgr    *discovery.DiscoveryManager
    config          *config.Config
}

func (a *App) startup(ctx context.Context) {
    a.ctx = ctx
    a.config = config.Load()
    a.transferMgr = transfer.NewTransferManager(a.config)
    a.discoveryMgr = discovery.NewDiscoveryManager(a.config.ListenPort)
    
    a.discoveryMgr.Start()
    go a.transferMgr.ProcessQueue()
}

// 前端可调用的方法
func (a *App) GetPeers() []*discovery.Peer {
    return a.discoveryMgr.GetPeers()
}

func (a *App) SendFile(peerID, filePath string) (string, error) {
    return a.transferMgr.SendFile(peerID, filePath)
}

func (a *App) CancelTransfer(transferID string) error {
    return a.transferMgr.CancelTransfer(transferID)
}

func (a *App) SelectFile() (string, error) {
    return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
        Title: "选择文件",
    })
}

func main() {
    app := &App{}
    
    err := wails.Run(&options.Options{
        Title:  "LanFileTransfer",
        Width:  1024,
        Height: 768,
        OnStartup: app.startup,
        Bind: []interface{}{
            app,
        },
    })
    
    if err != nil {
        log.Fatal(err)
    }
}
```

### 4.6 前端组件示例

```vue
<!-- frontend/src/components/TransferPanel.vue -->
<template>
  <div class="transfer-panel">
    <n-card title="传输任务">
      <n-space vertical>
        <n-button @click="handleSendFile" :loading="sending">
          发送文件
        </n-button>
        
        <n-data-table
          :columns="columns"
          :data="transfers"
          :pagination="false"
        />
      </n-space>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, h } from 'vue'
import { NButton, NCard, NSpace, NDataTable, NProgress, useMessage } from 'naive-ui'
import { SendFile, GetPeers, SelectFile } from '../../wailsjs/go/main/App'

const message = useMessage()
const transfers = ref([])
const sending = ref(false)

const columns = [
  { title: '文件名', key: 'fileName' },
  { title: '进度', key: 'progress', render: (row) => {
    return h(NProgress, {
      percentage: (row.bytesTransferred / row.fileSize) * 100,
      showIndicator: true
    })
  }},
  { title: '状态', key: 'status' },
  { title: '速度', key: 'speed', render: (row) => `${formatSpeed(row.speed)}` }
]

async function handleSendFile() {
  try {
    sending.value = true
    const filePath = await SelectFile()
    if (!filePath) return
    
    const peers = await GetPeers()
    if (peers.length === 0) {
      message.error('未发现可用设备')
      return
    }
    
    const transferId = await SendFile(peers[0].id, filePath)
    message.success('传输任务已创建')
  } catch (err) {
    message.error(`发送失败: ${err}`)
  } finally {
    sending.value = false
  }
}

function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec > 1048576) {
    return `${(bytesPerSec / 1048576).toFixed(2)} MB/s`
  }
  return `${(bytesPerSec / 1024).toFixed(2)} KB/s`
}
</script>
```

---

## 5. 网络协议设计

### 5.1 传输协议

```
┌─────────────────────────────────────────────────────────┐
│                    TCP 数据流                            │
├─────────────────────────────────────────────────────────┤
│  Header (JSON)  │  Chunk Data (Binary)                  │
│  固定长度 256B  │  变长 (默认 64KB)                     │
└─────────────────────────────────────────────────────────┘

Header 格式:
{
  "transferId": "transfer_1234567890_1234",
  "fileName": "example.txt",
  "fileSize": 1048576,
  "chunkIndex": 0,
  "totalChunks": 16,
  "checksum": "md5_hash"
}
```

### 5.2 控制消息（UDP）

```json
// 设备发现广播
{
  "type": "discovery",
  "deviceId": "device_uuid",
  "deviceName": "MyPC",
  "port": 9876,
  "timestamp": 1234567890
}

// 传输请求
{
  "type": "transfer_request",
  "transferId": "transfer_123",
  "fileName": "example.txt",
  "fileSize": 1048576
}

// 传输控制
{
  "type": "transfer_control",
  "transferId": "transfer_123",
  "command": "cancel" | "pause" | "resume"
}
```

---

## 6. 并发模型

### 6.1 Goroutine 使用策略

```
main goroutine
├── UI 事件循环 (Wails)
├── 设备发现 goroutine
│   └── mDNS 监听
├── 传输管理 goroutine
│   ├── 队列处理
│   └── 任务调度
├── TCP Server goroutine
│   ├── 连接监听
│   └── 数据传输 (每个连接一个 goroutine)
└── UDP Server goroutine
    └── 控制消息监听
```

### 6.2 Channel 设计

```go
// 传输任务队列
taskQueue := make(chan string, 100)

// 进度更新
progressChan := make(chan *TransferTask, 1000)

// 设备发现
peerFoundChan := make(chan *Peer, 50)

// 事件总线
eventChan := make(chan *Event, 500)
```

---

## 7. 错误处理

### 7.1 错误类型

```go
var (
    ErrPeerNotFound     = errors.New("peer not found")
    ErrFileNotFound     = errors.New("file not found")
    ErrTransferFailed   = errors.New("transfer failed")
    ErrConnectionLost   = errors.New("connection lost")
    ErrChecksumMismatch = errors.New("checksum mismatch")
)
```

### 7.2 重试机制

```go
func (tm *TransferManager) sendWithRetry(taskID string, data []byte, maxRetries int) error {
    for i := 0; i < maxRetries; i++ {
        if err := tm.tcpClient.Send(taskID, data); err == nil {
            return nil
        }
        time.Sleep(time.Duration(i+1) * time.Second)
    }
    return ErrTransferFailed
}
```

---

## 8. 构建与部署

### 8.1 开发环境

```bash
# 安装依赖
go mod tidy
cd frontend && npm install

# 开发模式（热重载）
wails dev

# 生产构建
wails build
```

### 8.2 交叉编译

```bash
# Windows
wails build -platform windows/amd64

# macOS
wails build -platform darwin/amd64

# Linux
wails build -platform linux/amd64
```

### 8.3 打包输出

```
build/
├── bin/
│   ├── LanFileTransfer.exe      # Windows
│   ├── LanFileTransfer          # macOS/Linux
│   └── ...
└── app/
    └── LanFileTransfer_1.0.0_x64.msi
```

---

## 9. 性能优化

### 9.1 传输优化
- 分块大小动态调整（根据网络状况）
- 并发传输限制（默认 3 个）
- 零拷贝传输（使用 `io.Copy`）

### 9.2 内存优化
- 对象池复用（sync.Pool）
- 避免频繁分配小对象
- 及时释放不用的资源

### 9.3 界面优化
- 前端虚拟列表（大数据量）
- 防抖/节流（进度更新）
- Web Worker（耗时计算）

---

## 10. 测试策略

### 10.1 单元测试

```go
func TestTransferManager_SendFile(t *testing.T) {
    mgr := NewTransferManager(testConfig)
    taskID, err := mgr.SendFile("peer1", "test.txt")
    assert.NoError(t, err)
    assert.NotEmpty(t, taskID)
}
```

### 10.2 集成测试
- 双机传输测试
- 大文件传输测试（>1GB）
- 断点续传测试

### 10.3 性能测试
```bash
go test -bench=. -benchmem ./internal/transfer
```

---

## 11. 项目里程碑

| 阶段 | 内容 | 交付物 |
|------|------|--------|
| **Phase 1** | 项目搭建、基础架构 | 项目骨架、CI/CD |
| **Phase 2** | 设备发现模块 | mDNS 发现、设备列表 |
| **Phase 3** | 文件传输核心 | 单文件传输、进度显示 |
| **Phase 4** | 多任务管理 | 并发传输、队列管理 |
| **Phase 5** | UI 完善 | 完整界面、交互优化 |
| **Phase 6** | 测试与优化 | 单元测试、性能调优 |
| **Phase 7** | 打包发布 | 安装包、文档 |

---

## 12. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|---------|
| mDNS 在某些网络不可用 | 设备发现失败 | 提供手动输入 IP 方式 |
| 大文件传输内存占用高 | 性能下降 | 流式传输、分块处理 |
| Wails 打包体积大 | 分发困难 | 优化依赖、压缩资源 |
| 跨平台兼容性问题 | 部分平台不可用 | 充分测试、条件编译 |

---

## 13. 参考资料

- [Wails 官方文档](https://wails.io/)
- [Go 标准库文档](https://pkg.go.dev/)
- [Vue 3 文档](https://vuejs.org/)
- [Naive UI 文档](https://www.naiveui.com/)
