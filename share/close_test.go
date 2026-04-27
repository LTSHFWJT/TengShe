package share

import "testing"

type testCloser struct {
	closed bool
}

func (closer *testCloser) Close() error {
	closer.closed = true
	return nil
}

func TestCloseQuietly(t *testing.T) {
	CloseQuietly(nil)

	closer := &testCloser{}
	CloseQuietly(closer)
	if !closer.closed {
		t.Fatal("closer was not closed")
	}
}
