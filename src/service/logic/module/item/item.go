package item

import (
	"fmt"

	"game/deps/mongoclient"
	"game/deps/xlog"
	"game/src/configdoc"
	"game/src/persist"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata/base"

	"google.golang.org/protobuf/proto"
)

type GamerPack struct {
	*base.GamerModBase
	gid int64
	*mongoclient.DataPersister[*pb.GamerPackData]
}

func NewGamerItemPack(modIndex int, data *persist.GamerData, doc *base.ExcelConf, docExt *base.ExcelConfExt) *GamerPack {
	return &GamerPack{
		GamerModBase:  base.NewGamerModBase(modIndex, doc, docExt),
		gid:           data.GamerId,
		DataPersister: persist.GetGamerModData[*pb.GamerPackData](data, persist.GamerPackModIndex),
	}
}

func (m *GamerPack) PackMap() map[string]*pb.Item {
	return m.Data().ItemMap
}

func (m *GamerPack) GetCount(itemId string) int64 {
	curItem := m.Data().ItemMap[itemId]
	if curItem == nil {
		return 0
	}
	return curItem.Num
}

func (m *GamerPack) AddItem(itemId string, num int64) (*pb.Item, errorpb.ERROR) {
	if num <= 0 {
		xlog.Warnf("add item invalid num. id %d num %v", itemId, num)
		return nil, errorpb.ERROR_REQUEST_PARAMS
	}
	item := m.Data().ItemMap[itemId]
	if item == nil {
		item = configdoc.BuildItem(itemId, 0)
		m.Data().ItemMap[itemId] = item
	}
	old := item.Num
	item.Num += num
	m.AddUpdateOp("it."+itemId, item)
	change := configdoc.CloneItem(item)
	change.Change = item.Num - old

	return change, errorpb.ERROR_SUCCESS
}
func validItem(it *pb.Item) bool {
	return it != nil && it.ConfId != "" && it.Num > 0
}

func itemStoreKey(itemId string, itemType int32) string {
	if itemType == int32(pb.ITEM_TYPE_TYPE_CURRENCY) {
		return currencyMapKey(itemId)
	}
	return itemMapKey(itemId)
}

func cloneItem(it *pb.Item) *pb.Item {
	if it == nil {
		return nil
	}
	return proto.Clone(it).(*pb.Item)
}

func itemMapKey(confId string) string     { return fmt.Sprintf("it.%s", confId) }
func currencyMapKey(confId string) string { return fmt.Sprintf("cu.%s", confId) }

func (m *GamerPack) isCurrency(it *pb.Item) bool {
	return it != nil && it.Type == int32(pb.ITEM_TYPE_TYPE_CURRENCY)
}

func (m *GamerPack) CheckEnough(items []*pb.Item) errorpb.ERROR {
	for _, it := range items {
		if !validItem(it) {
			return errorpb.ERROR_REQUEST_PARAMS
		}
		if m.GetItemCount(it) < it.Num {
			return errorpb.ERROR_ITEM_NOT_ENOUGH
		}
	}
	return errorpb.ERROR_SUCCESS
}

func (m *GamerPack) CheckEnoughMap(consume map[string]*pb.Item) errorpb.ERROR {
	for confId, it := range consume {
		if it == nil {
			return errorpb.ERROR_REQUEST_PARAMS
		}
		if it.ConfId != "" {
			it = cloneItem(it)
			it.ConfId = confId
		}
		if !validItem(it) {
			return errorpb.ERROR_REQUEST_PARAMS
		}
		if m.GetItemCount(it) < it.Num {
			return errorpb.ERROR_ITEM_NOT_ENOUGH
		}
	}
	return errorpb.ERROR_SUCCESS
}

func (m *GamerPack) GetItemCount(item *pb.Item) int64 {
	if item == nil || item.ConfId == "" {
		return 0
	}

	curItem := m.Data().ItemMap[item.ConfId]
	if curItem == nil {
		return 0
	}
	return curItem.Num
}
func (m *GamerPack) SubItems(items []*pb.Item) ([]*pb.Item, errorpb.ERROR) {
	if len(items) == 0 {
		return nil, errorpb.ERROR_REQUEST_PARAMS
	}
	changed := make([]*pb.Item, 0, len(items))
	for _, it := range items {
		if !validItem(it) {
			return nil, errorpb.ERROR_REQUEST_PARAMS
		}
		change, code := m.SubItem(it.ConfId, it.Num)
		if code != errorpb.ERROR_SUCCESS {
			return nil, code
		}
		if change != nil {
			changed = append(changed, change)
		}
	}
	return changed, errorpb.ERROR_SUCCESS
}

func (m *GamerPack) SubItem(itemId string, num int64) (*pb.Item, errorpb.ERROR) {
	if num <= 0 {
		xlog.Warnf("sub item invalid num. id %d num %v", itemId, num)
		return nil, errorpb.ERROR_REQUEST_PARAMS
	}
	if itemId == "" {
		return nil, errorpb.ERROR_REQUEST_PARAMS
	}
	item := m.Data().ItemMap[itemId]
	if item == nil || item.Num < num {
		return nil, errorpb.ERROR_ITEM_NOT_ENOUGH
	}
	old := item.Num
	item.Num -= num
	if item.Num == 0 {
		m.AddUnsetOp(itemStoreKey(itemId, item.Type))
		delete(m.Data().ItemMap, itemId)
	} else {
		m.AddUpdateOp(itemStoreKey(itemId, item.Type), item)
	}
	change := configdoc.CloneItem(item)
	change.Change = item.Num - old
	return change, errorpb.ERROR_SUCCESS
}

func (m *GamerPack) UseItem(item *pb.Item) []*pb.Item {
	if !validItem(item) {
		return nil
	}
	changed, code := m.SubItems([]*pb.Item{item})
	if code != errorpb.ERROR_SUCCESS {
		return nil
	}
	return changed
}

func (m *GamerPack) GetAfkRewardsWithTransItem(transItems []*pb.Item) []*pb.Item {
	out := make([]*pb.Item, 0, len(transItems))
	for _, it := range transItems {
		if validItem(it) {
			out = append(out, cloneItem(it))
		}
	}
	return out
}
