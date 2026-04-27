package icmptransport

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TransportName = "icmp"

	defaultPayloadMTU       = 888
	defaultRecvQueue        = 4096
	defaultAcceptQueue      = 64
	defaultSendWindow       = 128
	defaultHandshakeTimeout = 5 * time.Second
	defaultIdleTimeout      = 60 * time.Second
	defaultCloseTimeout     = 2 * time.Second
	defaultRetransmitMin    = 300 * time.Millisecond
	defaultRetransmitMax    = 3 * time.Second
	defaultMaxRetries       = 10
	defaultInitialWindow    = 4
)

type Config struct {
	PayloadMTU       int
	RecvQueue        int
	AcceptQueue      int
	SendWindow       int
	InitialWindow    int
	HandshakeTimeout time.Duration
	IdleTimeout      time.Duration
	CloseTimeout     time.Duration
	RetransmitMin    time.Duration
	RetransmitMax    time.Duration
	MaxRetries       int
}

func DefaultConfig() Config {
	return Config{
		PayloadMTU:       defaultPayloadMTU,
		RecvQueue:        defaultRecvQueue,
		AcceptQueue:      defaultAcceptQueue,
		SendWindow:       defaultSendWindow,
		InitialWindow:    defaultInitialWindow,
		HandshakeTimeout: defaultHandshakeTimeout,
		IdleTimeout:      defaultIdleTimeout,
		CloseTimeout:     defaultCloseTimeout,
		RetransmitMin:    defaultRetransmitMin,
		RetransmitMax:    defaultRetransmitMax,
		MaxRetries:       defaultMaxRetries,
	}
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
	if config.HandshakeTimeout <= 0 {
		config.HandshakeTimeout = defaultHandshakeTimeout
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
	return config
}

func DefaultConfigFromEnv() Config {
	config := DefaultConfig()
	applyIntEnv("TENGSHE_ICMP_MTU", &config.PayloadMTU)
	applyIntEnv("TENGSHE_ICMP_RECV_QUEUE", &config.RecvQueue)
	applyIntEnv("TENGSHE_ICMP_ACCEPT_QUEUE", &config.AcceptQueue)
	applyIntEnv("TENGSHE_ICMP_WINDOW", &config.SendWindow)
	applyIntEnv("TENGSHE_ICMP_INITIAL_WINDOW", &config.InitialWindow)
	applyDurationEnv("TENGSHE_ICMP_HANDSHAKE_TIMEOUT", &config.HandshakeTimeout)
	applyDurationEnv("TENGSHE_ICMP_IDLE_TIMEOUT", &config.IdleTimeout)
	applyDurationEnv("TENGSHE_ICMP_CLOSE_TIMEOUT", &config.CloseTimeout)
	applyDurationEnv("TENGSHE_ICMP_RETRANSMIT_MIN", &config.RetransmitMin)
	applyDurationEnv("TENGSHE_ICMP_RETRANSMIT_MAX", &config.RetransmitMax)
	applyIntEnv("TENGSHE_ICMP_MAX_RETRIES", &config.MaxRetries)
	return normalizeConfig(config)
}

func applyIntEnv(name string, dst *int) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err == nil {
		*dst = parsed
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
