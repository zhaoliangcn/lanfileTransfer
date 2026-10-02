package transfer

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCkptWriteFile(b *testing.B) {
	path := filepath.Join(b.TempDir(), "a.checkpoint")
	data := make([]byte, 9228)
	os.WriteFile(path, data, 0644)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := os.WriteFile(path, data, 0644); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCkptPersistentHandle(b *testing.B) {
	path := filepath.Join(b.TempDir(), "b.checkpoint")
	data := make([]byte, 9228)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			b.Fatal(err)
		}
		if err := f.Truncate(int64(len(data))); err != nil {
			b.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			b.Fatal(err)
		}
	}
}
