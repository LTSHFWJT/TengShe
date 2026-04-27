package icmptransport

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	frameMagic      uint32 = 0x54534943 // TSIC
	frameVersion    uint8  = 1
	frameHeaderSize        = 56
)

type frameType uint8

const (
	frameSYN frameType = iota + 1
	frameSYNACK
	frameDATA
	frameACK
	framePING
	framePONG
	frameCLOSE
	frameRESET
	frameCLOSEACK
)

type frame struct {
	Type       frameType
	Flags      uint16
	SessionID  uint64
	SenderID   uint64
	StreamID   uint32
	Seq        uint64
	Ack        uint64
	FragID     uint32
	FragIndex  uint16
	FragTotal  uint16
	PayloadLen uint16
	Payload    []byte
}

func encodeFrame(in frame) ([]byte, error) {
	if len(in.Payload) > 0xffff {
		return nil, fmt.Errorf("ICMP frame payload too large: %d", len(in.Payload))
	}
	out := make([]byte, frameHeaderSize+len(in.Payload))
	binary.BigEndian.PutUint32(out[0:4], frameMagic)
	out[4] = frameVersion
	out[5] = byte(in.Type)
	binary.BigEndian.PutUint16(out[6:8], in.Flags)
	binary.BigEndian.PutUint16(out[8:10], frameHeaderSize)
	binary.BigEndian.PutUint16(out[10:12], uint16(len(in.Payload)))
	binary.BigEndian.PutUint64(out[12:20], in.SessionID)
	binary.BigEndian.PutUint64(out[20:28], in.SenderID)
	binary.BigEndian.PutUint32(out[28:32], in.StreamID)
	binary.BigEndian.PutUint64(out[32:40], in.Seq)
	binary.BigEndian.PutUint64(out[40:48], in.Ack)
	binary.BigEndian.PutUint32(out[48:52], in.FragID)
	binary.BigEndian.PutUint16(out[52:54], in.FragIndex)
	binary.BigEndian.PutUint16(out[54:56], in.FragTotal)
	copy(out[frameHeaderSize:], in.Payload)
	return out, nil
}

func decodeFrame(in []byte) (frame, error) {
	if len(in) < frameHeaderSize {
		return frame{}, errors.New("ICMP frame too short")
	}
	if binary.BigEndian.Uint32(in[0:4]) != frameMagic {
		return frame{}, errors.New("ICMP frame magic mismatch")
	}
	if in[4] != frameVersion {
		return frame{}, fmt.Errorf("unsupported ICMP frame version %d", in[4])
	}
	headerLen := int(binary.BigEndian.Uint16(in[8:10]))
	payloadLen := int(binary.BigEndian.Uint16(in[10:12]))
	if headerLen < frameHeaderSize {
		return frame{}, fmt.Errorf("invalid ICMP frame header length %d", headerLen)
	}
	if len(in) < headerLen+payloadLen {
		return frame{}, fmt.Errorf("invalid ICMP frame payload length %d", payloadLen)
	}
	out := frame{
		Type:       frameType(in[5]),
		Flags:      binary.BigEndian.Uint16(in[6:8]),
		SessionID:  binary.BigEndian.Uint64(in[12:20]),
		SenderID:   binary.BigEndian.Uint64(in[20:28]),
		StreamID:   binary.BigEndian.Uint32(in[28:32]),
		Seq:        binary.BigEndian.Uint64(in[32:40]),
		Ack:        binary.BigEndian.Uint64(in[40:48]),
		FragID:     binary.BigEndian.Uint32(in[48:52]),
		FragIndex:  binary.BigEndian.Uint16(in[52:54]),
		FragTotal:  binary.BigEndian.Uint16(in[54:56]),
		PayloadLen: uint16(payloadLen),
	}
	if !validFrameType(out.Type) {
		return frame{}, fmt.Errorf("invalid ICMP frame type %d", out.Type)
	}
	if payloadLen > 0 {
		out.Payload = append([]byte(nil), in[headerLen:headerLen+payloadLen]...)
	}
	return out, nil
}

func validFrameType(typ frameType) bool {
	switch typ {
	case frameSYN, frameSYNACK, frameDATA, frameACK, framePING, framePONG, frameCLOSE, frameRESET, frameCLOSEACK:
		return true
	default:
		return false
	}
}
