package etcd

import (
	"context"
	"errors"
	"fmt"
	"game/deps/xlog"
	"net/url"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap/zapcore"
)

const (
	defaultDialTimeout    = 10 * time.Second
	defaultConnectTimeout = 10 * time.Second
	defaultPingTimeout    = 5 * time.Second
)

type EtcdClient struct {
	Config *clientv3.Config
	Client *clientv3.Client
}

func NewEtcdClient(dsn string, logger *xlog.MyLogger) (*EtcdClient, error) {
	cfg, err := ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse etcd DSN: %w", err)
	}

	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = defaultDialTimeout
	}
	cfg.Logger = xlog.ZapLogger(logger, zapcore.WarnLevel, 0)

	client, err := clientv3.New(*cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	ec := &EtcdClient{Config: cfg, Client: client}
	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
	err = client.Sync(ctx)
	cancel()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("etcd sync failed: %w", err)
	}

	if err := ec.Ping(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("etcd ping failed: %w", err)
	}

	return ec, nil
}

// ParseDSN parses an etcd DSN.
// Examples:
//   - etcd://127.0.0.1:2379
//   - etcd://10.0.0.1:2379,10.0.0.2:2379,10.0.0.3:2379
//   - etcd://user:pass@etcd.example.com:2379,etcd2.example.com:2379?dialTimeout=5s
func ParseDSN(dsn string) (*clientv3.Config, error) {
	if dsn == "" {
		return nil, errors.New("etcd DSN cannot be empty")
	}

	scheme, rest, ok := strings.Cut(dsn, "://")
	if !ok || rest == "" {
		return nil, fmt.Errorf("failed to parse etcd DSN: missing scheme or authority")
	}

	cfg := &clientv3.Config{}
	switch scheme {
	case "etcd":
	default:
		return nil, fmt.Errorf("unsupported etcd DSN scheme: %s", scheme)
	}

	authority, rawQuery, _ := strings.Cut(rest, "?")
	if authority == "" {
		return nil, errors.New("etcd DSN must contain at least one endpoint")
	}

	if at := strings.LastIndex(authority, "@"); at >= 0 {
		userInfo := authority[:at]
		authority = authority[at+1:]

		username, password, hasPassword := strings.Cut(userInfo, ":")
		if username == "" {
			return nil, errors.New("etcd DSN username cannot be empty when credentials are provided")
		}
		cfg.Username = username
		if hasPassword {
			cfg.Password = password
		}
	}

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to parse etcd DSN query: %w", err)
	}
	if len(query["addr"]) != 0 {
		return nil, errors.New("etcd DSN addr query parameter is no longer supported")
	}

	cfg.Endpoints = parseEndpoints(authority)
	if len(cfg.Endpoints) == 0 {
		return nil, errors.New("etcd DSN must contain at least one endpoint")
	}

	if timeoutStr := query.Get("dialTimeout"); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid dialTimeout format in DSN: %w", err)
		}
		cfg.DialTimeout = timeout
	}

	return cfg, nil
}

func parseEndpoints(authority string) []string {
	endpoints := strings.Split(authority, ",")
	cleaned := endpoints[:0]
	for _, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint != "" {
			cleaned = append(cleaned, endpoint)
		}
	}
	return cleaned
}

func (ec *EtcdClient) Close() {
	ec.Client.Close()
}

func (ec *EtcdClient) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	_, err := ec.Client.Get(ctx, "_etcd_ping_test_key_")
	if err != nil && !strings.Contains(err.Error(), "requested key not found") {
		return fmt.Errorf("etcd ping failed: %w", err)
	}
	return nil
}
