package share

import "testing"

func TestTransferBufferPool(t *testing.T) {
	buffer := AcquireTransferBuffer()
	if len(buffer) != TransferBufferSize {
		t.Fatalf("buffer len = %d, want %d", len(buffer), TransferBufferSize)
	}
	if cap(buffer) != TransferBufferSize {
		t.Fatalf("buffer cap = %d, want %d", cap(buffer), TransferBufferSize)
	}

	ReleaseTransferBuffer(buffer[:10])

	reused := AcquireTransferBuffer()
	if len(reused) != TransferBufferSize {
		t.Fatalf("reused buffer len = %d, want %d", len(reused), TransferBufferSize)
	}
	ReleaseTransferBuffer(reused)
}

func TestReleaseTransferBufferIgnoresUnexpectedSize(t *testing.T) {
	ReleaseTransferBuffer(make([]byte, TransferBufferSize+1))
	ReleaseTransferBuffer(make([]byte, TransferBufferSize-1))
}
