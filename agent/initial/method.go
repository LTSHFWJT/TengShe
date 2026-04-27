package initial

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"TengShe/protocol"
	"TengShe/share"
	"TengShe/utils"

	reuseport "github.com/libp2p/go-reuseport"
)

const CHAIN_NAME = "TENGSHE"

var START_FORWARDING string
var STOP_FORWARDING string

type DialFunc func() (net.Conn, error)

func achieveUUID(conn net.Conn, secret string) (uuid string) {
	rMessage := protocol.NewUpMsg(conn, secret, protocol.TEMP_UUID)
	fHeader, fMessage, err := protocol.DestructMessage(rMessage)

	if err != nil {
		share.CloseQuietly(conn)
		log.Fatalf("[*] Fail to achieve UUID, Error: %s", err.Error())
	}

	if fHeader.MessageType == protocol.UUID {
		mmess := fMessage.(*protocol.UUIDMess)
		uuid = mmess.UUID
	}

	return uuid
}

func NormalActive(userOptions *Options, proxy share.Proxy) (net.Conn, string) {
	return NormalActiveWithDial(userOptions, func() (net.Conn, error) {
		if proxy == nil {
			return net.Dial("tcp", userOptions.Connect)
		}
		return proxy.Dial()
	})
}

func NormalActiveWithDial(userOptions *Options, dial DialFunc) (net.Conn, string) {
	var sMessage, rMessage protocol.Message
	// just say hi!
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Shhh...")),
		Greeting:    "Shhh...",
		UUIDLen:     uint16(len(protocol.TEMP_UUID)),
		UUID:        protocol.TEMP_UUID,
		IsAdmin:     0,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Sender:      protocol.TEMP_UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	for {
		var (
			conn net.Conn
			err  error
		)

		conn, err = dial()

		if err != nil {
			log.Fatalf("[*] Error occurred: %s", err.Error())
		}

		conn, tlsWrapped, err := share.PrepareActiveUpstreamConn(conn, userOptions.TlsEnable, userOptions.Domain)
		if tlsWrapped {
			userOptions.Secret = ""
		}
		if err != nil {
			if share.IsConnPrepareStage(err, share.ConnPrepareStageTLS) {
				log.Printf("[*] Error occurred: %s", err.Error())
				share.CloseQuietly(conn)
				continue
			}
			log.Fatalf("[*] Error occurred: %s", err.Error())
		}

		sMessage = protocol.NewUpMsg(conn, userOptions.Secret, protocol.TEMP_UUID)

		protocol.SendMessage(sMessage, header, hiMess, false)

		rMessage = protocol.NewUpMsg(conn, userOptions.Secret, protocol.TEMP_UUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)

		if err != nil {
			share.CloseQuietly(conn)
			log.Fatalf("[*] Fail to connect %s, Error: %s", conn.RemoteAddr().String(), err.Error())
		}

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == "Keep silent" && mmess.IsAdmin == 1 {
				uuid := achieveUUID(conn, userOptions.Secret)
				return conn, uuid
			}
		}

		share.CloseQuietly(conn)
		log.Fatal("[*] Admin looks invalid!\n")
	}
}

func NormalPassive(userOptions *Options) (net.Conn, string) {
	listenAddr, _, err := utils.CheckIPPort(userOptions.Listen)
	if err != nil {
		log.Fatalf("[*] Error occurred: %s", err.Error())
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[*] Error occurred: %s", err.Error())
	}

	defer func() {
		share.CloseQuietly(listener)
	}()

	return NormalPassiveWithListener(userOptions, listener)
}

func NormalPassiveWithListener(userOptions *Options, listener net.Listener) (net.Conn, string) {
	var sMessage, rMessage protocol.Message

	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Keep silent")),
		Greeting:    "Keep silent",
		UUIDLen:     uint16(len(protocol.TEMP_UUID)),
		UUID:        protocol.TEMP_UUID,
		IsAdmin:     0,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Sender:      protocol.TEMP_UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[*] Error occurred: %s\n", err.Error())
			continue
		}

		conn, tlsWrapped, err := share.PreparePassiveUpstreamConn(conn, userOptions.TlsEnable)
		if tlsWrapped {
			userOptions.Secret = ""
		}
		if err != nil {
			if share.IsConnPrepareStage(err, share.ConnPrepareStageTLS) {
				log.Printf("[*] Error occurred: %s", err.Error())
				share.CloseQuietly(conn)
				continue
			}
			log.Fatalf("[*] Error occurred: %s", err.Error())
		}

		rMessage = protocol.NewUpMsg(conn, userOptions.Secret, protocol.TEMP_UUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)

		if err != nil {
			log.Printf("[*] Fail to set connection from %s, Error: %s\n", conn.RemoteAddr().String(), err.Error())
			share.CloseQuietly(conn)
			continue
		}

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == "Shhh..." && mmess.IsAdmin == 1 {
				sMessage = protocol.NewUpMsg(conn, userOptions.Secret, protocol.TEMP_UUID)
				protocol.SendMessage(sMessage, header, hiMess, false)
				uuid := achieveUUID(conn, userOptions.Secret)
				return conn, uuid
			}
		}

		share.CloseQuietly(conn)
		log.Println("[*] Incoming connection looks invalid.")
	}
}

// IPTable reuse port functions
func IPTableReusePassive(userOptions *Options) (net.Conn, string) {
	// call setReuseSecret first, cuz userOptions.Secret may be cleared if tls is enabled
	setReuseSecret(userOptions)
	SetPortReuseRules(userOptions.Listen, userOptions.ReusePort)
	go waitForExit(userOptions.Listen, userOptions.ReusePort)
	conn, uuid := NormalPassive(userOptions)
	return conn, uuid
}

func waitForExit(localPort, reusedPort string) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) //监听ctrl+c、kill命令
	for {
		<-sigs
		DeletePortReuseRules(localPort, reusedPort)
		os.Exit(0)
	}
}

func setReuseSecret(userOptions *Options) {
	firstSecret := utils.GetStringMd5(userOptions.Secret)
	secondSecret := utils.GetStringMd5(firstSecret)
	finalSecret := firstSecret[:24] + secondSecret[:24]
	START_FORWARDING = finalSecret[16:32]
	STOP_FORWARDING = finalSecret[32:]
}

func DeletePortReuseRules(localPort string, reusedPort string) error {
	var cmds []string

	cmds = append(cmds, fmt.Sprintf("iptables -t nat -D PREROUTING -p tcp --dport %s --syn -m recent --rcheck --seconds 3600 --name %s --rsource -j %s", reusedPort, strings.ToLower(CHAIN_NAME), CHAIN_NAME))
	cmds = append(cmds, fmt.Sprintf("iptables -D INPUT -p tcp -m string --string %s --algo bm -m recent --name %s --remove -j ACCEPT", STOP_FORWARDING, strings.ToLower(CHAIN_NAME)))
	cmds = append(cmds, fmt.Sprintf("iptables -D INPUT -p tcp -m string --string %s --algo bm -m recent --set --name %s --rsource -j ACCEPT", START_FORWARDING, strings.ToLower(CHAIN_NAME)))
	cmds = append(cmds, fmt.Sprintf("iptables -t nat -F %s", CHAIN_NAME))
	cmds = append(cmds, fmt.Sprintf("iptables -t nat -X %s", CHAIN_NAME))

	for _, each := range cmds {
		cmd := strings.Split(each, " ")
		exec.Command(cmd[0], cmd[1:]...).Run()
	}

	return nil
}

func SetPortReuseRules(localPort string, reusedPort string) error {
	var cmds []string

	cmds = append(cmds, fmt.Sprintf("iptables -t nat -N %s", CHAIN_NAME))                                                                                                                                      //新建自定义链
	cmds = append(cmds, fmt.Sprintf("iptables -t nat -A %s -p tcp -j REDIRECT --to-port %s", CHAIN_NAME, localPort))                                                                                           //将自定义链定义为转发流量至自定义监听端口
	cmds = append(cmds, fmt.Sprintf("iptables -A INPUT -p tcp -m string --string %s --algo bm -m recent --set --name %s --rsource -j ACCEPT", START_FORWARDING, strings.ToLower(CHAIN_NAME)))                  //设置当有一个报文带着特定字符串经过INPUT链时，将此报文的源地址加入一个特定列表中
	cmds = append(cmds, fmt.Sprintf("iptables -A INPUT -p tcp -m string --string %s --algo bm -m recent --name %s --remove -j ACCEPT", STOP_FORWARDING, strings.ToLower(CHAIN_NAME)))                          //设置当有一个报文带着特定字符串经过INPUT链时，将此报文的源地址从一个特定列表中移除
	cmds = append(cmds, fmt.Sprintf("iptables -t nat -A PREROUTING -p tcp --dport %s --syn -m recent --rcheck --seconds 3600 --name %s --rsource -j %s", reusedPort, strings.ToLower(CHAIN_NAME), CHAIN_NAME)) // 设置当有任意报文访问指定的复用端口时，检查特定列表，如果此报文的源地址在特定列表中且不超过3600秒，则执行自定义链

	for _, each := range cmds {
		cmd := strings.Split(each, " ")
		exec.Command(cmd[0], cmd[1:]...).Run() //添加规则
	}

	return nil
}

// soreuse port functions
func SoReusePassive(userOptions *Options) (net.Conn, string) {
	listenAddr := fmt.Sprintf("%s:%s", userOptions.ReuseHost, userOptions.ReusePort)

	listener, err := reuseport.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[*] Error occurred: %s", err.Error())
	}

	defer func() {
		share.CloseQuietly(listener)
	}()

	var sMessage, rMessage protocol.Message

	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Keep silent")),
		Greeting:    "Keep silent",
		UUIDLen:     uint16(len(protocol.TEMP_UUID)),
		UUID:        protocol.TEMP_UUID,
		IsAdmin:     0,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Sender:      protocol.TEMP_UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[*] Error occurred: %s\n", err.Error())
			continue
		}

		conn, tlsWrapped, err := share.PreparePassiveUpstreamTransport(conn, userOptions.TlsEnable)
		if tlsWrapped {
			userOptions.Secret = ""
		}
		if err != nil {
			log.Printf("[*] Error occurred: %s", err.Error())
			share.CloseQuietly(conn)
			continue
		}

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		buffer := make([]byte, 16)
		count, err := io.ReadFull(conn, buffer)
		conn.SetReadDeadline(time.Time{})

		if err != nil {
			if timeoutErr, ok := err.(net.Error); ok && timeoutErr.Timeout() {
				go ProxyStream(conn, buffer[:count], userOptions.ReusePort)
				continue
			} else {
				share.CloseQuietly(conn)
				continue
			}
		}

		if string(buffer[:count]) == share.AuthToken {
			conn.Write([]byte(share.AuthToken))
		} else {
			go ProxyStream(conn, buffer[:count], userOptions.ReusePort)
			continue
		}

		rMessage = protocol.NewUpMsg(conn, userOptions.Secret, protocol.TEMP_UUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)

		if err != nil {
			log.Printf("[*] Fail to set connection from %s, Error: %s\n", conn.RemoteAddr().String(), err.Error())
			share.CloseQuietly(conn)
			continue
		}

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == "Shhh..." && mmess.IsAdmin == 1 {
				sMessage = protocol.NewUpMsg(conn, userOptions.Secret, protocol.TEMP_UUID)
				protocol.SendMessage(sMessage, header, hiMess, false)
				uuid := achieveUUID(conn, userOptions.Secret)
				return conn, uuid
			}
		}

		share.CloseQuietly(conn)
		log.Println("[*] Incoming connection looks invalid.")
	}
}

// conn is not for TengShe, proxy conn to right port
func ProxyStream(conn net.Conn, message []byte, report string) {
	reuseAddr := fmt.Sprintf("127.0.0.1:%s", report)

	reuseConn, err := net.Dial("tcp", reuseAddr)

	if err != nil {
		fmt.Println(err)
		return
	}
	// send back the bytes we read before
	reuseConn.Write(message)

	go CopyTraffic(conn, reuseConn)
	CopyTraffic(reuseConn, conn)
}

func CopyTraffic(input, output net.Conn) {
	defer share.CloseQuietly(input)

	buf := make([]byte, 10240)

	for {
		count, err := input.Read(buf)
		if err != nil {
			if err == io.EOF && count > 0 {
				output.Write(buf[:count])
			}
			break
		}
		if count > 0 {
			output.Write(buf[:count])
		}
	}
}
