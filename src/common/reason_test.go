package common

import "testing"

func TestWithReasonMetaKVCloneAndKeepBase(t *testing.T) {
	base := ReasonTask
	r := WithReasonMetaKV(base, "action", "claim")
	if r == base {
		t.Fatalf("expect cloned reason")
	}
	if r.Id != base.Id || r.Str != base.Str {
		t.Fatalf("unexpected base fields: got=%+v base=%+v", r, base)
	}
	if r.MetaStr != `{"action":"claim"}` {
		t.Fatalf("unexpected metaStr: %s", r.MetaStr)
	}
	if base.MetaStr != "" {
		t.Fatalf("base metaStr should keep empty, got=%s", base.MetaStr)
	}
}

func TestWithReasonMetaReplacesExistingMeta(t *testing.T) {
	base := &Reason{Id: 5, Str: "任务领奖", MetaStr: `{"scope":"rpc"}`}
	r := WithReasonMeta(base, map[string]any{"scope": "logic", "step": int64(2)})
	if r.MetaStr != `{"scope":"logic","step":2}` {
		t.Fatalf("unexpected metaStr: %s", r.MetaStr)
	}
}

func TestReasonMetaInt64(t *testing.T) {
	r := WithReasonMeta(ReasonUnknown, map[string]any{"cost_item_id": int64(7001), "auto_use": true})
	v, ok := ReasonMetaInt64(r, "cost_item_id")
	if !ok || v != 7001 {
		t.Fatalf("unexpected int value: %v ok=%v", v, ok)
	}
}

func TestReasonMetaString(t *testing.T) {
	r := WithReasonMeta(ReasonUnknown, map[string]any{"reset": "daily"})
	v, ok := ReasonMetaString(r, "reset")
	if !ok || v != "daily" {
		t.Fatalf("unexpected string value: %q ok=%v", v, ok)
	}
}

func TestBuildReason(t *testing.T) {
	r := BuildReason(ReasonTask.Id, "", `{"gid":1}`)
	if r.Id != ReasonTask.Id {
		t.Fatalf("unexpected reason id: %d", r.Id)
	}
	if r.Str != ReasonTask.Str {
		t.Fatalf("unexpected reason str: %s", r.Str)
	}
	if r.MetaStr != `{"gid":1}` {
		t.Fatalf("unexpected metaStr: %s", r.MetaStr)
	}
}

func TestBuildReasonWrapsLegacyPlainTextAsDetailMeta(t *testing.T) {
	r := BuildReason(ReasonMailPick.Id, "", "pick mails: [1001 1002]")
	meta := ReasonMeta(r)
	if meta["detail"] != "pick mails: [1001 1002]" {
		t.Fatalf("unexpected detail meta: %#v", meta)
	}
}

func TestReasonIDMeta(t *testing.T) {
	id, metaStr := ReasonTask.IDMeta()
	if id != ReasonTask.Id {
		t.Fatalf("unexpected proto id: %d", id)
	}
	if metaStr != "" {
		t.Fatalf("unexpected proto metaStr: %s", metaStr)
	}

	id, metaStr = WithReasonMetaKV(ReasonTask, "reset", "daily").IDMeta()
	if id != ReasonTask.Id {
		t.Fatalf("unexpected proto id with meta: %d", id)
	}
	if metaStr != `{"reset":"daily"}` {
		t.Fatalf("unexpected proto metaStr with meta: %s", metaStr)
	}

	var nilReason *Reason
	id, metaStr = nilReason.IDMeta()
	if id != 0 || metaStr != "" {
		t.Fatalf("unexpected nil proto fields: id=%d metaStr=%q", id, metaStr)
	}
}
