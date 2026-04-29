//go:build !windows

package dnstransport

import (
	"os"
	"strings"
)

func systemResolverCandidates() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	return resolvConfNameservers(string(data))
}

func resolvConfNameservers(data string) []string {
	resolvers := make([]string, 0, 2)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.ToLower(fields[0]) != "nameserver" {
			continue
		}
		resolvers = append(resolvers, fields[1])
	}
	return uniqueResolvers(resolvers)
}
