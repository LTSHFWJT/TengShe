package dnstransport

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const queryPrefix = "ts"

func encodeQueryName(payload []byte, domain string, config Config) (string, error) {
	config = normalizeConfig(config)
	encoded := strings.ToLower(hex.EncodeToString(payload))
	labels := splitString(encoded, config.LabelMaxLen)
	parts := make([]string, 0, len(labels)+2)
	parts = append(parts, queryPrefix)
	parts = append(parts, labels...)
	parts = append(parts, domain)
	name := strings.Join(parts, ".")
	if len(name) > config.QueryMaxLen {
		return "", fmt.Errorf("DNS query name too long: %d > %d", len(name), config.QueryMaxLen)
	}
	return name + ".", nil
}

func decodeQueryName(name, domain string) ([]byte, error) {
	name = strings.ToLower(strings.Trim(strings.TrimSpace(name), "."))
	domain = strings.ToLower(strings.Trim(domain, "."))
	suffix := "." + domain
	if name != domain && !strings.HasSuffix(name, suffix) {
		return nil, fmt.Errorf("DNS query is outside tunnel domain %q", domain)
	}
	prefix := strings.TrimSuffix(name, suffix)
	prefix = strings.Trim(prefix, ".")
	if prefix == "" {
		return nil, fmt.Errorf("DNS query missing transport payload")
	}
	labels := strings.Split(prefix, ".")
	if len(labels) < 2 || labels[0] != queryPrefix {
		return nil, fmt.Errorf("DNS query missing %q prefix", queryPrefix)
	}
	encoded := strings.Join(labels[1:], "")
	if encoded == "" {
		return nil, fmt.Errorf("DNS query payload is empty")
	}
	return hex.DecodeString(encoded)
}

func encodeTextPayload(payload []byte) []string {
	encoded := strings.ToLower(hex.EncodeToString(payload))
	if encoded == "" {
		return []string{""}
	}
	return splitString(encoded, 240)
}

func decodeTextPayload(chunks []string) ([]byte, error) {
	encoded := strings.ToUpper(strings.Join(chunks, ""))
	if encoded == "" {
		return nil, fmt.Errorf("DNS TXT payload is empty")
	}
	return hex.DecodeString(encoded)
}

func maxPayloadMTUForDomain(domain string, config Config) (int, error) {
	config = normalizeConfig(config)
	if _, err := encodeQueryName(make([]byte, frameHeaderSize), domain, config); err != nil {
		return 0, fmt.Errorf("DNS query cannot fit frame header for domain %q: %w", domain, err)
	}
	low, high := 0, config.PayloadMTU
	for low < high {
		mid := (low + high + 1) / 2
		if _, err := encodeQueryName(make([]byte, frameHeaderSize+mid), domain, config); err == nil {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low, nil
}

func splitString(value string, size int) []string {
	if size <= 0 || size >= len(value) {
		if value == "" {
			return nil
		}
		return []string{value}
	}
	out := make([]string, 0, (len(value)+size-1)/size)
	for len(value) > 0 {
		n := size
		if len(value) < n {
			n = len(value)
		}
		out = append(out, value[:n])
		value = value[n:]
	}
	return out
}
