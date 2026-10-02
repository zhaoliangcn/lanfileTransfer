package transfer

import (
	"LanFileTransfer-Go/internal/config"
	"LanFileTransfer-Go/internal/file"
	"LanFileTransfer-Go/internal/network"
	"LanFileTransfer-Go/pkg/utils"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TransferEventType string

const (
	EventTransferStarted   TransferEventType = "transferStarted"
	EventTransferProgress  TransferEventType = "transferProgress"
	EventTransferCompleted TransferEventType = "transferCompleted"
	EventTransferFailed    TransferEventType = "transferFailed"
	EventTransferCancelled TransferEventType = "transferCancelled"
	EventTransferPaused    TransferEventType = "transferPaused"
	EventTransferResumed   TransferEventType = "transferResumed"
)

type TransferEvent struct {
	Type TransferEventType `json:"type"`
	Task *TransferTask     `json:"task"`
}

type TransferEventHandler func(event *TransferEvent)

type TransferManager struct {
	mu             sync.RWMutex
	tasks          map[string]*TransferTask
	sendQueue      chan string
	semMu          sync.RWMutex
	semaphore      chan struct{}
	maxConcurrent  int
	chunkSize      int
	autoReceive    bool
	tcpServer      *network.TCPServer
	ctx            context.Context
	cancel         context.CancelFunc
	eventListeners []TransferEventHandler
	activeSends    map[string]context.CancelFunc
	listenPort     int
	historyMgr     *HistoryManager
}

func NewTransferManager(cfg *config.Config, historyMgr *HistoryManager) *TransferManager {
	ctx, cancel := context.WithCancel(context.Background())
	maxConcurrent := cfg.MaxConcurrentTransfers
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	tm := &TransferManager{
		tasks:         make(map[string]*TransferTask),
		sendQueue:     make(chan string, 100),
		semaphore:     make(chan struct{}, maxConcurrent),
		maxConcurrent: maxConcurrent,
		chunkSize:     cfg.ChunkSize,
		autoReceive:   cfg.AutoReceive,
		ctx:           ctx,
		cancel:        cancel,
		activeSends:   make(map[string]context.CancelFunc),
		listenPort:    cfg.ListenPort,
		historyMgr:    historyMgr,
	}

	tm.OnEvent(func(event *TransferEvent) {
		if event.Type == EventTransferCompleted ||
			event.Type == EventTransferFailed ||
			event.Type == EventTransferCancelled {
			if tm.historyMgr != nil {
				task := *event.Task
				if err := tm.historyMgr.Save(&task); err != nil {
					utils.SugaredLog.Errorw("failed to save transfer history",
						"taskID", event.Task.ID,
						"error", err,
					)
				}
			}
		}
	})

	return tm
}

func (tm *TransferManager) Start() error {
	tcpCfg := &network.TCPConfig{
		Addr:        fmt.Sprintf(":%d", tm.listenPort),
		ReadTimeout: 30 * time.Second,
	}

	tm.tcpServer = network.NewTCPServer(tcpCfg)
	tm.tcpServer.OnConnect(tm.handleIncomingConnection)

	if err := tm.tcpServer.Start(); err != nil {
		return err
	}

	go tm.processSendQueue()

	return nil
}

func (tm *TransferManager) Stop() {
	tm.cancel()
	if tm.tcpServer != nil {
		tm.tcpServer.Stop()
	}
}

func (tm *TransferManager) SendFile(peerID, peerName, peerAddr string, filePath string) (string, error) {
	return tm.SendFileWithRelativePath(peerID, peerName, peerAddr, filePath, "")
}

func (tm *TransferManager) SendFileWithRelativePath(peerID, peerName, peerAddr string, filePath string, relativePath string) (string, error) {
	fileInfo, err := file.GetFileInfo(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	id := generateID()
	task := NewTransferTask(id, peerID, peerName, peerAddr, fileInfo.Name, filePath, fileInfo.Size, true)
	task.PeerName = peerName
	task.RelativePath = relativePath

	tm.mu.Lock()
	tm.tasks[id] = task
	tm.mu.Unlock()

	tm.emitEvent(&TransferEvent{Type: EventTransferStarted, Task: task})

	tm.sendQueue <- id

	return id, nil
}

func (tm *TransferManager) processSendQueue() {
	for {
		select {
		case <-tm.ctx.Done():
			return
		case taskID := <-tm.sendQueue:
			// Re-read the semaphore on every dispatch so a change to
			// MaxConcurrentTransfers takes effect without a restart.
			semaphore := tm.currentSemaphore()
			select {
			case semaphore <- struct{}{}:
				go func(id string, sem chan struct{}) {
					defer func() { <-sem }()
					tm.executeSend(id)
				}(taskID, semaphore)
			case <-tm.ctx.Done():
				return
			}
		}
	}
}

func (tm *TransferManager) currentSemaphore() chan struct{} {
	tm.semMu.RLock()
	defer tm.semMu.RUnlock()
	return tm.semaphore
}

// SetChunkSize changes the chunk size for transfers started from now on.
func (tm *TransferManager) SetChunkSize(size int) {
	if size <= 0 {
		return
	}
	if size > MaxChunkSize {
		size = MaxChunkSize
	}
	tm.mu.Lock()
	tm.chunkSize = size
	tm.mu.Unlock()
	utils.SugaredLog.Infow("chunk size updated", "chunkSize", size)
}

// SetMaxConcurrent rebuilds the send semaphore so the new limit applies
// immediately. Transfers already running are not interrupted.
func (tm *TransferManager) SetMaxConcurrent(n int) {
	if n <= 0 {
		return
	}
	tm.semMu.Lock()
	tm.maxConcurrent = n
	tm.semaphore = make(chan struct{}, n)
	tm.semMu.Unlock()
	utils.SugaredLog.Infow("max concurrent transfers updated", "maxConcurrent", n)
}

// SetAutoReceive controls whether inbound transfers are accepted. With it off,
// a transfer is refused instead of silently writing to disk.
func (tm *TransferManager) SetAutoReceive(enabled bool) {
	tm.mu.Lock()
	tm.autoReceive = enabled
	tm.mu.Unlock()
}

func (tm *TransferManager) autoReceiveEnabled() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.autoReceive
}

func (tm *TransferManager) chunkSizeValue() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.chunkSize
}

// chunkBufferPool recycles read buffers keyed by capacity. Buckets are required
// because the buffer must be at least chunkSize bytes: the send loop reads at
// `chunkIndex * chunkSize` but into a single buffer, so a smaller buffer would
// silently punch holes in the file whenever chunkSize is not 64KB.
var chunkBufferPool sync.Map

func getChunkBuffer(size int) *[]byte {
	if p, ok := chunkBufferPool.Load(size); ok {
		return p.(*sync.Pool).Get().(*[]byte)
	}

	p := &sync.Pool{
		New: func() any {
			b := make([]byte, size)
			return &b
		},
	}
	actual, _ := chunkBufferPool.LoadOrStore(size, p)
	return actual.(*sync.Pool).Get().(*[]byte)
}

func putChunkBuffer(size int, buf *[]byte) {
	if p, ok := chunkBufferPool.Load(size); ok {
		p.(*sync.Pool).Put(buf)
	}
}

func sendWithRetry(conn *network.TCPConnection, header *network.TransferHeader, data []byte, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := SendChunk(conn, header, data); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
		}
	}
	return fmt.Errorf("send failed after %d retries: %w", maxRetries, lastErr)
}

// writeCheckpoint persists resume state from inside the send loop. The payload
// is one bit per chunk, so it stays small even for multi-gigabyte files.
//
// It writes straight to the destination rather than through a temp+rename.
// Creating a fresh file costs ~5ms on Windows (vs ~1ms to overwrite one), which
// would put the ceiling back down near 90MB/s. A process killed mid-write is
// still safe: os.WriteFile truncates first, so the file comes out short and
// Deserialize rejects it, falling back to a full re-send instead of trusting a
// half-written bitmap.
var (
	checkpointMu    sync.Mutex
	checkpointFiles = map[string]*os.File{}
)

func writeCheckpoint(taskID, path string, chunkMgr *ChunkManager) {
	if path == "" {
		return
	}

	data, err := chunkMgr.Serialize()
	if err != nil {
		utils.SugaredLog.Errorw("failed to serialize checkpoint",
			"taskID", taskID, "error", err)
		return
	}

	if err := writeCheckpointFile(path, data); err != nil {
		utils.SugaredLog.Errorw("failed to write checkpoint",
			"taskID", taskID, "error", err)
	}
}

func writeCheckpointFile(path string, data []byte) error {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()

	f, ok := checkpointFiles[path]
	if !ok {
		var err error
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		checkpointFiles[path] = f
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := f.Truncate(int64(len(data))); err != nil {
		return err
	}
	_, err := f.Write(data)
	return err
}

func closeCheckpoint(path string) {
	if path == "" {
		return
	}

	checkpointMu.Lock()
	defer checkpointMu.Unlock()

	if f, ok := checkpointFiles[path]; ok {
		delete(checkpointFiles, path)
		f.Close()
	}
}

func (tm *TransferManager) executeSend(taskID string) {
	tm.mu.RLock()
	task, exists := tm.tasks[taskID]
	tm.mu.RUnlock()

	if !exists {
		return
	}

	ctx, cancel := context.WithCancel(tm.ctx)
	tm.mu.Lock()
	tm.activeSends[taskID] = cancel
	tm.mu.Unlock()

	defer func() {
		tm.mu.Lock()
		delete(tm.activeSends, taskID)
		tm.mu.Unlock()
		cancel()
	}()

	tcpCfg := &network.TCPConfig{
		ReadTimeout: 30 * time.Second,
	}
	conn, err := network.DialTCP(task.PeerAddr, tcpCfg)
	if err != nil {
		tm.failTask(taskID, fmt.Sprintf("connection failed: %v", err))
		return
	}
	defer conn.Close()

	task.Status = StatusTransferring
	tm.emitEvent(&TransferEvent{Type: EventTransferProgress, Task: task})

	fileReader, err := file.NewFileReader(task.FilePath)
	if err != nil {
		tm.failTask(taskID, fmt.Sprintf("failed to open file: %v", err))
		return
	}
	defer fileReader.Close()

	chunkSize := tm.chunkSizeValue()
	chunkMgr := NewChunkManager(task.FileSize, chunkSize)
	var startChunk int64 = 0

	if task.CheckpointPath != "" {
		cpData, err := os.ReadFile(task.CheckpointPath)
		if err == nil && len(cpData) > 0 {
			if err := chunkMgr.Deserialize(cpData); err != nil {
				// A checkpoint written under a different chunk size (or a corrupt
				// one) cannot be mapped onto this chunk layout. Start over rather
				// than trusting a partial match.
				utils.SugaredLog.Warnw("discarding unusable checkpoint",
					"taskID", taskID,
					"error", err,
				)
			} else {
				startChunk = chunkMgr.GetTransferredCount()
				task.BytesTransferred = chunkMgr.GetTransferredBytes()
				task.Progress = chunkMgr.GetProgress()
				utils.SugaredLog.Infow("resuming transfer from checkpoint",
					"taskID", taskID,
					"startChunk", startChunk,
					"progress", task.Progress,
				)
			}
		}
	}

	if task.CheckpointPath == "" {
		configDir, _ := os.UserConfigDir()
		cpDir := filepath.Join(configDir, "LanFileTransfer", "checkpoints")
		os.MkdirAll(cpDir, 0755)
		task.CheckpointPath = filepath.Join(cpDir, taskID+".checkpoint")
	}
	ckptPath := task.CheckpointPath
	defer closeCheckpoint(ckptPath)

	missingChunks := chunkMgr.GetMissingChunks()
	readSize := chunkMgr.ChunkSize()
	bufPtr := getChunkBuffer(readSize)
	buf := *bufPtr
	bufReturned := false
	returnBuf := func() {
		if !bufReturned {
			putChunkBuffer(readSize, bufPtr)
			bufReturned = true
		}
	}
	defer returnBuf()

	lastSpeedTime := time.Now()
	lastSpeedBytes := task.BytesTransferred
	shortRead := ""
	// Highest byte offset actually covered by a chunk read in this run. The
	// chunk table precomputes sizes from the file size measured at task creation,
	// so it cannot be trusted to detect a file that shrank since then.
	coveredTo := int64(0)

	for _, chunkIndex := range missingChunks {
		select {
		case <-ctx.Done():
			returnBuf()
			task.Status = StatusCancelled
			tm.emitEvent(&TransferEvent{Type: EventTransferCancelled, Task: task})
			return
		default:
		}

		offset := chunkIndex * int64(chunkSize)
		n, err := fileReader.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			returnBuf()
			tm.failTask(taskID, fmt.Sprintf("failed to read file at offset %d: %v", offset, err))
			return
		}

		// The file may have shrunk since GetFileInfo() measured it. Stop instead
		// of silently reporting success on a partially written file.
		if n == 0 {
			shortRead = fmt.Sprintf("unexpected end of file at offset %d (file size %d, expected %d)",
				offset, task.FileSize, task.FileSize)
			break
		}
		if chunkIndex < chunkMgr.TotalChunks()-1 && n != readSize {
			shortRead = fmt.Sprintf("short read at offset %d: got %d bytes, want %d",
				offset, n, readSize)
			break
		}
		if end := offset + int64(n); end > coveredTo {
			coveredTo = end
		}

		checksum := GenerateChunkChecksum(buf[:n])
		header := &network.TransferHeader{
			TransferID:    taskID,
			FileName:      task.FileName,
			FileSize:      int64(n),
			TotalFileSize: task.FileSize,
			ChunkIndex:    chunkIndex,
			ChunkOffset:   offset,
			TotalChunks:   chunkMgr.TotalChunks(),
			Checksum:      checksum,
			RelativePath:  task.RelativePath,
		}

		if err := sendWithRetry(conn, header, buf[:n], 3); err != nil {
			writeCheckpoint(taskID, task.CheckpointPath, chunkMgr)
			returnBuf()
			tm.failTask(taskID, fmt.Sprintf("failed to send chunk: %v", err))
			return
		}

		chunkMgr.MarkTransferred(chunkIndex, checksum)
		task.BytesTransferred = chunkMgr.GetTransferredBytes()
		task.Progress = chunkMgr.GetProgress()

		now := time.Now()
		elapsed := now.Sub(lastSpeedTime)
		if elapsed >= 500*time.Millisecond {
			deltaBytes := task.BytesTransferred - lastSpeedBytes
			task.Speed = float64(deltaBytes) / elapsed.Seconds()
			lastSpeedTime = now
			lastSpeedBytes = task.BytesTransferred
		}

		if chunkIndex%10 == 0 || chunkIndex == missingChunks[len(missingChunks)-1] {
			writeCheckpoint(taskID, task.CheckpointPath, chunkMgr)
		}

		tm.emitEvent(&TransferEvent{Type: EventTransferProgress, Task: task})
	}

	if shortRead != "" {
		writeCheckpoint(taskID, task.CheckpointPath, chunkMgr)
		returnBuf()
		tm.failTask(taskID, shortRead)
		return
	}

	// Guard against a truncated source: the peer would otherwise be told the
	// transfer completed while holding a short file.
	if coveredTo != task.FileSize {
		returnBuf()
		tm.failTask(taskID, fmt.Sprintf("incomplete transfer: covered %d of %d bytes",
			coveredTo, task.FileSize))
		return
	}

	closeCheckpoint(task.CheckpointPath)
	os.Remove(task.CheckpointPath)
	task.CheckpointPath = ""

	task.Status = StatusCompleted
	task.EndTime = time.Now().Format(time.RFC3339)
	task.Progress = 100
	tm.emitEvent(&TransferEvent{Type: EventTransferCompleted, Task: task})
}

func (tm *TransferManager) handleIncomingConnection(conn *network.TCPConnection) {
	go tm.receiveFile(conn)
}

func (tm *TransferManager) receiveFile(conn *network.TCPConnection) {
	defer conn.Close()

	var currentTask *TransferTask
	var fileWriter *file.FileWriter
	received := make(map[int64]bool)
	var receivedBytes int64

	// finishTask closes the writer and, unless the transfer turned out to be
	// incomplete, marks the task as completed. Partial transfers are reported
	// as failed so a truncated file is never presented as a success.
	finishTask := func(cause error) bool {
		if fileWriter != nil {
			fileWriter.Close()
			fileWriter = nil
		}
		if currentTask == nil {
			return true
		}

		if cause != nil {
			currentTask.Status = StatusFailed
			currentTask.Error = cause.Error()
			currentTask.EndTime = time.Now().Format(time.RFC3339)
			tm.emitEvent(&TransferEvent{Type: EventTransferFailed, Task: currentTask})
			return false
		}

		currentTask.Status = StatusCompleted
		currentTask.EndTime = time.Now().Format(time.RFC3339)
		currentTask.Progress = 100
		tm.emitEvent(&TransferEvent{Type: EventTransferCompleted, Task: currentTask})
		return true
	}

	for {
		header, data, err := ReceiveChunk(conn)
		if err != nil {
			if currentTask != nil {
				finishTask(fmt.Errorf("receive failed after %d/%d bytes: %w",
					receivedBytes, currentTask.FileSize, err))
			}
			return
		}

		if currentTask == nil || currentTask.ID != header.TransferID {
			// Refuse the transfer outright when auto-receive is off, instead of
			// writing whatever the sender pushes.
			if !tm.autoReceiveEnabled() {
				tm.failTask(header.TransferID, "incoming transfer refused: auto-receive is disabled")
				return
			}

			// A previous transfer shared this connection and never reached its
			// last chunk; report it as failed rather than leaving it "running".
			if currentTask != nil {
				finishTask(fmt.Errorf("connection reused after %d/%d bytes",
					receivedBytes, currentTask.FileSize))
			}

			var savePath string
			if header.RelativePath != "" {
				savePath = filepath.Join(file.GetDefaultSavePath(), header.RelativePath)
				if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
					tm.failTask("", fmt.Sprintf("failed to create directory: %v", err))
					return
				}
			} else {
				savePath = file.GetAvailableFileName(
					filepath.Join(file.GetDefaultSavePath(), header.FileName))
			}

			peerAddr := conn.RemoteAddr().String()
			host, _, _ := net.SplitHostPort(peerAddr)
			if host == "" {
				host = peerAddr
			}

			currentTask = NewTransferTask(
				header.TransferID, "", "", host,
				header.FileName, savePath, header.TotalFileSize, false,
			)
			currentTask.RelativePath = header.RelativePath
			currentTask.Status = StatusTransferring

			received = make(map[int64]bool)
			receivedBytes = 0

			tm.mu.Lock()
			tm.tasks[currentTask.ID] = currentTask
			tm.mu.Unlock()

			tm.emitEvent(&TransferEvent{Type: EventTransferStarted, Task: currentTask})

			fileWriter, err = file.NewFileWriter(savePath)
			if err != nil {
				finishTask(fmt.Errorf("failed to create file: %w", err))
				return
			}
		}

		if !VerifyChunk(data, header.Checksum) {
			finishTask(fmt.Errorf("chunk %d checksum mismatch", header.ChunkIndex))
			return
		}

		if !received[header.ChunkIndex] {
			// Chunks may resume at a non-zero index, so the write position has to
			// come from the header instead of the current file length.
			if _, err := fileWriter.WriteAt(data, header.ChunkOffset); err != nil {
				finishTask(fmt.Errorf("failed to write chunk %d: %w", header.ChunkIndex, err))
				return
			}
			received[header.ChunkIndex] = true
			receivedBytes += int64(len(data))
			currentTask.BytesTransferred = receivedBytes
			currentTask.Progress = currentTask.CalculateProgress()
			tm.emitEvent(&TransferEvent{Type: EventTransferProgress, Task: currentTask})
		}

		if receivedBytes >= currentTask.FileSize {
			finishTask(nil)
			return
		}
	}
}

func (tm *TransferManager) CancelTransfer(taskID string) error {
	tm.mu.RLock()
	task, exists := tm.tasks[taskID]
	tm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.Status == StatusCompleted || task.Status == StatusCancelled {
		return fmt.Errorf("task already completed or cancelled")
	}

	tm.mu.RLock()
	cancel, hasCancel := tm.activeSends[taskID]
	tm.mu.RUnlock()

	if hasCancel {
		cancel()
	}

	task.Status = StatusCancelled
	task.EndTime = time.Now().Format(time.RFC3339)
	tm.emitEvent(&TransferEvent{Type: EventTransferCancelled, Task: task})

	return nil
}

func (tm *TransferManager) ResumeTransfer(taskID string) error {
	tm.mu.RLock()
	task, exists := tm.tasks[taskID]
	tm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.Status != StatusFailed && task.Status != StatusCancelled {
		return fmt.Errorf("task is not in a resumable state: %s", task.Status)
	}

	if !task.IsSender {
		return fmt.Errorf("only sender can resume transfer")
	}

	task.Status = StatusPending
	task.EndTime = ""
	task.Error = ""

	tm.emitEvent(&TransferEvent{Type: EventTransferResumed, Task: task})

	tm.sendQueue <- taskID

	return nil
}

func (tm *TransferManager) ResumeFailedTransfers() (int, error) {
	tm.mu.RLock()
	var failedIDs []string
	for _, task := range tm.tasks {
		if task.IsSender && task.Status == StatusFailed {
			failedIDs = append(failedIDs, task.ID)
		}
	}
	tm.mu.RUnlock()

	count := 0
	for _, id := range failedIDs {
		if err := tm.ResumeTransfer(id); err != nil {
			utils.SugaredLog.Errorw("failed to resume transfer", "taskID", id, "error", err)
			continue
		}
		count++
	}

	return count, nil
}

func (tm *TransferManager) failTask(taskID string, errMsg string) {
	tm.mu.RLock()
	task, exists := tm.tasks[taskID]
	tm.mu.RUnlock()

	if !exists {
		return
	}

	task.Status = StatusFailed
	task.EndTime = time.Now().Format(time.RFC3339)
	task.Error = errMsg
	utils.SugaredLog.Errorw("transfer failed",
		"taskID", taskID,
		"error", errMsg,
	)

	tm.emitEvent(&TransferEvent{Type: EventTransferFailed, Task: task})
}

func (tm *TransferManager) GetTask(taskID string) *TransferTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return nil
	}
	taskCopy := *task
	return &taskCopy
}

func (tm *TransferManager) GetAllTasks() []*TransferTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]*TransferTask, 0, len(tm.tasks))
	for _, task := range tm.tasks {
		taskCopy := *task
		tasks = append(tasks, &taskCopy)
	}
	return tasks
}

func (tm *TransferManager) GetActiveTasks() []*TransferTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var active []*TransferTask
	for _, task := range tm.tasks {
		if task.Status == StatusPending || task.Status == StatusTransferring {
			taskCopy := *task
			active = append(active, &taskCopy)
		}
	}
	return active
}

func (tm *TransferManager) OnEvent(handler TransferEventHandler) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.eventListeners = append(tm.eventListeners, handler)
}

func (tm *TransferManager) emitEvent(event *TransferEvent) {
	tm.mu.RLock()
	listeners := make([]TransferEventHandler, len(tm.eventListeners))
	copy(listeners, tm.eventListeners)
	tm.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

func (tm *TransferManager) HistoryManager() *HistoryManager {
	return tm.historyMgr
}

func generateID() string {
	return fmt.Sprintf("transfer_%d_%d", time.Now().UnixNano(), time.Now().UnixMilli()%10000)
}
