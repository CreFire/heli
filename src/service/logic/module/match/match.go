package match

import matchbiz "game/src/service/logic/module/matchbiz"

type MatchData = matchbiz.MatchData

func NewMatchData() *MatchData { return matchbiz.NewMatchData() }
