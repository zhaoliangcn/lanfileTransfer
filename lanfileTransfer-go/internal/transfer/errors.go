package transfer

import "errors"

var (
	ErrPeerNotFound     = errors.New("peer not found")
	ErrFileNotFound     = errors.New("file not found")
	ErrTransferFailed   = errors.New("transfer failed")
	ErrConnectionLost   = errors.New("connection lost")
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrTaskNotFound     = errors.New("task not found")
	ErrTaskNotResumable = errors.New("task not in a resumable state")
	ErrNotSender        = errors.New("only sender can resume transfer")
)
