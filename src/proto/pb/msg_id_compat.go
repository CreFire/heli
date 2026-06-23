package pb

// MSG_ID keeps compatibility with older code paths that address protocol messages
// by one raw uint32 id. WebSocket frames still split this value as:
// mainCmdID = low 16 bits, subCmdID = high 16 bits.
type MSG_ID int32

const (
	MSG_ID_NONE MSG_ID = 0

	// MVP client protocol ids from README.md.
	MSG_ID_LOGIN_REQ     MSG_ID = 1001
	MSG_ID_LOGIN_RESP    MSG_ID = 1002
	MSG_ID_MATCH_REQ     MSG_ID = 1101
	MSG_ID_MATCH_RESP    MSG_ID = 1102
	MSG_ID_PLAYER_OP     MSG_ID = 1201
	MSG_ID_GAME_SNAPSHOT MSG_ID = 1202
	MSG_ID_HEARTBEAT     MSG_ID = 9001
)

var MSG_ID_name = map[int32]string{
	0:    "MSG_ID_NONE",
	1001: "MSG_ID_LOGIN_REQ",
	1002: "MSG_ID_LOGIN_RESP",
	1101: "MSG_ID_MATCH_REQ",
	1102: "MSG_ID_MATCH_RESP",
	1201: "MSG_ID_PLAYER_OP",
	1202: "MSG_ID_GAME_SNAPSHOT",
	9001: "MSG_ID_HEARTBEAT",
}

var MSG_ID_value = map[string]int32{
	"MSG_ID_NONE":          0,
	"MSG_ID_LOGIN_REQ":     1001,
	"MSG_ID_LOGIN_RESP":    1002,
	"MSG_ID_MATCH_REQ":     1101,
	"MSG_ID_MATCH_RESP":    1102,
	"MSG_ID_PLAYER_OP":     1201,
	"MSG_ID_GAME_SNAPSHOT": 1202,
	"MSG_ID_HEARTBEAT":     9001,
}
