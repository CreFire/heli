package gateuser

import (
	"testing"

	"game/deps/msg"
	"game/deps/xtime"
	"game/src/proto/pb"

	"github.com/sasha-s/go-deadlock"
)

func setClientRequestHistory(user *GateUser, times []int64) {
	user.ClientReqHead = 0
	user.ClientReqCount = len(times)
	for i := range user.ClientReqTimes {
		user.ClientReqTimes[i] = 0
	}
	copy(user.ClientReqTimes[:], times)
}

func disableDeadlockForBenchmark(b *testing.B) {
	oldDisable := deadlock.Opts.Disable
	oldDisableLockOrderDetection := deadlock.Opts.DisableLockOrderDetection
	deadlock.Opts.Disable = true
	deadlock.Opts.DisableLockOrderDetection = true
	b.Cleanup(func() {
		deadlock.Opts.Disable = oldDisable
		deadlock.Opts.DisableLockOrderDetection = oldDisableLockOrderDetection
	})
}

func TestGateUserMgrClearSessBySessNoUser(t *testing.T) {
	mgr := NewGateUserMgr()

	if mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected clear sess to fail when user is missing")
	}
}

func TestGateUserMgrReplaceSessBySessNoUser(t *testing.T) {
	mgr := NewGateUserMgr()

	if mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected replace sess to fail when user is missing")
	}
}

func TestGateUserMgrReplaceSessBySessSessMismatch(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if mgr.ReplaceSess(1001, 3003, 4004) {
		t.Fatalf("expected replace sess to fail on session mismatch")
	}
	if got := user.GetSessId(); got != 2002 {
		t.Fatalf("expected session to remain unchanged, got %d", got)
	}
}

func TestGateUserMgrClearSessBySessSessMismatch(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if mgr.ClearSessBySess(1001, 3003) {
		t.Fatalf("expected clear sess to fail on session mismatch")
	}
	if got := user.GetSessId(); got != 2002 {
		t.Fatalf("expected session to remain unchanged, got %d", got)
	}
}

func TestGateUserMgrClearSessBySess(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected clear sess to succeed")
	}
	if got := user.GetSessId(); got != 0 {
		t.Fatalf("expected sess id to be cleared, got %d", got)
	}
	if _, ok := mgr.Get(1001); !ok {
		t.Fatalf("expected gate user to remain cached")
	}
}

func TestGateUserMgrClearSessBySessIdempotentAfterAlreadyOffline(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected initial clear sess to succeed")
	}
	if mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected repeated clear with old session to fail once offline")
	}
	if got := user.GetSessId(); got != 0 {
		t.Fatalf("expected session to stay cleared, got %d", got)
	}
}

func TestGateUserMgrDelBySessNoUser(t *testing.T) {
	mgr := NewGateUserMgr()

	if mgr.DelBySess(1001, 2002) {
		t.Fatalf("expected delete to fail when user is missing")
	}
}

func TestGateUserMgrDelBySessSessMismatchKeepsUser(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if mgr.DelBySess(1001, 3003) {
		t.Fatalf("expected delete to fail on session mismatch")
	}
	if got := user.GetSessId(); got != 2002 {
		t.Fatalf("expected session to remain unchanged, got %d", got)
	}
	if _, ok := mgr.Get(1001); !ok {
		t.Fatalf("expected gate user to remain cached")
	}
}

func TestGateUserMgrDelOfflineAfterDisconnect(t *testing.T) {
	mgr := NewGateUserMgr()
	mgr.AddGateUser(1001, 2002, true)

	if !mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected disconnect clear to succeed")
	}
	if !mgr.DelOffline(1001) {
		t.Fatalf("expected offline user to be deleted")
	}
	if _, ok := mgr.Get(1001); ok {
		t.Fatalf("expected gate user to be deleted")
	}
}

func TestGateUserMgrDelOfflineKeepsOnlineUser(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected disconnect clear to succeed")
	}
	if !mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected reconnect replace to succeed")
	}

	if mgr.DelOffline(1001) {
		t.Fatalf("expected offline delete to ignore reconnected user")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected new session to remain, got %d", got)
	}
	if _, ok := mgr.Get(1001); !ok {
		t.Fatalf("expected gate user to remain cached")
	}
}

func TestGateUserMgrReplaceSessBySessReplacesMatchingSession(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected matching session to be replaced")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected session to switch to new session, got %d", got)
	}
}

func TestGateUserMgrReplaceSessBySessAllowsReconnectAfterOffline(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected disconnect clear to succeed")
	}
	if !mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected offline session to be replaced by reconnect")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected reconnect to restore session, got %d", got)
	}
}

func TestGateUserMgrReplaceSessBySessRejectsNewerSession(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected initial session replace to succeed")
	}
	if mgr.ReplaceSess(1001, 2002, 4004) {
		t.Fatalf("expected stale session replace to be rejected")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected current session to remain, got %d", got)
	}
}

func TestGateUserMgrDelBySessAfterReconnect(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected disconnect clear to succeed")
	}

	if !mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected reconnect replace to succeed")
	}

	if mgr.DelBySess(1001, 2002) {
		t.Fatalf("expected old session logout to be ignored after reconnect")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected new session to remain, got %d", got)
	}
	if !mgr.DelBySess(1001, 3003) {
		t.Fatalf("expected current session logout to delete gate user")
	}
	if _, ok := mgr.Get(1001); ok {
		t.Fatalf("expected gate user to be deleted")
	}
}

func TestGateUserMgrAddGateUserForceInitPreservesPendingLogoutReason(t *testing.T) {
	mgr := NewGateUserMgr()
	mgr.AddGateUser(1001, 2002, true)
	mgr.SetLogoutReason(1001, 2002, "new_device_login")

	mgr.AddGateUser(1001, 3003, true)

	if got := mgr.TakeLogoutReason(1001, 2002); got != "new_device_login" {
		t.Fatalf("expected old session logout reason to survive force init, got %q", got)
	}
}

func TestGateUserMgrDeletingGateUserDropsPendingLogoutReason(t *testing.T) {
	mgr := NewGateUserMgr()
	mgr.AddGateUser(1001, 2002, true)
	mgr.SetLogoutReason(1001, 2002, "new_device_login")
	mgr.AddGateUser(1001, 3003, true)

	if !mgr.DelBySess(1001, 3003) {
		t.Fatalf("expected current session delete to succeed")
	}
	if got := mgr.TakeLogoutReason(1001, 2002); got != "" {
		t.Fatalf("expected pending logout reason to be removed with gate user, got %q", got)
	}
}

func TestGateUserMgrClearSessBySessAfterReplaceSess(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected replace sess to succeed")
	}
	if mgr.ClearSessBySess(1001, 2002) {
		t.Fatalf("expected stale disconnect clear to fail after replace")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected current session to remain, got %d", got)
	}
}

func TestGateUserMgrDelOfflineAfterReplaceSess(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	if !mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected replace sess to succeed")
	}
	if mgr.DelOffline(1001) {
		t.Fatalf("expected offline delete to ignore replaced online session")
	}
	if got := user.GetSessId(); got != 3003 {
		t.Fatalf("expected current session to remain, got %d", got)
	}
	if _, ok := mgr.Get(1001); !ok {
		t.Fatalf("expected gate user to remain cached")
	}
}

func TestGateUserMgrCheckAndUpdateSeqAckAndLimit(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	limited, err := mgr.CheckAndUpdateSeqAckAndLimit(1001, 2002, 0, 0)
	if err == nil {
		t.Fatalf("expected invalid seq to fail before rate limit")
	}
	if limited {
		t.Fatalf("expected invalid seq to skip rate limit result")
	}

	now := xtime.NowUnixMs()
	history := make([]int64, clientBurstLimit)
	for i := range history {
		history[i] = now - clientBurstWindowMs
	}
	setClientRequestHistory(user, history)

	for seq := uint16(1); seq <= clientRequestLimit-clientBurstLimit; seq++ {
		limited, err = mgr.CheckAndUpdateSeqAckAndLimit(1001, 2002, seq, 0)
		if err != nil {
			t.Fatalf("expected seq %d to pass, err=%v", seq, err)
		}
		if limited {
			t.Fatalf("expected seq %d to stay under limit", seq)
		}
	}

	limited, err = mgr.CheckAndUpdateSeqAckAndLimit(1001, 2002, clientRequestLimit+1, 0)
	if err != nil {
		t.Fatalf("expected over-limit request to keep seq valid, err=%v", err)
	}
	if !limited {
		t.Fatalf("expected request over limit to be rate limited")
	}
}

func TestGateUserMgrCheckAndUpdateSeqAckAndLimitIgnoresOldSessionAfterReconnect(t *testing.T) {
	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)
	if !mgr.ReplaceSess(1001, 2002, 3003) {
		t.Fatalf("expected reconnect replace to succeed")
	}

	limited, err := mgr.CheckAndUpdateSeqAckAndLimit(1001, 2002, 1, 0)
	if err == nil {
		t.Fatalf("expected old session update to fail")
	}
	if limited {
		t.Fatalf("expected old session update to skip rate-limit result")
	}
	if got := user.Ack; got != 0 {
		t.Fatalf("expected ack to stay unchanged, got %d", got)
	}

	limited, err = mgr.CheckAndUpdateSeqAckAndLimit(1001, 3003, 1, 0)
	if err != nil {
		t.Fatalf("expected current session update to pass, err=%v", err)
	}
	if limited {
		t.Fatalf("expected current session update to stay under limit")
	}
	if got := user.Ack; got != 1 {
		t.Fatalf("expected ack to update for current session, got %d", got)
	}
}

func TestGateUserAddSendMessageSeqAckIgnoresOldSessionAfterReconnect(t *testing.T) {
	user := NewGateUser(1001, 2002)
	user.SetSessId(3003)
	data := msg.NewMsg(pb.MSG_ID_HEART_RSP, nil)

	if user.AddSendMessageSeqAck(2002, data) {
		t.Fatalf("expected old session send to be ignored")
	}
	if got := user.Seq; got != 0 {
		t.Fatalf("expected seq to remain unchanged, got %d", got)
	}
	if got := user.GetAllSendMessage().Len(); got != 0 {
		t.Fatalf("expected send queue to stay empty, got %d", got)
	}

	if !user.AddSendMessageSeqAck(3003, data) {
		t.Fatalf("expected current session send to succeed")
	}
	if got := user.Seq; got != 1 {
		t.Fatalf("expected seq to advance for current session, got %d", got)
	}
	if got := user.GetAllSendMessage().Len(); got != 1 {
		t.Fatalf("expected send queue to contain current session message, got %d", got)
	}
}

func TestGateUserCanReconnectRejectsAfterTokenUseLimit(t *testing.T) {
	user := NewGateUser(1001, 2002)
	xtime.SetGmAdd(0)
	t.Cleanup(func() {
		xtime.SetGmAdd(0)
	})

	for i := range int64(tokenUsedMaxTimes) {
		xtime.SetGmAdd(i * (reconnectShortWindowSec + 1))
		if !user.CanReconnect() {
			t.Fatalf("expected reconnect %d to pass", i+1)
		}
	}

	xtime.SetGmAdd(tokenUsedMaxTimes * (reconnectShortWindowSec + 1))
	if user.CanReconnect() {
		t.Fatalf("expected reconnect over token use limit to fail")
	}
}

func TestGateUserCanReconnectRejectsBurstWithinShortWindowAndRecovers(t *testing.T) {
	user := NewGateUser(1001, 2002)
	xtime.SetGmAdd(0)
	t.Cleanup(func() {
		xtime.SetGmAdd(0)
	})

	for i := range int64(reconnectShortWindowMax - 1) {
		if !user.CanReconnect() {
			t.Fatalf("expected reconnect %d within short window to pass", i+1)
		}
	}
	if user.CanReconnect() {
		t.Fatalf("expected reconnect over short-window limit to fail")
	}

	xtime.SetGmAdd(reconnectShortWindowSec + 1)
	if !user.CanReconnect() {
		t.Fatalf("expected reconnect after short window to recover")
	}
}

func TestGateUserTryAcceptClientRequestWindowLimit(t *testing.T) {
	user := NewGateUser(1001, 2002)

	for i := range clientBurstLimit {
		if !user.TryAcceptClientRequest() {
			t.Fatalf("expected burst warmup request %d to pass", i)
		}
	}
	if user.TryAcceptClientRequest() {
		t.Fatalf("expected request over burst limit to be rejected")
	}

	now := xtime.NowUnixMs()
	windowHistory := make([]int64, clientRequestLimit)
	for i := range windowHistory {
		windowHistory[i] = now - clientBurstWindowMs
	}
	setClientRequestHistory(user, windowHistory)
	if user.TryAcceptClientRequest() {
		t.Fatalf("expected request over 5s window limit to be rejected")
	}

	expiredHistory := make([]int64, clientRequestLimit)
	for i := range expiredHistory {
		expiredHistory[i] = now - clientRequestWindowMs
	}
	setClientRequestHistory(user, expiredHistory)
	if !user.TryAcceptClientRequest() {
		t.Fatalf("expected request after 5s window to pass again")
	}
}

func BenchmarkGateUserTryAcceptClientRequestAllow(b *testing.B) {
	disableDeadlockForBenchmark(b)

	user := NewGateUser(1001, 2002)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		setClientRequestHistory(user, []int64{xtime.NowUnixMs() - clientRequestWindowMs})
		if !user.TryAcceptClientRequest() {
			b.Fatalf("expected request to be allowed at iteration %d", i)
		}
	}
}

func BenchmarkGateUserTryAcceptClientRequestReject(b *testing.B) {
	disableDeadlockForBenchmark(b)

	user := NewGateUser(1001, 2002)
	now := xtime.NowUnixMs()
	history := make([]int64, clientBurstLimit)
	for i := range history {
		history[i] = now
	}
	setClientRequestHistory(user, history)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if user.TryAcceptClientRequest() {
			b.Fatalf("expected request to be rejected at iteration %d", i)
		}
	}
}

func BenchmarkGateUserMgrCheckAndUpdateSeqAckAndLimit(b *testing.B) {
	disableDeadlockForBenchmark(b)

	mgr := NewGateUserMgr()
	user := mgr.AddGateUser(1001, 2002, true)

	var seq uint16
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq++
		history := make([]int64, clientBurstLimit)
		now := xtime.NowUnixMs()
		for j := range history {
			history[j] = now - clientBurstWindowMs
		}
		setClientRequestHistory(user, history)
		limited, err := mgr.CheckAndUpdateSeqAckAndLimit(1001, 2002, seq, 0)
		if err != nil {
			b.Fatalf("expected combined check to pass, err=%v", err)
		}
		if limited {
			b.Fatalf("expected combined check to stay under limit at iteration %d", i)
		}
	}
}
