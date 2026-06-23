package persist

import (
	"game/deps/mongoclient"
	"game/src/proto/pb"
	"slices"

	"github.com/samber/lo"
	"github.com/sasha-s/go-deadlock"
)

const (
	GLOBAL_MAIL_DATA_ID        int64 = 1
	WORLD_BOSS_RANK_DATA_ID    int64 = 3
	REALM_RANK_DATA_ID         int64 = 4
	GLOBAL_MAIL_DATA_FILED           = "global_mail_data"
	WORLD_BOSS_RANK_DATA_FILED       = "world_boss_rank_data"
	REALM_RANK_DATA_FILED            = "realm_rank_data"
)

type PublicGlobalData struct {
	GlobalMail *mongoclient.DataPersister[*pb.GlobalMailData] `bson:"global_mail_data"`
	MailIds    []int64                                        `bson:"-"`
	RWMutex    *deadlock.RWMutex                              `bson:"-"`
}

func NewPublicGlobalData() *PublicGlobalData {
	return &PublicGlobalData{
		GlobalMail: mongoclient.NewDataPersister(
			newGlobalMailData(),
			GLOBAL_MAIL_DATA_FILED,
			GetPublicDataCollection(),
			GLOBAL_MAIL_DATA_ID,
		),

		RWMutex: &deadlock.RWMutex{},
	}
}

func newGlobalMailData() *pb.GlobalMailData {
	return &pb.GlobalMailData{MailData: make(map[int64]*pb.GlobalMail)}
}

func LoadPublicGlobalData() (*PublicGlobalData, error) {
	pd := NewPublicGlobalData()
	if err := pd.GlobalMail.Load(); err != nil {
		return nil, err
	}
	pd.MailIds = lo.Keys(pd.GlobalMail.Data().MailData)
	slices.Sort(pd.MailIds)
	return pd, nil
}
