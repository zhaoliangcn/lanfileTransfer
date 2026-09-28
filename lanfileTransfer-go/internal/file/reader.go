package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime string `json:"modTime"`
}

type FileReader struct {
	file   *os.File
	path   string
	size   int64
	offset int64
}

func NewFileReader(path string) (*FileReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	return &FileReader{
		file: file,
		path: path,
		size: info.Size(),
	}, nil
}

func (fr *FileReader) Read(buf []byte) (int, error) {
	n, err := fr.file.Read(buf)
	if n > 0 {
		fr.offset += int64(n)
	}
	return n, err
}

func (fr *FileReader) ReadAt(buf []byte, offset int64) (int, error) {
	return fr.file.ReadAt(buf, offset)
}

func (fr *FileReader) Seek(offset int64, whence int) (int64, error) {
	newOffset, err := fr.file.Seek(offset, whence)
	if err == nil {
		fr.offset = newOffset
	}
	return newOffset, err
}

func (fr *FileReader) Close() error {
	return fr.file.Close()
}

func (fr *FileReader) Size() int64 {
	return fr.size
}

func (fr *FileReader) Offset() int64 {
	return fr.offset
}

func (fr *FileReader) Path() string {
	return fr.path
}

type FileWriter struct {
	file *os.File
	path string
	size int64
}

func NewFileWriter(path string) (*FileWriter, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	return &FileWriter{
		file: file,
		path: path,
	}, nil
}

func (fw *FileWriter) Write(data []byte) (int, error) {
	n, err := fw.file.Write(data)
	if n > 0 {
		fw.size += int64(n)
	}
	return n, err
}

func (fw *FileWriter) WriteAt(data []byte, offset int64) (int, error) {
	n, err := fw.file.WriteAt(data, offset)
	if n > 0 {
		if offset+int64(n) > fw.size {
			fw.size = offset + int64(n)
		}
	}
	return n, err
}

func (fw *FileWriter) Close() error {
	return fw.file.Close()
}

func (fw *FileWriter) Size() int64 {
	return fw.size
}

func (fw *FileWriter) Path() string {
	return fw.path
}

func NewResumableWriter(path string) (*FileWriter, int64, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, 0, err
	}

	var existingSize int64
	if info, err := os.Stat(path); err == nil {
		existingSize = info.Size()
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, 0, err
	}

	if existingSize > 0 {
		if _, err := file.Seek(existingSize, io.SeekStart); err != nil {
			file.Close()
			return nil, 0, err
		}
	}

	return &FileWriter{
		file: file,
		path: path,
		size: existingSize,
	}, existingSize, nil
}

func GetFileInfo(path string) (*FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return &FileInfo{
		Name:    info.Name(),
		Path:    path,
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
	}, nil
}

func GetFolderSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// defaultSavePath is where inbound transfers land. It is overridable at
// runtime so the Settings page takes effect without a restart; previously the
// configured value was written to config.json but never read by anything.
var (
	defaultSavePathMu sync.RWMutex
	defaultSavePath   = platformDefaultSavePath()
)

func platformDefaultSavePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads", "LanFileTransfer")
}

// SetDefaultSavePath overrides the destination for inbound transfers. An empty
// path restores the platform default. Returns the path actually in effect.
func SetDefaultSavePath(path string) string {
	defaultSavePathMu.Lock()
	if strings.TrimSpace(path) == "" {
		defaultSavePath = platformDefaultSavePath()
	} else {
		defaultSavePath = path
	}
	resolved := defaultSavePath
	defaultSavePathMu.Unlock()

	EnsureDir(resolved)
	return resolved
}

func GetDefaultSavePath() string {
	defaultSavePathMu.RLock()
	defer defaultSavePathMu.RUnlock()
	return defaultSavePath
}

func ResolvePath(baseDir, fileName string) string {
	return filepath.Join(baseDir, fileName)
}

func IsPathWritable(path string) bool {
	testFile := filepath.Join(path, ".write_test")
	if err := os.MkdirAll(path, 0755); err != nil {
		return false
	}
	if err := os.WriteFile(testFile, []byte{}, 0644); err != nil {
		return false
	}
	os.Remove(testFile)
	return true
}

func GetAvailableFileName(path string) string {
	if !FileExists(path) {
		return path
	}

	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := filepath.Base(path)
	name := base[:len(base)-len(ext)]

	for i := 1; i < 1000; i++ {
		newName := fmt.Sprintf("%s_%d%s", name, i, ext)
		newPath := filepath.Join(dir, newName)
		if !FileExists(newPath) {
			return newPath
		}
	}

	return path
}
