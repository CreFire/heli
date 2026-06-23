package etcd

import (
	"errors"
	"fmt"
	"game/deps/xlog"
	"strconv"
	"strings"
)

func RegistrarKey(cluster, serverType string, serverId int32) string {
	return fmt.Sprintf("/%s/%s/%d", cluster, serverType, serverId)
}

// RegistrarPrefix returns the etcd prefix for instance keys: /<cluster>/<service>/.
func RegistrarPrefix(cluster, serverType string) string {
	return fmt.Sprintf("/%s/%s/", cluster, serverType)
}

func ParseServerInfoFromKey(key string) (cluster, serverType string, serverId int) {
	parts := strings.Split(strings.Trim(key, "/"), "/")
	if len(parts) != 3 {
		return "", "", 0
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		xlog.Warnf("parse serverInfo from etcd key : %s", key)
		return "", "", 0
	}
	return parts[0], parts[1], id
}

// ParseDiscoveryTarget parses a discovery path in the form /<cluster>/<service>.
// Example:
//   - /prod/logic
func ParseDiscoveryTarget(path string) (cluster string, serviceName string, err error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return "", "", errors.New("discovery target path is empty")
	}

	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return "", "", errors.New("discovery target path must be /<cluster>/<service>")
	}

	cluster = strings.TrimSpace(parts[0])
	serviceName = strings.TrimSpace(parts[1])
	if cluster == "" {
		return "", "", errors.New("discovery target cluster is empty")
	}
	if serviceName == "" {
		return "", "", errors.New("discovery target service name is empty")
	}
	return cluster, serviceName, nil
}

func servicePrefix(cluster, serviceName string) (string, error) {
	cluster = strings.TrimSpace(cluster)
	if cluster == "" {
		return "", errors.New("cluster name is empty")
	}
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return "", errors.New("service name is empty")
	}
	if strings.Contains(serviceName, "/") {
		return "", errors.New("service name cannot contain '/'")
	}
	return RegistrarPrefix(cluster, serviceName), nil
}
