# 性能优化方法论 · Performance Methodology (LanFileTransfer-Go)

> 文档语言：中文 | Document language: Chinese
> 状态：记录 13 → 111 MB/s 两轮优化的完整过程。第 2 节的方法论与具体数值解耦，可迁移到其他项目；第 3 节的数值以当前代码为准。
> 相关：[ARCHITECTURE.md](ARCHITECTURE.md) §9（规划中的优化项）、[CODE_REVIEW.md](CODE_REVIEW.md)

---

## 1. 结果摘要

| 阶段 | 千兆 LAN 实测 | 主导瓶颈 | 修复 |
|---|---|---|---|
| 起点 | 平均 **13 MB/s** | checkpoint 每 10 块 `json.Marshal` 整张 chunk 表（O(F²)） | 位图 checkpoint |
| 第一轮后 | 平均 **43 MB/s** | 热循环每块遍历整张 chunk 表（O(F²)）＋ checkpoint 每次重新打开文件 | O(1) 计数器 ＋ 持续句柄 |
| 现在 | 最高 **111 MB/s** | 网络本身（千兆实际上限 113–117 MB/s，已达 95–98%） | — |

两轮的根因是**同一类：藏在「每块都要调一次」的函数里的隐藏 O(F²)**。它们的单次成本随文件增大而增大，因此小文件测试永远测不出来 —— 这是本次最大的教训。

> 口径说明：起点与第一轮是多次传输的**平均值**，111 MB/s 是**峰值**。由于 111 已贴近千兆实际上限，平均值应当也在 100+ 量级，但两者口径不同，不宜直接相除计算提升倍数。

---

## 2. 方法论

### 2.1 第 1 步 · 把「慢」换算成「每块预算」

原始观察「43 MB/s」无法直接指导优化 —— 它没告诉你时间花在哪。第一步是把它换算成单次迭代的时间预算：

```
目标吞吐 ÷ 分块大小 = 每块预算

43 MB/s ÷ 64 KiB = 656 块/s → 1.52 ms/块
```

然后列出**已知成本**，剩下的残差就是搜索范围：

| 每块成本项 | 实测 | 占比 |
|---|---|---|
| MD5 校验 `GenerateChunkChecksum` | 87 µs | ≈6% |
| checkpoint 落盘（每 10 块一次，摊薄） | 130 µs | ≈9% |
| **已解释** | **0.22 ms** | **≈14%** |
| **未解释（搜索范围）** | **1.30 ms** | **≈86%** |

**这一步的价值**：把「慢」变成「有 1.30 ms 不知道花在哪」。搜索范围一旦确定，后面就是纯粹的定位工作。

> 通用公式：`每块预算(ms) = 分块大小(B) ÷ 吞吐(B/s) × 1000`
> 本项目统一按十进制 MB（10⁶ B）换算，与 3.2 节的 1.52 ms ↔ 43 MB/s、0.62 ms ↔ 105 MB/s 口径一致。

### 2.2 第 2 步 · 用测量排除假设，而不是靠直觉

「感觉是磁盘慢」「感觉是 MD5 慢」这类直觉在本项目里连续错了两次。排除必须带测量证据：

| 假设 | 排除依据 | 测量手段 | 结果 |
|---|---|---|---|
| MD5 是瓶颈 | 单独基准 | `BenchmarkGenerateChunkChecksum` | 635–749 MB/s；千兆下每端仅占 15–18% 单核 |
| 磁盘慢 | 裸盘顺序读写 | 顺序写/读 2 GB | 写 882 MB/s、读 1278 MB/s |
| 磁盘是网络共享 | 盘符类型 | `Win32_LogicalDisk.DriveType` | `3` = 本地；`Get-PSDrive` 无 `DisplayRoot` |
| 页缓存装不下 | 内存容量 | `Win32_OperatingSystem` | 31.6 GB，空闲 16.4 GB（4+4 GB 轻松容纳） |
| 收发同盘争用 | 分区所属物理盘 | `Get-Partition.DiskNumber` | C: 在 disk 1、D: 在 disk 0，两块独立 SSD |
| 协议/序列化慢 | 纯协议基线 | 只跑 `BuildTransferPacket` + `ParseTransferPacket` | 323–466 MB/s |

**规则：没有测量的「排除」等于没排除。** 反过来，没有排除就上 profile，很容易把时间浪费在本来就没问题的地方。

### 2.3 第 3 步 · 建三条基线，分离代码与环境

单点数字无法区分「代码慢」还是「环境慢」。并排跑三条：

| 基线 | 测的是什么 | 本项目实测 |
|---|---|---|
| **纯协议**（无 MD5 / 文件 IO / 事件） | 序列化 + TCP 的上限 | 323–466 MB/s |
| **同机 loopback 完整链路** | 代码本身的上限 | 128MB: 53–79 → 134–204 MB/s |
| **真实 LAN** | 最终交付目标 | 13 → 43 → 111 MB/s |

三者的关系本身就是诊断信息：

- 纯协议 ≫ loopback → 瓶颈在传输逻辑（MD5 / checkpoint / 事件 / IO）
- loopback ≫ LAN → 瓶颈在网络或对端
- loopback 在大文件上显著下滑 → 存在随文件规模增长的成本

⚠️ **loopback 与 LAN 不可直接比较**：loopback 是**同一台机器同时跑收发两端**（2× MD5 ＋ 2× 文件 IO 挤一个 CPU），LAN 是两台机器各干各的。本项目 4GB loopback 只有 61.5 MB/s 而 LAN 有 111 MB/s，正是这个原因，不是回归。

### 2.4 第 4 步 · 对热循环做 O(F²) 检查

这是两轮 bug 的共同特征，也是最可复用的一条检查规则：

> **对热循环里每次迭代调用的每个函数，问一句：它的工作量是否随文件大小 F 增长？**

若热循环每块执行一次、单次成本 O(F)，则整个传输是 O(F²)。这类 bug 有三个特征，可以用来快速识别：

1. **只在大文件上出现** —— F 小时可忽略，测试和短传输永远看不到
2. **吞吐随文件变大而下降** —— 与「网络上限」的直觉相反
3. **藏在语义无辜的读接口里** —— `GetProgress()` 看起来只是读一个百分比

```bash
# 定位热循环后，对被调用的函数扫一遍全表访问
grep -nE 'range cm\.chunks|len\(cm\.chunks\)|for .* range .*(chunks|received|tasks)' \
  internal/transfer/*.go
```

本项目命中的两处：

| 位置 | 问题 | 单次成本（4 GB / 16 GB 文件） |
|---|---|---|
| `chunk.go` `GetTransferredBytes` + `GetProgress` | 每块全表扫描，且 `GetProgress` 内部又调一次前者，等于扫两遍 | 762 µs / 2,516 µs |
| `manager.go` `writeCheckpoint` → `os.WriteFile` | 每 10 块重新 `OpenFile` + `Write` + `Close` | 1.05 ms/次（非 O(F)，但固定成本过高） |

第 1 处是真 O(F²)；第 2 处是**固定成本**，但因为调用频率高（每 640 KB 一次），累积起来同样可观。**两类要一起看**：一个管规模，一个管单价。

### 2.5 第 5 步 · 用 profile 定位，但先剔除测量污染

`go test -cpuprofile` 出来的第一版 profile 里，`runtime.cgocall` 占 57%、`warmFile` 占 13% —— 后者是**测试夹具自己**在读 128 MB 预热文件，与被测代码无关。直接看会得出错误结论。

三个容易踩的读法问题：

1. **`-top -cum` 是按累计时间排序的扁平列表，不是调用树** —— 相邻两行没有父子关系，不能顺着缩进读
2. **`-peek <symbol>` 才给调用者/被调用者归因** —— 要回答「谁在调 `os.WriteFile`」必须用它
3. **夹具代码要排除** —— 预热读、建源文件、TempDir 清理都该在取样区间之外

取样区间外的做法：把 profile 的 start/stop 包在被测调用周围，而不是包在整个测试函数上。

### 2.6 第 6 步 · benchmark 给数字，test 给防线

两者职责不同，缺一不可：

**benchmark 出量化证据**（决定「值不值得修」）：

```bash
go test ./internal/transfer -run XXX -bench . -benchtime 300x
```

| 基准 | 修复前 | 修复后 | 倍数 |
|---|---|---|---|
| `BenchmarkChunkStatsPerChunk/4GB` | 762,354 ns/op | **46 ns/op** | 16,573× |
| `BenchmarkChunkStatsPerChunk/16GB` | 2,515,694 ns/op | **69 ns/op** | 36,459× |
| `BenchmarkChunkStatsOneScan`（16 GB） | 1,320,329 ns/scan | **24 ns/scan** | 55,013× |
| `BenchmarkWriteCheckpoint` | 1,524,147 ns/op | **531,130 ns/op** | 2.9× |
| `BenchmarkCkptWriteFile` vs `BenchmarkCkptPersistentHandle` | 1,108,776 ns | **138,043 ns** | 8.0× |

**test 做回归防线**，但**必须是确定性的**。这里踩过一个反例：

```go
// ✗ 反例：紧时间断言，CI 上必 flaky
if warm > 2*time.Millisecond { t.Error(...) }
// 实测本机 2.53 ms 就超了，换台机器更不可控
```

两种可靠写法，本项目两种都用了：

```go
// ✓ 写法 A：给巨大裕度的规模测试 —— 线性实现在数学上不可能通过
// O(1) 实现约 20 ms；线性实现 200,000 × 2 × 200,000 次迭代 ≈ 120 s
// 2 s 阈值对 O(1) 有 100× 裕度，对线性实现差 60× 无法通过
TestChunkStatsCostDoesNotScaleWithFileSize   // chunk_test.go:489

// ✓ 写法 B：断言结构而非时间 —— 完全不涉及时钟
checkpointMu.Lock()
_, keptOpen := checkpointFiles[path]
checkpointMu.Unlock()
if !keptOpen { t.Error("句柄未复用，每 10 块都要重新打开文件") }
TestCheckpointWriteCostIsBounded             // chunk_test.go:364
```

**原则：时间断言只用来「抓住数量级回归」，不用来「验证微优化」。** 后者交给 benchmark。

### 2.7 第 7 步 · 在目标规模复验

本次最关键的一步，也是最容易被跳过的一步。

| 文件 | F（chunk 数） | 每块全表扫描成本 |
|---|---|---|
| 128 MB（原 loopback 测试） | 2,048 | 0.04 ms —— **可忽略，测不出问题** |
| 4.5 GB（真实场景） | 68,800 | **0.80 ms —— 占预算 53%** |

**修复前的 loopback 测试通过了，用户实测却只有 43 MB/s —— 因为测试规模不对。**

做法：加一个由环境变量门控的大文件测试，默认跳过，CI 不受影响：

```bash
LFT_LARGE_MB=4096 go test ./internal/transfer -run TestLoopbackThroughputLargeFile -v
```

`TestLoopbackThroughputLargeFile`（`loopback_perf_test.go:244`）在修复后 4 GB 实测 61.5 MB/s；同机收发的解释见 2.3。

### 2.8 第 8 步 · 确认瓶颈已转移，然后停手

优化有终点。判断标准是**残差是否已落到不可控的外部因素上**：

```
111 MB/s ÷ (113–117 MB/s)（千兆实际上限） = 95–98%
```

当吞吐贴近传输介质的物理上限时，继续优化代码不会有收益。本项目明确**决定不做**的事：

| 项目 | profile 占比 / 成本 | 不做的理由 |
|---|---|---|
| MD5 换更快的算法或去掉 | 635–749 MB/s，千兆下占单核 15–18% | 需要改协议，收益为 0（网络已满） |
| `BuildTransferPacket` 的 64 KB 额外分配 | 2.53% | 不是主导项 |
| 每块 `emitEvent` 推送到 WebView | 未单独量化 | 111 MB/s 下已达上限，量化也无处可用 |
| C 接收端每块 `fopen`/`fclose` | 未测 | 对端未报瓶颈，且 LAN 已满 |

**「不做」和「做」同样是方法论的一部分** —— 没有第 1 步的预算分解和第 8 步的上限核对，就无法有依据地停手。

---

## 3. 案例复盘

### 3.1 第一轮：13 → 43 MB/s（checkpoint 位图化）

**症状**：4.5 GB 文件在千兆 LAN 上只有约 13 MB/s，且吞吐随文件增大而**下降**。

**定位**：`ChunkManager.Serialize` 是 `json.Marshal(cm.chunks)`，整张表约 115 字节/chunk，发送循环每 10 块（640 KB）重写一次 —— 每传 640 KB 就要 marshal + 落盘 7.9 MB：

| 文件 | 表大小 | 单次 checkpoint | 吞吐上限 |
|---|---|---|---|
| 64 MB | 110 KB | 6 ms | 102 MB/s |
| 1 GB | 1.8 MB | 19 ms | 33 MB/s |
| 4 GB | 7.1 MB | 112 ms | 5.6 MB/s |
| 16 GB | 28.7 MB | 239 ms | 2.6 MB/s |

**修复**：恢复续传只需「这块传没传」一个 bit。改为 `[4B magic][8B chunk count][ceil(F/8) 字节]` 位图。

- 4.5 GB 的 checkpoint：7.9 MB → **9,228 字节**
- 单次落盘：130 ms → **1.33 ms**，吞吐上限 → **469 MB/s**
- 旧格式保持可读，并补上原代码缺失的 chunk 数校验

**附带修复**：`Deserialize` 的返回值原本被丢弃，损坏/不匹配的 checkpoint 会被静默信任并跳块续传。

**关键决策**：**不做 temp+rename**。Windows 上新建文件约 5 ms、覆盖约 1 ms，走 temp+rename 单是建文件就足以把吞吐压到 ~90 MB/s。改为直接写目标文件，代价是进程被杀会留下短文件 —— 而 `Deserialize` 恰好会拒绝长度不符的位图，比旧格式更严格。

### 3.2 第二轮：43 → 111 MB/s（O(1) 统计 ＋ 持续句柄）

**按 2.1 的流程**，43 MB/s 的 1.52 ms 预算里只有 0.22 ms 被解释，残差 1.30 ms。

**按 2.4 的规则扫热循环**，`executeSend`（`manager.go:486-487`）每块调用：

```go
task.BytesTransferred = chunkMgr.GetTransferredBytes()  // 全表扫描
task.Progress        = chunkMgr.GetProgress()           // 内部又调一次，再扫一遍
```

`cm.chunks` 是预先分配的 `[]*ChunkInfo`，每个元素独立堆分配 —— 扫描既要走两遍 68,800 个指针的切片（1.1 MB），还要对每个元素各解引用一次（每块共 137,600 次），每次解引用都是独立堆对象上大概率 cache miss 的访存。仅指针切片的 1.1 MB 就是被传数据（64 KB）的 17 倍。

**修复**（`chunk.go:36`）：在 `ChunkManager` 上维护 `transferredCount` / `transferredBytes` 两个 O(1) 计数器，由 `MarkTransferred` 增量维护，由两个 checkpoint 反序列化器各重算一次，读接口直接返回。

**同时按 2.4 的「固定成本」分支**处理 checkpoint：`os.WriteFile` 每次 1.05 ms，改为按路径持有 `*os.File`，复用 `Seek + Truncate + Write`（0.13 ms）。由此引入一个必须处理的正确性问题：

> 成功路径要 `os.Remove` checkpoint（`manager.go:522`），而 **Windows 无法删除仍被打开的文件**。因此 `executeSend` 既在 `Remove` 之前显式 `closeCheckpoint`，又用 `defer` 兜住所有失败/取消出口。

**结果**：

| 每块成本构成 | 优化前 | 优化后 |
|---|---|---|
| 全表扫描 | 800 µs | ~0 |
| checkpoint（每 10 块摊薄） | 152 µs | 53 µs |
| MD5 | 87 µs | 87 µs（不动） |
| 网络写 ＋ 文件读 ＋ 事件（残差推算） | ~480 µs | ~480 µs |
| **合计** | **1.52 ms** | **~0.62 ms** |
| **对应吞吐** | **43 MB/s** | **预测 ~105 MB/s / 实测 111 MB/s** |

---

## 4. 踩过的坑

### 4.1 Windows 文件语义

| 坑 | 现象 | 正确做法 |
|---|---|---|
| 删除打开中的文件 | `os.Remove` 报 `Access is denied` | 先 `closeCheckpoint` 再 `Remove` |
| 新建 vs 覆盖的成本差 | temp+rename 单建文件就吃掉 5 ms | checkpoint 直写目标文件 |
| 测试泄漏句柄 | `t.TempDir()` 清理报 *being used by another process* | 直调 `writeCheckpoint` 的测试必须 `defer closeCheckpoint(path)` |

### 4.2 测试正确性

| 坑 | 现象 | 正确做法 |
|---|---|---|
| `GetTask` 返回浅拷贝 | 改拷贝不生效，断言总是旧值 | 直接改 `mgr.tasks[taskID]`（`manager.go`） |
| 紧时间断言 | 本机 2.53 ms 超阈值，CI 更不可控 | 见 2.6 的写法 A / 写法 B |
| 全局状态污染 | `file.SetDefaultSavePath` 影响后续测试 | `t.Cleanup(func() { file.SetDefaultSavePath("") })` |
| 环境变量门控 | 大文件测试会拖垮 CI | `LFT_LARGE_MB` 未设置时 `t.Skip` |

### 4.3 测量本身

| 坑 | 现象 | 正确做法 |
|---|---|---|
| 分配污染基准 | PowerShell 循环里 `New-Object byte[]` 让测量虚高 10× | 用 Go benchmark，预分配 buffer |
| 冷热页缓存 | 首跑 51 MB/s、次跑 105 MB/s | 预热源文件 ＋ 交替顺序重复 ≥3 轮 |
| profile 被夹具污染 | `warmFile` 占 13% 样本 | 取样区间只包被测调用 |
| `-top -cum` 被当成调用树 | 顺着缩进读出错误的父子关系 | 用 `-peek` 做归因 |
| loopback ≠ LAN | 4 GB loopback 61.5 MB/s「变慢了」 | 见 2.3，两端同机 CPU 争用 |
| 覆盖式写文件 | 二次写 `bench_test.go` 把 `BenchmarkChunkStats` 冲掉了 | 覆盖已有文件前先读；新增内容用追加 |

---

## 5. 可复用检查清单

新项目或新性能问题时，按顺序过一遍：

- [ ] **换算预算**：目标吞吐 ÷ 单位大小 = 单次迭代预算；列出已知成本，算出残差
- [ ] **排除假设**：磁盘、内存、网络共享、校验算法 —— 每一项都要有测量数据
- [ ] **建三条基线**：纯协议 / 同机 loopback / 真实链路，用三者差距定位层
- [ ] **扫 O(F²)**：热循环里每个被调函数，问「工作量是否随规模增长」
- [ ] **区分两类成本**：随规模增长的（O(F²)）＋ 调用频率高的固定成本
- [ ] **读 profile 前剔除夹具**，用 `-peek` 归因而非 `-top -cum` 的相邻行
- [ ] **benchmark 出数字，test 做防线**；时间断言只抓数量级，微优化交 benchmark
- [ ] **在目标规模复验**，并让大文件测试可通过环境变量跑、默认跳过
- [ ] **核对物理上限**，残差落到外部因素即停手，并记录「明确不做的事」
- [ ] **改完跑 `gofmt` / `go vet` / `go test`**，Windows 上额外确认句柄生命周期

---

## 6. 附录：命令

```bash
# 基准（-run XXX 跳过测试）
go test ./internal/transfer -run XXX -bench . -benchtime 300x

# 单个基准
go test ./internal/transfer -run XXX -bench BenchmarkChunkStatsPerChunk -benchtime 100x

# CPU profile 并归因
go test ./internal/transfer -run TestLoopbackThroughput -cpuprofile cpu.out
go tool pprof -top -cum -nodecount=30 cpu.out
go tool pprof -peek 'transfer\.writeCheckpoint' cpu.out

# 同机 loopback 吞吐（128 MB，含纯协议基线）
go test ./internal/transfer -run TestLoopbackThroughput -v

# 大文件复验（默认跳过）
LFT_LARGE_MB=4096 go test ./internal/transfer -run TestLoopbackThroughputLargeFile -v

# 常规验证
gofmt -l . && go build ./... && go vet . ./internal/... ./pkg/...
go test . ./internal/... ./pkg/... -count=1
```

### 硬件与环境（本项目实测机器）

| 项 | 值 |
|---|---|
| CPU | Intel Core i7-4800MQ @ 2.70 GHz |
| 内存 | 31.6 GB（测试时空闲 16.4 GB） |
| 磁盘 | 3 块独立 SSD：D: disk 0、C: disk 1；顺序写 882 MB/s、读 1278 MB/s |
| 网络 | 千兆有线 LAN，实际上限 113–117 MB/s |
| 工具链 | Go 1.23.0 / gcc 32 位（故 `-race` 仅在 CI 的 ubuntu 上跑） |
