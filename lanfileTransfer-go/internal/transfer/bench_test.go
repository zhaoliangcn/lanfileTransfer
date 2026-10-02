package transfer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkChunkStatsPerChunk(b *testing.B) {
	for _, gb := range []int{4, 16} {
		fileSize := int64(gb) << 30
		b.Run(fmt.Sprintf("%dGB", gb), func(b *testing.B) {
			cm := NewChunkManager(fileSize, 64*1024)
			f := cm.TotalChunks()
			for i := int64(0); i < f; i++ {
				cm.MarkTransferred(i, "x")
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = cm.GetTransferredBytes()
				_ = cm.GetProgress()
			}
		})
	}
}

func BenchmarkWriteCheckpoint(b *testing.B) {
	cm := NewChunkManager(int64(4500)<<20, 64*1024)
	total := cm.TotalChunks()
	for i := int64(0); i < total; i++ {
		cm.MarkTransferred(i, "x")
	}
	path := filepath.Join(b.TempDir(), "cp.checkpoint")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		b.Fatalf("seed: %v", err)
	}
	b.Cleanup(func() { closeCheckpoint(path) })
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeCheckpoint("task", path, cm)
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/write")
}

func BenchmarkGenerateChunkChecksum(b *testing.B) {
	data := make([]byte, 64*1024)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateChunkChecksum(data)
	}
}
