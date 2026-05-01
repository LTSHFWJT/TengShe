package smbtransport

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TransportName = "smb"

	defaultSMBPort       = 445
	defaultDialTimeout   = 10 * time.Second
	defaultIOTimeout     = 0
	defaultBufferSize    = 4096
	defaultMaxChunk      = 64 * 1024
	defaultAcceptBacklog = 128
	defaultRetryInterval = 200 * time.Millisecond
)

type Config struct {
	SMBPort        int
	SMBUser        string
	SMBPassword    string
	SMBDomain      string
	SMBWorkstation string
	SMBNullSession bool
	SMBLocalUser   bool
	DialTimeout    time.Duration
	IOTimeout      time.Duration
	BufferSize     int
	MaxChunk       int
	AcceptBacklog  int
	RetryInterval  time.Duration
	SecuritySDDL   string
}

func DefaultConfig() Config {
	return Config{
		SMBPort:       defaultSMBPort,
		DialTimeout:   defaultDialTimeout,
		IOTimeout:     defaultIOTimeout,
		BufferSize:    defaultBufferSize,
		MaxChunk:      defaultMaxChunk,
		AcceptBacklog: defaultAcceptBacklog,
		RetryInterval: defaultRetryInterval,
	}
}

func DefaultConfigFromEnv() Config {
	config := DefaultConfig()
	applyIntEnv("TENGSHE_SMB_PORT", &config.SMBPort)
	applyStringEnv("TENGSHE_SMB_USER", &config.SMBUser)
	applyStringEnv("TENGSHE_SMB_PASSWORD", &config.SMBPassword)
	applyStringEnv("TENGSHE_SMB_DOMAIN", &config.SMBDomain)
	applyStringEnv("TENGSHE_SMB_WORKSTATION", &config.SMBWorkstation)
	applyBoolEnv("TENGSHE_SMB_NULL_SESSION", &config.SMBNullSession)
	applyBoolEnv("TENGSHE_SMB_LOCAL_USER", &config.SMBLocalUser)
	applyDurationEnv("TENGSHE_SMB_DIAL_TIMEOUT", &config.DialTimeout)
	applyDurationEnv("TENGSHE_SMB_IO_TIMEOUT", &config.IOTimeout)
	applyIntEnv("TENGSHE_SMB_BUFFER", &config.BufferSize)
	applyIntEnv("TENGSHE_SMB_MAX_CHUNK", &config.MaxChunk)
	applyIntEnv("TENGSHE_SMB_ACCEPT_BACKLOG", &config.AcceptBacklog)
	applyDurationEnv("TENGSHE_SMB_RETRY_INTERVAL", &config.RetryInterval)
	applyStringEnv("TENGSHE_SMB_SECURITY_SDDL", &config.SecuritySDDL)
	return normalizeConfig(config)
}

func normalizeConfig(config Config) Config {
	if config.SMBPort <= 0 || config.SMBPort > 65535 {
		config.SMBPort = defaultSMBPort
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = defaultDialTimeout
	}
	if config.IOTimeout < 0 {
		config.IOTimeout = defaultIOTimeout
	}
	if config.BufferSize <= 0 {
		config.BufferSize = defaultBufferSize
	}
	if config.MaxChunk <= 0 {
		config.MaxChunk = defaultMaxChunk
	}
	if config.AcceptBacklog <= 0 {
		config.AcceptBacklog = defaultAcceptBacklog
	}
	if config.AcceptBacklog > 255 {
		config.AcceptBacklog = 255
	}
	if config.RetryInterval <= 0 {
		config.RetryInterval = defaultRetryInterval
	}
	config.SMBUser = strings.TrimSpace(config.SMBUser)
	config.SMBDomain = strings.TrimSpace(config.SMBDomain)
	config.SMBWorkstation = strings.TrimSpace(config.SMBWorkstation)
	if config.SMBUser == "" && config.SMBPassword == "" {
		config.SMBNullSession = true
	}
	config.SecuritySDDL = strings.TrimSpace(config.SecuritySDDL)
	return config
}

func applyStringEnv(name string, dst *string) {
	value := strings.TrimSpace(os.Getenv(name))
	if value != "" {
		*dst = value
	}
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

func applyBoolEnv(name string, dst *bool) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		*dst = parsed
	}
}
