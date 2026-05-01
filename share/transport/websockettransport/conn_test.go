package websockettransport

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestListenDialRoundTrip(t *testing.T) {
	config := DefaultConfig()
	listener, err := ListenConfig(context.Background(), "127.0.0.1:0/tengshe", config)
	if err != nil {
		t.Fatalf("ListenConfig error: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	client, err := DialContext(context.Background(), listener.Addr().String(), "", config)
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer client.Close()

	var server net.Conn
	select {
	case server = <-accepted:
	case err := <-acceptErr:
		t.Fatalf("Accept error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Accept timed out")
	}
	defer server.Close()
	if got := server.RemoteAddr().String(); got == "" {
		t.Fatal("server RemoteAddr returned an empty address")
	} else if _, _, err := net.SplitHostPort(got); err != nil {
		t.Fatalf("server RemoteAddr = %q, want host:port: %v", got, err)
	}
	if got := server.LocalAddr().String(); !hasWebSocketScheme(got) {
		t.Fatalf("server LocalAddr = %q, want ws/wss URL", got)
	}

	payload := []byte("hello over websocket")
	if _, err := client.Write(payload); err != nil {
		t.Fatalf("client Write error: %v", err)
	}

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(server, buf); err != nil {
		t.Fatalf("server ReadFull error: %v", err)
	}
	if string(buf) != string(payload) {
		t.Fatalf("server read %q, want %q", string(buf), string(payload))
	}

	reply := []byte("ack")
	if _, err := server.Write(reply); err != nil {
		t.Fatalf("server Write error: %v", err)
	}

	got := make([]byte, len(reply))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatalf("client ReadFull error: %v", err)
	}
	if string(got) != string(reply) {
		t.Fatalf("client read %q, want %q", string(got), string(reply))
	}
}

func TestListenDialRequiresExactQuery(t *testing.T) {
	config := DefaultConfig()
	listener, err := ListenConfig(context.Background(), "127.0.0.1:0/tengshe?token=one", config)
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

	client, err := DialContext(context.Background(), listener.Addr().String(), "", config)
	if err != nil {
		t.Fatalf("DialContext with matching query error: %v", err)
	}
	client.Close()

	select {
	case server := <-accepted:
		server.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("Accept timed out")
	}

	badAddr := strings.Replace(listener.Addr().String(), "token=one", "token=two", 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if conn, err := DialContext(ctx, badAddr, "", config); err == nil {
		conn.Close()
		t.Fatal("DialContext with mismatched query unexpectedly succeeded")
	}
}

func TestAcceptReturnsClosedAfterClose(t *testing.T) {
	listener, err := ListenConfig(context.Background(), "127.0.0.1:0/tengshe", DefaultConfig())
	if err != nil {
		t.Fatalf("ListenConfig error: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after Close error = %v, want net.ErrClosed", err)
	}
}

func TestAcceptedConnSurvivesListenerClose(t *testing.T) {
	listener, err := ListenConfig(context.Background(), "127.0.0.1:0/tengshe", DefaultConfig())
	if err != nil {
		t.Fatalf("ListenConfig error: %v", err)
	}

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	client, err := DialContext(context.Background(), listener.Addr().String(), "", DefaultConfig())
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer client.Close()

	var server net.Conn
	select {
	case server = <-accepted:
	case err := <-acceptErr:
		t.Fatalf("Accept error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Accept timed out")
	}
	defer server.Close()

	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener error: %v", err)
	}

	payload := []byte("still alive")
	if _, err := client.Write(payload); err != nil {
		t.Fatalf("client Write after listener Close error: %v", err)
	}

	got := make([]byte, len(payload))
	if _, err := io.ReadFull(server, got); err != nil {
		t.Fatalf("server ReadFull after listener Close error: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("server read %q, want %q", string(got), string(payload))
	}
}

func TestWSSListenDial(t *testing.T) {
	config := DefaultConfig()
	listener, err := ListenConfig(context.Background(), "wss://127.0.0.1:0/tengshe", config)
	if err != nil {
		t.Fatalf("ListenConfig wss error: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	client, err := DialContext(context.Background(), listener.Addr().String(), "", config)
	if err != nil {
		t.Fatalf("DialContext wss error: %v", err)
	}
	defer client.Close()

	select {
	case server := <-accepted:
		server.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("Accept wss timed out")
	}
}

func TestDialUsesHostOverrideForRequestHost(t *testing.T) {
	config := DefaultConfig()
	config.Host = "front.example.test"
	listener, err := ListenConfig(context.Background(), "127.0.0.1:0/tengshe", config)
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

	client, err := DialContext(context.Background(), listener.Addr().String(), "", config)
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer client.Close()

	select {
	case server := <-accepted:
		defer server.Close()
		wsConn, ok := server.(*Conn)
		if !ok {
			t.Fatalf("accepted conn type = %T, want *Conn", server)
		}
		if got := wsConn.Request().Host; got != config.Host {
			t.Fatalf("request host = %q, want %q", got, config.Host)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept timed out")
	}
}

func TestListenDialRejectsWrongPath(t *testing.T) {
	config := DefaultConfig()
	listener, err := ListenConfig(context.Background(), "127.0.0.1:0/tengshe", config)
	if err != nil {
		t.Fatalf("ListenConfig error: %v", err)
	}
	defer listener.Close()

	badAddr := listener.Addr().String()
	badAddr = badAddr[:len(badAddr)-len("/tengshe")] + "/wrong"

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := DialContext(ctx, badAddr, "", config); err == nil {
		t.Fatal("DialContext with wrong path unexpectedly succeeded")
	}
}

func TestReadRejectsTextFrame(t *testing.T) {
	config := DefaultConfig()
	listener, err := ListenConfig(context.Background(), "127.0.0.1:0/tengshe", config)
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

	client, err := DialContext(context.Background(), listener.Addr().String(), "", config)
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer client.Close()

	server := <-accepted
	defer server.Close()
	wsServer := server.(*Conn)
	wsServer.Conn.PayloadType = websocket.TextFrame
	if _, err := wsServer.Conn.Write([]byte("text")); err != nil {
		t.Fatalf("server text Write error: %v", err)
	}

	buf := make([]byte, 4)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("client Read text frame unexpectedly succeeded")
	}
}

func TestWriteSplitsLargePayload(t *testing.T) {
	config := DefaultConfig()
	config.MaxFramePayload = 8
	listener, err := ListenConfig(context.Background(), "127.0.0.1:0/tengshe", config)
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

	client, err := DialContext(context.Background(), listener.Addr().String(), "", config)
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer client.Close()

	server := <-accepted
	defer server.Close()

	payload := []byte("0123456789abcdef0123456789abcdef")
	n, err := client.Write(payload)
	if err != nil {
		t.Fatalf("client Write error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("client Write wrote %d, want %d", n, len(payload))
	}

	got := make([]byte, len(payload))
	if _, err := io.ReadFull(server, got); err != nil {
		t.Fatalf("server ReadFull error: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("server read %q, want %q", string(got), string(payload))
	}
}
