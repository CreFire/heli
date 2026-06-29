package item

import (
	"game/src/common"
	"game/src/configdoc"
	"game/src/persist"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/gamedata/base"
	"game/src/service/logic/iface"

	"github.com/samber/lo"
)

func init() {
	gamedata.RegisterMod(persist.GamerPackModIndex, func(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) base.IGamerModBase {
		return NewGamerItemPack(modIndex, data, doc, docExt)
	})
}

func GetPackModel(m *gamedata.GamerModel) *GamerPack {
	return gamedata.GetGamerModel[*GamerPack](m, persist.GamerPackModIndex)
}

type ItemModule struct {
	ctx   iface.IGamerContext
	model *gamedata.GamerModel
}

func NewItemModule(ctx iface.IGamerContext, model *gamedata.GamerModel) *ItemModule {
	return &ItemModule{
		ctx:   ctx,
		model: model,
	}
}

func (s *ItemModule) SendPackInfo() {
	s2c := &pb.PackInfoNTF{
		All:   true,
		Items: GetPackModel(s.model).PackMap(),
	}
	s.ctx.SendMsg(s2c)
}

func (s *ItemModule) LegacyLoadBag() map[string]interface{} {
	items := make(map[int32]int64, len(GetPackModel(s.model).PackMap()))
	for id, item := range GetPackModel(s.model).PackMap() {
		if item != nil {
			items[id] = item.Num
		}
	}
	return map[string]interface{}{"bag": items, "itemMap": items}
}

func (s *ItemModule) AddItems(items []*pb.Item, reason *common.Reason) ([]*pb.Item, errorpb.ERROR) {
	if len(items) == 0 {
		return nil, errorpb.ERROR_REQUEST_PARAMS
	}
	result := newItemChangeResult(len(items))
	pack := GetPackModel(s.model)
	for _, item := range configdoc.ItemMerge(items) {
		if item == nil || item.ConfId <= 0 || item.Num <= 0 {
			return nil, errorpb.ERROR_REQUEST_PARAMS
		}
		change, code := pack.AddItem(item.ConfId, item.Num)
		if code != errorpb.ERROR_SUCCESS {
			return result.retItems, code
		}
		result.addNtf(change)
		result.addRet(change)
	}
	result.SyncItemNtf(s.ctx)
	return result.retItems, errorpb.ERROR_SUCCESS
}

func (s *ItemModule) UseItem(item *pb.Item) []*pb.Item {
	if item == nil {
		return nil
	}
	changed, code := GetPackModel(s.model).SubItems([]*pb.Item{item})
	if code != errorpb.ERROR_SUCCESS {
		return nil
	}
	newItemChangeResult(len(changed)).addRet(changed...).SyncItemNtf(s.ctx)
	return changed
}

func (s *ItemModule) GetAfkRewardsWithTransItem(transItems []*pb.Item) []*pb.Item {
	return GetPackModel(s.model).GetAfkRewardsWithTransItem(transItems)
}
func (s *ItemModule) DelAllItem() errorpb.ERROR {
	if s.model == nil {
		s.ctx.Logger().Error("model is nil in DelAllItem,")
		return errorpb.ERROR_FAILED
	}
	packModel := GetPackModel(s.model)
	if packModel == nil {
		s.ctx.Logger().Error("pack model is nil in DelAllItem,")
		return errorpb.ERROR_FAILED
	}

	data := packModel.Data()
	if data == nil || data.ItemMap == nil {
		s.ctx.Logger().Debug("no items to delete,")
		return errorpb.ERROR_SUCCESS
	}

	var itemsToDelete []*pb.Item
	itemsToDelete = make([]*pb.Item, 0, len(data.ItemMap))

	for confId, item := range data.ItemMap {
		if confId <= 0 || item.Num <= 0 {
			continue
		}

		conf := s.ctx.Doc().TbItem.Get(confId)
		if conf == nil {
			s.ctx.Logger().Warn("item config not found, skipping deletion, confId=%v", confId)
			continue
		}

		itemsToDelete = append(itemsToDelete, &pb.Item{
			ConfId: confId,
			Num:    item.Num,
		})
	}

	if len(itemsToDelete) == 0 {
		s.ctx.Logger().Debug("no valid items to delete,")
		return errorpb.ERROR_SUCCESS
	}

	reason := &common.Reason{
		Str: "del_all_items",
	}

	_, errCode := s.SubItems(itemsToDelete, reason)
	if errCode != errorpb.ERROR_SUCCESS {
		s.ctx.Logger().Error("failed to delete all items, errCode=%v", errCode)
		return errCode
	}
	s.ctx.Logger().Info("successfully deleted all items, count=%v", len(itemsToDelete))
	return errorpb.ERROR_SUCCESS
}

func (s *ItemModule) SubItems(consume []*pb.Item, reason *common.Reason) ([]*pb.Item, errorpb.ERROR) {
	if len(consume) == 0 {
		return nil, errorpb.ERROR_SUCCESS
	}
	if errCode := s.CheckEnough(consume); errCode != errorpb.ERROR_SUCCESS {
		return nil, errCode
	}
	s.ctx.Logger().Debug("sub consume:%v reason:%v", consume, reason)
	result := newItemChangeResult(len(consume))
	errCode := errorpb.ERROR_SUCCESS

	consumeMap := configdoc.ItemMerge(consume)
	for _, item := range consumeMap {
		var retItems []*pb.Item
		code := errorpb.ERROR_SUCCESS
		retItems, code = s.subPackItem(item, result, reason)
		errCode = lo.Ternary(code == errorpb.ERROR_SUCCESS, errCode, code)
		result.addRet(retItems...)
	}

	result.SyncItemNtf(s.ctx)
	return result.retItems, errCode
}

func (s *ItemModule) subPackItem(item *pb.Item, result *changeItemResult, reason *common.Reason) ([]*pb.Item, errorpb.ERROR) {
	cur, errCode := GetPackModel(s.model).SubItem(item.ConfId, item.Num)
	if errCode != errorpb.ERROR_SUCCESS {
		s.ctx.Logger().Warn("sub items default err. confId:%v num:%v reason:%v errCode:%v", item.ConfId, item.Num, reason, errCode)
		return nil, errCode
	}
	result.addNtf(cur)
	return []*pb.Item{cur}, errorpb.ERROR_SUCCESS
}

func (s *ItemModule) CheckEnough(consume []*pb.Item) errorpb.ERROR {
	if len(consume) == 0 {
		return errorpb.ERROR_SUCCESS
	}

	itemMap := configdoc.ItemMerge(consume)
	return s.CheckEnoughMap(itemMap)
}

func (s *ItemModule) CheckEnoughMap(consume map[int32]*pb.Item) errorpb.ERROR {
	for _, item := range consume {
		if item.Num > s.GetItemCount(item) {
			return errorpb.ERROR_ITEM_NOT_ENOUGH
		}
	}
	return errorpb.ERROR_SUCCESS
}

func (s *ItemModule) GetItemCount(item *pb.Item) int64 {
	if item == nil {
		return 0
	}
	conf := s.ctx.Doc().TbItem.Get(item.ConfId)
	if conf == nil {
		return 0
	}

	return GetPackModel(s.model).GetCount(item.ConfId)
}

type changeItemResult struct {
	ntfItems map[int32]*pb.Item
	retItems []*pb.Item
}

func newItemChangeResult(size int) *changeItemResult {
	return &changeItemResult{
		ntfItems: make(map[int32]*pb.Item, size),
		retItems: make([]*pb.Item, 0, size),
	}
}

func (r *changeItemResult) addNtf(item *pb.Item) *changeItemResult {
	r.ntfItems[item.ConfId] = item
	return r
}

func (r *changeItemResult) addRet(items ...*pb.Item) *changeItemResult {
	r.retItems = append(r.retItems, items...)
	return r
}
func (r *changeItemResult) SyncItemNtf(ctx iface.IGamerContext) {
	if len(r.ntfItems) == 0 {
		return
	}
	ntf := &pb.PackInfoNTF{
		Items: r.ntfItems,
	}
	ctx.SendMsg(ntf)
}
