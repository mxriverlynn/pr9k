package interactionchannel

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"testing"
)

// TestFrameSizeCapRejectsOversize verifies that readMessage returns an error
// when the announced frame length exceeds maxMessageSize, without attempting
// to allocate or read the oversized body.
func TestFrameSizeCapRejectsOversize(t *testing.T) {
	t.Parallel()

	// Announce 2 MiB — double the 1 MiB cap.
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 2<<20)
	br := bufio.NewReader(bytes.NewReader(hdr[:]))

	_, err := readMessage(br)
	if err == nil {
		t.Fatal("readMessage: expected error for oversize frame, got nil")
	}
}
