package controller

type IState interface {
	Name() string
	OnEnter(*Robot) error
	onLeave(*Robot)
	onUpdate(*Robot)
	register(*Robot)
}

const (
	STATE_NULL   = "NULL"
	STATE_AUTH   = "AUTH"
	STATE_LOGIN  = "LOGIN"
	STATE_STRESS = "STRESS" // 压测
	STATE_HEART  = "HEART"
	STATE_GM     = "GM"
	STATE_BATTLE = "BATTLE"
	STATE_PACK   = "pack"
)
