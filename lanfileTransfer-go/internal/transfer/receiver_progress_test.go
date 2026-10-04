package transfer

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"LanFileTransfer-Go/internal/config"
	"LanFileTransfer-Go/internal/file"
)

func TestProgressLimiterRateLimits(t *testing.T) {
	p := newProgressLimiter()
	if !p.allow() {
		t.Fatal("the first notification must be allowed")
	}
	// Every one of these lands within microseconds of the first, so all of them
	// must be rejected. An unthrottled loop would allow every call.
	for i := 0; i < 1000; i++ {
		if p.allow() {
			t.Fatalf("call %d after the first should have been throttled", i)
		}
	}
	time.Sleep(progressEventInterval + 20*time.Millisecond)
	if !p.allow() {
		t.Error("the limiter should reopen once the interval has elapsed")
	}
}

func TestSpeedMeterSamples(t *testing.T) {
	m := &speedMeter{window: 50 * time.Millisecond}

	if _, updated := m.update(1000); updated {
		t.Error("the first sample has no elapsed time and must not report")
	}
	if speed, updated := m.update(1100); updated || speed != 0 {
		t.Errorf("inside the window got (%.1f, %v), want (0, false)", speed, updated)
	}
	time.Sleep(60 * time.Millisecond)
	speed, updated := m.update(1200)
	if !updated {
		t.Fatal("the window should have elapsed")
	}
	// 200 bytes over ~60ms is a few KB/s; just assert it is the right order.
	if speed < 1000 || speed > 100000 {
		t.Errorf("speed = %.1f B/s, want order 1e3..1e5", speed)
	}
}

// Progress notifications are delivered synchronously to the webview, one per
// chunk, so an unthrottled stream saturates the UI thread at line rate. The
// receiving task must still end up reporting completion.
func TestReceiverThrottlesProgressAndCompletes(t *testing.T) {
	const sizeMB = 1 // 16 chunks

	saveDir := t.TempDir()
	file.SetDefaultSavePath(saveDir)
	t.Cleanup(func() { file.SetDefaultSavePath("") })

	src := makeSource(t, t.TempDir(), sizeMB)

	rxCfg := config.DefaultConfig()
	rxCfg.ListenPort = freeTCPPort(t)
	rxCfg.AutoReceive = true
	rx := NewTransferManager(rxCfg, nil)

	var mu sync.Mutex
	var started, progress, completed int
	var lastProgress float64 = -1
	var completedAt float64
	rx.OnEvent(func(e *TransferEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch e.Type {
		case EventTransferStarted:
			started++
		case EventTransferProgress:
			progress++
			if e.Task.Progress < lastProgress {
				t.Errorf("progress went backwards: %.2f after %.2f",
					e.Task.Progress, lastProgress)
			}
			if e.Task.BytesTransferred > e.Task.FileSize {
				t.Errorf("received %d bytes for a %d byte file",
					e.Task.BytesTransferred, e.Task.FileSize)
			}
			lastProgress = e.Task.Progress
		case EventTransferCompleted:
			completed++
			completedAt = e.Task.Progress
		}
	})

	if err := rx.Start(); err != nil {
		t.Fatalf("receiver start: %v", err)
	}
	defer rx.Stop()
	time.Sleep(150 * time.Millisecond)

	txCfg := config.DefaultConfig()
	txCfg.ListenPort = freeTCPPort(t)
	tx := NewTransferManager(txCfg, nil)

	taskID, err := tx.SendFile("peer", "Peer",
		fmt.Sprintf("127.0.0.1:%d", rxCfg.ListenPort), src)
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	tx.executeSend(taskID)

	deadline := time.Now().Add(30 * time.Second)
	for {
		rt := rx.GetTask(taskID)
		if rt != nil && rt.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("receiver never completed: %+v", rt)
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	totalChunks := sizeMB * 1024 * 1024 / DefaultChunkSize
	t.Logf("chunks=%d started=%d progress=%d completed=%d lastProgress=%.2f completedAt=%.2f",
		totalChunks, started, progress, completed, lastProgress, completedAt)

	if started != 1 {
		t.Errorf("started events = %d, want 1", started)
	}
	if completed != 1 {
		t.Errorf("completed events = %d, want 1", completed)
	}
	if completedAt != 100 {
		t.Errorf("completed event progress = %.2f, want 100", completedAt)
	}
	if progress == 0 {
		t.Error("no progress event at all")
	}
	if progress >= totalChunks {
		t.Errorf("emitted %d progress events for %d chunks; notifications are not rate limited",
			progress, totalChunks)
	}

	// The task itself must still be exactly correct regardless of throttling.
	rt := rx.GetTask(taskID)
	if rt == nil {
		t.Fatal("receiver lost the task")
	}
	if rt.BytesTransferred != rt.FileSize {
		t.Errorf("receiver tracked %d of %d bytes", rt.BytesTransferred, rt.FileSize)
	}
	if rt.Progress != 100 {
		t.Errorf("receiver task progress = %.2f, want 100", rt.Progress)
	}
}
