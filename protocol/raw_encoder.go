package protocol

import (
	"bytes"
	"encoding/binary"
	"reflect"
)

func encodeRawHeader(header *Header, dataLen uint64) []byte {
	var headerBuffer bytes.Buffer
	messageTypeBuf := make([]byte, 2)
	routeLenBuf := make([]byte, 4)
	dataLenBuf := make([]byte, 8)

	binary.BigEndian.PutUint16(messageTypeBuf, header.MessageType)
	binary.BigEndian.PutUint32(routeLenBuf, header.RouteLen)
	binary.BigEndian.PutUint64(dataLenBuf, dataLen)

	headerBuffer.Write([]byte(header.Sender))
	headerBuffer.Write([]byte(header.Accepter))
	headerBuffer.Write(messageTypeBuf)
	headerBuffer.Write(routeLenBuf)
	headerBuffer.Write([]byte(header.Route))
	headerBuffer.Write(dataLenBuf)

	return headerBuffer.Bytes()
}

func encodeRawPayload(mess interface{}) []byte {
	if dataBuffer, ok := encodeRawPayloadFast(mess); ok {
		return dataBuffer
	}

	var dataBuffer bytes.Buffer
	messType := reflect.TypeOf(mess).Elem()
	messValue := reflect.ValueOf(mess).Elem()

	for i := 0; i < messType.NumField(); i++ {
		inter := messValue.Field(i).Interface()

		switch value := inter.(type) {
		case string:
			dataBuffer.Write([]byte(value))
		case uint16:
			buffer := make([]byte, 2)
			binary.BigEndian.PutUint16(buffer, value)
			dataBuffer.Write(buffer)
		case uint32:
			buffer := make([]byte, 4)
			binary.BigEndian.PutUint32(buffer, value)
			dataBuffer.Write(buffer)
		case uint64:
			buffer := make([]byte, 8)
			binary.BigEndian.PutUint64(buffer, value)
			dataBuffer.Write(buffer)
		case []byte:
			dataBuffer.Write(value)
		}
	}

	return dataBuffer.Bytes()
}

func encodeRawPayloadFast(mess interface{}) ([]byte, bool) {
	switch value := mess.(type) {
	case *FileData:
		return encodeBytesPayload(value.Data), true
	case *SocksTCPData:
		return encodeSeqBytesPayload(value.Seq, value.Data), true
	case *SocksUDPData:
		return encodeSeqBytesPayload(value.Seq, value.Data), true
	case *ForwardData:
		return encodeSeqBytesPayload(value.Seq, value.Data), true
	case *BackwardData:
		return encodeSeqBytesPayload(value.Seq, value.Data), true
	case *SocksTCPFin:
		return encodeSeqPayload(value.Seq), true
	case *ForwardFin:
		return encodeSeqPayload(value.Seq), true
	case *BackWardFin:
		return encodeSeqPayload(value.Seq), true
	default:
		return nil, false
	}
}

func encodeBytesPayload(data []byte) []byte {
	buffer := make([]byte, 8+len(data))
	binary.BigEndian.PutUint64(buffer[:8], uint64(len(data)))
	copy(buffer[8:], data)
	return buffer
}

func encodeSeqBytesPayload(seq uint64, data []byte) []byte {
	buffer := make([]byte, 16+len(data))
	binary.BigEndian.PutUint64(buffer[:8], seq)
	binary.BigEndian.PutUint64(buffer[8:16], uint64(len(data)))
	copy(buffer[16:], data)
	return buffer
}

func encodeSeqPayload(seq uint64) []byte {
	buffer := make([]byte, 8)
	binary.BigEndian.PutUint64(buffer, seq)
	return buffer
}
