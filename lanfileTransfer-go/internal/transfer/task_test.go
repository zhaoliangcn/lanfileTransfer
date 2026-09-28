package transfer

import (
	"testing"
	"time"
)

func TestNewTransferTask(t *testing.T) {
	task := NewTransferTask("test-id", "peer-1", "PeerOne", "", "file.txt", "/path/to/file.txt", 1024, true)

	if task.ID != "test-id" {
		t.Errorf("expected ID 'test-id', got '%s'", task.ID)
	}
	if task.PeerID != "peer-1" {
		t.Errorf("expected PeerID 'peer-1', got '%s'", task.PeerID)
	}
	if task.FileName != "file.txt" {
		t.Errorf("expected FileName 'file.txt', got '%s'", task.FileName)
	}
	if task.FileSize != 1024 {
		t.Errorf("expected FileSize 1024, got %d", task.FileSize)
	}
	if task.IsSender != true {
		t.Errorf("expected IsSender true, got %v", task.IsSender)
	}
	if task.Status != StatusPending {
		t.Errorf("initial status should be pending, got %s", task.Status)
	}
	if task.Type != TransferTypeFile {
		t.Errorf("initial type should be file, got %s", task.Type)
	}
}

func TestTransferTaskCalculateProgress(t *testing.T) {
	tests := []struct {
		name             string
		fileSize         int64
		bytesTransferred int64
		want             float64
	}{
		{"zero file size", 0, 0, 0},
		{"no progress", 1000, 0, 0},
		{"half progress", 1000, 500, 50},
		{"full progress", 1000, 1000, 100},
		{"over 100 percent", 1000, 2000, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := NewTransferTask("id", "peer", "peerName", "", "file", "path", tt.fileSize, true)
			task.BytesTransferred = tt.bytesTransferred
			got := task.CalculateProgress()
			if got != tt.want {
				t.Errorf("CalculateProgress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransferTaskCalculateSpeed(t *testing.T) {
	task := NewTransferTask("id", "peer", "peerName", "", "file", "path", 1000, true)
	task.BytesTransferred = 500

	elapsed := 10 * time.Second
	speed := task.CalculateSpeed(elapsed)

	expected := 50.0
	if speed != expected {
		t.Errorf("CalculateSpeed() = %v, want %v", speed, expected)
	}
}

func TestTransferTaskCalculateSpeedZeroElapsed(t *testing.T) {
	task := NewTransferTask("id", "peer", "peerName", "", "file", "path", 1000, true)
	task.BytesTransferred = 500

	speed := task.CalculateSpeed(0)
	if speed != 0 {
		t.Errorf("CalculateSpeed() with zero elapsed should be 0, got %v", speed)
	}
}

func TestTransferTaskElapsed(t *testing.T) {
	task := NewTransferTask("id", "peer", "peerName", "", "file", "path", 1000, true)

	elapsed := task.Elapsed()
	if elapsed < 0 {
		t.Errorf("Elapsed() should not be negative, got %v", elapsed)
	}
}

func TestTransferTaskRemaining(t *testing.T) {
	task := NewTransferTask("id", "peer", "peerName", "", "file", "path", 1000, true)
	task.BytesTransferred = 500
	task.Speed = 100

	remaining := task.Remaining()
	expected := time.Duration(5) * time.Second

	if remaining < expected-time.Second || remaining > expected+time.Second {
		t.Errorf("Remaining() = %v, want approximately %v", remaining, expected)
	}
}

func TestTransferTaskRemainingZeroSpeed(t *testing.T) {
	task := NewTransferTask("id", "peer", "peerName", "", "file", "path", 1000, true)
	task.BytesTransferred = 500
	task.Speed = 0

	remaining := task.Remaining()
	if remaining != 0 {
		t.Errorf("Remaining() with zero speed should be 0, got %v", remaining)
	}
}

func TestTransferTaskStatusConstants(t *testing.T) {
	if StatusPending != "pending" {
		t.Errorf("StatusPending should be 'pending'")
	}
	if StatusTransferring != "transferring" {
		t.Errorf("StatusTransferring should be 'transferring'")
	}
	if StatusCompleted != "completed" {
		t.Errorf("StatusCompleted should be 'completed'")
	}
	if StatusFailed != "failed" {
		t.Errorf("StatusFailed should be 'failed'")
	}
	if StatusCancelled != "cancelled" {
		t.Errorf("StatusCancelled should be 'cancelled'")
	}
	if StatusPaused != "paused" {
		t.Errorf("StatusPaused should be 'paused'")
	}
}

func TestTransferTaskTypeConstants(t *testing.T) {
	if TransferTypeFile != "file" {
		t.Errorf("TransferTypeFile should be 'file'")
	}
	if TransferTypeFolder != "folder" {
		t.Errorf("TransferTypeFolder should be 'folder'")
	}
}
