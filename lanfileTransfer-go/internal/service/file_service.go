package service

import (
	"LanFileTransfer-Go/internal/file"
	"fmt"
	"time"
)

type FileService struct{}

func NewFileService() *FileService {
	return &FileService{}
}

func (s *FileService) SelectFile(path string) (*file.FileInfo, error) {
	return file.GetFileInfo(path)
}

func (s *FileService) GetFileInfo(path string) (*file.FileInfo, error) {
	return file.GetFileInfo(path)
}

func (s *FileService) GetFolderSize(path string) (int64, error) {
	return file.GetFolderSize(path)
}

func (s *FileService) EnsureDirectory(path string) error {
	return file.EnsureDir(path)
}

func (s *FileService) FileExists(path string) bool {
	return file.FileExists(path)
}

func (s *FileService) GetDefaultSavePath() string {
	return file.GetDefaultSavePath()
}

func (s *FileService) ResolvePath(baseDir, fileName string) string {
	return file.ResolvePath(baseDir, fileName)
}

func (s *FileService) GetAvailableFileName(path string) string {
	return file.GetAvailableFileName(path)
}

func (s *FileService) IsPathWritable(path string) bool {
	return file.IsPathWritable(path)
}

func (s *FileService) FormatFileSize(bytes int64) string {
	switch {
	case bytes >= 1073741824:
		return fmt.Sprintf("%.2f GB", float64(bytes)/1073741824)
	case bytes >= 1048576:
		return fmt.Sprintf("%.2f MB", float64(bytes)/1048576)
	case bytes >= 1024:
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func (s *FileService) FormatSpeed(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= 1073741824:
		return fmt.Sprintf("%.2f GB/s", bytesPerSec/1073741824)
	case bytesPerSec >= 1048576:
		return fmt.Sprintf("%.2f MB/s", bytesPerSec/1048576)
	case bytesPerSec >= 1024:
		return fmt.Sprintf("%.2f KB/s", bytesPerSec/1024)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}

func (s *FileService) FormatDuration(d time.Duration) string {
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
