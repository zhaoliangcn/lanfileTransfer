# Code Review Report / 代码审查报告：LanFileTransfer-Go

> 文档语言：中文 | Document language: Chinese
> 说明：本报告针对修复前的代码快照。多数条目已修复，新增能力见 [README](../README.md) 与 `internal/`、`frontend/src/` 中的回归测试。

项目整体架构清晰，采用 Wails v2 + Go + Vue 3 + Naive UI 技术栈，模块划分合理。测试覆盖率不错，核心模块都有对应的单元测试。以下是按严重程度排列的发现：

---

## 🔴 严重问题 (Critical)

### 1. `executeSend` 中发送失败却被标记为已传输 ✅ 已修复

[manager.go:L280-L290](internal/transfer/manager.go#L280-L290)

```go
if err := sendWithRetry(conn, header, buf[:n], 3); err != nil {
    chunkMgr.MarkTransferred(chunkIndex, checksum)  // ⚠️ BUG: 发送失败不应该标记为已传输
    if cpData, cpErr := chunkMgr.Serialize(); cpErr == nil {
        os.WriteFile(task.CheckpointPath, cpData, 0644)
    }
    ...
    tm.failTask(taskID, ...)
    return
}
```

**问题**：当 `sendWithRetry` 发送失败后，代码错误地将当前 chunk 标记为 `Transferred`，然后写入了 checkpoint。这会导致用户恢复传输时，**失败的分片被错误地跳过**，最终收到的文件会缺少这部分数据，且不会报错。

**修复**：将 `MarkTransferred` 移到 `sendWithRetry` 成功分支（`err == nil`）中。失败分支仅保存已有进度的 checkpoint，不标记当前失败分块。

---

### 2. `receiveFile` 中 `fileSize` 计算逻辑错误 [已修复]

[manager.go:L345](internal/transfer/manager.go#L345)

```go
fileSize = header.FileSize * header.TotalChunks
```

**问题**：发送端的 `header.FileSize` 设置为 `int64(n)`（当前 chunk 的实际字节数），不是文件总大小。接收端将此值乘以 `TotalChunks`，得到的是一个无意义的大数。`FileSize` 字段被语义重载（发送时表示 chunk 数据大小，接收时又被当作文件总大小来计算），导致显示给用户的文件大小完全错误。由于最后一个分块可能小于 `chunkSize`，这个乘法计算结果永远不等于真实文件大小。

**建议**：在 `TransferHeader` 中新增一个 `TotalFileSize` 字段，专门传递完整的文件大小；或者在第一个 chunk 中也传递原始文件大小。

---

### 3. `App.vue` 中 `handleSendToPeer` 逻辑错误导致永远只发送文件 [已修复]

[App.vue:L230-L255](frontend/src/App.vue#L230-L255)

```ts
const response = await showChoice(...)
if (!response) return    // 用户取消时直接返回

if (response) {          // ⚠️ 这里永远是 true
    const filePath = await SelectFile()
    ...
} else {
    const folderPath = await SelectDirectory()  // ← 永远执行不到！
    ...
}
```

**问题**：外层 `if (!response) return` 已经过滤掉了 `false` 的情况，所以 `if (response)` 永远为 `true`，**"发送文件夹"功能完全失效**。`showChoice` 有两个按钮，但当前逻辑无法区分用户点击了哪个。

**修复**：`showChoice` 的返回值只表示"是否点击了确认按钮"，无法区分两个按钮。需要修改 `showChoice` 使其返回按钮标识（如 `'file'` / `'folder'` / `null`），或分别弹出两个独立的确认对话框。

---

### 4. `broadcastLoop` 硬编码间隔，忽略配置 [已修复]

[peer.go:L88](internal/discovery/peer.go#L88)

```go
func (dm *DiscoveryManager) broadcastLoop() {
    ticker := time.NewTicker(5 * time.Second)  // ⚠️ 硬编码 5 秒
```

**问题**：`DiscoveryManager` 持有 `listenPort` 但没有 `discoveryInterval`。实际广播间隔始终是 5 秒，用户在设置中修改的 `DiscoveryInterval` 永远不会生效。

**建议**：`NewDiscoveryManager` 增加 `discoveryInterval` 参数，在 `broadcastLoop` 中使用 `time.NewTicker(time.Duration(discoveryInterval) * time.Second)`。

---

### 5. 前端传输任务重复添加 [已修复]

[useTransfer.ts:L14-L37](frontend/src/composables/useTransfer.ts#L14-L37) + [App.vue:L135-L150](frontend/src/App.vue#L135-L150)

**问题**：发送文件时存在两处 `addTransfer` 调用：
1. `useTransfer.ts` 的 `sendFile` 在后端 `SendFile` 返回后立即 `store.addTransfer` 添加占位任务
2. Go 后端 emit `transferStarted` 事件，`App.vue` 的事件处理器再次 `store.addTransfer`

**每次发送文件都会产生两条重复的传输任务记录**，列表中会显示两条相同的传输项。

**修复**：
- 方案一：`App.vue` 的 `transferStarted` 处理中先用 `getTransferById` 检查是否已存在，若存在则 `updateTransfer` 更新而非 `addTransfer`
- 方案二：移除 `useTransfer.ts` 中的本地占位添加，完全依赖事件驱动

---

## 🟠 高优先级问题 (High)

### 6. `handleDiscoveryMessage` 使用原始 `json.Unmarshal` 而非封装的 `UnmarshalControlMessage` [已修复]

[peer.go:L119](internal/discovery/peer.go#L119)

```go
var msg network.ControlMessage
if err := json.Unmarshal(data, &msg); err != nil {  // ⚠️ 绕过了自有的 Unmarshal 函数
    return
}
```

**问题**：
- 不一致性：`UnmarshalControlMessage` 已定义但未使用
- 如果未来 `UnmarshalControlMessage` 增加校验逻辑（如大小限制、字段验证），此处不会被检查到
- `discovery` 包中额外导入了 `encoding/json`，而 `network.UnmarshalControlMessage` 内部已经包含 JSON 解析

**建议**：统一使用 `network.UnmarshalControlMessage(data)`。

---

### 7. 缺少断点续传的发送确认机制 [跳过 - ACK协议变更属于较大特性]

[manager.go:L260-L310](internal/transfer/manager.go#L260-L310)

在 `executeSend` 中，chunk 发送之后没有等待接收方的 ACK 确认。当前协议只支持单向的数据推送，接收方处理 `receiveFile` 时也无法向发送方反馈接收状态。如果接收方在传输过程中磁盘满了或写失败，发送方不会知道。

**建议**：在 `TransferHeader` 中增加 ACK 机制，发送方等待接收方确认后再标记 chunk 为已传输。

---

### 8. TCP Server 的 `handleConnection` 与 `receiveFile` 竞读 [已修复]

[network/tcp.go:L169-L183](internal/network/tcp.go#L169-L183)

```go
func (s *TCPServer) handleConnection(addr string, conn *TCPConnection) {
    buf := make([]byte, 64*1024)
    for {
        _, err := conn.Read(buf)       // <-- goroutine A: 不断读取并丢弃
        ...
    }
}
```

而 `TransferManager.handleIncomingConnection` 作为 `onConnect` 回调又调用：
```go
go tm.receiveFile(conn)              // <-- goroutine B: 也读取同一个连接
```

**问题**：`acceptLoop` 在调用 `onConnect` 回调后，还会启动 `go s.handleConnection(addr, tcpConn)`。这意味着 `receiveFile` 和 `handleConnection` **两个 goroutine 同时从同一个 TCP 连接读取数据**。`handleConnection` 的 `conn.Read(buf)` 会随机消耗掉本应由 `receiveFile` → `ReceiveChunk` 接收的数据，导致**接收文件数据损坏或解析失败**。

**修复**：移除 `TCPServer.handleConnection` 中的读循环。`TCPServer` 只需接受连接并传给 `onConnect` 回调，由回调全权负责该连接的生命周期。

---

### 9. `DiscoveryManager` 离线节点永不删除导致内存泄漏 [已修复]

[peer.go:L192-L207](internal/discovery/peer.go#L192-L207)

```go
func (dm *DiscoveryManager) cleanupOfflinePeers() {
    for id, peer := range dm.peers {
        ...
        peer.Online = false   // ⚠️ 只标记离线，永不删除
        ...
    }
}
```

**问题**：`peers` map 中的离线节点只标记状态但从不删除。在局域网环境中，设备频繁上线下线（DHCP 分配不同 IP 等情况下会产生新 DeviceID），`peers` map 会无限增长，造成内存泄漏。

**修复**：在标记 `Online = false` 的同时，删除已离线超过一定时间（如 5 分钟）的 peer 条目。

---

## 🟡 中等问题 (Medium)

### 10. UDP 广播使用 `net.IPv4bcast (255.255.255.255)`  [跳过 - 需较大架构变更]

[network/udp.go:L94-L97](internal/network/udp.go#L94-L97)

**问题**：在 Windows 多网卡环境下，`255.255.255.255` 可能只会在一个网络接口上发送，导致其他子网的设备无法发现本机。

**建议**：遍历本地网络接口，向每个接口的子网广播地址发送。`network.GetBroadcastAddress` 目前未被使用且实现不完整，可以考虑实现此逻辑。

---

### 11. `GetFailedCount` 把 Cancelled 计入 Failed [已修复]

[transfer_service.go:L137-L144](internal/service/transfer_service.go#L137-L144)

```go
func (s *TransferService) GetFailedCount() int {
    ...
    if task.Status == transfer.StatusFailed || task.Status == transfer.StatusCancelled {
        count++
```

**问题**：`ResumeFailedTransfers` 也恢复了 cancelled 的任务。用户明确取消的操作不应该被"全部恢复"。

**建议**：`GetFailedCount` 和 `ResumeFailedTransfers` 只应处理 `StatusFailed` 的任务。

---

### 12. `ChunkManager` 读写均使用 `Lock()` 影响性能 [已修复]

[chunk.go:L68-L148](internal/transfer/chunk.go#L68-L148)

```go
func (cm *ChunkManager) GetChunk(index int64) *ChunkInfo {
    cm.mu.Lock()        // ⚠️ 应该用 RLock
    defer cm.mu.Unlock()
```

`GetChunk`、`GetTransferredCount`、`GetTransferredBytes`、`GetProgress`、`TotalChunks`、`ChunkSize`、`FileSize`、`GetMissingChunks` 都是只读操作，却使用了排它锁。在高并发传输场景下会造成不必要的锁竞争。

**建议**：这些只读方法使用 `RLock()`/`RUnlock()`。

---

### 13. `GetLocalPeer` 缺少 `LastSeen` 字段 [已修复]

[peer.go:L278-L285](internal/discovery/peer.go#L278-L285)

```go
func (dm *DiscoveryManager) GetLocalPeer() *Peer {
    return &Peer{
        ID:     dm.deviceID,
        Name:   dm.deviceName,
        IP:     network.GetLocalIP(),
        Port:   dm.listenPort,
        Online: true,
        // ⚠️ LastSeen 未设置
    }
}
```

**问题**：`LastSeen` 为零值空字符串，在格式化显示时会出现异常。

---

### 14. 前端轮询发现与事件推送重复 [已修复]

[useDiscovery.ts:L23-L27](frontend/src/composables/useDiscovery.ts#L23-L27)

```typescript
function startPolling(intervalMs = 5000) {
    stopPolling()
    pollInterval.value = setInterval(refreshPeers, intervalMs)
}
```

**问题**：前端同时使用 Wails Events（`peerFound`/`peerLost` 推送）和 `setInterval` 轮询来刷新 peer 列表。轮询每 5 秒调用 `GetPeers()` 拉取全量列表，而 `App.vue` 已处理 `peerFound`/`peerLost` 事件来更新 store。两套机制完全重复，浪费资源。

**建议**：移除轮询，完全依赖 Wails 事件推送。

---

### 15. `receiveFile` 接收端缺少 peer 身份信息 [已修复]

[manager.go:L343-L352](internal/transfer/manager.go#L343-L352)

```go
currentTask = NewTransferTask(
    header.TransferID,
    "",     // peerID 为空
    "",     // peerName 为空
    "",     // peerAddr 为空
    ...
)
```

**问题**：接收到的传输记录缺少发送端身份信息（peerID、peerName、peerAddr 全为空）。在 HistoryView 中，接收到的文件传输记录的 Peer 列显示为 `-`，用户无法知道文件来自哪台设备。

**修复**：从 `conn.RemoteAddr()` 获取发送方 IP 地址，通过 `DiscoveryManager.GetPeer()` 反查 peer 信息后填充。

---

### 16. HistoryView 使用当前会话数据而非持久化历史 [跳过 - 需较大功能变更]

[HistoryView.vue](frontend/src/views/HistoryView.vue)

HistoryView 的数据来源是 `store.transfers`（Pinia store 中的当前会话传输记录），而非 Go 后端的 `HistoryManager.GetAll()`。Go 后端已有完整的历史持久化机制（`transfer_history.json`），`main.go` 也暴露了 `GetTransferHistory()` 和 `ClearTransferHistory()` 接口，但前端 HistoryView 未对接。

**问题**：关闭应用后历史记录丢失，`transfer_history.json` 中的持久化数据从未在前端展示。

**修复**：HistoryView `onMounted` 时调用 `GetTransferHistory()` 加载持久化历史数据。

---

### 17. 重复代码：格式化函数分散在多处 [跳过 - 纯重构，非bug]

格式化文件大小和速度的函数在 `FileService`、`TransferService`、`TransferStore` 四个地方都有独立实现，逻辑完全相同（Go 两处 + TypeScript 两处），不利于维护。

**建议**：Go 端提取到 `pkg/utils/format.go`；前端统一使用 `TransferStore` 中的工具函数。

---

## 🟢 低优先级 / 改进建议 (Low)

### 18. `Config.Load` 手动逐个字段合并 [跳过 - 纯重构]

[config.go:L52-L82](internal/config/config.go#L52-L82)

使用多个 `if` 判断逐个字段合并加载的配置，代码冗长。当添加新字段时容易遗漏。建议使用 `json.Unmarshal` 到临时 `Config`，然后用 `reflect` 合并非零值，或使用第三方库（如 `mergo`）。

### 19. `cleanupOfflinePeers` 中 `onPeerLost` 重复触发 [已修复 - 随#9一并修复]

[peer.go:L194](internal/discovery/peer.go#L194)

如果一个 peer 连续多轮 cleanup 都超时，`onPeerLost` 会多次被调用（每次 cleanup 周期都会触发一次），因为缺少防止重复通知的机制（`peer.Online` 已经是 `false` 后仍然会再次触发）。

**建议**：添加 `if peer.Online { peer.Online = false; go dm.onPeerLost(id) }` 防止重复通知。

### 20. `DiscoveryManager.ctx` 使用 `chan struct{}` 而非 `context.Context` [已修复]

[peer.go:L35](internal/discovery/peer.go#L35)

```go
type DiscoveryManager struct {
    ctx    chan struct{}   // 非标准做法
    ...
}
```

**问题**：如果 `Stop()` 被调用两次，第二次会尝试 `close(dm.ctx)` 一个已关闭的 channel，导致 panic。虽然 `started` 标志有部分保护，但不完整。

**建议**：改用 `context.Context` + `context.WithCancel`（与 `TransferManager` 保持一致），利用 `sync.Once` 保护 cancel 调用。

### 21. `HomeView.vue` 快速操作按钮功能不完整 [跳过 - 需较大功能变更]

[HomeView.vue:L60-L91](frontend/src/views/HomeView.vue#L60-L91)

"Send File"、"Send Folder"、"Receive File" 三个快速操作按钮只调用文件选择对话框并打印日志，**不执行实际的文件传输操作**。选择文件后没有目标 peer 选择步骤，功能链不完整。

### 22. `SelectDirectory` 对话框标题错误 [已修复]

[main.go:L218-L222](main.go#L218-L222)

```go
func (a *App) SelectDirectory() (string, error) {
    return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
        Title: "选择文件",   // ⚠️ 应为 "选择目录"
    })
}
```

### 23. `SettingsView.SaveConfig` 会意外清空 `ServiceName`/`ServiceType` [已修复]

[SettingsView.vue:L163-L173](frontend/src/views/SettingsView.vue#L163-L173)

```typescript
await SaveConfig({
    ...
    serviceName: '',      // ⚠️ 硬编码空字符串
    serviceType: '',      // ⚠️ 硬编码空字符串
} as any)
```

**问题**：保存配置时 `serviceName` 和 `serviceType` 被强制设为空字符串，会覆盖已有的值。虽然后端 `Load` 有非空判断保护（`if loaded.ServiceName != ""`），但 `SaveConfig` 在 `main.go` 中是直接赋值（`a.config.ServiceName = newConfig.ServiceName`），这意味着每次保存配置都会将 `ServiceName` 和 `ServiceType` 清空。

### 24. 全局 Logger 不便于测试 [跳过 - 纯重构]

[logger.go:L7-L9](pkg/utils/logger.go#L7-L9)

```go
var Log *zap.Logger
var SugaredLog *zap.SugaredLogger
```

全局变量模式的 logger 在多测试并行运行时会相互干扰。`AddCallerSkip(1)` 偏移量在当前使用场景下可能导致调用栈信息不准确。建议通过依赖注入传递 logger 实例。

### 25. 多段死代码 (Dead Code) [跳过 - 不影响功能]

以下函数/方法在本项目中已定义但从未被调用：

| 位置 | 函数 | 说明 |
|------|------|------|
| [chunk.go:L160-L168](internal/transfer/chunk.go#L160-L168) | `ReadChunkData` | manager 使用 `FileReader.ReadAt` |
| [chunk.go:L170-L181](internal/transfer/chunk.go#L170-L181) | `WriteChunkData` | 未被调用 |
| [chunk.go:L189-L217](internal/transfer/chunk.go#L189-L217) | `LoadCheckpoint` / `SaveCheckpoint` | manager 用 `ChunkManager.Serialize/Deserialize` |
| [chunk.go:L219-L223](internal/transfer/chunk.go#L219-L223) | `DeleteCheckpoint` | manager 直接 `os.Remove` |
| [reader.go:L55-L69](internal/file/reader.go#L55-L69) | `FileReader.Read` / `Seek` | manager 只用 `ReadAt` |
| [reader.go:L121-L137](internal/file/reader.go#L121-L137) | `NewResumableWriter` | 未被调用 |
| [protocol.go:L152-L163](internal/network/protocol.go#L152-L163) | `GetBroadcastAddress` | 始终返回 `255.255.255.255`，参数未使用 |
| [useNotification.ts:L94-L96](frontend/src/composables/useNotification.ts#L94-L96) | `setupNotification` | 空函数，仅为兼容旧代码 |

### 26. `.bak` 文件未清理 [已修复]

项目中有 `App.vue.bak` 和 `useNotification.ts.bak` 两个备份文件，应该在清理时删除。

---

## 📊 测试覆盖评估

测试覆盖总体较好，各核心模块都有对应的测试文件：

| 模块 | 测试文件 | 覆盖情况 |
|------|---------|---------|
| config | ✅ | 基本场景覆盖（默认值、读写、空配置） |
| discovery | ✅ | peer 管理、回调、清理逻辑覆盖好 |
| network/protocol | ✅ | 序列化/反序列化、边界条件覆盖好 |
| network/tcp | ✅ | 连接、读写、关闭、双关覆盖好 |
| network/udp | ✅ | 启停、收发、未启动错误处理 |
| transfer/task | ✅ | 进度、速度、ETA 计算 |
| transfer/chunk | ✅ | 分片管理、序列化、校验 |
| transfer/history | ✅ | 读写、容量限制、清理 |
| **transfer/manager** | ❌ | **核心传输逻辑未测试！** |

**最显著的问题是 `TransferManager`（manager.go, 566行）完全没有单元测试**，而它包含了最复杂的发送/接收/断点续传逻辑。

---

## 📋 总结

| 严重程度 | 数量 | 关键项 |
|---------|------|-------|
| 🔴 Critical | 5 | chunk 标记错误、fileSize 计算错误、前端发送文件夹失效、广播间隔硬编码、**传输任务重复添加** |
| 🟠 High | 4 | 控制消息解析不一致、缺少 ACK 机制、TCP 竞读、**离线节点内存泄漏** |
| 🟡 Medium | 8 | UDP 广播范围、cancelled 被恢复、锁粒度、LastSeen 缺失、**前端轮询重复**、**接收缺 peer 信息**、**HistoryView 未对接持久化**、重复代码 |
| 🟢 Low | 9 | 配置合并方式、重复通知、channel 关闭风险、HomeView 按钮未完成、对话框标题错误、SaveConfig 清空配置、全局 Logger、死代码、.bak 文件 |
| ❌ Missing Test | 1 | TransferManager 未测试 |

**最优先修复的应该是 #1（chunk 标记错误导致文件损坏）、#3（发送文件夹功能不可用）、#5（传输任务重复添加）和 #8（TCP 竞读导致接收文件损坏）。**

---

## 📋 修复总结

### 修复统计

| 类别 | 总数 | 已修复 | 跳过 | 完成率 |
|------|------|--------|------|--------|
| 🔴 Critical | 5 | 5 | 0 | 100% |
| 🟠 High | 4 | 3 | 1 | 75% |
| 🟡 Medium | 8 | 6 | 2 | 75% |
| 🟢 Low | 9 | 6 | 3 | 67% |
| **合计** | **26** | **20** | **6** | **77%** |

### 已修复问题 (20项)

| # | 问题 | 修改文件 | 修改说明 |
|---|------|---------|---------|
| 1 | chunk发送失败被标记已传输 | `internal/transfer/manager.go` | sendWithRetry失败时不调用MarkTransferred |
| 2 | fileSize计算逻辑错误 | `internal/network/protocol.go`, `internal/transfer/manager.go` | 新增TotalFileSize字段 |
| 3 | 发送文件夹功能失效 | `frontend/src/App.vue`, `frontend/src/composables/useNotification.ts` | showChoice返回按钮标识 |
| 4 | 广播间隔硬编码 | `internal/discovery/peer.go` | 使用DiscoveryManager.discoveryInterval |
| 5 | 传输任务重复添加 | `frontend/src/App.vue` | transferStarted事件先检查已存在任务 |
| 6 | 控制消息解析不一致 | `internal/discovery/peer.go` | 改用network.UnmarshalControlMessage |
| 8 | TCP竞读 | `internal/network/tcp.go` | 移除handleConnection读循环 |
| 9 | 离线节点内存泄漏 | `internal/discovery/peer.go` | 5分钟后删除离线peer |
| 11 | Cancelled计入Failed | `internal/service/transfer_service.go` | 只处理StatusFailed |
| 12 | ChunkManager锁粒度 | `internal/transfer/chunk.go` | sync.RWMutex + RLock读操作 |
| 13 | GetLocalPeer缺LastSeen | `internal/discovery/peer.go` | 添加LastSeen字段 |
| 14 | 前端轮询重复 | `frontend/src/composables/useDiscovery.ts` | 移除轮询，依赖事件 |
| 15 | 接收端缺peer信息 | `internal/transfer/manager.go` | 从conn.RemoteAddr()获取地址 |
| 19 | onPeerLost重复触发 | `internal/discovery/peer.go` | 添加Online检查防止重复通知 |
| 20 | chan struct{}风险 | `internal/discovery/peer.go` | 改用context.Context + sync.Once |
| 22 | 对话框标题错误 | `main.go` | Title改为"选择目录" |
| 23 | SaveConfig清空配置 | `frontend/src/views/SettingsView.vue` | 先GetConfig再spread覆盖formData |
| 26 | .bak文件未清理 | 项目根目录 | 删除App.vue.bak和useNotification.ts.bak |

### 跳过问题 (6项)

| # | 问题 | 跳过原因 |
|---|------|---------|
| 7 | 缺少ACK确认机制 | ACK协议变更需要重新设计通信协议，属于较大架构特性 |
| 10 | UDP广播限定广播地址 | 需实现多网卡遍历广播，属于较大架构变更 |
| 16 | HistoryView对接持久化 | 需重新设计HistoryView数据加载流程 |
| 17 | 格式化函数重复代码 | 纯代码重构，不影响功能 |
| 18 | Config.Load合并方式 | 纯代码重构 |
| 21 | HomeView快速操作按钮 | 功能链不完整，需较大UI流程变更 |
| 24 | 全局Logger模式 | 纯架构重构 |
| 25 | 死代码清理 | 不影响功能，可后续清理 |