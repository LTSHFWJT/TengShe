package protocol

import (
	"testing"

	tengcrypto "TengShe/crypto"
)

func BenchmarkRawSocksTCPDataConstruct(b *testing.B) {
	benchmarkRawDataConstruct(b, SOCKSTCPDATA, &SocksTCPData{
		Seq:     1,
		DataLen: uint64(len(benchmarkPayload)),
		Data:    benchmarkPayload,
	})
}

func BenchmarkRawForwardDataConstruct(b *testing.B) {
	benchmarkRawDataConstruct(b, FORWARDDATA, &ForwardData{
		Seq:     1,
		DataLen: uint64(len(benchmarkPayload)),
		Data:    benchmarkPayload,
	})
}

func BenchmarkRawBackwardDataConstruct(b *testing.B) {
	benchmarkRawDataConstruct(b, BACKWARDDATA, &BackwardData{
		Seq:     1,
		DataLen: uint64(len(benchmarkPayload)),
		Data:    benchmarkPayload,
	})
}

func benchmarkRawDataConstruct(b *testing.B, messageType uint16, body interface{}) {
	header := &Header{
		Sender:      "NODE000001",
		Accepter:    ADMIN_UUID,
		MessageType: messageType,
		RouteLen:    uint32(len("NODE000002")),
		Route:       "NODE000002",
	}
	secret := tengcrypto.KeyPadding([]byte("benchmark-secret"))
	message := &RawMessage{UUID: "NODE000001", CryptoSecret: secret}

	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkPayload)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ConstructMessage(message, header, body, false)
		message.HeaderBuffer = nil
		message.DataBuffer = nil
	}
}

var benchmarkPayload = newBenchmarkPayload()

func newBenchmarkPayload() []byte {
	payload := make([]byte, 20480)
	var x uint32 = 0x12345678
	for i := range payload {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		payload[i] = byte(x)
	}
	return payload
}
