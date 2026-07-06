package handler

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"TengShe/agent/manager"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/share"
	"TengShe/utils"
)

type Socks struct {
	Username string
	Password string
}

type Setting struct {
	method       string
	isAuthed     bool
	tcpConnected bool
	isUDP        bool
	success      bool
	tcpConn      net.Conn
	udpListener  *net.UDPConn
}

const socksDataEnqueueTimeout = 30 * time.Second

type socksDataReader struct {
	ch  <-chan []byte
	buf []byte
}

func newSocks() *Socks {
	return new(Socks)
}

func newSocksDataReader(ch <-chan []byte) *socksDataReader {
	return &socksDataReader{ch: ch}
}

func (reader *socksDataReader) read(n int) ([]byte, bool) {
	if n <= 0 {
		return nil, true
	}

	for len(reader.buf) < n {
		data, ok := <-reader.ch
		if !ok {
			return nil, false
		}
		if len(data) == 0 {
			continue
		}
		reader.buf = append(reader.buf, data...)
	}

	data := make([]byte, n)
	copy(data, reader.buf[:n])
	reader.buf = reader.buf[n:]
	return data, true
}

func (reader *socksDataReader) drainBuffered() []byte {
	if len(reader.buf) == 0 {
		return nil
	}

	data := make([]byte, len(reader.buf))
	copy(data, reader.buf)
	reader.buf = nil
	return data
}

func readSocksTarget(reader *socksDataReader, atyp byte) (string, string, bool) {
	var host string

	switch atyp {
	case 0x01:
		ip, ok := reader.read(4)
		if !ok {
			return "", "", false
		}
		host = net.IPv4(ip[0], ip[1], ip[2], ip[3]).String()
	case 0x03:
		hostLen, ok := reader.read(1)
		if !ok {
			return "", "", false
		}
		hostData, ok := reader.read(int(hostLen[0]))
		if !ok {
			return "", "", false
		}
		host = string(hostData)
	case 0x04:
		ip, ok := reader.read(16)
		if !ok {
			return "", "", false
		}
		host = net.IP(ip).String()
	default:
		return "", "", false
	}

	portData, ok := reader.read(2)
	if !ok {
		return "", "", false
	}

	port := utils.Int2Str(int(portData[0])<<8 | int(portData[1]))
	return host, port, true
}

func (socks *Socks) start(mgr *manager.Manager) {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSREADY,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))), // No need to set route when agent send mess to admin
		Route:       protocol.TEMP_ROUTE,
	}

	succMess := &protocol.SocksReady{
		OK: 1,
	}

	failMess := &protocol.SocksReady{
		OK: 0,
	}

	mgrTask := &manager.SocksTask{
		Mode: manager.S_CHECKSOCKSREADY, // to make sure the map is clean
	}
	mgr.SocksManager.TaskChan <- mgrTask
	result := <-mgr.SocksManager.ResultChan
	if !result.OK {
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
		return
	}

	protocol.ConstructMessage(sMessage, header, succMess, false)
	sMessage.SendMessage()
}

func (socks *Socks) handleSocks(mgr *manager.Manager, dataChan chan []byte, seq uint64) {
	setting := new(Setting)
	reader := newSocksDataReader(dataChan)

	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	defer func() { // no matter what happened, after the function return,tell admin that works done
		finHeader := &protocol.Header{
			Sender:      tsruntime.Component().UUID,
			Accepter:    protocol.ADMIN_UUID,
			MessageType: protocol.SOCKSTCPFIN,
			RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))), // No need to set route when agent send mess to admin
			Route:       protocol.TEMP_ROUTE,
		}

		finMess := &protocol.SocksTCPFin{
			Seq: seq,
		}

		protocol.SendMessage(sMessage, finHeader, finMess, false)
	}()

	for {
		if !setting.isAuthed && setting.method == "" {
			if !socks.checkMethod(setting, reader, seq) {
				return
			}
		} else if !setting.isAuthed && setting.method == "PASSWORD" {
			if !socks.auth(setting, reader, seq) {
				return
			}
		} else if setting.isAuthed && !setting.tcpConnected && !setting.isUDP {
			if !socks.buildConn(mgr, setting, reader, seq) {
				return
			}

			if !setting.tcpConnected && !setting.isUDP {
				return
			}
		} else if setting.isAuthed && setting.tcpConnected && !setting.isUDP { //All done!
			go proxyC2STCP(setting.tcpConn, dataChan, reader.drainBuffered())
			proxyS2CTCP(setting.tcpConn, seq)
			return
		} else if setting.isAuthed && setting.isUDP && setting.success {
			go proxyC2SUDP(mgr, setting.udpListener, seq)
			proxyS2CUDP(mgr, setting.udpListener, seq)
			return
		} else {
			return
		}
	}
}

func (socks *Socks) checkMethod(setting *Setting, reader *socksDataReader, seq uint64) bool {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))), // No need to set route when agent send mess to admin
		Route:       protocol.TEMP_ROUTE,
	}

	failMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x05, 0xff})),
		Data:    []byte{0x05, 0xff},
	}

	noneMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x05, 0x00})),
		Data:    []byte{0x05, 0x00},
	}

	passMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x05, 0x02})),
		Data:    []byte{0x05, 0x02},
	}

	methodHeader, ok := reader.read(2)
	if !ok {
		return false
	}

	if methodHeader[0] != 0x05 {
		setting.method = "ILLEGAL"
		return true
	}

	methods, ok := reader.read(int(methodHeader[1]))
	if !ok {
		return false
	}

	var supportMethodFinded, userPassFinded, noAuthFinded bool

	for _, method := range methods {
		if method == 0x00 {
			noAuthFinded = true
			supportMethodFinded = true
		} else if method == 0x02 {
			userPassFinded = true
			supportMethodFinded = true
		}
	}

	if !supportMethodFinded {
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
		setting.method = "ILLEGAL"
		return true
	}

	if noAuthFinded && (socks.Username == "" && socks.Password == "") {
		protocol.ConstructMessage(sMessage, header, noneMess, false)
		sMessage.SendMessage()
		setting.method = "NONE"
		setting.isAuthed = true
		return true
	} else if userPassFinded && (socks.Username != "" && socks.Password != "") {
		protocol.ConstructMessage(sMessage, header, passMess, false)
		sMessage.SendMessage()
		setting.method = "PASSWORD"
		return true
	}

	protocol.ConstructMessage(sMessage, header, failMess, false)
	sMessage.SendMessage()
	setting.method = "ILLEGAL"
	return true
}

func (socks *Socks) auth(setting *Setting, reader *socksDataReader, seq uint64) bool {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	failMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x01, 0x01})),
		Data:    []byte{0x01, 0x01},
	}

	succMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x01, 0x00})),
		Data:    []byte{0x01, 0x00},
	}

	authHeader, ok := reader.read(2)
	if !ok {
		return false
	}

	if authHeader[0] != 0x01 {
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
		setting.isAuthed = false
		return true
	}

	username, ok := reader.read(int(authHeader[1]))
	if !ok {
		return false
	}

	passLen, ok := reader.read(1)
	if !ok {
		return false
	}

	password, ok := reader.read(int(passLen[0]))
	if !ok {
		return false
	}

	clientName := string(username)
	clientPass := string(password)

	if clientName != socks.Username || clientPass != socks.Password {
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
		setting.isAuthed = false
		return true
	}
	// username && password all fits!
	protocol.ConstructMessage(sMessage, header, succMess, false)
	sMessage.SendMessage()
	setting.isAuthed = true
	return true
}

func (socks *Socks) buildConn(mgr *manager.Manager, setting *Setting, reader *socksDataReader, seq uint64) bool {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))), // No need to set route when agent send mess to admin
		Route:       protocol.TEMP_ROUTE,
	}

	failMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})),
		Data:    []byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	requestHeader, ok := reader.read(4)
	if !ok {
		return false
	}

	if requestHeader[0] != 0x05 {
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
		return true
	}

	switch requestHeader[1] {
	case 0x01:
		return tcpConnect(mgr, setting, requestHeader[3], reader, seq)
	case 0x02:
		tcpBind(mgr, setting, requestHeader[3], reader, seq)
	case 0x03:
		return udpAssociate(mgr, setting, requestHeader[3], reader, seq)
	default:
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
	}
	return true
}

// TCPConnect 如果是代理tcp
func tcpConnect(mgr *manager.Manager, setting *Setting, atyp byte, reader *socksDataReader, seq uint64) bool {
	var err error

	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	failMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})),
		Data:    []byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	succMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})),
		Data:    []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	if atyp != 0x01 && atyp != 0x03 && atyp != 0x04 {
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
		setting.tcpConnected = false
		return true
	}

	host, port, ok := readSocksTarget(reader, atyp)
	if !ok {
		return false
	}

	setting.tcpConn, err = net.DialTimeout("tcp", net.JoinHostPort(host, port), 10*time.Second)

	if err != nil {
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
		setting.tcpConnected = false
		return true
	}
	share.ConfigureTCPConn(setting.tcpConn)

	mgrTask := &manager.SocksTask{
		Mode: manager.S_CHECKTCP,
		Seq:  seq,
	}
	mgr.SocksManager.TaskChan <- mgrTask
	socksResult := <-mgr.SocksManager.ResultChan
	if !socksResult.OK { // if admin has already send fin,then close the conn and set setting.tcpConnected -> false
		share.CloseQuietly(setting.tcpConn)
		protocol.ConstructMessage(sMessage, header, failMess, false)
		sMessage.SendMessage()
		setting.tcpConnected = false
		return true
	}

	protocol.ConstructMessage(sMessage, header, succMess, false)
	sMessage.SendMessage()
	setting.tcpConnected = true
	return true
}

func proxyC2STCP(conn net.Conn, dataChan chan []byte, firstData []byte) {
	if len(firstData) > 0 {
		if err := share.WriteFull(conn, firstData); err != nil {
			share.CloseQuietly(conn)
			return
		}
	}

	for {
		data, ok := <-dataChan
		if !ok { // no need to send FIN actively
			share.CloseQuietly(conn)
			return
		}
		if err := share.WriteFull(conn, data); err != nil {
			share.CloseQuietly(conn)
			return
		}
	}
}

func proxyS2CTCP(conn net.Conn, seq uint64) {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))), // No need to set route when agent send mess to admin
		Route:       protocol.TEMP_ROUTE,
	}

	buffer := share.AcquireTransferBuffer()
	defer share.ReleaseTransferBuffer(buffer)

	for {
		length, err := conn.Read(buffer)
		if err != nil {
			share.CloseQuietly(conn) // close conn immediately
			return
		}

		dataMess := &protocol.SocksTCPData{
			Seq:     seq,
			DataLen: uint64(length),
			Data:    buffer[:length],
		}

		protocol.SendMessage(sMessage, header, dataMess, false)
	}
}

// TCPBind TCPBind方式
func tcpBind(mgr *manager.Manager, setting *Setting, atyp byte, reader *socksDataReader, seq uint64) {
	fmt.Println("Not ready") //limited use, add to Todo
	setting.tcpConnected = false
}

type socksLocalAddr struct {
	Host string
	Port int
}

func (addr *socksLocalAddr) byteArray() []byte {
	bytes := make([]byte, 6)
	copy(bytes[:4], net.ParseIP(addr.Host).To4())
	bytes[4] = byte(addr.Port >> 8)
	bytes[5] = byte(addr.Port % 256)
	return bytes
}

// Based on rfc1928,agent must send message strictly
// UDPAssociate UDPAssociate方式
func udpAssociate(mgr *manager.Manager, setting *Setting, atyp byte, reader *socksDataReader, seq uint64) bool {
	setting.isUDP = true

	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	dataHeader := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	assHeader := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.UDPASSSTART,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	failMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(len([]byte{0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})),
		Data:    []byte{0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	if atyp != 0x01 && atyp != 0x03 && atyp != 0x04 {
		protocol.ConstructMessage(sMessage, dataHeader, failMess, false)
		sMessage.SendMessage()
		setting.success = false
		return true
	}

	host, port, ok := readSocksTarget(reader, atyp)
	if !ok {
		return false
	}

	udpListenerAddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	if err != nil {
		protocol.ConstructMessage(sMessage, dataHeader, failMess, false)
		sMessage.SendMessage()
		setting.success = false
		return true
	}

	udpListener, err := net.ListenUDP("udp", udpListenerAddr)
	if err != nil {
		protocol.ConstructMessage(sMessage, dataHeader, failMess, false)
		sMessage.SendMessage()
		setting.success = false
		return true
	}

	sourceAddr := net.JoinHostPort(host, port)

	mgrTask := &manager.SocksTask{
		Mode: manager.S_CHECKUDP,
		Seq:  seq,
	}

	mgr.SocksManager.TaskChan <- mgrTask
	socksResult := <-mgr.SocksManager.ResultChan
	if !socksResult.OK {
		share.CloseQuietly(udpListener) // close listener,because tcp conn is closed
		protocol.ConstructMessage(sMessage, dataHeader, failMess, false)
		sMessage.SendMessage()
		setting.success = false
		return true
	}

	mgrTask = &manager.SocksTask{
		Mode: manager.S_GETUDPCHANS,
		Seq:  seq,
	}
	mgr.SocksManager.TaskChan <- mgrTask
	socksResult = <-mgr.SocksManager.ResultChan

	if !socksResult.OK { // no need to close listener,cuz TCPFIN has helped us
		protocol.ConstructMessage(sMessage, dataHeader, failMess, false)
		sMessage.SendMessage()
		setting.success = false
		return true
	}

	readyChan := socksResult.ReadyChan

	assMess := &protocol.UDPAssStart{
		Seq:           seq,
		SourceAddrLen: uint16(len([]byte(sourceAddr))),
		SourceAddr:    sourceAddr,
	}

	protocol.ConstructMessage(sMessage, assHeader, assMess, false)
	sMessage.SendMessage()

	if adminResponse, ok := <-readyChan; adminResponse != "" && ok {
		temp := strings.Split(adminResponse, ":")
		adminAddr := temp[0]
		adminPort, _ := strconv.Atoi(temp[1])

		localAddr := socksLocalAddr{adminAddr, adminPort}
		buf := make([]byte, 10)
		copy(buf, []byte{0x05, 0x00, 0x00, 0x01})
		copy(buf[4:], localAddr.byteArray())

		dataMess := &protocol.SocksTCPData{
			Seq:     seq,
			DataLen: 10,
			Data:    buf,
		}

		protocol.ConstructMessage(sMessage, dataHeader, dataMess, false)
		sMessage.SendMessage()

		setting.udpListener = udpListener
		setting.success = true
		return true
	}

	protocol.ConstructMessage(sMessage, dataHeader, failMess, false)
	sMessage.SendMessage()
	setting.success = false
	return true
}

// proxyC2SUDP 代理C-->Sudp流量
func proxyC2SUDP(mgr *manager.Manager, listener *net.UDPConn, seq uint64) {
	mgrTask := &manager.SocksTask{
		Mode: manager.S_GETUDPCHANS,
		Seq:  seq,
	}
	mgr.SocksManager.TaskChan <- mgrTask
	result := <-mgr.SocksManager.ResultChan
	if !result.OK {
		share.CloseQuietly(listener)
		return
	}
	dataChan := result.DataChan

	defer func() {
		// Just avoid panic
		if r := recover(); r != nil {
			go func() { //continue to read channel,avoid some remaining data sent by admin blocking our dispatcher
				for {
					_, ok := <-dataChan
					if !ok {
						return
					}
				}
			}()
		}
	}()

	for {
		var remote string
		var udpData []byte

		data, ok := <-dataChan
		if !ok {
			share.CloseQuietly(listener)
			return
		}

		buf := []byte(data)
		if len(buf) < 4 {
			continue
		}

		if buf[0] != 0x00 || buf[1] != 0x00 || buf[2] != 0x00 {
			continue
		}

		udpHeader := make([]byte, 0, 1024)
		addrtype := buf[3]

		if addrtype == 0x01 { //IPV4
			if len(buf) < 10 {
				continue
			}
			ip := net.IPv4(buf[4], buf[5], buf[6], buf[7])
			remote = fmt.Sprintf("%s:%d", ip.String(), uint(buf[8])<<8+uint(buf[9]))
			udpData = buf[10:]
			udpHeader = append(udpHeader, buf[:10]...)
		} else if addrtype == 0x03 { //DOMAIN
			if len(buf) < 7 {
				continue
			}
			nmlen := int(buf[4])
			if len(buf) < 7+nmlen {
				continue
			}
			nmbuf := buf[5 : 5+nmlen+2]
			remote = fmt.Sprintf("%s:%d", nmbuf[:nmlen], uint(nmbuf[nmlen])<<8+uint(nmbuf[nmlen+1]))
			udpData = buf[8+nmlen:]
			udpHeader = append(udpHeader, buf[:8+nmlen]...)
		} else if addrtype == 0x04 { //IPV6
			if len(buf) < 22 {
				continue
			}
			ip := net.IP{buf[4], buf[5], buf[6], buf[7],
				buf[8], buf[9], buf[10], buf[11], buf[12],
				buf[13], buf[14], buf[15], buf[16], buf[17],
				buf[18], buf[19]}
			remote = fmt.Sprintf("[%s]:%d", ip.String(), uint(buf[20])<<8+uint(buf[21]))
			udpData = buf[22:]
			udpHeader = append(udpHeader, buf[:22]...)
		} else {
			continue
		}

		remoteAddr, err := net.ResolveUDPAddr("udp", remote)
		if err != nil {
			continue
		}

		mgrTask = &manager.SocksTask{
			Mode:            manager.S_UPDATEUDPHEADER,
			Seq:             seq,
			SocksHeaderAddr: remote,
			SocksHeader:     udpHeader,
		}
		mgr.SocksManager.TaskChan <- mgrTask
		<-mgr.SocksManager.ResultChan

		if _, err := listener.WriteToUDP(udpData, remoteAddr); err != nil {
			continue
		}
	}
}

// proxyS2CUDP 代理S-->Cudp流量
func proxyS2CUDP(mgr *manager.Manager, listener *net.UDPConn, seq uint64) {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSUDPDATA,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	buffer := share.AcquireTransferBuffer()
	defer share.ReleaseTransferBuffer(buffer)

	var data []byte
	var finalLength int

	for {
		length, addr, err := listener.ReadFromUDP(buffer)
		if err != nil {
			share.CloseQuietly(listener)
			return
		}

		mgrTask := &manager.SocksTask{
			Mode:            manager.S_GETUDPHEADER,
			Seq:             seq,
			SocksHeaderAddr: addr.String(),
		}
		mgr.SocksManager.TaskChan <- mgrTask
		result := <-mgr.SocksManager.ResultChan
		if result.OK {
			finalLength = len(result.SocksUDPHeader) + length
			data = make([]byte, 0, finalLength)
			data = append(data, result.SocksUDPHeader...)
			data = append(data, buffer[:length]...)
		} else {
			return
		}

		dataMess := &protocol.SocksUDPData{
			Seq:     seq,
			DataLen: uint64(finalLength),
			Data:    data,
		}

		protocol.SendMessage(sMessage, header, dataMess, false)
	}
}

func DispathSocksMess(mgr *manager.Manager) {
	socks := newSocks()

	for {
		message := <-mgr.SocksManager.SocksMessChan

		switch mess := message.(type) {
		case *protocol.SocksStart:
			socks.Username = mess.Username
			socks.Password = mess.Password
			go socks.start(mgr)
		case *protocol.SocksTCPData:
			mgrTask := &manager.SocksTask{
				Mode: manager.S_GETTCPDATACHAN,
				Seq:  mess.Seq,
			}
			mgr.SocksManager.TaskChan <- mgrTask
			result := <-mgr.SocksManager.ResultChan

			if !enqueueSocksData(result.DataChan, mess.Data) {
				mgr.SocksManager.TaskChan <- &manager.SocksTask{
					Mode: manager.S_CLOSETCP,
					Seq:  mess.Seq,
				}
				continue
			}

			// if not exist
			if !result.SocksSeqExist {
				go socks.handleSocks(mgr, result.DataChan, mess.Seq)
			}
		case *protocol.SocksTCPFin:
			mgrTask := &manager.SocksTask{
				Mode: manager.S_CLOSETCP,
				Seq:  mess.Seq,
			}
			mgr.SocksManager.TaskChan <- mgrTask
		case *protocol.SocksUDPData:
			mgrTask := &manager.SocksTask{
				Mode: manager.S_GETUDPCHANS,
				Seq:  mess.Seq,
			}
			mgr.SocksManager.TaskChan <- mgrTask
			result := <-mgr.SocksManager.ResultChan

			if result.OK {
				if !enqueueSocksData(result.DataChan, mess.Data) {
					mgr.SocksManager.TaskChan <- &manager.SocksTask{
						Mode: manager.S_CLOSETCP,
						Seq:  mess.Seq,
					}
				}
			}
		case *protocol.UDPAssRes:
			mgrTask := &manager.SocksTask{
				Mode: manager.S_GETUDPCHANS,
				Seq:  mess.Seq,
			}
			mgr.SocksManager.TaskChan <- mgrTask
			result := <-mgr.SocksManager.ResultChan

			if result.OK {
				if !enqueueSocksString(result.ReadyChan, mess.Addr) {
					mgr.SocksManager.TaskChan <- &manager.SocksTask{
						Mode: manager.S_CLOSETCP,
						Seq:  mess.Seq,
					}
				}
			}
		}

	}
}

func enqueueSocksData(ch chan []byte, data []byte) (ok bool) {
	if ch == nil {
		return false
	}

	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case ch <- data:
		return true
	default:
	}

	timer := time.NewTimer(socksDataEnqueueTimeout)
	defer timer.Stop()

	select {
	case ch <- data:
		return true
	case <-timer.C:
		return false
	}
}

func enqueueSocksString(ch chan string, data string) (ok bool) {
	if ch == nil {
		return false
	}

	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case ch <- data:
		return true
	default:
	}

	timer := time.NewTimer(socksDataEnqueueTimeout)
	defer timer.Stop()

	select {
	case ch <- data:
		return true
	case <-timer.C:
		return false
	}
}
