package transfer

import (
	"LanFileTransfer-Go/internal/network"
	"LanFileTransfer-Go/pkg/utils"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	DefaultChunkSize = 64 * 1024
	MaxChunkSize     = 1024 * 1024
)

type ChunkInfo struct {
	Index       int64  `json:"index"`
	Offset      int64  `json:"offset"`
	Size        int    `json:"size"`
	Checksum    string `json:"checksum"`
	Transferred bool   `json:"transferred"`
}

type ChunkManager struct {
	mu          sync.RWMutex
	chunkSize   int
	chunks      []*ChunkInfo
	fileSize    int64
	totalChunks int64
}

func NewChunkManager(fileSize int64, chunkSize int) *ChunkManager {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if chunkSize > MaxChunkSize {
		chunkSize = MaxChunkSize
	}

	totalChunks := (fileSize + int64(chunkSize) - 1) / int64(chunkSize)

	cm := &ChunkManager{
		chunkSize:   chunkSize,
		fileSize:    fileSize,
		totalChunks: totalChunks,
		chunks:      make([]*ChunkInfo, totalChunks),
	}

	for i := int64(0); i < totalChunks; i++ {
		offset := i * int64(chunkSize)
		size := chunkSize
		if offset+int64(size) > fileSize {
			size = int(fileSize - offset)
		}

		cm.chunks[i] = &ChunkInfo{
			Index:  i,
			Offset: offset,
			Size:   size,
		}
	}

	return cm
}

func (cm *ChunkManager) GetChunk(index int64) *ChunkInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if index < 0 || index >= cm.totalChunks {
		return nil
	}
	return cm.chunks[index]
}

func (cm *ChunkManager) MarkTransferred(index int64, checksum string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if index >= 0 && index < cm.totalChunks {
		cm.chunks[index].Transferred = true
		cm.chunks[index].Checksum = checksum
	}
}

func (cm *ChunkManager) GetTransferredCount() int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var count int64
	for _, chunk := range cm.chunks {
		if chunk.Transferred {
			count++
		}
	}
	return count
}

func (cm *ChunkManager) GetTransferredBytes() int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var bytes int64
	for _, chunk := range cm.chunks {
		if chunk.Transferred {
			bytes += int64(chunk.Size)
		}
	}
	return bytes
}

func (cm *ChunkManager) GetProgress() float64 {
	if cm.fileSize == 0 {
		return 0
	}
	return float64(cm.GetTransferredBytes()) / float64(cm.fileSize) * 100
}

func (cm *ChunkManager) TotalChunks() int64 {
	return cm.totalChunks
}

func (cm *ChunkManager) ChunkSize() int {
	return cm.chunkSize
}

func (cm *ChunkManager) FileSize() int64 {
	return cm.fileSize
}

func (cm *ChunkManager) GetMissingChunks() []int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var missing []int64
	for _, chunk := range cm.chunks {
		if !chunk.Transferred {
			missing = append(missing, chunk.Index)
		}
	}
	return missing
}

func (cm *ChunkManager) Serialize() ([]byte, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return json.Marshal(cm.chunks)
}

func (cm *ChunkManager) Deserialize(data []byte) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return json.Unmarshal(data, &cm.chunks)
}

func SendChunk(conn *network.TCPConnection, header *network.TransferHeader, data []byte) error {
	packet, err := network.BuildTransferPacket(header, data)
	if err != nil {
		return err
	}
	return conn.Send(packet)
}

func ReceiveChunk(conn *network.TCPConnection) (*network.TransferHeader, []byte, error) {
	return network.ParseTransferPacket(conn)
}

func ReadChunkData(filePath string, offset int64, size int) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data := make([]byte, size)
	n, err := file.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return data[:n], nil
}

func WriteChunkData(filePath string, offset int64, data []byte) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteAt(data, offset)
	return err
}

func VerifyChunk(data []byte, expectedChecksum string) bool {
	if expectedChecksum == "" {
		return true
	}
	actual := utils.CalculateMD5(data)
	return actual == expectedChecksum
}

func GenerateChunkChecksum(data []byte) string {
	return utils.CalculateMD5(data)
}

func LoadCheckpoint(path string) (map[string]*ChunkInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*ChunkInfo), nil
		}
		return nil, err
	}

	var chunks map[string]*ChunkInfo
	if err := json.Unmarshal(data, &chunks); err != nil {
		return nil, err
	}
	return chunks, nil
}

func SaveCheckpoint(path string, chunks map[string]*ChunkInfo) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	data, err := json.MarshalIndent(chunks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint data: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	return os.Rename(tmpPath, path)
}

func DeleteCheckpoint(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
