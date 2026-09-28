package transfer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNewHistoryManager(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	if hm == nil {
		t.Fatal("NewHistoryManager should not return nil")
	}

	entries := hm.GetAll()
	if entries == nil {
		t.Errorf("GetAll should return non-nil slice")
	}
	if len(entries) != 0 {
		t.Errorf("new history manager should have 0 entries, got %d", len(entries))
	}
}

func TestHistoryManagerSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	task := NewTransferTask("test-id", "peer-1", "PeerOne", "", "file.txt", "/path/file.txt", 1024, true)
	task.Status = StatusCompleted
	task.EndTime = "2025-01-01T00:00:00Z"

	err := hm.Save(task)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	hm2 := NewHistoryManager(dir, 100)
	entries, err := hm2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "test-id" {
		t.Errorf("expected ID 'test-id', got '%s'", entries[0].ID)
	}
	if entries[0].Status != StatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", entries[0].Status)
	}
}

func TestHistoryManagerSaveMultiple(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	for i := 0; i < 5; i++ {
		task := NewTransferTask(
			fmt.Sprintf("id-%d", i),
			"peer-1",
			"PeerOne",
			"",
			fmt.Sprintf("file-%d.txt", i),
			"/path/file.txt",
			1024,
			true,
		)
		task.Status = StatusCompleted
		hm.Save(task)
	}

	entries := hm.GetAll()
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

func TestHistoryManagerMaxSize(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 3)

	for i := 0; i < 10; i++ {
		task := NewTransferTask(
			fmt.Sprintf("id-%d", i),
			"peer-1",
			"PeerOne",
			"",
			fmt.Sprintf("file-%d.txt", i),
			"/path/file.txt",
			1024,
			true,
		)
		task.Status = StatusCompleted
		hm.Save(task)
	}

	entries := hm.GetAll()
	if len(entries) > 3 {
		t.Errorf("expected at most 3 entries, got %d", len(entries))
	}
	if len(entries) > 0 && entries[0].ID != "id-7" {
		t.Errorf("expected oldest entry to be 'id-7', got '%s'", entries[0].ID)
	}
}

func TestHistoryManagerClearCompleted(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	task1 := NewTransferTask("completed-id", "peer-1", "PeerOne", "", "file1.txt", "/path/f1", 100, true)
	task1.Status = StatusCompleted
	hm.Save(task1)

	task2 := NewTransferTask("pending-id", "peer-1", "PeerOne", "", "file2.txt", "/path/f2", 100, true)
	task2.Status = StatusPending
	hm.Save(task2)

	err := hm.ClearCompleted()
	if err != nil {
		t.Fatalf("ClearCompleted failed: %v", err)
	}

	entries := hm.GetAll()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after clear, got %d", len(entries))
	}
	if entries[0].ID != "pending-id" {
		t.Errorf("expected pending task to remain, got '%s'", entries[0].ID)
	}
}

func TestHistoryManagerClearAll(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	for i := 0; i < 5; i++ {
		task := NewTransferTask(
			fmt.Sprintf("id-%d", i),
			"peer-1",
			"PeerOne",
			"",
			fmt.Sprintf("file-%d.txt", i),
			"/path/file.txt",
			1024,
			true,
		)
		task.Status = StatusCompleted
		hm.Save(task)
	}

	err := hm.ClearAll()
	if err != nil {
		t.Fatalf("ClearAll failed: %v", err)
	}

	entries := hm.GetAll()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after ClearAll, got %d", len(entries))
	}
}

func TestHistoryManagerPersistAndReload(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	for i := 0; i < 3; i++ {
		task := NewTransferTask(
			fmt.Sprintf("persist-id-%d", i),
			"peer-1",
			"PeerOne",
			"",
			fmt.Sprintf("persist-file-%d.txt", i),
			"/path/file.txt",
			2048,
			true,
		)
		task.Status = StatusFailed
		task.Error = "test error"
		hm.Save(task)
	}

	hm2 := NewHistoryManager(dir, 100)
	entries, err := hm2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for i, entry := range entries {
		expectedID := fmt.Sprintf("persist-id-%d", i)
		if entry.ID != expectedID {
			t.Errorf("entry %d: expected ID '%s', got '%s'", i, expectedID, entry.ID)
		}
		if entry.Error != "test error" {
			t.Errorf("entry %d: expected error 'test error', got '%s'", i, entry.Error)
		}
	}
}

func TestHistoryManagerLoadNonExistent(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	entries, err := hm.Load()
	if err != nil {
		t.Fatalf("Load non-existent file should not error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for non-existent file, got %d", len(entries))
	}
}

func TestHistoryManagerLoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	historyDir := filepath.Join(dir, "LanFileTransfer")
	os.MkdirAll(historyDir, 0755)
	os.WriteFile(filepath.Join(historyDir, historyFileName), []byte("invalid json"), 0644)

	entries, err := hm.Load()
	if err != nil {
		t.Fatalf("Load corrupted file should not error: %v", err)
	}
	if entries == nil {
		t.Errorf("entries should be non-nil after loading corrupted file")
	}
}

func TestHistoryManagerSaveDifferentStatuses(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	statuses := []TransferStatus{StatusCompleted, StatusFailed, StatusCancelled, StatusPending, StatusTransferring}
	for i, s := range statuses {
		task := NewTransferTask(
			fmt.Sprintf("status-id-%d", i),
			"peer-1",
			"PeerOne",
			"",
			"file.txt",
			"/path/file.txt",
			100,
			i%2 == 0,
		)
		task.Status = s
		hm.Save(task)
	}

	entries := hm.GetAll()
	if len(entries) != len(statuses) {
		t.Errorf("expected %d entries, got %d", len(statuses), len(entries))
	}
}

func TestHistoryManagerFilePath(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 100)

	expectedPath := filepath.Join(dir, "LanFileTransfer", historyFileName)
	if hm.FilePath() != expectedPath {
		t.Errorf("expected FilePath '%s', got '%s'", expectedPath, hm.FilePath())
	}
}

func TestHistoryManagerConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	hm := NewHistoryManager(dir, 1000)

	for i := 0; i < 100; i++ {
		task := NewTransferTask(
			fmt.Sprintf("concurrent-id-%d", i),
			"peer-1",
			"PeerOne",
			"",
			"file.txt",
			"/path/file.txt",
			100,
			true,
		)
		task.Status = StatusCompleted
		hm.Save(task)
	}

	entries := hm.GetAll()
	if len(entries) != 100 {
		t.Errorf("expected 100 entries, got %d", len(entries))
	}
}
