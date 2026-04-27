//go:build windows

package initial

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"TengShe/admin/printer"
)

const (
	NORMAL_ACTIVE = iota
	NORMAL_PASSIVE
	SOCKS5_PROXY_ACTIVE
	HTTP_PROXY_ACTIVE
)

type Options struct {
	Mode         uint8
	Secret       string
	Transport    string
	Listen       string
	Connect      string
	Socks5Proxy  string
	Socks5ProxyU string
	Socks5ProxyP string
	HttpProxy    string
	Downstream   string
	Domain       string
	TlsEnable    bool
	Heartbeat    bool
}

var args *Options

func newFlagSet() (*flag.FlagSet, *Options) {
	options := new(Options)
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	flagSet.StringVar(&options.Secret, "s", "", "Communication secret")
	flagSet.StringVar(&options.Transport, "transport", "tcp", "Underlying transport: tcp or icmp")
	flagSet.StringVar(&options.Listen, "l", "", "Listen port")
	flagSet.StringVar(&options.Connect, "c", "", "The node address when you actively connect to it")
	flagSet.StringVar(&options.Socks5Proxy, "socks5-proxy", "", "The socks5 server ip:port you want to use")
	flagSet.StringVar(&options.Socks5ProxyU, "socks5-proxyu", "", "socks5 username")
	flagSet.StringVar(&options.Socks5ProxyP, "socks5-proxyp", "", "socks5 password")
	flagSet.StringVar(&options.HttpProxy, "http-proxy", "", "The http proxy server ip:port you want to use")
	flagSet.StringVar(&options.Downstream, "down", "raw", "Downstream data type you want to use")
	flagSet.StringVar(&options.Domain, "domain", "", "Domain name for TLS SNI/WS")
	flagSet.BoolVar(&options.TlsEnable, "tls-enable", false, "Encrypt connection by TLS")
	flagSet.BoolVar(&options.Heartbeat, "heartbeat", false, "Send heartbeat packet to first agent")

	flagSet.Usage = func() {
		newUsage(flagSet)
	}

	return flagSet, options
}

func newUsage(flagSet *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, `
Usages:
	>> ./tengshe_admin -l <port> -s [secret]
	>> ./tengshe_admin -c <ip:port> -s [secret]
	>> ./tengshe_admin -c <ip:port> -s [secret] --socks5-proxy <ip:port> --socks5-proxyu [username] --socks5-proxyp [password]
`)
	flagSet.PrintDefaults()
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
	if args.Transport == "icmp" && args.Listen == "" && args.Connect == "" && args.Socks5Proxy == "" && args.HttpProxy == "" {
		args.Listen = "0.0.0.0"
	}

	if args.Transport == "icmp" {
		if args.Connect != "" && args.Socks5Proxy == "" && args.HttpProxy == "" {
			args.Mode = NORMAL_ACTIVE
		} else if args.Connect == "" && args.Listen != "" && args.Socks5Proxy == "" && args.HttpProxy == "" {
			args.Mode = NORMAL_PASSIVE
		} else {
			flagSet.Usage()
			os.Exit(0)
		}
	} else if args.Listen != "" && args.Connect == "" && args.Socks5Proxy == "" && args.HttpProxy == "" { // ./tengshe_admin -l <port> -s [secret]
		args.Mode = NORMAL_PASSIVE
	} else if args.Connect != "" && args.Listen == "" && args.Socks5Proxy == "" && args.HttpProxy == "" { // ./tengshe_admin -c <ip:port> -s [secret]
		args.Mode = NORMAL_ACTIVE
	} else if args.Connect != "" && args.Listen == "" && args.Socks5Proxy != "" && args.HttpProxy == "" { // ./tengshe_admin -c <ip:port> -s [secret] --proxy <ip:port> --proxyu [username] --proxyp [password]
		args.Mode = SOCKS5_PROXY_ACTIVE
	} else if args.Connect != "" && args.Listen == "" && args.Socks5Proxy == "" && args.HttpProxy != "" {
		args.Mode = HTTP_PROXY_ACTIVE
	} else { // Wrong format
		flagSet.Usage()
		os.Exit(0)
	}

	if args.Domain == "" && args.Connect != "" {
		addrSlice := strings.SplitN(args.Connect, ":", 2)
		args.Domain = addrSlice[0]
	}

	if err := checkOptions(args); err != nil {
		printer.Fail("[*] Options err: %s\r\n", err.Error())
		os.Exit(0)
	}

	switch args.Mode {
	case NORMAL_PASSIVE:
		if args.Transport == "icmp" {
			printer.Warning("[*] Starting admin node on ICMP address %s\r\n", args.Listen)
		} else {
			printer.Warning("[*] Starting admin node on port %s\r\n", args.Listen)
		}
	case NORMAL_ACTIVE:
		printer.Warning("[*] Trying to connect node actively")
	case SOCKS5_PROXY_ACTIVE:
		printer.Warning("[*] Trying to connect node actively via socks5 proxy %s\r\n", args.Socks5Proxy)
	case HTTP_PROXY_ACTIVE:
		printer.Warning("[*] Trying to connect node actively via http proxy %s\r\n", args.HttpProxy)
	}

	return args
}

func checkOptions(option *Options) error {
	var err error

	if option.Transport != "tcp" && option.Transport != "icmp" {
		return fmt.Errorf("transport must be tcp or icmp")
	}

	if option.Transport == "icmp" {
		if option.TlsEnable {
			return fmt.Errorf("tls-enable is not supported with ICMP transport in the first implementation")
		}
		if option.Socks5Proxy != "" || option.HttpProxy != "" {
			return fmt.Errorf("proxy active mode is not supported with ICMP transport")
		}
		if option.Downstream != "" && option.Downstream != "raw" {
			return fmt.Errorf("ICMP transport currently supports raw downstream only")
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

	return err
}

func normalizeICMPListen(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || isDecimal(value) {
		return "0.0.0.0", nil
	}
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("invalid ICMP listen address %q", value)
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
		return "", fmt.Errorf("ICMP transport currently supports IPv4 only: %q", value)
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
