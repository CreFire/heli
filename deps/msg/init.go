package msg

import (
	_ "game/src/proto/pb"
)

var PbParser *PBParser

func init() {
	PbParser = NewPBParser()
}
