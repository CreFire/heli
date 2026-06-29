package kitex

import (
	"fmt"
	"strings"

	"github.com/cloudwego/kitex/pkg/discovery"
	"github.com/cloudwego/kitex/pkg/registry"
	registryetcd "github.com/kitex-contrib/registry-etcd"
)

func normalizeEtcdEndpoint(dsn string) string {
	endpoint := strings.TrimSpace(dsn)
	endpoint = strings.TrimPrefix(endpoint, "etcd://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	return endpoint
}

func NewEtcdRegistry(dsn string) (registry.Registry, error) {
	endpoint := normalizeEtcdEndpoint(dsn)
	if endpoint == "" {
		return nil, fmt.Errorf("empty etcd dsn")
	}
	return registryetcd.NewEtcdRegistry([]string{endpoint})
}

func NewEtcdResolver(dsn string) (discovery.Resolver, error) {
	endpoint := normalizeEtcdEndpoint(dsn)
	if endpoint == "" {
		return nil, fmt.Errorf("empty etcd dsn")
	}
	return registryetcd.NewEtcdResolver([]string{endpoint})
}
