package share

import (
	"errors"
	"net"
	"testing"
	"time"

	"TengShe/protocol"
)

func TestPrepareRawUpstreamConn(t *testing.T) {
	testPrepareRawConn(t, PrepareActiveUpstreamConn, PreparePassiveUpstreamConn)
}

func TestPrepareRawDownstreamConn(t *testing.T) {
	testPrepareRawConn(t, PrepareActiveDownstreamConn, PreparePassiveDownstreamConn)
}

func testPrepareRawConn(
	t *testing.T,
	active func(net.Conn, bool, string) (net.Conn, bool, error),
	passive func(net.Conn, bool) (net.Conn, bool, error),
) {
	t.Helper()

	oldUpstream, oldDownstream := protocol.Upstream, protocol.Downstream
	defer func() {
		protocol.Upstream, protocol.Downstream = oldUpstream, oldDownstream
	}()

	protocol.SetUpDownStream("raw", "raw")
	GeneratePreAuthToken("prepare-secret")

	client, server := net.Pipe()
	defer CloseQuietly(client)
	defer CloseQuietly(server)

	errCh := make(chan error, 2)
	go func() {
		_, tlsWrapped, err := passive(server, false)
		if tlsWrapped {
			errCh <- errors.New("passive conn unexpectedly wrapped TLS")
			return
		}
		errCh <- err
	}()

	go func() {
		_, tlsWrapped, err := active(client, false, "example.test")
		if tlsWrapped {
			errCh <- errors.New("active conn unexpectedly wrapped TLS")
			return
		}
		errCh <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("prepare raw conn: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out preparing raw conn")
		}
	}
}

func TestConnPrepareStage(t *testing.T) {
	err := &ConnPrepareError{Stage: ConnPrepareStagePreAuth, Err: errors.New("preauth failed")}
	if !IsConnPrepareStage(err, ConnPrepareStagePreAuth) {
		t.Fatal("expected preauth stage")
	}
	if IsConnPrepareStage(err, ConnPrepareStageTLS) {
		t.Fatal("did not expect tls stage")
	}
}
