package options

const (
	WRITE_CHAN_SIZE_C         = 10000
	WRITE_CHAN_SIZE_S         = 100000
	DEFAULT_BUFF_SIZE         = 1 * 1024 * 1024
	DEFAULT_CLIENT_CONN_HEART = 5 * 60 //Second
	NET_RECONNECT_INV         = 5      //Second
	COMPRESS_LIMIT            = 1024
	// MIN_READ_SIZE ensures we can always read the 2-byte frame head length
	// plus the maximum supported head bytes (MAX_HEAD_LEN = 1024 in netmgr).
	MIN_READ_SIZE        = 2 + 1024
	DELAY_WRITE_MS       = 5
	DELAY_SEND_QUEUE_LEN = 5
	MAX_SEND_QUEUE_LEN   = 256
)
