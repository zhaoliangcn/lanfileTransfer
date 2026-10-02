package transfer

import (
	"LanFileTransfer-Go/internal/network"
	"LanFileTransfer-Go/pkg/utils"
	"bytes"
	"encoding/binary"
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

	transferredCount int64
	transferredBytes int64
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
		c := cm.chunks[index]
		if !c.Transferred {
			c.Transferred = true
			cm.transferredCount++
			cm.transferredBytes += int64(c.Size)
		}
		c.Checksum = checksum
	}
}

func (cm *ChunkManager) GetTransferredCount() int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.transferredCount
}

func (cm *ChunkManager) GetTransferredBytes() int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.transferredBytes
}

func (cm *ChunkManager) GetProgress() float64 {
	if cm.fileSize == 0 {
		return 0
	}
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return float64(cm.transferredBytes) / float64(cm.fileSize) * 100
}

func (cm *ChunkManager) recalcLocked() {
	var count, bytes int64
	for _, chunk := range cm.chunks {
		if chunk.Transferred {
			count++
			bytes += int64(chunk.Size)
		}
	}
	cm.transferredCount = count
	cm.transferredBytes = bytes
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

// checkpointFormatMagic prefixes the bitmap checkpoint so Deserialize can tell
// it apart from the legacy JSON-array format.
var checkpointFormatMagic = [4]byte{'L', 'F', 'T', 'C'}

// checkpointHeaderSize is magic (4 bytes) + chunk count (big-endian uint64).
const checkpointHeaderSize = 12

// Serialize encodes the completed-chunk set as a bitmap: one bit per chunk.
// Nothing else in the record is needed to resume -- GetMissingChunks only reads
// Transferred, and Index/Offset/Size are recomputed by NewChunkManager.
//
// The previous format was json.Marshal over the whole []*ChunkInfo, which the
// send loop rewrote in full every 10 chunks (640KB of payload). Its size grew
// with the file (~115 bytes/chunk), so a 4.5GB transfer paid ~8.5MB of marshal
// and disk work per 640KB sent and plateaued around 13MB/s. A bitmap is fixed
// at ceil(n/8) bytes instead.
func (cm *ChunkManager) Serialize() ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	total := len(cm.chunks)
	buf := make([]byte, checkpointHeaderSize+(total+7)/8)
	copy(buf[:4], checkpointFormatMagic[:])
	binary.BigEndian.PutUint64(buf[4:], uint64(total))

	for i, chunk := range cm.chunks {
		if chunk.Transferred {
			buf[checkpointHeaderSize+i/8] |= 1 << (uint(i) % 8)
		}
	}
	return buf, nil
}

func (cm *ChunkManager) Deserialize(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty checkpoint")
	}
	if data[0] == '[' {
		return cm.deserializeLegacy(data)
	}
	return cm.deserializeBitmap(data)
}

func (cm *ChunkManager) deserializeBitmap(data []byte) error {
	if len(data) < checkpointHeaderSize || !bytes.Equal(data[:4], checkpointFormatMagic[:]) {
		return fmt.Errorf("unrecognised checkpoint header")
	}

	storedTotal := binary.BigEndian.Uint64(data[4:])
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if storedTotal != uint64(len(cm.chunks)) {
		return fmt.Errorf("checkpoint has %d chunks but the file needs %d",
			storedTotal, len(cm.chunks))
	}

	need := checkpointHeaderSize + (len(cm.chunks)+7)/8
	if len(data) < need {
		return fmt.Errorf("truncated checkpoint: have %d bytes, need %d", len(data), need)
	}

	bits := data[checkpointHeaderSize:]
	for i, chunk := range cm.chunks {
		chunk.Transferred = bits[i/8]&(1<<(uint(i)%8)) != 0
	}
	cm.recalcLocked()
	return nil
}

// deserializeLegacy accepts checkpoints written before the bitmap format. Those
// held Index/Offset/Size/Checksum too, but only Transferred is consumed.
func (cm *ChunkManager) deserializeLegacy(data []byte) error {
	var stored []*ChunkInfo
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("invalid legacy checkpoint: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if len(stored) != len(cm.chunks) {
		return fmt.Errorf("legacy checkpoint has %d chunks but the file needs %d",
			len(stored), len(cm.chunks))
	}

	for i, chunk := range stored {
		if chunk == nil {
			continue
		}
		cm.chunks[i].Transferred = chunk.Transferred
	}
	cm.recalcLocked()
	return nil
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
