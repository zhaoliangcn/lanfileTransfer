package network

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

type TransferHeader struct {
	TransferID    string `json:"transferId"`
	FileName      string `json:"fileName"`
	FileSize      int64  `json:"fileSize"`
	TotalFileSize int64  `json:"totalFileSize"`
	ChunkIndex    int64  `json:"chunkIndex"`
	ChunkOffset   int64  `json:"chunkOffset"`
	TotalChunks   int64  `json:"totalChunks"`
	Checksum      string `json:"checksum"`
	RelativePath  string `json:"relativePath,omitempty"`
}

type ControlMessage struct {
	Type       string `json:"type"`
	DeviceID   string `json:"deviceId,omitempty"`
	DeviceName string `json:"deviceName,omitempty"`
	Port       int    `json:"port,omitempty"`
	Timestamp  int64  `json:"timestamp,omitempty"`
	TransferID string `json:"transferId,omitempty"`
	FileName   string `json:"fileName,omitempty"`
	FileSize   int64  `json:"fileSize,omitempty"`
	Command    string `json:"command,omitempty"`
	Message    string `json:"message,omitempty"`
	TargetID   string `json:"targetId,omitempty"`
	// AuthToken is the shared secret for privileged operations (shutdown,
	// restart). Absent in older builds, which is exactly why the receiver
	// rejects such messages.
	AuthToken string `json:"authToken,omitempty"`
}

const (
	ControlTypeDiscovery       = "discovery"
	ControlTypeTransferRequest = "transfer_request"
	ControlTypeTransferControl = "transfer_control"
	ControlTypeChatMessage     = "chat_message"
	ControlTypeSystemCommand   = "system_command"
)

const (
	ControlCommandCancel = "cancel"
	ControlCommandPause  = "pause"
	ControlCommandResume = "resume"
)

const (
	ControlCommandShutdown = "shutdown"
	ControlCommandRestart  = "restart"
)

const (
	HeaderSize     = 4
	MaxHeaderSize  = 4096
	ControlMsgSize = 1024
)

func MarshalHeader(header *TransferHeader) ([]byte, error) {
	data, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}

	if len(data) > MaxHeaderSize {
		return nil, fmt.Errorf("header too large: %d bytes", len(data))
	}

	packet := make([]byte, HeaderSize+len(data))
	binary.BigEndian.PutUint32(packet[:HeaderSize], uint32(len(data)))
	copy(packet[HeaderSize:], data)

	return packet, nil
}

func UnmarshalHeader(reader io.Reader) (*TransferHeader, error) {
	sizeBuf := make([]byte, HeaderSize)
	if _, err := io.ReadFull(reader, sizeBuf); err != nil {
		return nil, err
	}

	dataSize := binary.BigEndian.Uint32(sizeBuf)
	if dataSize == 0 || dataSize > MaxHeaderSize {
		return nil, fmt.Errorf("invalid header size: %d", dataSize)
	}

	data := make([]byte, dataSize)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	var header TransferHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}

	return &header, nil
}

func MarshalControlMessage(msg *ControlMessage) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	if len(data) > ControlMsgSize {
		return nil, fmt.Errorf("control message too large: %d bytes", len(data))
	}

	return data, nil
}

func UnmarshalControlMessage(data []byte) (*ControlMessage, error) {
	var msg ControlMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func BuildTransferPacket(header *TransferHeader, chunkData []byte) ([]byte, error) {
	headerData, err := MarshalHeader(header)
	if err != nil {
		return nil, err
	}

	packet := make([]byte, len(headerData)+len(chunkData))
	copy(packet, headerData)
	copy(packet[len(headerData):], chunkData)

	return packet, nil
}

func ParseTransferPacket(reader io.Reader) (*TransferHeader, []byte, error) {
	header, err := UnmarshalHeader(reader)
	if err != nil {
		return nil, nil, err
	}

	chunkData := make([]byte, header.FileSize)
	n, err := io.ReadFull(reader, chunkData)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, nil, err
	}

	return header, chunkData[:n], nil
}

func GetLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func GetBroadcastAddress(subnet string) string {
	ip := net.ParseIP(subnet)
	if ip == nil {
		return "255.255.255.255"
	}

	ip = ip.To4()
	if ip == nil {
		return "255.255.255.255"
	}

	return "255.255.255.255"
}
