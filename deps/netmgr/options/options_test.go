package options

import (
	"game/src/configdoc"
	"testing"
)

func TestSetNetCfgSetsTimeout(t *testing.T) {
	opt := NewMsgQueOptions()
	opt.SetListenParams(NewListenParams("127.0.0.1:0"))
	cfg := &configdoc.Net{
		MaxConn:            321,
		CltReadTimeout:     300,
		CltReadBufferSize:  4096,
		CltWriteBufferSize: 8192,
		CltWriteChanSize:   128,
		CltEnableDH:        false,
		Compress:           true,
		CompressMode:       1,
		CompressLimit:      1024,
		DelayWrite:         5,
	}

	opt.SetNetCfg(cfg)

	if opt.Timeout != cfg.CltReadTimeout {
		t.Fatalf("timeout not applied, got=%d want=%d", opt.Timeout, cfg.CltReadTimeout)
	}
	if opt.ListenParams.MaxConn != int(cfg.MaxConn) {
		t.Fatalf("max conn not applied, got=%d want=%d", opt.ListenParams.MaxConn, cfg.MaxConn)
	}
}

func TestSetIsGate(t *testing.T) {
	opt := NewMsgQueOptions()

	opt.SetIsGate(true)

	if !opt.IsGate {
		t.Fatalf("isGate not applied")
	}
}

func TestSetNetCfgNilNoChange(t *testing.T) {
	opt := NewMsgQueOptions()
	opt.Timeout = 123

	opt.SetNetCfg(nil)

	if opt.Timeout != 123 {
		t.Fatalf("timeout should remain unchanged, got=%d want=%d", opt.Timeout, 123)
	}
}

func TestNewMsgQueOptionsDefaultsToTCP(t *testing.T) {
	opt := NewMsgQueOptions()

	if opt.Transport != TransportTCP {
		t.Fatalf("unexpected default transport: %v", opt.Transport)
	}
	if opt.WSPath != DefaultWSPath {
		t.Fatalf("unexpected default websocket path: %s", opt.WSPath)
	}
}

func TestKCPOptionsDoNotRequireWebSocketPath(t *testing.T) {
	opt := NewMsgQueOptions()
	opt.Transport = TransportKCP
	opt.WSPath = ""
	opt.SetListenParams(NewListenParams("127.0.0.1:0"))

	if err := opt.CheckOptions(); err != nil {
		t.Fatalf("kcp options should not require websocket path: %v", err)
	}
}

func TestMergeOptionsDoesNotReloadTransport(t *testing.T) {
	cur := NewMsgQueOptions()
	cur.Transport = TransportWebSocket
	cur.WSPath = "/ws"

	next := NewMsgQueOptions()
	next.Transport = TransportTCP
	next.WSPath = "/other"
	next.Timeout = 77

	merged := MergeOptions(cur, next)

	if merged.Transport != TransportWebSocket {
		t.Fatalf("transport should not be reloadable, got=%v", merged.Transport)
	}
	if merged.WSPath != "/ws" {
		t.Fatalf("websocket path should not be reloadable, got=%s", merged.WSPath)
	}
	if merged.Timeout != 77 {
		t.Fatalf("reloadable timeout not applied, got=%d", merged.Timeout)
	}
}
