package protocol

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"reflect"
)

const (
	maxRawRouteLen = 1 << 20      // 1 MiB, far above normal multi-hop route strings.
	maxRawDataLen  = 64 * 1 << 20 // 64 MiB, far above current file/stream chunk sizes.
)

var errRawMessageTooLarge = errors.New("raw message too large")
var errRawPayloadMalformed = errors.New("raw payload malformed")

func readRawFrame(conn net.Conn) (*Header, []byte, error) {
	var (
		header         = new(Header)
		senderBuf      = make([]byte, 10)
		accepterBuf    = make([]byte, 10)
		messageTypeBuf = make([]byte, 2)
		routeLenBuf    = make([]byte, 4)
		dataLenBuf     = make([]byte, 8)
	)

	if _, err := io.ReadFull(conn, senderBuf); err != nil {
		return header, nil, err
	}
	header.Sender = string(senderBuf)

	if _, err := io.ReadFull(conn, accepterBuf); err != nil {
		return header, nil, err
	}
	header.Accepter = string(accepterBuf)

	if _, err := io.ReadFull(conn, messageTypeBuf); err != nil {
		return header, nil, err
	}
	header.MessageType = binary.BigEndian.Uint16(messageTypeBuf)

	if _, err := io.ReadFull(conn, routeLenBuf); err != nil {
		return header, nil, err
	}
	header.RouteLen = binary.BigEndian.Uint32(routeLenBuf)
	if header.RouteLen > maxRawRouteLen {
		return header, nil, errRawMessageTooLarge
	}

	routeBuf := make([]byte, header.RouteLen)
	if _, err := io.ReadFull(conn, routeBuf); err != nil {
		return header, nil, err
	}
	header.Route = string(routeBuf)

	if _, err := io.ReadFull(conn, dataLenBuf); err != nil {
		return header, nil, err
	}
	header.DataLen = binary.BigEndian.Uint64(dataLenBuf)
	if header.DataLen > maxRawDataLen {
		return header, nil, errRawMessageTooLarge
	}

	dataBuf := make([]byte, header.DataLen)
	if _, err := io.ReadFull(conn, dataBuf); err != nil {
		return header, nil, err
	}

	return header, dataBuf, nil
}

func decodeRawPayload(messageType uint16, dataBuf []byte) (interface{}, error) {
	if mess, ok, err := decodeRawPayloadFast(messageType, dataBuf); ok {
		return mess, err
	}

	mess := newRawPayload(messageType)
	messType := reflect.TypeOf(mess).Elem()
	messValue := reflect.ValueOf(mess).Elem()

	var ptr uint64
	for i := 0; i < messType.NumField(); i++ {
		inter := messValue.Field(i).Interface()
		field := messValue.FieldByName(messType.Field(i).Name)

		switch inter.(type) {
		case string:
			tmp := messValue.FieldByName(messType.Field(i).Name + "Len")
			var stringLen uint64
			switch stringLenTmp := tmp.Interface().(type) {
			case uint16:
				stringLen = uint64(stringLenTmp)
			case uint32:
				stringLen = uint64(stringLenTmp)
			case uint64:
				stringLen = stringLenTmp
			}
			field.SetString(string(dataBuf[ptr : ptr+stringLen]))
			ptr += stringLen
		case uint16:
			field.SetUint(uint64(binary.BigEndian.Uint16(dataBuf[ptr : ptr+2])))
			ptr += 2
		case uint32:
			field.SetUint(uint64(binary.BigEndian.Uint32(dataBuf[ptr : ptr+4])))
			ptr += 4
		case uint64:
			field.SetUint(binary.BigEndian.Uint64(dataBuf[ptr : ptr+8]))
			ptr += 8
		case []byte:
			tmp := messValue.FieldByName(messType.Field(i).Name + "Len")
			var byteLen uint64
			switch byteLenTmp := tmp.Interface().(type) {
			case uint16:
				byteLen = uint64(byteLenTmp)
			case uint32:
				byteLen = uint64(byteLenTmp)
			case uint64:
				byteLen = byteLenTmp
			}
			field.SetBytes(dataBuf[ptr : ptr+byteLen])
			ptr += byteLen
		default:
			return nil, errors.New("unknown error")
		}
	}

	return mess, nil
}

func decodeRawPayloadFast(messageType uint16, dataBuf []byte) (interface{}, bool, error) {
	switch messageType {
	case FILEDATA:
		data, err := decodeBytesPayload(dataBuf)
		if err != nil {
			return nil, true, err
		}
		return &FileData{DataLen: uint64(len(data)), Data: data}, true, nil
	case SOCKSTCPDATA:
		seq, data, err := decodeSeqBytesPayload(dataBuf)
		if err != nil {
			return nil, true, err
		}
		return &SocksTCPData{Seq: seq, DataLen: uint64(len(data)), Data: data}, true, nil
	case SOCKSUDPDATA:
		seq, data, err := decodeSeqBytesPayload(dataBuf)
		if err != nil {
			return nil, true, err
		}
		return &SocksUDPData{Seq: seq, DataLen: uint64(len(data)), Data: data}, true, nil
	case FORWARDDATA:
		seq, data, err := decodeSeqBytesPayload(dataBuf)
		if err != nil {
			return nil, true, err
		}
		return &ForwardData{Seq: seq, DataLen: uint64(len(data)), Data: data}, true, nil
	case BACKWARDDATA:
		seq, data, err := decodeSeqBytesPayload(dataBuf)
		if err != nil {
			return nil, true, err
		}
		return &BackwardData{Seq: seq, DataLen: uint64(len(data)), Data: data}, true, nil
	case SOCKSTCPFIN:
		seq, err := decodeSeqPayload(dataBuf)
		if err != nil {
			return nil, true, err
		}
		return &SocksTCPFin{Seq: seq}, true, nil
	case FORWARDFIN:
		seq, err := decodeSeqPayload(dataBuf)
		if err != nil {
			return nil, true, err
		}
		return &ForwardFin{Seq: seq}, true, nil
	case BACKWARDFIN:
		seq, err := decodeSeqPayload(dataBuf)
		if err != nil {
			return nil, true, err
		}
		return &BackWardFin{Seq: seq}, true, nil
	default:
		return nil, false, nil
	}
}

func decodeBytesPayload(dataBuf []byte) ([]byte, error) {
	if len(dataBuf) < 8 {
		return nil, errRawPayloadMalformed
	}
	dataLen := binary.BigEndian.Uint64(dataBuf[:8])
	if dataLen != uint64(len(dataBuf)-8) {
		return nil, errRawPayloadMalformed
	}
	return dataBuf[8:], nil
}

func decodeSeqBytesPayload(dataBuf []byte) (uint64, []byte, error) {
	if len(dataBuf) < 16 {
		return 0, nil, errRawPayloadMalformed
	}
	seq := binary.BigEndian.Uint64(dataBuf[:8])
	dataLen := binary.BigEndian.Uint64(dataBuf[8:16])
	if dataLen != uint64(len(dataBuf)-16) {
		return 0, nil, errRawPayloadMalformed
	}
	return seq, dataBuf[16:], nil
}

func decodeSeqPayload(dataBuf []byte) (uint64, error) {
	if len(dataBuf) != 8 {
		return 0, errRawPayloadMalformed
	}
	return binary.BigEndian.Uint64(dataBuf), nil
}

func newRawPayload(messageType uint16) interface{} {
	switch messageType {
	case HI:
		return new(HIMess)
	case UUID:
		return new(UUIDMess)
	case CHILDUUIDREQ:
		return new(ChildUUIDReq)
	case CHILDUUIDRES:
		return new(ChildUUIDRes)
	case MYINFO:
		return new(MyInfo)
	case MYMEMO:
		return new(MyMemo)
	case SHELLREQ:
		return new(ShellReq)
	case SHELLRES:
		return new(ShellRes)
	case SHELLCOMMAND:
		return new(ShellCommand)
	case SHELLRESULT:
		return new(ShellResult)
	case SHELLEXIT:
		return new(ShellExit)
	case LISTENREQ:
		return new(ListenReq)
	case LISTENRES:
		return new(ListenRes)
	case SSHREQ:
		return new(SSHReq)
	case SSHRES:
		return new(SSHRes)
	case SSHCOMMAND:
		return new(SSHCommand)
	case SSHRESULT:
		return new(SSHResult)
	case SSHEXIT:
		return new(SSHExit)
	case SSHTUNNELREQ:
		return new(SSHTunnelReq)
	case SSHTUNNELRES:
		return new(SSHTunnelRes)
	case FILESTATREQ:
		return new(FileStatReq)
	case FILESTATRES:
		return new(FileStatRes)
	case FILEDATA:
		return new(FileData)
	case FILEERR:
		return new(FileErr)
	case FILEDOWNREQ:
		return new(FileDownReq)
	case FILEDOWNRES:
		return new(FileDownRes)
	case SOCKSSTART:
		return new(SocksStart)
	case SOCKSTCPDATA:
		return new(SocksTCPData)
	case SOCKSUDPDATA:
		return new(SocksUDPData)
	case UDPASSSTART:
		return new(UDPAssStart)
	case UDPASSRES:
		return new(UDPAssRes)
	case SOCKSTCPFIN:
		return new(SocksTCPFin)
	case SOCKSREADY:
		return new(SocksReady)
	case FORWARDTEST:
		return new(ForwardTest)
	case FORWARDSTART:
		return new(ForwardStart)
	case FORWARDREADY:
		return new(ForwardReady)
	case FORWARDDATA:
		return new(ForwardData)
	case FORWARDFIN:
		return new(ForwardFin)
	case BACKWARDTEST:
		return new(BackwardTest)
	case BACKWARDREADY:
		return new(BackwardReady)
	case BACKWARDSTART:
		return new(BackwardStart)
	case BACKWARDSEQ:
		return new(BackwardSeq)
	case BACKWARDDATA:
		return new(BackwardData)
	case BACKWARDFIN:
		return new(BackWardFin)
	case BACKWARDSTOP:
		return new(BackwardStop)
	case BACKWARDSTOPDONE:
		return new(BackwardStopDone)
	case CONNECTSTART:
		return new(ConnectStart)
	case CONNECTDONE:
		return new(ConnectDone)
	case NODEOFFLINE:
		return new(NodeOffline)
	case NODEREONLINE:
		return new(NodeReonline)
	case UPSTREAMOFFLINE:
		return new(UpstreamOffline)
	case UPSTREAMREONLINE:
		return new(UpstreamReonline)
	case SHUTDOWN:
		return new(Shutdown)
	case HEARTBEAT:
		return new(HeartbeatMsg)
	}

	return nil
}
