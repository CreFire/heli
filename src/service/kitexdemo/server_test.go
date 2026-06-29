package kitexdemo

import "testing"

func TestEchoImplEcho(t *testing.T) {
	impl := &EchoImpl{}
	resp, err := impl.Echo(nil, nil)
	if err != nil {
		t.Fatalf("Echo returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("Echo returned nil response")
	}
	if resp.Message != "" {
		t.Fatalf("unexpected response message: %q", resp.Message)
	}
}
