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

	if args.Listen != "" && args.Connect == "" && args.Socks5Proxy == "" && args.HttpProxy == "" { // ./tengshe_admin -l <port> -s [secret]
		args.Mode = NORMAL_PASSIVE
		printer.Warning("[*] Starting admin node on port %s\r\n", args.Listen)
	} else if args.Connect != "" && args.Listen == "" && args.Socks5Proxy == "" && args.HttpProxy == "" { // ./tengshe_admin -c <ip:port> -s [secret]
		args.Mode = NORMAL_ACTIVE
		printer.Warning("[*] Trying to connect node actively")
	} else if args.Connect != "" && args.Listen == "" && args.Socks5Proxy != "" && args.HttpProxy == "" { // ./tengshe_admin -c <ip:port> -s [secret] --proxy <ip:port> --proxyu [username] --proxyp [password]
		args.Mode = SOCKS5_PROXY_ACTIVE
		printer.Warning("[*] Trying to connect node actively via socks5 proxy %s\r\n", args.Socks5Proxy)
	} else if args.Connect != "" && args.Listen == "" && args.Socks5Proxy == "" && args.HttpProxy != "" {
		args.Mode = HTTP_PROXY_ACTIVE
		printer.Warning("[*] Trying to connect node actively via http proxy %s\r\n", args.HttpProxy)
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

	return args
}

func checkOptions(option *Options) error {
	var err error

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
