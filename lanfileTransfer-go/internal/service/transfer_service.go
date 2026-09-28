package service

import (
	"LanFileTransfer-Go/internal/file"
	"LanFileTransfer-Go/internal/transfer"
	"LanFileTransfer-Go/pkg/utils"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TransferService struct {
	transferMgr *transfer.TransferManager
}

func NewTransferService(transferMgr *transfer.TransferManager) *TransferService {
	return &TransferService{
		transferMgr: transferMgr,
	}
}

func (s *TransferService) SendFile(peerID, peerName, peerAddr, filePath string) (string, error) {
	utils.SugaredLog.Infow("sending file",
		"peerID", peerID,
		"peerName", peerName,
		"filePath", filePath,
	)
	return s.transferMgr.SendFile(peerID, peerName, peerAddr, filePath)
}

func (s *TransferService) SendFolder(peerID, peerName, peerAddr, folderPath string) ([]string, error) {
	utils.SugaredLog.Infow("sending folder",
		"peerID", peerID,
		"peerName", peerName,
		"folderPath", folderPath,
	)

	info, err := os.Stat(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to access folder: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", folderPath)
	}

	folderName := filepath.Base(folderPath)

	var taskIDs []string

	err = filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			relPath = info.Name()
		}
		relativePath := filepath.Join(folderName, relPath)

		taskID, err := s.transferMgr.SendFileWithRelativePath(peerID, peerName, peerAddr, path, relativePath)
		if err != nil {
			utils.SugaredLog.Errorw("failed to send file in folder",
				"path", path,
				"error", err,
			)
			return nil
		}
		taskIDs = append(taskIDs, taskID)

		return nil
	})

	if err != nil {
		return taskIDs, fmt.Errorf("folder send incomplete: %w", err)
	}

	return taskIDs, nil
}

func (s *TransferService) ResumeTransfer(transferID string) error {
	utils.SugaredLog.Infow("resuming transfer", "transferID", transferID)
	return s.transferMgr.ResumeTransfer(transferID)
}

func (s *TransferService) ResumeFailedTransfers() (int, error) {
	utils.SugaredLog.Infow("resuming all failed transfers")
	return s.transferMgr.ResumeFailedTransfers()
}

func (s *TransferService) CancelTransfer(transferID string) error {
	utils.SugaredLog.Infow("cancelling transfer", "transferID", transferID)
	return s.transferMgr.CancelTransfer(transferID)
}

func (s *TransferService) GetTask(transferID string) *transfer.TransferTask {
	return s.transferMgr.GetTask(transferID)
}

func (s *TransferService) GetAllTasks() []*transfer.TransferTask {
	return s.transferMgr.GetAllTasks()
}

func (s *TransferService) GetActiveTasks() []*transfer.TransferTask {
	return s.transferMgr.GetActiveTasks()
}

func (s *TransferService) GetCompletedTasks() []*transfer.TransferTask {
	allTasks := s.transferMgr.GetAllTasks()
	completed := make([]*transfer.TransferTask, 0)
	for _, task := range allTasks {
		if task.Status == transfer.StatusCompleted {
			completed = append(completed, task)
		}
	}
	return completed
}

func (s *TransferService) GetFailedTasks() []*transfer.TransferTask {
	allTasks := s.transferMgr.GetAllTasks()
	failed := make([]*transfer.TransferTask, 0)
	for _, task := range allTasks {
		if task.Status == transfer.StatusFailed {
			failed = append(failed, task)
		}
	}
	return failed
}

func (s *TransferService) GetTaskCount() int {
	return len(s.transferMgr.GetAllTasks())
}

func (s *TransferService) GetActiveCount() int {
	return len(s.transferMgr.GetActiveTasks())
}

func (s *TransferService) GetFailedCount() int {
	allTasks := s.transferMgr.GetAllTasks()
	count := 0
	for _, task := range allTasks {
		if task.Status == transfer.StatusFailed {
			count++
		}
	}
	return count
}

func (s *TransferService) FormatTransferSpeed(speed float64) string {
	switch {
	case speed >= 1073741824:
		return fmt.Sprintf("%.2f GB/s", speed/1073741824)
	case speed >= 1048576:
		return fmt.Sprintf("%.2f MB/s", speed/1048576)
	case speed >= 1024:
		return fmt.Sprintf("%.2f KB/s", speed/1024)
	default:
		return fmt.Sprintf("%.0f B/s", speed)
	}
}

func (s *TransferService) FormatFileSize(size int64) string {
	switch {
	case size >= 1073741824:
		return fmt.Sprintf("%.2f GB", float64(size)/1073741824)
	case size >= 1048576:
		return fmt.Sprintf("%.2f MB", float64(size)/1048576)
	case size >= 1024:
		return fmt.Sprintf("%.2f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func (s *TransferService) FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	sec := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, sec)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, sec)
	}
	return fmt.Sprintf("%ds", sec)
}

func (s *TransferService) OnTransferEvent(handler transfer.TransferEventHandler) {
	s.transferMgr.OnEvent(handler)
}

func (s *TransferService) GetDefaultSavePath() string {
	return file.GetDefaultSavePath()
}

func (s *TransferService) GetAvailableFileName(path string) string {
	return file.GetAvailableFileName(path)
}

func (s *TransferService) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *TransferService) EnsureDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

func (s *TransferService) IsFolder(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func (s *TransferService) GetFolderContents(folderPath string) ([]*file.FileInfo, error) {
	info, err := os.Stat(folderPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", folderPath)
	}

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, err
	}

	var contents []*file.FileInfo
	for _, entry := range entries {
		childInfo, err := entry.Info()
		if err != nil {
			continue
		}
		contents = append(contents, &file.FileInfo{
			Name:  entry.Name(),
			Path:  filepath.Join(folderPath, entry.Name()),
			Size:  childInfo.Size(),
			IsDir: childInfo.IsDir(),
		})
	}

	return contents, nil
}

func (s *TransferService) FormatFilePath(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if len(dir) > 30 {
		parts := strings.Split(dir, string(filepath.Separator))
		if len(parts) > 3 {
			dir = parts[0] + string(filepath.Separator) + "..." + string(filepath.Separator) + parts[len(parts)-1]
		}
	}
	return filepath.Join(dir, base)
}
