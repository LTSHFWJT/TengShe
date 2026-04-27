package initial

import (
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"strings"
)

const (
	NORMAL_ACTIVE = iota
	NORMAL_RECONNECT_ACTIVE
	NORMAL_PASSIVE
	SOCKS5_PROXY_ACTIVE
	HTTP_PROXY_ACTIVE
	SOCKS5_PROXY_RECONNECT_ACTIVE
	HTTP_PROXY_RECONNECT_ACTIVE
	SO_REUSE_PASSIVE
	IPTABLES_REUSE_PASSIVE
)

type Options struct {
	Mode         int
	Secret       string
	Transport    string
	Listen       string
	Reconnect    uint64
	Connect      string
	ReuseHost    string
	ReusePort    string
	Socks5Proxy  string
	Socks5ProxyU string
	Socks5ProxyP string
	HttpProxy    string
	Upstream     string
	Downstream   string
	Charset      string
	Domain       string
	TlsEnable    bool
}

var args *Options

func newFlagSet() (*flag.FlagSet, *Options) {
	options := new(Options)
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	flagSet.StringVar(&options.Secret, "s", "", "")
	flagSet.StringVar(&options.Transport, "transport", "tcp", "")
	flagSet.StringVar(&options.Listen, "l", "", "")
	flagSet.Uint64Var(&options.Reconnect, "reconnect", 0, "")
	flagSet.StringVar(&options.Connect, "c", "", "")
	flagSet.StringVar(&options.ReuseHost, "rehost", "", "")
	flagSet.StringVar(&options.ReusePort, "report", "", "")
	flagSet.StringVar(&options.Socks5Proxy, "socks5-proxy", "", "")
	flagSet.StringVar(&options.Socks5ProxyU, "socks5-proxyu", "", "")
	flagSet.StringVar(&options.Socks5ProxyP, "socks5-proxyp", "", "")
	flagSet.StringVar(&options.HttpProxy, "http-proxy", "", "")
	flagSet.StringVar(&options.Upstream, "up", "raw", "")
	flagSet.StringVar(&options.Downstream, "down", "raw", "")
	flagSet.StringVar(&options.Charset, "cs", "utf-8", "")
	flagSet.StringVar(&options.Domain, "domain", "", "")
	flagSet.BoolVar(&options.TlsEnable, "tls-enable", false, "")

	flagSet.Usage = func() {}

	return flagSet, options
}

// ParseOptions Parsing user's options
func ParseOptions() *Options {
	flagSet, options := newFlagSet()
	args = options

	flagSet.Parse(os.Args[1:])

	args.Transport = strings.ToLower(strings.TrimSpace(args.Transport))
	if args.Transport == "" {
		args.Transport = "tcp"
	}
	if args.Transport == "icmp" && args.Listen == "" && args.Connect == "" && args.Reconnect == 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" {
		args.Listen = "0.0.0.0"
	}

	if args.Transport == "icmp" {
		if args.Connect != "" && args.Reconnect == 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" {
			args.Mode = NORMAL_ACTIVE
		} else if args.Connect != "" && args.Reconnect != 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" {
			args.Mode = NORMAL_RECONNECT_ACTIVE
		} else if args.Connect == "" && args.Listen != "" && args.Reconnect == 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" {
			args.Mode = NORMAL_PASSIVE
		} else {
			os.Exit(1)
		}
	} else if args.Listen != "" && args.Connect == "" && args.Reconnect == 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" { // ./tengshe_agent -l <port> -s [secret]
		args.Mode = NORMAL_PASSIVE
	} else if args.Listen == "" && args.Connect != "" && args.Reconnect == 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" { // ./tengshe_agent -c <ip:port> -s [secret]
		args.Mode = NORMAL_ACTIVE
	} else if args.Listen == "" && args.Connect != "" && args.Reconnect != 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" { // ./tengshe_agent -c <ip:port> -s [secret] --reconnect <seconds>
		args.Mode = NORMAL_RECONNECT_ACTIVE
	} else if args.Listen == "" && args.Connect == "" && args.Reconnect == 0 && args.ReuseHost != "" && args.ReusePort != "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" { // ./tengshe_agent --rehost <ip> --report <port> -s [secret]
		args.Mode = SO_REUSE_PASSIVE
		log.Printf("[*] Starting agent node passively.Now reusing host %s, port %s(SO_REUSEPORT,SO_REUSEADDR)\n", args.ReuseHost, args.ReusePort)
	} else if args.Listen != "" && args.Connect == "" && args.Reconnect == 0 && args.ReuseHost == "" && args.ReusePort != "" && args.Socks5Proxy == "" && args.Socks5ProxyU == "" && args.Socks5ProxyP == "" && args.HttpProxy == "" { // ./tengshe_agent -l <port> --report <port> -s [secret]
		args.Mode = IPTABLES_REUSE_PASSIVE
		log.Printf("[*] Starting agent node passively.Now reusing port %s(IPTABLES)\n", args.ReusePort)
	} else if args.Listen == "" && args.Connect != "" && args.Reconnect == 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy != "" && args.HttpProxy == "" { // ./tengshe_agent -c <ip:port> -s [secret] --proxy <ip:port> --proxyu [username] --proxyp [password]
		args.Mode = SOCKS5_PROXY_ACTIVE
		log.Printf("[*] Starting agent node actively.Connecting to %s via socks5 proxy %s\n", args.Connect, args.Socks5Proxy)
	} else if args.Listen == "" && args.Connect != "" && args.Reconnect != 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy != "" && args.HttpProxy == "" { // ./tengshe_agent -c <ip:port> -s [secret] --proxy <ip:port> --proxyu [username] --proxyp [password] --reconnect <seconds>
		args.Mode = SOCKS5_PROXY_RECONNECT_ACTIVE
		log.Printf("[*] Starting agent node actively.Connecting to %s via socks5 proxy %s.Reconnecting every %d seconds\n", args.Connect, args.Socks5Proxy, args.Reconnect)
	} else if args.Listen == "" && args.Connect != "" && args.Reconnect == 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.HttpProxy != "" {
		args.Mode = HTTP_PROXY_ACTIVE
		log.Printf("[*] Starting agent node actively.Connecting to %s via http proxy %s\n", args.Connect, args.HttpProxy)
	} else if args.Listen == "" && args.Connect != "" && args.Reconnect != 0 && args.ReuseHost == "" && args.ReusePort == "" && args.Socks5Proxy == "" && args.HttpProxy != "" {
		args.Mode = HTTP_PROXY_RECONNECT_ACTIVE
		log.Printf("[*] Starting agent node actively.Connecting to %s via http proxy %s.Reconnecting every %d seconds\n", args.Connect, args.HttpProxy, args.Reconnect)
	} else {
		os.Exit(1)
	}

	if args.Charset != "utf-8" && args.Charset != "gbk" {
		log.Fatalf("[*] Charset must be set as 'utf-8'(default) or 'gbk'")
	}

	if args.Domain == "" && args.Connect != "" {
		addrSlice := strings.SplitN(args.Connect, ":", 2)
		args.Domain = addrSlice[0]
	}

	if err := checkOptions(args); err != nil {
		log.Fatalf("[*] Options err: %s\n", err.Error())
	}

	switch args.Mode {
	case NORMAL_PASSIVE:
		if args.Transport == "icmp" {
			log.Printf("[*] Starting agent node passively.Now listening on ICMP address %s\n", args.Listen)
		} else {
			log.Printf("[*] Starting agent node passively.Now listening on port %s\n", args.Listen)
		}
	case NORMAL_ACTIVE:
		log.Printf("[*] Starting agent node actively.Connecting to %s\n", args.Connect)
	case NORMAL_RECONNECT_ACTIVE:
		log.Printf("[*] Starting agent node actively.Connecting to %s.Reconnecting every %d seconds\n", args.Connect, args.Reconnect)
	}

	return args
}

func checkOptions(option *Options) error {
	var err error

	if option.Transport != "tcp" && option.Transport != "icmp" {
		return errors.New("transport must be tcp or icmp")
	}

	if option.Transport == "icmp" {
		if option.TlsEnable {
			return errors.New("tls-enable is not supported with ICMP transport in the first implementation")
		}
		if option.Socks5Proxy != "" || option.HttpProxy != "" || option.Socks5ProxyU != "" || option.Socks5ProxyP != "" {
			return errors.New("proxy active mode is not supported with ICMP transport")
		}
		if option.ReuseHost != "" || option.ReusePort != "" {
			return errors.New("port reuse mode is not supported with ICMP transport")
		}
		if option.Upstream != "" && option.Upstream != "raw" {
			return errors.New("ICMP transport currently supports raw upstream only")
		}
		if option.Downstream != "" && option.Downstream != "raw" {
			return errors.New("ICMP transport currently supports raw downstream only")
		}
		if option.Listen == "" {
			option.Listen = "0.0.0.0"
		}
		option.Listen, err = normalizeICMPListen(option.Listen)
		if err != nil {
			return err
		}
		if option.Connect != "" {
			option.Connect, err = normalizeICMPPeer(option.Connect)
			return err
		}
		return nil
	}

	if args.Connect != "" {
		_, err = net.ResolveTCPAddr("", option.Connect)
	}

	if args.Socks5Proxy != "" {
		_, err = net.ResolveTCPAddr("", option.Socks5Proxy)
	}

	if args.HttpProxy != "" {
		_, err = net.ResolveTCPAddr("", option.HttpProxy)
	}

	if args.ReuseHost != "" {
		if addr := net.ParseIP(args.ReuseHost); addr == nil {
			err = errors.New("ReuseHost is not a valid IP addr")
		}
	}

	return err
}

func normalizeICMPListen(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || isDecimal(value) {
		return "0.0.0.0", nil
	}
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return "", errors.New("invalid ICMP listen address")
	}
	return ip.String(), nil
}

func normalizeICMPPeer(value string) (string, error) {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	} else if strings.Count(value, ":") == 1 {
		host, port, ok := strings.Cut(value, ":")
		if ok && isDecimal(port) {
			value = host
		}
	}
	ip, err := net.ResolveIPAddr("ip4", value)
	if err != nil {
		return "", err
	}
	if ip == nil || ip.IP == nil || ip.IP.To4() == nil {
		return "", errors.New("ICMP transport currently supports IPv4 only")
	}
	return ip.IP.String(), nil
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
