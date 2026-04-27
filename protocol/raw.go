package protocol

import (
	"net"

	"TengShe/crypto"
)

type RawProto struct{}

type RawMessage struct {
	// Essential component to apply a Message
	UUID         string
	Conn         net.Conn
	CryptoSecret []byte
	MessageType  uint16
	// Prepared buffer
	HeaderBuffer []byte
	DataBuffer   []byte
}

func (proto *RawProto) CNegotiate() error { return nil }

func (proto *RawProto) SNegotiate() error { return nil }

func (message *RawMessage) ConstructHeader() {}

func (message *RawMessage) ConstructData(header *Header, mess interface{}, isPass bool) {
	message.MessageType = header.MessageType

	// isPass means the payload already belongs to another node. Keep the bytes
	// unchanged so intermediate nodes can forward them without re-encoding.
	var dataBuffer []byte
	if !isPass {
		dataBuffer = encodeRawPayload(mess)
	} else {
		dataBuffer = mess.([]byte)
	}

	message.DataBuffer = dataBuffer
	// Encrypt&Compress data
	if !isPass {
		message.DataBuffer = crypto.GzipCompress(message.DataBuffer)
		message.DataBuffer = crypto.AESEncrypt(message.DataBuffer, message.CryptoSecret)
	}
	// Calculate the whole data's length
	message.HeaderBuffer = encodeRawHeader(header, uint64(len(message.DataBuffer)))
}

func (message *RawMessage) ConstructSuffix() {}

func (message *RawMessage) DeconstructHeader() {}

func (message *RawMessage) DeconstructData() (*Header, interface{}, error) {
	header, dataBuf, err := readRawFrame(message.Conn)
	if err != nil {
		return header, nil, err
	}

	if header.Accepter == TEMP_UUID || message.UUID == ADMIN_UUID || message.UUID == header.Accepter {
		dataBuf = crypto.AESDecrypt(dataBuf, message.CryptoSecret) // use dataBuf directly to save the memory
	} else {
		// Intermediate nodes return the encoded payload for route pass-through.
		return header, dataBuf, nil
	}
	// Decompress the data
	dataBuf = crypto.GzipDecompress(dataBuf)
	mess, err := decodeRawPayload(header.MessageType, dataBuf)
	return header, mess, err
}

func (message *RawMessage) DeconstructSuffix() {}

func (message *RawMessage) SendMessage() {
	frame := drainRawFrame(message)
	if sender := SenderForConn(message.Conn); sender != nil {
		_ = sender.SendFrame(LaneForMessageType(message.MessageType), frame)
		return
	}

	_ = writeFull(message.Conn, frame)
}

func drainRawFrame(message *RawMessage) []byte {
	finalBuffer := make([]byte, 0, len(message.HeaderBuffer)+len(message.DataBuffer))
	finalBuffer = append(finalBuffer, message.HeaderBuffer...)
	finalBuffer = append(finalBuffer, message.DataBuffer...)
	message.HeaderBuffer = nil
	message.DataBuffer = nil
	return finalBuffer
}
