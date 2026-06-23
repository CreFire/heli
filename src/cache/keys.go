package cache

import (
	"fmt"
)

func AccountSessionKey(account string) string {
	return fmt.Sprintf("session:%s", account)
}

func ServerInfoKey(serviceName string, instId int32) string {
	return fmt.Sprintf("server:%s:%d", serviceName, instId)
}

func ServerOnlineKey(serviceName string) string {
	return fmt.Sprintf("server:%s:online", serviceName)
}

func ServerBattleLoadKey(serviceName string) string {
	return fmt.Sprintf("server:%s:load", serviceName)
}

func GamerOnlineKey(gamerId int64) string {
	return fmt.Sprintf("gamer:%d", gamerId)
}

// -------------------------redis lock key--------------------------------
func AccountLoginLockKey(account string) string {
	return fmt.Sprintf("account_login:%s:lock", account)
}

func GamerLoginLockKey(gamerId int64) string {
	return fmt.Sprintf("gamer_login:%d:lock", gamerId)
}

// -------------------------redis login queue key------------------------
func LoginQueueKey() string {
	return "login_queue"
}

// -------------------------redis red flag key------------------------
func GamerRedFlagKey(gamerId int64) string {
	return fmt.Sprintf("gamer:%d:redflag", gamerId)
}
