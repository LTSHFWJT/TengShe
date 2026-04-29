package dnstransport

import (
	"os"
	"strings"
)

const defaultFallbackResolver = "127.0.0.1:53"

func defaultResolverAddress() string {
	if value := strings.TrimSpace(os.Getenv("TENGSHE_DNS_RESOLVER")); value != "" {
		if normalized, err := normalizeResolver(value); err == nil {
			return normalized
		}
	}
	for _, candidate := range systemResolverCandidates() {
		normalized, err := normalizeResolver(candidate)
		if err == nil {
			return normalized
		}
	}
	return defaultFallbackResolver
}

func uniqueResolvers(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
