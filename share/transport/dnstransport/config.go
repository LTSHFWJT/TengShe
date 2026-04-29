package dnstransport

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TransportName = "dns"

	defaultPayloadMTU       = 180
	defaultRecvQueue        = 512
	defaultAcceptQueue      = 64
	defaultSendWindow       = 32
	defaultInitialWindow    = 4
	defaultHandshakeTimeout = 5 * time.Second
	defaultQueryTimeout     = 5 * time.Second
	defaultPollInterval     = 300 * time.Millisecond
	defaultIdlePollInterval = 2 * time.Second
	defaultIdleTimeout      = 60 * time.Second
	defaultCloseTimeout     = 2 * time.Second
	defaultRetransmitMin    = 500 * time.Millisecond
	defaultRetransmitMax    = 5 * time.Second
	defaultMaxRetries       = 10
	defaultTTL              = 0
	defaultQueryMaxLen      = 220
	defaultLabelMaxLen      = 57
	defaultCodec            = "hex"
	defaultEDNS0PayloadSize = 1232
	defaultResponseCache    = 1024
	defaultPendingWait      = 150 * time.Millisecond
)

type Config struct {
	PayloadMTU       int
	RecvQueue        int
	AcceptQueue      int
	SendWindow       int
	InitialWindow    int
	QueryMaxLen      int
	LabelMaxLen      int
	TTL              uint32
	HandshakeTimeout time.Duration
	QueryTimeout     time.Duration
	PollInterval     time.Duration
	IdlePollInterval time.Duration
	IdleTimeout      time.Duration
	CloseTimeout     time.Duration
	RetransmitMin    time.Duration
	RetransmitMax    time.Duration
	MaxRetries       int
	EDNS0            bool
	EDNS0PayloadSize int
	Codec            string
	ResponseCache    int
	PendingWait      time.Duration
}

func DefaultConfig() Config {
	return Config{
		PayloadMTU:       defaultPayloadMTU,
		RecvQueue:        defaultRecvQueue,
		AcceptQueue:      defaultAcceptQueue,
		SendWindow:       defaultSendWindow,
		InitialWindow:    defaultInitialWindow,
		QueryMaxLen:      defaultQueryMaxLen,
		LabelMaxLen:      defaultLabelMaxLen,
		TTL:              defaultTTL,
		HandshakeTimeout: defaultHandshakeTimeout,
		QueryTimeout:     defaultQueryTimeout,
		PollInterval:     defaultPollInterval,
		IdlePollInterval: defaultIdlePollInterval,
		IdleTimeout:      defaultIdleTimeout,
		CloseTimeout:     defaultCloseTimeout,
		RetransmitMin:    defaultRetransmitMin,
		RetransmitMax:    defaultRetransmitMax,
		MaxRetries:       defaultMaxRetries,
		EDNS0:            true,
		EDNS0PayloadSize: defaultEDNS0PayloadSize,
		Codec:            defaultCodec,
		ResponseCache:    defaultResponseCache,
		PendingWait:      defaultPendingWait,
	}
}

func DefaultConfigFromEnv() Config {
	config := DefaultConfig()
	applyIntEnv("TENGSHE_DNS_MTU", &config.PayloadMTU)
	applyIntEnv("TENGSHE_DNS_RECV_QUEUE", &config.RecvQueue)
	applyIntEnv("TENGSHE_DNS_ACCEPT_QUEUE", &config.AcceptQueue)
	applyIntEnv("TENGSHE_DNS_WINDOW", &config.SendWindow)
	applyIntEnv("TENGSHE_DNS_INITIAL_WINDOW", &config.InitialWindow)
	applyIntEnv("TENGSHE_DNS_QUERY_MAXLEN", &config.QueryMaxLen)
	applyIntEnv("TENGSHE_DNS_LABEL_MAXLEN", &config.LabelMaxLen)
	applyUint32Env("TENGSHE_DNS_TTL", &config.TTL)
	applyDurationEnv("TENGSHE_DNS_HANDSHAKE_TIMEOUT", &config.HandshakeTimeout)
	applyDurationEnv("TENGSHE_DNS_QUERY_TIMEOUT", &config.QueryTimeout)
	applyDurationEnv("TENGSHE_DNS_POLL_INTERVAL", &config.PollInterval)
	applyDurationEnv("TENGSHE_DNS_IDLE_POLL_INTERVAL", &config.IdlePollInterval)
	applyDurationEnv("TENGSHE_DNS_IDLE_TIMEOUT", &config.IdleTimeout)
	applyDurationEnv("TENGSHE_DNS_CLOSE_TIMEOUT", &config.CloseTimeout)
	applyDurationEnv("TENGSHE_DNS_RETRANSMIT_MIN", &config.RetransmitMin)
	applyDurationEnv("TENGSHE_DNS_RETRANSMIT_MAX", &config.RetransmitMax)
	applyIntEnv("TENGSHE_DNS_MAX_RETRIES", &config.MaxRetries)
	applyBoolEnv("TENGSHE_DNS_EDNS0", &config.EDNS0)
	applyIntEnv("TENGSHE_DNS_EDNS0_PAYLOAD_SIZE", &config.EDNS0PayloadSize)
	applyStringEnv("TENGSHE_DNS_CODEC", &config.Codec)
	applyIntEnv("TENGSHE_DNS_RESPONSE_CACHE", &config.ResponseCache)
	applyDurationEnv("TENGSHE_DNS_PENDING_WAIT", &config.PendingWait)
	return normalizeConfig(config)
}

func normalizeConfig(config Config) Config {
	if config.PayloadMTU <= 0 {
		config.PayloadMTU = defaultPayloadMTU
	}
	if config.RecvQueue <= 0 {
		config.RecvQueue = defaultRecvQueue
	}
	if config.AcceptQueue <= 0 {
		config.AcceptQueue = defaultAcceptQueue
	}
	if config.SendWindow <= 0 {
		config.SendWindow = defaultSendWindow
	}
	if config.InitialWindow <= 0 {
		config.InitialWindow = defaultInitialWindow
	}
	if config.InitialWindow > config.SendWindow {
		config.InitialWindow = config.SendWindow
	}
	if config.QueryMaxLen <= 0 || config.QueryMaxLen > 253 {
		config.QueryMaxLen = defaultQueryMaxLen
	}
	if config.LabelMaxLen <= 0 || config.LabelMaxLen > 63 {
		config.LabelMaxLen = defaultLabelMaxLen
	}
	if config.HandshakeTimeout <= 0 {
		config.HandshakeTimeout = defaultHandshakeTimeout
	}
	if config.QueryTimeout <= 0 {
		config.QueryTimeout = defaultQueryTimeout
	}
	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.IdlePollInterval <= 0 {
		config.IdlePollInterval = defaultIdlePollInterval
	}
	if config.IdlePollInterval < config.PollInterval {
		config.IdlePollInterval = config.PollInterval
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = defaultIdleTimeout
	}
	if config.CloseTimeout <= 0 {
		config.CloseTimeout = defaultCloseTimeout
	}
	if config.RetransmitMin <= 0 {
		config.RetransmitMin = defaultRetransmitMin
	}
	if config.RetransmitMax <= 0 {
		config.RetransmitMax = defaultRetransmitMax
	}
	if config.RetransmitMax < config.RetransmitMin {
		config.RetransmitMax = config.RetransmitMin
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = defaultMaxRetries
	}
	if config.EDNS0PayloadSize <= 0 {
		config.EDNS0PayloadSize = defaultEDNS0PayloadSize
	}
	if config.ResponseCache <= 0 {
		config.ResponseCache = defaultResponseCache
	}
	if config.PendingWait < 0 {
		config.PendingWait = 0
	}
	if config.PendingWait == 0 {
		config.PendingWait = defaultPendingWait
	}
	if config.PendingWait > config.QueryTimeout/2 {
		config.PendingWait = config.QueryTimeout / 2
	}
	config.Codec = strings.ToLower(strings.TrimSpace(config.Codec))
	if config.Codec != defaultCodec {
		config.Codec = defaultCodec
	}
	return config
}

func applyIntEnv(name string, dst *int) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		*dst = parsed
	}
}

func applyUint32Env(name string, dst *uint32) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return
	}
	if parsed, err := strconv.ParseUint(value, 10, 32); err == nil {
		*dst = uint32(parsed)
	}
}

func applyBoolEnv(name string, dst *bool) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return
	}
	switch value {
	case "1", "true", "yes", "on":
		*dst = true
	case "0", "false", "no", "off":
		*dst = false
	}
}

func applyStringEnv(name string, dst *string) {
	value := strings.TrimSpace(os.Getenv(name))
	if value != "" {
		*dst = value
	}
}

func applyDurationEnv(name string, dst *time.Duration) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		*dst = parsed
		return
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		*dst = time.Duration(parsed) * time.Millisecond
	}
}
