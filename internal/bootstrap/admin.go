package bootstrap

import (
	"context"
	"net"
	"os"

	admininitial "TengShe/admin/initial"
	"TengShe/admin/printer"
	"TengShe/admin/topology"
	"TengShe/protocol"
	"TengShe/share"
	"TengShe/share/transport/stream"
)

type AdminSession struct {
	Topology *topology.Topology
	Root     *admininitial.AdminConn
	Accepted <-chan *admininitial.AdminConn
}

func ConnectAdmin(options *admininitial.Options) *AdminSession {
	share.GeneratePreAuthToken(options.Secret)
	protocol.SetUpDownStream("raw", options.Downstream)

	topo := topology.NewTopology()
	go topo.Run()

	printer.Warning("[*] Waiting for new connection...\r\n")

	var root *admininitial.AdminConn
	accepted := make(chan *admininitial.AdminConn, 16)

	if options.Transport != stream.ProtocolTCP {
		switch options.Mode {
		case admininitial.NORMAL_ACTIVE:
			root = admininitial.NormalActiveWithDial(options, topo, func() (net.Conn, error) {
				return stream.Dial(context.Background(), options.Transport, options.Connect, options.Listen)
			})
		case admininitial.NORMAL_PASSIVE:
			listener, err := stream.Listen(context.Background(), options.Transport, options.Listen)
			if err != nil {
				printer.Fail("[*] Error occurred: %s", err.Error())
				os.Exit(0)
			}
			root = admininitial.NormalPassiveWithListener(options, topo, accepted, listener)
		default:
			printer.Fail("[*] Unsupported %s admin mode", options.Transport)
			os.Exit(0)
		}
		return &AdminSession{
			Topology: topo,
			Root:     root,
			Accepted: accepted,
		}
	}

	switch options.Mode {
	case admininitial.NORMAL_ACTIVE:
		root = admininitial.NormalActive(options, topo, nil)
	case admininitial.NORMAL_PASSIVE:
		root = admininitial.NormalPassive(options, topo, accepted)
	case admininitial.SOCKS5_PROXY_ACTIVE:
		proxy := share.NewSocks5Proxy(options.Connect, options.Socks5Proxy, options.Socks5ProxyU, options.Socks5ProxyP)
		root = admininitial.NormalActive(options, topo, proxy)
	case admininitial.HTTP_PROXY_ACTIVE:
		proxy := share.NewHTTPProxy(options.Connect, options.HttpProxy)
		root = admininitial.NormalActive(options, topo, proxy)
	default:
		printer.Fail("[*] Unknown Mode")
		os.Exit(0)
	}

	return &AdminSession{
		Topology: topo,
		Root:     root,
		Accepted: accepted,
	}
}
