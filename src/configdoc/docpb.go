package configdoc

import (
	cfg "game/src/proto/docpb"
	"game/src/proto/pb"
	"math/rand"
)

type DocPbConfig struct {
	*cfg.Tables
	ExcelVersion int32
	Changed      *cfg.Tables
}

func BuildItem(confId string, num int64) *pb.Item {
	return &pb.Item{
		ConfId: confId,
		Num:    num,
	}
}
func BuildItemByItemData(itemDatas ...*cfg.ItemItemData) []*pb.Item {
	res := make([]*pb.Item, 0, len(itemDatas))
	for _, data := range itemDatas {
		if data == nil || data.Value == 0 {
			continue
		}
		res = append(res, BuildItem(data.ItemId, int64(data.Value)))
	}
	return res
}

func BuildItemByItemkv(datas ...*cfg.Itemkv) []*pb.Item {
	res := make([]*pb.Item, 0, len(datas))
	for _, data := range datas {
		if data == nil || data.Value == 0 {
			continue
		}
		res = append(res, BuildItem(string(data.Id), int64(data.Value)))
	}
	return res
}

// RandItem 表示一个随机道具结构
type RandItem struct {
	ConfId string // 配置 ID
	MinNum int64  // 最小值
	MaxNum int64  // 最大值
}

// GetRandomNum 获取一个随机数量，数量在 MinNum 和 MaxNum 之间
// 如果 MinNum 和 MaxNum 相同，则直接返回 MinNum
func (item *RandItem) GetRandomNum() int64 {
	if item.MinNum == item.MaxNum {
		return item.MinNum
	}
	return item.MinNum + rand.Int63n(item.MaxNum-item.MinNum)
}

// RandItemManager 管理多个 RandItem
type RandItemManager struct {
	items map[string]*RandItem // 通过 ConfId 存储 RandItem
}

// NewRandItemManager 创建一个新的 RandItemManager
func NewRandItemManager() *RandItemManager {
	return &RandItemManager{
		items: make(map[string]*RandItem),
	}
}

// AddItem 添加一个 RandItem
func (manager *RandItemManager) AddItem(item *RandItem) {
	manager.items[item.ConfId] = item
}

// GetItem 根据 ConfId 获取 RandItem
func (manager *RandItemManager) GetItem(confId string) (*RandItem, bool) {
	item, exists := manager.items[confId]
	return item, exists
}
