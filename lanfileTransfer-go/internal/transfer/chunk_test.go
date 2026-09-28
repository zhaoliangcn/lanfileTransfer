package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewChunkManager(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	expectedTotal := int64(4)
	if cm.TotalChunks() != expectedTotal {
		t.Errorf("expected %d chunks, got %d", expectedTotal, cm.TotalChunks())
	}
	if cm.FileSize() != 1000 {
		t.Errorf("expected file size 1000, got %d", cm.FileSize())
	}
	if cm.ChunkSize() != 256 {
		t.Errorf("expected chunk size 256, got %d", cm.ChunkSize())
	}
}

func TestNewChunkManagerDefaultChunkSize(t *testing.T) {
	cm := NewChunkManager(1000, 0)

	if cm.ChunkSize() != DefaultChunkSize {
		t.Errorf("expected default chunk size %d, got %d", DefaultChunkSize, cm.ChunkSize())
	}
}

func TestNewChunkManagerMaxChunkSize(t *testing.T) {
	cm := NewChunkManager(1000, MaxChunkSize+1)

	if cm.ChunkSize() != MaxChunkSize {
		t.Errorf("expected max chunk size %d, got %d", MaxChunkSize, cm.ChunkSize())
	}
}

func TestChunkManagerLastChunkSize(t *testing.T) {
	cm := NewChunkManager(1000, 256)
	lastChunk := cm.GetChunk(3)

	if lastChunk == nil {
		t.Fatal("expected last chunk to exist")
	}
	expectedSize := 1000 - 3*256
	if lastChunk.Size != expectedSize {
		t.Errorf("expected last chunk size %d, got %d", expectedSize, lastChunk.Size)
	}
}

func TestChunkManagerGetChunkInvalid(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	if chunk := cm.GetChunk(-1); chunk != nil {
		t.Errorf("expected nil for negative index, got %+v", chunk)
	}
	if chunk := cm.GetChunk(100); chunk != nil {
		t.Errorf("expected nil for out-of-range index, got %+v", chunk)
	}
}

func TestChunkManagerMarkTransferred(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	cm.MarkTransferred(0, "checksum1")

	chunk := cm.GetChunk(0)
	if chunk == nil {
		t.Fatal("expected chunk to exist")
	}
	if !chunk.Transferred {
		t.Errorf("chunk should be marked as transferred")
	}
	if chunk.Checksum != "checksum1" {
		t.Errorf("expected checksum 'checksum1', got '%s'", chunk.Checksum)
	}
}

func TestChunkManagerMarkTransferredInvalidIndex(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	cm.MarkTransferred(-1, "test")
	cm.MarkTransferred(100, "test")

	count := cm.GetTransferredCount()
	if count != 0 {
		t.Errorf("expected 0 transferred chunks, got %d", count)
	}
}

func TestChunkManagerGetTransferredCount(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	if count := cm.GetTransferredCount(); count != 0 {
		t.Errorf("initial transferred count should be 0, got %d", count)
	}

	cm.MarkTransferred(0, "c1")
	cm.MarkTransferred(1, "c2")

	if count := cm.GetTransferredCount(); count != 2 {
		t.Errorf("expected 2 transferred chunks, got %d", count)
	}
}

func TestChunkManagerGetTransferredBytes(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	cm.MarkTransferred(0, "c1")
	cm.MarkTransferred(1, "c2")

	bytes := cm.GetTransferredBytes()
	if bytes != 512 {
		t.Errorf("expected 512 transferred bytes, got %d", bytes)
	}
}

func TestChunkManagerGetProgress(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	cm.MarkTransferred(0, "c1")
	cm.MarkTransferred(1, "c2")

	progress := cm.GetProgress()
	expected := 51.2
	if progress != expected {
		t.Errorf("expected progress %.1f, got %.1f", expected, progress)
	}
}

func TestChunkManagerGetProgressZeroFileSize(t *testing.T) {
	cm := NewChunkManager(0, 256)

	progress := cm.GetProgress()
	if progress != 0 {
		t.Errorf("expected progress 0 for zero file size, got %f", progress)
	}
}

func TestChunkManagerGetMissingChunks(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	missing := cm.GetMissingChunks()
	if len(missing) != 4 {
		t.Errorf("expected 4 missing chunks initially, got %d", len(missing))
	}

	cm.MarkTransferred(0, "c1")
	cm.MarkTransferred(2, "c3")

	missing = cm.GetMissingChunks()
	if len(missing) != 2 {
		t.Errorf("expected 2 missing chunks, got %d", len(missing))
	}
	for _, idx := range missing {
		if idx == 0 || idx == 2 {
			t.Errorf("transferred chunk %d should not be in missing list", idx)
		}
	}
}

func TestChunkManagerSerializeDeserialize(t *testing.T) {
	cm := NewChunkManager(1000, 256)

	cm.MarkTransferred(0, "c1")
	cm.MarkTransferred(2, "c3")

	data, err := cm.Serialize()
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("serialized data should not be empty")
	}

	cm2 := NewChunkManager(1000, 256)
	if err := cm2.Deserialize(data); err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if !cm2.GetChunk(0).Transferred {
		t.Errorf("chunk 0 should be transferred after deserialize")
	}
	if cm2.GetChunk(1).Transferred {
		t.Errorf("chunk 1 should not be transferred after deserialize")
	}
	if !cm2.GetChunk(2).Transferred {
		t.Errorf("chunk 2 should be transferred after deserialize")
	}
}

func TestGenerateChunkChecksum(t *testing.T) {
	data := []byte("test data for checksum")
	checksum := GenerateChunkChecksum(data)

	if checksum == "" {
		t.Errorf("checksum should not be empty")
	}

	checksum2 := GenerateChunkChecksum(data)
	if checksum != checksum2 {
		t.Errorf("checksums should be consistent for same data: %s vs %s", checksum, checksum2)
	}
}

func TestReadWriteChunkData(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	originalData := []byte("hello world this is chunk data")
	offset := int64(100)

	if err := WriteChunkData(filePath, offset, originalData); err != nil {
		t.Fatalf("WriteChunkData failed: %v", err)
	}

	readData, err := ReadChunkData(filePath, offset, len(originalData))
	if err != nil {
		t.Fatalf("ReadChunkData failed: %v", err)
	}

	if string(readData) != string(originalData) {
		t.Errorf("read data mismatch: got '%s', want '%s'", string(readData), string(originalData))
	}
}

func TestVerifyChunk(t *testing.T) {
	data := []byte("verify this data")

	checksum := GenerateChunkChecksum(data)

	if !VerifyChunk(data, checksum) {
		t.Errorf("VerifyChunk should return true for valid checksum")
	}

	if VerifyChunk(data, "invalid") {
		t.Errorf("VerifyChunk should return false for invalid checksum")
	}

	if !VerifyChunk(data, "") {
		t.Errorf("VerifyChunk should return true for empty checksum")
	}
}

func TestSaveLoadDeleteCheckpoint(t *testing.T) {
	dir := t.TempDir()
	cpPath := filepath.Join(dir, "checkpoint.json")

	chunks := map[string]*ChunkInfo{
		"0": {Index: 0, Size: 256, Checksum: "c1", Transferred: true},
		"1": {Index: 1, Size: 256, Checksum: "c2", Transferred: false},
	}

	if err := SaveCheckpoint(cpPath, chunks); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	loaded, err := LoadCheckpoint(cpPath)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(loaded))
	}
	if loaded["0"].Checksum != "c1" {
		t.Errorf("expected checksum 'c1', got '%s'", loaded["0"].Checksum)
	}

	if err := DeleteCheckpoint(cpPath); err != nil {
		t.Fatalf("DeleteCheckpoint failed: %v", err)
	}

	if _, err := os.Stat(cpPath); !os.IsNotExist(err) {
		t.Errorf("checkpoint file should be deleted")
	}
}

func TestLoadCheckpointNonExistent(t *testing.T) {
	chunks, err := LoadCheckpoint("/nonexistent/path/checkpoint.json")
	if err != nil {
		t.Fatalf("LoadCheckpoint for non-existent path should not error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected empty map, got %d entries", len(chunks))
	}
}

func TestErrors(t *testing.T) {
	if ErrPeerNotFound == nil {
		t.Errorf("ErrPeerNotFound should not be nil")
	}
	if ErrFileNotFound == nil {
		t.Errorf("ErrFileNotFound should not be nil")
	}
	if ErrTransferFailed == nil {
		t.Errorf("ErrTransferFailed should not be nil")
	}
	if ErrConnectionLost == nil {
		t.Errorf("ErrConnectionLost should not be nil")
	}
	if ErrChecksumMismatch == nil {
		t.Errorf("ErrChecksumMismatch should not be nil")
	}
	if ErrTaskNotFound == nil {
		t.Errorf("ErrTaskNotFound should not be nil")
	}
	if ErrTaskNotResumable == nil {
		t.Errorf("ErrTaskNotResumable should not be nil")
	}
	if ErrNotSender == nil {
		t.Errorf("ErrNotSender should not be nil")
	}
}
