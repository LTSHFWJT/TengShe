package agentbootstrap

import (
	"context"
	"log"
	"net"

	agentinitial "TengShe/agent/initial"
	"TengShe/protocol"
	"TengShe/share"
	"TengShe/share/transport/stream"
)

type Session struct {
	Conn    net.Conn
	UUID    string
	Cleanup func()
}

func Connect(options *agentinitial.Options) *Session {
	share.GeneratePreAuthToken(options.Secret)
	protocol.SetUpDownStream(options.Upstream, options.Downstream)

	session := &Session{
		Cleanup: func() {},
	}

	if options.Transport != stream.ProtocolTCP {
		switch options.Mode {
		case agentinitial.NORMAL_PASSIVE:
			listener, err := stream.Listen(context.Background(), options.Transport, options.Listen)
			if err != nil {
				log.Fatalf("[*] Error occurred: %s", err.Error())
			}
			session.Cleanup = func() {
				_ = listener.Close()
			}
			session.Conn, session.UUID = agentinitial.NormalPassiveWithListener(options, listener)
		case agentinitial.NORMAL_RECONNECT_ACTIVE:
			fallthrough
		case agentinitial.NORMAL_ACTIVE:
			session.Conn, session.UUID = agentinitial.NormalActiveWithDial(options, func() (net.Conn, error) {
				return stream.Dial(context.Background(), options.Transport, options.Connect, options.Listen)
			})
		default:
			log.Fatalf("[*] Unsupported %s agent mode", options.Transport)
		}
		return session
	}

	switch options.Mode {
	case agentinitial.NORMAL_PASSIVE:
		session.Conn, session.UUID = agentinitial.NormalPassive(options)
	case agentinitial.NORMAL_RECONNECT_ACTIVE:
		fallthrough
	case agentinitial.NORMAL_ACTIVE:
		session.Conn, session.UUID = agentinitial.NormalActive(options, nil)
	case agentinitial.SOCKS5_PROXY_RECONNECT_ACTIVE:
		fallthrough
	case agentinitial.SOCKS5_PROXY_ACTIVE:
		proxy := share.NewSocks5Proxy(options.Connect, options.Socks5Proxy, options.Socks5ProxyU, options.Socks5ProxyP)
		session.Conn, session.UUID = agentinitial.NormalActive(options, proxy)
	case agentinitial.HTTP_PROXY_RECONNECT_ACTIVE:
		fallthrough
	case agentinitial.HTTP_PROXY_ACTIVE:
		proxy := share.NewHTTPProxy(options.Connect, options.HttpProxy)
		session.Conn, session.UUID = agentinitial.NormalActive(options, proxy)
	case agentinitial.IPTABLES_REUSE_PASSIVE:
		session.Cleanup = func() {
			agentinitial.DeletePortReuseRules(options.Listen, options.ReusePort)
		}
		session.Conn, session.UUID = agentinitial.IPTableReusePassive(options)
	case agentinitial.SO_REUSE_PASSIVE:
		session.Conn, session.UUID = agentinitial.SoReusePassive(options)
	default:
		log.Fatal("[*] Unknown Mode")
	}

	return session
}
