package persist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"game/deps/mongoclient"
	"game/deps/xlog"
	"game/src/proto/pb"
	"time"

	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var LazyLoadIndex = []int{}
var InitLoadStatus = []int{}

func init() {
	InitLoadStatus = make([]int, GamerDataModIndexMax)
	for i := range GamerDataModIndexMax {
		InitLoadStatus[i] = 1
		if lo.Contains(LazyLoadIndex, i) {
			InitLoadStatus[i] = 0
		}
	}
}

func (gd *GamerData) withCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), mongoOpTimeout)
}

func (gd *GamerData) InitLoad() error {
	col := GetGamerDataCollection()
	ctx, cancel := gd.withCtx()
	defer cancel()
	start := time.Now()
	si := col.FindOne(ctx, bson.M{"_id": gd.GamerId})
	xlog.Debugf("[DB] FindOne query, gid:%d, cost:%v query:%+v", gd.GamerId, time.Since(start), bson.M{"_id": gd.GamerId})

	// 检查查询错误
	if err := si.Err(); err != nil {
		xlog.Errorf("[DB] Get GamerData failed, gamerId: %d error: %v", gd.GamerId, err)
		return err
	}

	if err := si.Decode(gd); err != nil {
		xlog.Errorf("[DB] Get GamerData decode failed, gamerId: %d error: %v", gd.GamerId, err)
		return err
	}

	if raw, _ := si.Raw(); len(raw) > 64*1024 || time.Since(start) > 5*time.Millisecond {
		xlog.Warnf("gamer data too large or load slow, gid:%d  len:%d cost time: %v", gd.GamerId, len(raw), time.Since(start))
	}

	// 只对真正解出来的部分置位。若你的 bson:",inline" 能保证全部字段都有默认值，这里可置 true。
	for _, p := range gd.pes {
		if p != nil {
			p.SetLoaded()
		}
	}

	if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
		xlog.Debugf("[DB] Get GamerData success, gamerId: %d, time: %v", gd.GamerId, time.Since(start))
	}

	return nil
}

func (gd *GamerData) LoadModels(modelsIndex ...int) error {
	if len(modelsIndex) == 0 {
		return nil
	}

	// 构建 Projection
	proj := bson.M{"_id": 1}
	for _, v := range modelsIndex {
		proj[IndexToFiledMap[v]] = 1
	}

	col := GetGamerDataCollection()
	ctx, cancel := gd.withCtx()
	defer cancel()

	opt := options.FindOne().SetProjection(proj)
	si := col.FindOne(ctx, bson.M{"_id": gd.GamerId}, opt)
	if err := si.Err(); err != nil {
		return err
	}

	if raw, _ := si.Raw(); len(raw) > 64*1024 {
		xlog.Warnf("gamer data too large, gid:%d  len:%d", gd.GamerId, len(raw))
	}

	if err := si.Decode(gd); err != nil {
		xlog.Errorf("[DB] LoadModels decode failed, gid:%d err:%v", gd.GamerId, err)
		return err
	}

	// 标记已加载
	for _, p := range modelsIndex {
		if gd.pes[p] != nil {
			gd.pes[p].SetLoaded()
		}
	}
	return nil
}

func (gd *GamerData) Save() error {
	col := GetGamerDataCollection()
	ctx, cancel := gd.withCtx()
	defer cancel()

	saveStart := time.Now()
	save := make([]bson.M, 0, GamerDataModIndexMax)
	for _, p := range gd.pes {
		if p == nil {
			continue
		}
		doc := p.SaveDocs()
		if len(doc) == 0 {
			continue
		}
		save = append(save, doc)
	}

	if len(save) > 0 {
		doc, err := mongoclient.MergeBsonDocs(save...)
		if err != nil {
			js, _ := json.Marshal(save)
			xlog.Errorf("merge doc gid:%d failed. err:%v data%s", gd.GamerId, err, js)
			return err
		}
		prepareCost := time.Since(saveStart)
		_, err = col.UpdateByID(ctx, gd.GamerId, doc, options.UpdateOne().SetUpsert(true))
		totalCost := time.Since(saveStart)
		dbCost := totalCost - prepareCost
		if totalCost > 5*time.Millisecond {
			xlog.Warnf("gamer data save slow, gid:%d saveDocsCount:%d mergedDocLen:%d totalCost:%v prepareCost:%v dbCost:%v", gd.GamerId, len(save), len(doc), totalCost, prepareCost, dbCost)
		}
		if err != nil {
			js, _ := json.Marshal(save)
			xlog.Errorf("[DB] save gamer data failed gid:%d saveDocsCount:%d mergedDocLen:%d totalCost:%v prepareCost:%v dbCost:%v err:%v data:%s", gd.GamerId, len(save), len(doc), totalCost, prepareCost, dbCost, err, js)
			return err
		}

		xlog.Debugf("[DB] update gamer data success, gid:%d, saveDocsCount:%d mergedDocLen:%d totalCost:%v prepareCost:%v dbCost:%v", gd.GamerId, len(save), len(doc), totalCost, prepareCost, dbCost)
		xlog.Debugf("gamer data save success, gid:%d ", gd.GamerId)
	}

	return nil
}

func (gd *GamerData) SaveDocs() bson.Raw {
	save := make([]bson.M, 0, GamerDataModIndexMax)
	for _, p := range gd.pes {
		if p == nil {
			continue
		}
		doc := p.SaveDocs()
		if len(doc) == 0 {
			continue
		}
		save = append(save, doc)
	}
	if len(save) == 0 {
		return nil
	}
	doc, err := mongoclient.MergeBsonDocs(save...)

	if err != nil {
		js, _ := json.Marshal(save)
		xlog.Errorf("merge doc gid:%d failed. err:%v data%s", gd.GamerId, err, js)
		return nil
	}
	if len(doc) == 0 {
		return nil
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		xlog.Errorf("[DB] save gamer data failed gid:%d err:%v", gd.GamerId, err)
		return nil
	}

	return bson.Raw(raw)

}

func BatchSaveGamers(gids []int64, raws []bson.Raw) error {
	if len(gids) != len(raws) {
		xlog.Errorf("[DB] BatchSaveGamers failed, gids:%d, raws:%d", len(gids), len(raws))
		return fmt.Errorf("batch save gamers len mismatch gids:%d raws:%d", len(gids), len(raws))
	}

	models := make([]mongo.WriteModel, 0, 100)
	saveErrs := make([]error, 0, 4)
	flush := func() {
		if len(models) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
		defer cancel()
		_, err := GetGamerDataCollection().BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
		if err != nil {
			xlog.Errorf("[DB] BatchSave failed, error: %v", err)
			saveErrs = append(saveErrs, err)
		}
		models = make([]mongo.WriteModel, 0, 100)
	}

	for i, v := range gids {
		if raws[i] == nil || v == 0 {
			continue
		}

		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": v}).
			SetUpdate(raws[i]).
			SetUpsert(true))

		if len(models) >= 100 {
			flush()
		}
	}
	flush()

	return errors.Join(saveErrs...)

}

// BatchLoadGamerPlayerData 批量加载玩家基本信息(PlayerBase)
func BatchLoadGamerPlayerBaseData(gamerIds []int64) (map[int64]*pb.PlayerBase, error) {
	if len(gamerIds) == 0 {
		return make(map[int64]*pb.PlayerBase), nil
	}

	// 构造查询条件
	filter := bson.M{"_id": bson.M{"$in": gamerIds}}
	projection := bson.M{GAMER_PLAYER_MONGO_FIELD: 1, "_id": 1}

	cursor, err := GetSecondaryGamerDataCollection().Find(context.Background(), filter, options.Find().SetProjection(projection))
	if err != nil {
		xlog.Errorf("batch query gamer data failed, err: %v", err)
		return nil, err
	}
	defer func() {
		if err := cursor.Close(context.Background()); err != nil {
			xlog.Errorf("close cursor failed, err: %v", err)
		}
	}()

	// 处理查询结果
	result := make(map[int64]*pb.PlayerBase, len(gamerIds))
	for cursor.Next(context.Background()) {
		var doc struct {
			ID         int64          `bson:"_id"`
			PlayerData *pb.PlayerBase `bson:"player_data"`
		}

		if err := cursor.Decode(&doc); err != nil {
			xlog.Errorf("decode gamer data failed, err: %v", err)
			continue
		}

		result[doc.ID] = doc.PlayerData
	}

	if err := cursor.Err(); err != nil {
		xlog.Errorf("cursor error, err: %v", err)
		return nil, err
	}

	return result, nil
}
