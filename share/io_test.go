package share

import (
	"bytes"
	"io"
	"net"
	"testing"
)

func TestWriteFull(t *testing.T) {
	client, server := net.Pipe()
	defer CloseQuietly(client)
	defer CloseQuietly(server)

	want := bytes.Repeat([]byte("x"), TransferBufferSize+17)
	errCh := make(chan error, 1)
	go func() {
		errCh <- WriteFull(client, want)
	}()

	got := make([]byte, len(want))
	if _, err := io.ReadFull(server, got); err != nil {
		t.Fatalf("ReadFull() error = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("WriteFull() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("WriteFull() wrote unexpected data")
	}
}
