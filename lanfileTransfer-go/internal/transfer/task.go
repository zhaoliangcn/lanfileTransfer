package transfer

import (
	"time"
)

type TransferType string

const (
	TransferTypeFile   TransferType = "file"
	TransferTypeFolder TransferType = "folder"
)

type TransferStatus string

const (
	StatusPending      TransferStatus = "pending"
	StatusTransferring TransferStatus = "transferring"
	StatusCompleted    TransferStatus = "completed"
	StatusFailed       TransferStatus = "failed"
	StatusCancelled    TransferStatus = "cancelled"
	StatusPaused       TransferStatus = "paused"
)

type TransferTask struct {
	ID               string         `json:"id"`
	PeerID           string         `json:"peerId"`
	PeerName         string         `json:"peerName"`
	PeerAddr         string         `json:"peerAddr"`
	FileName         string         `json:"fileName"`
	FilePath         string         `json:"filePath"`
	FileSize         int64          `json:"fileSize"`
	BytesTransferred int64          `json:"bytesTransferred"`
	Type             TransferType   `json:"type"`
	Status           TransferStatus `json:"status"`
	IsSender         bool           `json:"isSender"`
	StartTime        string         `json:"startTime"`
	EndTime          string         `json:"endTime"`
	Speed            float64        `json:"speed"`
	Progress         float64        `json:"progress"`
	Error            string         `json:"error,omitempty"`
	CheckpointPath   string         `json:"checkpointPath,omitempty"`
	RelativePath     string         `json:"relativePath,omitempty"`
}

func (t *TransferTask) CalculateProgress() float64 {
	if t.FileSize == 0 {
		return 0
	}
	progress := float64(t.BytesTransferred) / float64(t.FileSize) * 100
	if progress > 100 {
		progress = 100
	}
	return progress
}

func (t *TransferTask) CalculateSpeed(elapsed time.Duration) float64 {
	if elapsed.Seconds() <= 0 {
		return 0
	}
	return float64(t.BytesTransferred) / elapsed.Seconds()
}

func (t *TransferTask) Elapsed() time.Duration {
	start, err := time.Parse(time.RFC3339, t.StartTime)
	if err != nil {
		return 0
	}
	if t.Status == StatusCompleted || t.Status == StatusFailed || t.Status == StatusCancelled {
		end, err := time.Parse(time.RFC3339, t.EndTime)
		if err != nil {
			return 0
		}
		return end.Sub(start)
	}
	return time.Since(start)
}

func (t *TransferTask) Remaining() time.Duration {
	if t.Speed <= 0 {
		return 0
	}
	remaining := int64(float64(t.FileSize-t.BytesTransferred) / t.Speed)
	return time.Duration(remaining) * time.Second
}

func NewTransferTask(id, peerID, peerName, peerAddr, fileName, filePath string, fileSize int64, isSender bool) *TransferTask {
	return &TransferTask{
		ID:        id,
		PeerID:    peerID,
		PeerName:  peerName,
		PeerAddr:  peerAddr,
		FileName:  fileName,
		FilePath:  filePath,
		FileSize:  fileSize,
		Type:      TransferTypeFile,
		Status:    StatusPending,
		IsSender:  isSender,
		StartTime: time.Now().Format(time.RFC3339),
	}
}
