package dnstransport

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestDirectUDPConnRoundTrip(t *testing.T) {
	config := DefaultConfig()
	config.PayloadMTU = 32
	config.PollInterval = 10 * time.Millisecond
	config.IdlePollInterval = 20 * time.Millisecond
	config.QueryTimeout = 500 * time.Millisecond
	config.HandshakeTimeout = time.Second
	config.RetransmitMin = 20 * time.Millisecond
	config.RetransmitMax = 50 * time.Millisecond
	config.MaxRetries = 5

	listener, err := ListenConfig("127.0.0.1:0/t.example", config)
	if err != nil {
		t.Fatalf("ListenConfig error: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	serverAddr := listener.socket.pc.LocalAddr().(*net.UDPAddr)
	client, err := DialContext(
		timedContext(t, time.Second),
		"t.example@"+serverAddr.String(),
		"",
		config,
	)
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer client.Close()

	var server net.Conn
	select {
	case server = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("Accept timed out")
	}
	defer server.Close()

	if _, err := client.Write([]byte("client to server")); err != nil {
		t.Fatalf("client Write error: %v", err)
	}
	buf := make([]byte, len("client to server"))
	if _, err := io.ReadFull(server, buf); err != nil {
		t.Fatalf("server Read error: %v", err)
	}
	if string(buf) != "client to server" {
		t.Fatalf("server read %q", buf)
	}

	if _, err := server.Write([]byte("server to client")); err != nil {
		t.Fatalf("server Write error: %v", err)
	}
	buf = make([]byte, len("server to client"))
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("client Read error: %v", err)
	}
	if string(buf) != "server to client" {
		t.Fatalf("client read %q", buf)
	}
}

func TestPollIntervalUsesPayloadActivity(t *testing.T) {
	config := DefaultConfig()
	config.PollInterval = 100 * time.Millisecond
	config.IdlePollInterval = 2 * time.Second

	conn := &Conn{
		socket: &packetSocket{config: config},
	}

	now := time.Now()
	conn.lastPayloadUnixNano.Store(now.Add(-3 * time.Second).UnixNano())
	conn.touchRx()

	if got := conn.currentPollInterval(time.Now()); got != config.IdlePollInterval {
		t.Fatalf("currentPollInterval after control-only rx = %s, want %s", got, config.IdlePollInterval)
	}

	conn.lastPayloadUnixNano.Store(time.Now().UnixNano())
	if got := conn.currentPollInterval(time.Now()); got != config.PollInterval {
		t.Fatalf("currentPollInterval after payload activity = %s, want %s", got, config.PollInterval)
	}
}

func timedContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}
