package share

import (
	"net"
	"time"
)

const defaultTCPKeepAlive = 30 * time.Second

func ConfigureTCPConn(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok || tcpConn == nil {
		return
	}

	_ = tcpConn.SetNoDelay(true)
	_ = tcpConn.SetKeepAlive(true)
	_ = tcpConn.SetKeepAlivePeriod(defaultTCPKeepAlive)
}
