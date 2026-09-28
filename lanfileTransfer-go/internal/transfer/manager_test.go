package transfer

import (
	"bytes"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"LanFileTransfer-Go/internal/config"
	"LanFileTransfer-Go/internal/file"
	"LanFileTransfer-Go/internal/network"
)

// Regression test: the send path used to take a 64KB buffer from a pool while
// seeking by chunkSize. Any chunkSize other than 64KB therefore left holes in
// the transferred file. Drive a real transfer over a socket pair and compare
// bytes.
func TestExecuteSendPreservesBytesForNonDefaultChunkSize(t *testing.T) {
	for _, chunkSize := range []int{16 * 1024, 64 * 1024, 128 * 1024, 1024 * 1024} {
		t.Run(filepath.Base(t.Name())+itoa(chunkSize), func(t *testing.T) {
			payload := make([]byte, chunkSize*3+1234)
			if _, err := rand.Read(payload); err != nil {
				t.Fatalf("rand failed: %v", err)
			}

			dir := t.TempDir()
			srcPath := filepath.Join(dir, "source.bin")
			if err := os.WriteFile(srcPath, payload, 0644); err != nil {
				t.Fatalf("write source: %v", err)
			}

			// Receiver: stands in for the remote peer, reassembling by offset.
			received := make(chan []byte, 1)
			rx := startReceiver(func(c net.Conn) {
				var out bytes.Buffer
				conn := network.NewTCPConnection(c, &network.TCPConfig{})
				var expect int64
				for expect < int64(len(payload)) {
					header, data, err := network.ParseTransferPacket(conn)
					if err != nil {
						t.Errorf("receiver ParseTransferPacket: %v", err)
						received <- out.Bytes()
						return
					}
					if !VerifyChunk(data, header.Checksum) {
						t.Errorf("chunk %d checksum mismatch", header.ChunkIndex)
					}
					if err := writeAtOffset(&out, data, header.ChunkOffset); err != nil {
						t.Errorf("writeAtOffset: %v", err)
						received <- out.Bytes()
						return
					}
					expect += int64(len(data))
				}
				received <- out.Bytes()
			})
			defer rx.close()

			cfg := config.DefaultConfig()
			cfg.ChunkSize = chunkSize
			mgr := NewTransferManager(cfg, nil)
			if mgr.chunkSize != chunkSize {
				t.Fatalf("manager chunkSize = %d, want %d", mgr.chunkSize, chunkSize)
			}

			taskID, err := mgr.SendFile("peer-1", "Peer One", rx.addr, srcPath)
			if err != nil {
				t.Fatalf("SendFile: %v", err)
			}
			mgr.executeSend(taskID)

			task := mgr.GetTask(taskID)
			if task == nil {
				t.Fatal("task missing after send")
			}
			if task.Status != StatusCompleted {
				t.Fatalf("status = %q (%s), want completed", task.Status, task.Error)
			}
			if task.BytesTransferred != int64(len(payload)) {
				t.Errorf("BytesTransferred = %d, want %d", task.BytesTransferred, len(payload))
			}

			got := <-received
			if !bytes.Equal(got, payload) {
				t.Errorf("payload mismatch: got %d bytes, want %d bytes (first diff at %d)",
					len(got), len(payload), firstDiff(got, payload))
			}
		})
	}
}

// A file that shrinks between measuring it and reading it must not be reported
// as a successful transfer.
func TestExecuteSendFailsWhenFileIsTruncated(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "shrinking.bin")
	if err := os.WriteFile(srcPath, bytes.Repeat([]byte("A"), 64*1024), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	rx := startReceiver(func(c net.Conn) {
		conn := network.NewTCPConnection(c, &network.TCPConfig{})
		network.ParseTransferPacket(conn) // drain the first chunk, then hang up
	})
	defer rx.close()

	cfg := config.DefaultConfig()
	mgr := NewTransferManager(cfg, nil)
	taskID, err := mgr.SendFile("peer-1", "Peer One", rx.addr, srcPath)
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}

	// Truncate after the size was captured by SendFile.
	if err := os.Truncate(srcPath, 10); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	mgr.executeSend(taskID)

	task := mgr.GetTask(taskID)
	if task.Status == StatusCompleted {
		t.Errorf("status = completed for truncated file, want failed")
	}
	if task.Error == "" {
		t.Errorf("expected an error message for truncated file")
	}
}

// Resume must notify the UI with a dedicated event, otherwise the frontend keeps
// rendering the task under its previous "failed" status.
func TestResumeTransferEmitsResumedEvent(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewTransferManager(cfg, nil)

	info, err := file.GetFileInfo(writeTemp(t, []byte("data")))
	if err != nil {
		t.Fatalf("GetFileInfo: %v", err)
	}
	task := NewTransferTask("resume-me", "p1", "Peer", "127.0.0.1:1", info.Name, info.Path, info.Size, true)
	task.Status = StatusFailed
	task.Error = "boom"
	task.EndTime = "2024-01-01T00:00:00Z"

	mgr.tasks[task.ID] = task

	var got []TransferEventType
	mgr.OnEvent(func(e *TransferEvent) { got = append(got, e.Type) })

	if err := mgr.ResumeTransfer(task.ID); err != nil {
		t.Fatalf("ResumeTransfer: %v", err)
	}

	found := false
	for _, t := range got {
		if t == EventTransferResumed {
			found = true
		}
	}
	if !found {
		t.Errorf("events = %v, want one %q", got, EventTransferResumed)
	}
}

func writeAtOffset(buf *bytes.Buffer, data []byte, offset int64) error {
	if offset < 0 {
		return os.ErrInvalid
	}
	if int64(buf.Len()) < offset+int64(len(data)) {
		buf.Write(make([]byte, offset+int64(len(data))-int64(buf.Len())))
	}
	existing := buf.Bytes()[offset : offset+int64(len(data))]
	copy(existing, data)
	return nil
}

func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}
