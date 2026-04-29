//go:build windows

package dnstransport

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

func systemResolverCandidates() []string {
	resolvers := make([]string, 0, 4)
	resolvers = append(resolvers, registryResolverCandidates(`SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`)...)
	resolvers = append(resolvers, registryResolverCandidates(`SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces`)...)
	return uniqueResolvers(resolvers)
}

func registryResolverCandidates(path string) []string {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
	if err != nil {
		return nil
	}
	defer root.Close()

	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	resolvers := make([]string, 0, len(names))
	for _, name := range names {
		key, err := registry.OpenKey(root, name, registry.READ)
		if err != nil {
			continue
		}
		resolvers = append(resolvers, registryStringResolvers(key, "NameServer")...)
		resolvers = append(resolvers, registryStringResolvers(key, "DhcpNameServer")...)
		key.Close()
	}
	return resolvers
}

func registryStringResolvers(key registry.Key, name string) []string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return nil
	}
	return splitWindowsResolverList(value)
}

func splitWindowsResolverList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ' ' || r == ',' || r == ';'
	})
}
