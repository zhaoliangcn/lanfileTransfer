package network

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestMarshalUnmarshalHeader(t *testing.T) {
	header := &TransferHeader{
		TransferID:  "test-transfer-id",
		FileName:    "testfile.txt",
		FileSize:    1024,
		ChunkIndex:  5,
		TotalChunks: 10,
		Checksum:    "abc123def456",
	}

	data, err := MarshalHeader(header)
	if err != nil {
		t.Fatalf("MarshalHeader failed: %v", err)
	}

	if len(data) <= HeaderSize {
		t.Errorf("marshaled data too small: %d bytes", len(data))
	}

	reader := bytes.NewReader(data)
	parsed, err := UnmarshalHeader(reader)
	if err != nil {
		t.Fatalf("UnmarshalHeader failed: %v", err)
	}

	if parsed.TransferID != header.TransferID {
		t.Errorf("TransferID mismatch: got '%s', want '%s'", parsed.TransferID, header.TransferID)
	}
	if parsed.FileName != header.FileName {
		t.Errorf("FileName mismatch: got '%s', want '%s'", parsed.FileName, header.FileName)
	}
	if parsed.FileSize != header.FileSize {
		t.Errorf("FileSize mismatch: got %d, want %d", parsed.FileSize, header.FileSize)
	}
	if parsed.ChunkIndex != header.ChunkIndex {
		t.Errorf("ChunkIndex mismatch: got %d, want %d", parsed.ChunkIndex, header.ChunkIndex)
	}
	if parsed.TotalChunks != header.TotalChunks {
		t.Errorf("TotalChunks mismatch: got %d, want %d", parsed.TotalChunks, header.TotalChunks)
	}
	if parsed.Checksum != header.Checksum {
		t.Errorf("Checksum mismatch: got '%s', want '%s'", parsed.Checksum, header.Checksum)
	}
}

func TestMarshalHeaderTooLarge(t *testing.T) {
	longName := strings.Repeat("a", MaxHeaderSize+1)
	header := &TransferHeader{
		TransferID: "id",
		FileName:   longName,
		FileSize:   100,
	}

	_, err := MarshalHeader(header)
	if err == nil {
		t.Errorf("expected error for oversized header")
	}
}

func TestUnmarshalHeaderInvalidSize(t *testing.T) {
	sizeBuf := []byte{0, 0, 0, 0}
	reader := bytes.NewReader(sizeBuf)
	_, err := UnmarshalHeader(reader)
	if err == nil {
		t.Errorf("expected error for zero size")
	}

	largeSizeBuf := []byte{0, 0, 2, 0}
	reader = bytes.NewReader(largeSizeBuf)
	_, err = UnmarshalHeader(reader)
	if err == nil {
		t.Errorf("expected error for size exceeding MaxHeaderSize")
	}
}

func TestUnmarshalHeaderTruncated(t *testing.T) {
	sizeBuf := []byte{0, 0, 0, 10}
	truncatedData := []byte{1, 2, 3}
	reader := bytes.NewReader(append(sizeBuf, truncatedData...))
	_, err := UnmarshalHeader(reader)
	if err == nil {
		t.Errorf("expected error for truncated data")
	}
}

func TestMarshalUnmarshalControlMessage(t *testing.T) {
	msg := &ControlMessage{
		Type:       ControlTypeDiscovery,
		DeviceID:   "device-1",
		DeviceName: "TestDevice",
		Port:       9876,
		Timestamp:  1234567890,
	}

	data, err := MarshalControlMessage(msg)
	if err != nil {
		t.Fatalf("MarshalControlMessage failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("marshaled data should not be empty")
	}

	parsed, err := UnmarshalControlMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalControlMessage failed: %v", err)
	}

	if parsed.Type != msg.Type {
		t.Errorf("Type mismatch: got '%s', want '%s'", parsed.Type, msg.Type)
	}
	if parsed.DeviceID != msg.DeviceID {
		t.Errorf("DeviceID mismatch: got '%s', want '%s'", parsed.DeviceID, msg.DeviceID)
	}
	if parsed.DeviceName != msg.DeviceName {
		t.Errorf("DeviceName mismatch: got '%s', want '%s'", parsed.DeviceName, msg.DeviceName)
	}
	if parsed.Port != msg.Port {
		t.Errorf("Port mismatch: got %d, want %d", parsed.Port, msg.Port)
	}
}

func TestMarshalControlMessageTooLarge(t *testing.T) {
	msg := &ControlMessage{
		Type:       strings.Repeat("x", ControlMsgSize),
		DeviceName: "large",
	}

	_, err := MarshalControlMessage(msg)
	if err == nil {
		t.Errorf("expected error for oversized control message")
	}
}

func TestUnmarshalControlMessageInvalidJSON(t *testing.T) {
	_, err := UnmarshalControlMessage([]byte("invalid json"))
	if err == nil {
		t.Errorf("expected error for invalid JSON")
	}
}

func TestBuildTransferPacket(t *testing.T) {
	header := &TransferHeader{
		TransferID:  "test-id",
		FileName:    "test.bin",
		FileSize:    5,
		ChunkIndex:  0,
		TotalChunks: 1,
	}
	data := []byte("hello")

	packet, err := BuildTransferPacket(header, data)
	if err != nil {
		t.Fatalf("BuildTransferPacket failed: %v", err)
	}

	reader := bytes.NewReader(packet)
	parsedHeader, parsedData, err := ParseTransferPacket(reader)
	if err != nil {
		t.Fatalf("ParseTransferPacket failed: %v", err)
	}

	if parsedHeader.TransferID != header.TransferID {
		t.Errorf("TransferID mismatch")
	}
	if !bytes.Equal(parsedData, data) {
		t.Errorf("data mismatch: got '%s', want '%s'", string(parsedData), string(data))
	}
}

func TestBuildTransferPacketEmptyData(t *testing.T) {
	header := &TransferHeader{
		TransferID: "test-id",
		FileName:   "empty.bin",
		FileSize:   0,
	}

	packet, err := BuildTransferPacket(header, []byte{})
	if err != nil {
		t.Fatalf("BuildTransferPacket failed: %v", err)
	}

	reader := bytes.NewReader(packet)
	parsedHeader, parsedData, err := ParseTransferPacket(reader)
	if err != nil {
		t.Fatalf("ParseTransferPacket failed: %v", err)
	}

	if parsedHeader.TransferID != header.TransferID {
		t.Errorf("TransferID mismatch")
	}
	if len(parsedData) != 0 {
		t.Errorf("expected empty data, got %d bytes", len(parsedData))
	}
}

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP()
	if ip == "" {
		t.Errorf("GetLocalIP should return a non-empty IP")
	}
}

func TestControlTypeConstants(t *testing.T) {
	if ControlTypeDiscovery != "discovery" {
		t.Errorf("ControlTypeDiscovery should be 'discovery'")
	}
	if ControlTypeTransferRequest != "transfer_request" {
		t.Errorf("ControlTypeTransferRequest should be 'transfer_request'")
	}
	if ControlTypeTransferControl != "transfer_control" {
		t.Errorf("ControlTypeTransferControl should be 'transfer_control'")
	}
}

func TestControlCommandConstants(t *testing.T) {
	if ControlCommandCancel != "cancel" {
		t.Errorf("ControlCommandCancel should be 'cancel'")
	}
	if ControlCommandPause != "pause" {
		t.Errorf("ControlCommandPause should be 'pause'")
	}
	if ControlCommandResume != "resume" {
		t.Errorf("ControlCommandResume should be 'resume'")
	}
}

func TestBroadcastAddress(t *testing.T) {
	ip := GetLocalIP()
	if ip == "" {
		t.Skip("no local IP available")
	}

	addr := GetBroadcastAddress(ip)
	if addr == "" {
		t.Errorf("broadcast address should not be empty")
	}
}

func TestMarshalUnmarshalControlMessageAllFields(t *testing.T) {
	msg := &ControlMessage{
		Type:       ControlTypeTransferRequest,
		DeviceID:   "dev-1",
		DeviceName: "MyPC",
		Port:       9999,
		Timestamp:  999888777,
		TransferID: "transfer-abc",
		FileName:   "document.pdf",
		FileSize:   1024000,
		Command:    ControlCommandPause,
	}

	data, err := MarshalControlMessage(msg)
	if err != nil {
		t.Fatalf("MarshalControlMessage failed: %v", err)
	}

	parsed, err := UnmarshalControlMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalControlMessage failed: %v", err)
	}

	if !reflect.DeepEqual(msg, parsed) {
		t.Errorf("round-trip mismatch:\n  original: %+v\n  parsed:   %+v", msg, parsed)
	}
}

func TestParseTransferPacketIncomplete(t *testing.T) {
	_, _, err := ParseTransferPacket(bytes.NewReader([]byte{0, 0, 0, 5, 1, 2}))
	if err == nil {
		t.Errorf("expected error for incomplete packet")
	}
}

// A resumed transfer starts at a non-zero chunk, so the receiver cannot derive
// the write position from the running byte count. Both the wire length and
// ChunkOffset must survive a round trip for the C receiver's WriteAt to land on
// the right byte.
func TestTransferHeaderChunkOffsetRoundTrip(t *testing.T) {
	header := &TransferHeader{
		TransferID:    "transfer_resume_test",
		FileName:      "big.bin",
		FileSize:      1024,
		TotalFileSize: 10 * 1024,
		ChunkIndex:    7,
		ChunkOffset:   7 * 64 * 1024,
		TotalChunks:   10,
		Checksum:      "d41d8cd98f00b204e9800998ecf8427e",
	}

	packet, err := BuildTransferPacket(header, []byte("payload"))
	if err != nil {
		t.Fatalf("BuildTransferPacket failed: %v", err)
	}

	parsed, data, err := ParseTransferPacket(bytes.NewReader(packet))
	if err != nil {
		t.Fatalf("ParseTransferPacket failed: %v", err)
	}

	if parsed.ChunkOffset != header.ChunkOffset {
		t.Errorf("ChunkOffset mismatch: got %d, want %d", parsed.ChunkOffset, header.ChunkOffset)
	}
	if parsed.ChunkIndex != header.ChunkIndex {
		t.Errorf("ChunkIndex mismatch: got %d, want %d", parsed.ChunkIndex, header.ChunkIndex)
	}
	if parsed.TotalFileSize != header.TotalFileSize {
		t.Errorf("TotalFileSize mismatch: got %d, want %d", parsed.TotalFileSize, header.TotalFileSize)
	}
	if string(data) != "payload" {
		t.Errorf("payload mismatch: got %q, want %q", data, "payload")
	}
}

// The C implementation omits relativePath when it is empty while Go emits it
// unconditionally for headers it builds. Both variants must parse, and the
// payload must still be found immediately after the wire-declared header.
func TestTransferHeaderEmptyRelativePath(t *testing.T) {
	for _, tt := range []struct {
		name         string
		relativePath string
	}{
		{"with relative path", "folder/sub/file.txt"},
		{"empty relative path", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			header := &TransferHeader{
				TransferID:   "t1",
				FileName:     "f.txt",
				FileSize:     4,
				ChunkIndex:   0,
				ChunkOffset:  0,
				TotalChunks:  1,
				RelativePath: tt.relativePath,
			}

			packet, err := BuildTransferPacket(header, []byte("data"))
			if err != nil {
				t.Fatalf("BuildTransferPacket failed: %v", err)
			}

			parsed, data, err := ParseTransferPacket(bytes.NewReader(packet))
			if err != nil {
				t.Fatalf("ParseTransferPacket failed: %v", err)
			}
			if string(data) != "data" {
				t.Errorf("payload mismatch: got %q, want %q", data, "data")
			}
			if parsed.RelativePath != tt.relativePath {
				t.Errorf("RelativePath mismatch: got %q, want %q", parsed.RelativePath, tt.relativePath)
			}
		})
	}
}
