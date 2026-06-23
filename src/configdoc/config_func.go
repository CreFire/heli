package configdoc

import (
	"game/src/proto/pb"
)

func BuildItemsByConfId(confId string, num int64) []*pb.Item {
	items := make([]*pb.Item, 0, 1)
	item := &pb.Item{ConfId: confId, Num: num}
	items = append(items, item)
	return items
}

func CloneItem(item *pb.Item) *pb.Item {
	return &pb.Item{
		InstId:  item.InstId,
		ConfId:  item.ConfId,
		Type:    item.Type,
		Num:     item.Num,
		Expires: item.Expires,
		Ctime:   item.Ctime,
		Change:  item.Change,
	}
}

func MergeItemToMap(itemMap map[string]*pb.Item, confId string, num int64) map[string]*pb.Item {
	if num <= 0 {
		return itemMap
	}
	if itemMap == nil {
		itemMap = make(map[string]*pb.Item, 1)
	}
	if item := itemMap[confId]; item != nil {
		item.Num += num
		return itemMap
	}
	itemMap[confId] = BuildItem(confId, num)
	return itemMap
}

func ItemMerge(items []*pb.Item) map[string]*pb.Item {
	if len(items) == 0 {
		return map[string]*pb.Item{}
	}

	itemMap := make(map[string]*pb.Item, len(items))
	for _, item := range items {
		if i, ok := itemMap[item.ConfId]; ok {
			i.Num += item.Num
		} else {
			itemMap[item.ConfId] = BuildItem(item.ConfId, item.Num)
		}
	}
	return itemMap
}

func ItemMapToSlice(itemMap map[string]*pb.Item) []*pb.Item {
	if len(itemMap) == 0 {
		return nil
	}
	items := make([]*pb.Item, 0, len(itemMap))
	for _, item := range itemMap {
		items = append(items, item)
	}
	return items
}
