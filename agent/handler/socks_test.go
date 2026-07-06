package handler

import (
	"bytes"
	"net"
	"testing"
	"time"

	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/share"
)

func TestCheckMethodPreservesPipelinedConnectRequest(t *testing.T) {
	messages, cleanup := setupSocksProtocolTest(t)
	defer cleanup()

	connectRequest := []byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, 0x1f, 0x90}
	packet := append([]byte{0x05, 0x01, 0x00}, connectRequest...)
	dataChan := make(chan []byte, 1)
	dataChan <- packet

	setting := new(Setting)
	reader := newSocksDataReader(dataChan)
	if ok := newSocks().checkMethod(setting, reader, 7); !ok {
		t.Fatal("checkMethod returned false")
	}

	if !setting.isAuthed || setting.method != "NONE" {
		t.Fatalf("setting = %+v, want no-auth success", setting)
	}

	message := readSocksProtocolMessage(t, messages).(*protocol.SocksTCPData)
	if !bytes.Equal(message.Data, []byte{0x05, 0x00}) {
		t.Fatalf("method response = %v, want [5 0]", message.Data)
	}

	if got := reader.drainBuffered(); !bytes.Equal(got, connectRequest) {
		t.Fatalf("buffered data = %v, want %v", got, connectRequest)
	}
}

func TestReadSocksTargetReadsSplitConnectRequest(t *testing.T) {
	dataChan := make(chan []byte, 3)
	dataChan <- []byte{127}
	dataChan <- []byte{0, 0}
	dataChan <- []byte{1, 0x1f, 0x90, 'G', 'E', 'T'}

	reader := newSocksDataReader(dataChan)
	host, port, ok := readSocksTarget(reader, 0x01)
	if !ok {
		t.Fatal("readSocksTarget returned false")
	}
	if host != "127.0.0.1" || port != "8080" {
		t.Fatalf("target = %s:%s, want 127.0.0.1:8080", host, port)
	}

	if got := reader.drainBuffered(); !bytes.Equal(got, []byte("GET")) {
		t.Fatalf("buffered data = %q, want GET", got)
	}
}

func setupSocksProtocolTest(t *testing.T) (<-chan interface{}, func()) {
	t.Helper()

	protocol.SetUpDownStream("raw", "raw")
	agentConn, adminConn := net.Pipe()
	ctx := tsruntime.NewContext(agentConn, "secret", protocol.TEMP_UUID, false)
	ctx.ApplyGlobal()

	messages := make(chan interface{}, 4)
	go func() {
		rMessage := protocol.NewUpMsg(adminConn, "secret", protocol.ADMIN_UUID)
		for {
			_, message, err := protocol.DestructMessage(rMessage)
			if err != nil {
				return
			}
			messages <- message
		}
	}()

	cleanup := func() {
		ctx.UpdateConn(nil)
		share.CloseQuietly(agentConn)
		share.CloseQuietly(adminConn)
	}

	return messages, cleanup
}

func readSocksProtocolMessage(t *testing.T, messages <-chan interface{}) interface{} {
	t.Helper()

	select {
	case message := <-messages:
		return message
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for protocol message")
	}

	return nil
}
