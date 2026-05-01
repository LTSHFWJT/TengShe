//go:build windows

package smbtransport

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestLocalPipeRoundTrip(t *testing.T) {
	name := "tengshe-test-" + time.Now().Format("150405.000000000")
	listener, err := ListenConfig(context.Background(), "pipe:"+name, DefaultConfig())
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

	client, err := DialContext(context.Background(), "pipe://./"+name, "", DefaultConfig())
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer client.Close()

	var server net.Conn
	select {
	case err := <-acceptErr:
		t.Fatalf("Accept error: %v", err)
	case server = <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("Accept timed out")
	}
	defer server.Close()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("client write error: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(server, buf); err != nil {
		t.Fatalf("server read error: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("server read %q, want ping", string(buf))
	}

	if _, err := server.Write([]byte("pong")); err != nil {
		t.Fatalf("server write error: %v", err)
	}
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("client read error: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("client read %q, want pong", string(buf))
	}
}

func TestListenerCloseDoesNotCloseAcceptedConn(t *testing.T) {
	name := "tengshe-test-close-" + time.Now().Format("150405.000000000")
	listener, err := ListenConfig(context.Background(), "pipe:"+name, DefaultConfig())
	if err != nil {
		t.Fatalf("ListenConfig error: %v", err)
	}

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	client, err := DialContext(context.Background(), "pipe://./"+name, "", DefaultConfig())
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer client.Close()

	var server net.Conn
	select {
	case server = <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("Accept timed out")
	}
	defer server.Close()

	if err := listener.Close(); err != nil {
		t.Fatalf("listener close error: %v", err)
	}

	if _, err := client.Write([]byte("ok")); err != nil {
		t.Fatalf("write after listener close error: %v", err)
	}
	buf := make([]byte, 2)
	if _, err := io.ReadFull(server, buf); err != nil {
		t.Fatalf("read after listener close error: %v", err)
	}
	if string(buf) != "ok" {
		t.Fatalf("server read %q, want ok", string(buf))
	}
}

func TestPeerCloseMapsToEOF(t *testing.T) {
	name := "tengshe-test-eof-" + time.Now().Format("150405.000000000")
	listener, err := ListenConfig(context.Background(), "pipe:"+name, DefaultConfig())
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

	client, err := DialContext(context.Background(), "pipe://./"+name, "", DefaultConfig())
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}

	var server net.Conn
	select {
	case server = <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("Accept timed out")
	}
	defer server.Close()

	if err := client.Close(); err != nil {
		t.Fatalf("client close error: %v", err)
	}
	buf := make([]byte, 1)
	_, err = server.Read(buf)
	if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("server read error = %v, want EOF or net.ErrClosed", err)
	}
}
