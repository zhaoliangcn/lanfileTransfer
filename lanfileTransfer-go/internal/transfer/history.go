package transfer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const (
	historyFileName = "transfer_history.json"
)

type HistoryManager struct {
	mu       sync.RWMutex
	filePath string
	entries  []*TransferTask
	maxSize  int
}

func NewHistoryManager(configDir string, maxSize int) *HistoryManager {
	historyDir := filepath.Join(configDir, "LanFileTransfer")
	os.MkdirAll(historyDir, 0755)

	return &HistoryManager{
		filePath: filepath.Join(historyDir, historyFileName),
		maxSize:  maxSize,
	}
}

func (hm *HistoryManager) Load() ([]*TransferTask, error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	data, err := os.ReadFile(hm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			hm.entries = make([]*TransferTask, 0)
			return hm.entries, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		hm.entries = make([]*TransferTask, 0)
		return hm.entries, nil
	}

	var entries []*TransferTask
	if err := json.Unmarshal(data, &entries); err != nil {
		hm.entries = make([]*TransferTask, 0)
		return hm.entries, nil
	}

	if entries == nil {
		entries = make([]*TransferTask, 0)
	}

	hm.entries = entries
	return entries, nil
}

func (hm *HistoryManager) Save(task *TransferTask) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.entries = append(hm.entries, task)

	if len(hm.entries) > hm.maxSize {
		start := len(hm.entries) - hm.maxSize
		hm.entries = hm.entries[start:]
	}

	return hm.persist()
}

func (hm *HistoryManager) ClearCompleted() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var active []*TransferTask
	for _, t := range hm.entries {
		if t.Status != StatusCompleted && t.Status != StatusFailed && t.Status != StatusCancelled {
			active = append(active, t)
		}
	}
	hm.entries = active

	return hm.persist()
}

func (hm *HistoryManager) GetAll() []*TransferTask {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make([]*TransferTask, len(hm.entries))
	copy(result, hm.entries)
	return result
}

func (hm *HistoryManager) ClearAll() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.entries = make([]*TransferTask, 0)
	return hm.persist()
}

func (hm *HistoryManager) persist() error {
	data, err := json.MarshalIndent(hm.entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(hm.filePath, data, 0644)
}

func (hm *HistoryManager) FilePath() string {
	return hm.filePath
}
