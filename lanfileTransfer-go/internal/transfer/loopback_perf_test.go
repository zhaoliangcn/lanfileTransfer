package transfer

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"LanFileTransfer-Go/internal/config"
	"LanFileTransfer-Go/internal/file"
	"LanFileTransfer-Go/internal/network"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func makeSource(t *testing.T, dir string, sizeMB int) string {
	t.Helper()
	path := filepath.Join(dir, "src.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	defer f.Close()

	block := make([]byte, 4<<20)
	if _, err := rand.Read(block); err != nil {
		t.Fatalf("rand: %v", err)
	}
	for written := 0; written < sizeMB; {
		n := 4
		if sizeMB-written < n {
			n = sizeMB - written
		}
		if _, err := f.Write(block[:n<<20]); err != nil {
			t.Fatalf("write source: %v", err)
		}
		written += n
	}
	return path
}

// warmFile faults the whole file into the page cache so the measured run is not
// dominated by a cold read.
func warmFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	if _, err := io.Copy(io.Discard, f); err != nil {
		t.Fatalf("warm: %v", err)
	}
}

type runResult struct {
	sendOnly time.Duration
	e2e      time.Duration
}

func loopbackTransfer(t *testing.T, src string, sizeMB int, withEventListener bool) runResult {
	t.Helper()

	saveDir := filepath.Join(t.TempDir(), "save")
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		t.Fatalf("save dir: %v", err)
	}
	file.SetDefaultSavePath(saveDir)

	rxCfg := config.DefaultConfig()
	rxCfg.ListenPort = freeTCPPort(t)
	rxCfg.AutoReceive = true
	rx := NewTransferManager(rxCfg, nil)
	if withEventListener {
		// 近似 Wails runtime.EventsEmit 的同步 JSON 序列化部分
		rx.OnEvent(func(e *TransferEvent) {
			if e.Type == EventTransferProgress {
				_, _ = json.Marshal(e)
			}
		})
	}
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

	start := time.Now()
	tx.executeSend(taskID)
	sendOnly := time.Since(start)

	task := tx.GetTask(taskID)
	if task == nil || task.Status != StatusCompleted {
		t.Fatalf("send failed: %+v", task)
	}

	deadline := time.Now().Add(60 * time.Second)
	for {
		rxTask := rx.GetTask(taskID)
		if rxTask != nil && rxTask.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("receiver never completed: %+v", rxTask)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return runResult{sendOnly: sendOnly, e2e: time.Since(start)}
}

func mbps(sizeMB int, d time.Duration) float64 {
	return float64(sizeMB) / d.Seconds()
}

// 纯协议基线：同样的 BuildTransferPacket + ParseTransferPacket，没有 MD5、
// 没有文件 IO、没有事件 —— 用来界定「网络+序列化」能达到多少。
func rawProtocolBaseline(t *testing.T, sizeMB int) time.Duration {
	t.Helper()

	payload := make([]byte, 4<<20)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan int64, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- -1
			return
		}
		defer conn.Close()
		c := network.NewTCPConnection(conn, &network.TCPConfig{})
		var got int64
		for {
			h, data, err := network.ParseTransferPacket(c)
			if err != nil {
				done <- got
				return
			}
			_ = h
			got += int64(len(data))
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c := network.NewTCPConnection(conn, &network.TCPConfig{})

	chunk := make([]byte, 64*1024)
	start := time.Now()
	var sent int64
	target := int64(sizeMB) << 20
	for sent < target {
		n := copy(chunk, payload[sent%int64(len(payload)):])
		header := &network.TransferHeader{
			TransferID: "raw", ChunkIndex: sent / (64 * 1024),
			ChunkOffset: sent, FileSize: int64(n),
		}
		packet, err := network.BuildTransferPacket(header, chunk[:n])
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if err := c.Send(packet); err != nil {
			t.Fatalf("send: %v", err)
		}
		sent += int64(n)
	}
	conn.Close()
	elapsed := time.Since(start)

	got := <-done
	if got != sent {
		t.Errorf("receiver got %d bytes, sender sent %d", got, sent)
	}
	t.Logf("raw protocol: %dMB in %v -> %.1f MB/s (无 MD5/文件/事件)",
		sizeMB, elapsed.Round(time.Millisecond), mbps(sizeMB, elapsed))
	return elapsed
}

func TestLoopbackThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("throughput test")
	}
	const sizeMB = 128

	t.Cleanup(func() { file.SetDefaultSavePath("") })

	src := makeSource(t, t.TempDir(), sizeMB)
	warmFile(t, src)

	t.Run("rawProtocol", func(t *testing.T) { rawProtocolBaseline(t, sizeMB) })

	variants := []struct {
		name     string
		listener bool
	}{{"noListener", false}, {"jsonListener", true}}

	for i := 0; i < 3; i++ {
		for _, v := range variants {
			t.Run(fmt.Sprintf("%s-%d", v.name, i), func(t *testing.T) {
				warmFile(t, src)
				r := loopbackTransfer(t, src, sizeMB, v.listener)
				t.Logf("%s: send-only %.1f MB/s  e2e %.1f MB/s",
					v.name, mbps(sizeMB, r.sendOnly), mbps(sizeMB, r.e2e))
			})
		}
	}
}

func TestLoopbackThroughputLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("throughput test")
	}
	spec := os.Getenv("LFT_LARGE_MB")
	if spec == "" {
		t.Skip("set LFT_LARGE_MB=<MB> to run the large-file loopback test")
	}
	sizeMB, err := strconv.Atoi(spec)
	if err != nil || sizeMB <= 0 {
		t.Fatalf("LFT_LARGE_MB=%q is not a positive integer", spec)
	}

	t.Cleanup(func() { file.SetDefaultSavePath("") })

	src := makeSource(t, t.TempDir(), sizeMB)
	warmFile(t, src)
	r := loopbackTransfer(t, src, sizeMB, false)
	t.Logf("large %dMB: send-only %.1f MB/s  e2e %.1f MB/s",
		sizeMB, mbps(sizeMB, r.sendOnly), mbps(sizeMB, r.e2e))
}
