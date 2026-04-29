package dnstransport

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	frameMagic      uint32 = 0x5453444e // TSDN
	frameVersion    uint8  = 1
	frameHeaderSize        = 64
	maxUint16              = 0xffff
)

type frameType uint8

const (
	frameSYN frameType = iota + 1
	frameSYNACK
	frameDATA
	frameACK
	framePOLL
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
	PacketID   uint64
	Seq        uint64
	Ack        uint64
	FragID     uint32
	FragIndex  uint16
	FragTotal  uint16
	PayloadLen uint16
	Payload    []byte
}

func encodeFrame(in frame) ([]byte, error) {
	if len(in.Payload) > maxUint16 {
		return nil, fmt.Errorf("DNS frame payload too large: %d", len(in.Payload))
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
	binary.BigEndian.PutUint64(out[32:40], in.PacketID)
	binary.BigEndian.PutUint64(out[40:48], in.Seq)
	binary.BigEndian.PutUint64(out[48:56], in.Ack)
	binary.BigEndian.PutUint32(out[56:60], in.FragID)
	binary.BigEndian.PutUint16(out[60:62], in.FragIndex)
	binary.BigEndian.PutUint16(out[62:64], in.FragTotal)
	copy(out[frameHeaderSize:], in.Payload)
	return out, nil
}

func decodeFrame(in []byte) (frame, error) {
	if len(in) < frameHeaderSize {
		return frame{}, errors.New("DNS frame too short")
	}
	if binary.BigEndian.Uint32(in[0:4]) != frameMagic {
		return frame{}, errors.New("DNS frame magic mismatch")
	}
	if in[4] != frameVersion {
		return frame{}, fmt.Errorf("unsupported DNS frame version %d", in[4])
	}
	headerLen := int(binary.BigEndian.Uint16(in[8:10]))
	payloadLen := int(binary.BigEndian.Uint16(in[10:12]))
	if headerLen < frameHeaderSize {
		return frame{}, fmt.Errorf("invalid DNS frame header length %d", headerLen)
	}
	if len(in) < headerLen+payloadLen {
		return frame{}, fmt.Errorf("invalid DNS frame payload length %d", payloadLen)
	}
	out := frame{
		Type:       frameType(in[5]),
		Flags:      binary.BigEndian.Uint16(in[6:8]),
		SessionID:  binary.BigEndian.Uint64(in[12:20]),
		SenderID:   binary.BigEndian.Uint64(in[20:28]),
		StreamID:   binary.BigEndian.Uint32(in[28:32]),
		PacketID:   binary.BigEndian.Uint64(in[32:40]),
		Seq:        binary.BigEndian.Uint64(in[40:48]),
		Ack:        binary.BigEndian.Uint64(in[48:56]),
		FragID:     binary.BigEndian.Uint32(in[56:60]),
		FragIndex:  binary.BigEndian.Uint16(in[60:62]),
		FragTotal:  binary.BigEndian.Uint16(in[62:64]),
		PayloadLen: uint16(payloadLen),
	}
	if !validFrameType(out.Type) {
		return frame{}, fmt.Errorf("invalid DNS frame type %d", out.Type)
	}
	if payloadLen > 0 {
		out.Payload = append([]byte(nil), in[headerLen:headerLen+payloadLen]...)
	}
	return out, nil
}

func validFrameType(typ frameType) bool {
	switch typ {
	case frameSYN, frameSYNACK, frameDATA, frameACK, framePOLL, framePONG, frameCLOSE, frameRESET, frameCLOSEACK:
		return true
	default:
		return false
	}
}
