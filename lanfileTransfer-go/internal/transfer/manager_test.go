package transfer

import (
	"bytes"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// The send loop must leave behind a checkpoint that a later executeSend can
// pick up, and that checkpoint has to survive the move from the old
// json.Marshal-of-the-whole-table format to a bitmap. Exercises the full
// write -> fail -> read -> resume cycle: the first run is cut short on chunk 1
// by a truncated source, the second run must send only the missing chunks.
func TestExecuteSendPersistsBitmapCheckpointAndResumes(t *testing.T) {
	const chunkSize = 64 * 1024
	// 4 full chunks + a 1000-byte tail => 5 chunks.
	payload := make([]byte, chunkSize*4+1000)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand failed: %v", err)
	}
	const totalChunks = int64(5)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.bin")
	if err := os.WriteFile(srcPath, payload, 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.ChunkSize = chunkSize
	mgr := NewTransferManager(cfg, nil)

	// --- phase 1: capture the real size, then truncate so chunk 1 reads short.
	rx1, _, accepted1 := drainReceiver()
	defer rx1.close()

	taskID, err := mgr.SendFile("peer-1", "Peer One", rx1.addr, srcPath)
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	if err := os.Truncate(srcPath, chunkSize+10); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	mgr.executeSend(taskID)

	select {
	case <-accepted1:
		t.Log("phase1 receiver accepted the connection")
	case <-time.After(5 * time.Second):
		t.Fatal("phase1 receiver never accepted a connection")
	}

	task := mgr.GetTask(taskID)
	if task == nil {
		t.Fatal("task missing after first send")
	}
	if task.Status == StatusCompleted {
		t.Fatal("first run completed, want failure so a checkpoint exists")
	}
	if task.CheckpointPath == "" {
		t.Fatal("failed send left no checkpoint path")
	}

	// --- phase 1 assertion: bitmap on disk, not JSON.
	cpData, err := os.ReadFile(task.CheckpointPath)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	wantSize := checkpointHeaderSize + int((totalChunks+7)/8)
	if len(cpData) != wantSize {
		t.Fatalf("checkpoint is %d bytes, want %d (bitmap)", len(cpData), wantSize)
	}
	if cpData[0] == '[' {
		t.Fatal("checkpoint is legacy JSON, want bitmap")
	}

	half := NewChunkManager(int64(len(payload)), chunkSize)
	if err := half.Deserialize(cpData); err != nil {
		t.Fatalf("read back our own checkpoint: %v", err)
	}
	if half.GetTransferredCount() != 1 {
		t.Fatalf("checkpoint records %d completed chunks, want 1", half.GetTransferredCount())
	}

	// --- phase 2: restore the source and resume against a fresh receiver.
	if err := os.WriteFile(srcPath, payload, 0644); err != nil {
		t.Fatalf("restore source: %v", err)
	}
	rx2, got, accepted := drainReceiver()
	defer rx2.close()
	t.Logf("phase2 receiver listening on %s", rx2.addr)
	// GetTask hands back a copy, so point the manager's own task at the new peer.
	mgr.mu.Lock()
	mgr.tasks[taskID].PeerAddr = rx2.addr
	mgr.mu.Unlock()

	mgr.executeSend(taskID)

	task = mgr.GetTask(taskID)
	t.Logf("phase2 status=%q err=%q bytes=%d", task.Status, task.Error, task.BytesTransferred)
	if task.Status != StatusCompleted {
		t.Fatalf("resumed send status = %q (%s), want completed", task.Status, task.Error)
	}
	if task.BytesTransferred != int64(len(payload)) {
		t.Errorf("BytesTransferred = %d, want %d", task.BytesTransferred, len(payload))
	}
	if _, err := os.Stat(task.CheckpointPath); !os.IsNotExist(err) {
		t.Error("checkpoint should be removed after a completed transfer")
	}

	select {
	case <-accepted:
		t.Log("phase2 receiver accepted the connection")
	case <-time.After(5 * time.Second):
		t.Fatal("receiver never accepted a connection")
	}

	var chunks []chunkRecord
	select {
	case chunks = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver accepted but never finished reading")
	}
	var indices []int64
	var reassembled bytes.Buffer
	for _, c := range chunks {
		indices = append(indices, c.index)
		if err := writeAtOffset(&reassembled, c.data, c.index*int64(chunkSize)); err != nil {
			t.Fatalf("writeAtOffset: %v", err)
		}
	}

	if len(chunks) != 4 {
		t.Errorf("resumed send transmitted %d chunks (indices %v), want only the 4 missing ones", len(chunks), indices)
	}
	for i, want := range []int64{1, 2, 3, 4} {
		if i >= len(indices) || indices[i] != want {
			t.Errorf("chunk %d = %v, want indices [1 2 3 4]", i, indices)
			break
		}
	}

	// The resumed bytes must reproduce the source tail exactly. writeAtOffset
	// zero-pads up to the first write, so byte 0..chunkSize-1 holds the gap the
	// checkpoint claims is already on disk.
	gotBytes := reassembled.Bytes()
	if len(gotBytes) < chunkSize {
		t.Fatalf("reassembled %d bytes, want at least %d", len(gotBytes), chunkSize)
	}
	gotResumed := gotBytes[chunkSize:]
	wantResumed := payload[chunkSize:]
	if !bytes.Equal(gotResumed, wantResumed) {
		t.Errorf("resumed bytes mismatch: got %d, want %d (first diff at %d)",
			len(gotResumed), len(wantResumed), firstDiff(gotResumed, wantResumed))
	}
	if !bytes.Equal(gotBytes[:chunkSize], make([]byte, chunkSize)) {
		t.Error("chunk 0 was re-sent even though the checkpoint marks it complete")
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
