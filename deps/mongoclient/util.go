package mongoclient

import (
	"game/deps/xlog"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// MergeUpdates 合并多个只包含 $set 和 $unset 的 MongoDB 更新文档。
func MergeBsonDocs(updates ...bson.M) (bson.M, error) {
	if len(updates) == 0 {
		return bson.M{}, nil
	}

	// 初始化最终的 $set 和 $unset maps
	finalSet := make(bson.M, 16)
	finalUnset := make(bson.M, 16)

	// 遍历传入的每一个 update 文档
	for _, u := range updates {
		if len(u) == 0 {
			continue
		}

		// --- 处理 $set ---
		if fieldsToSet, ok := normalizeUpdateFields(u["$set"]); ok {
			for field, value := range fieldsToSet {
				finalSet[field] = value
				// 同字段若之前出现在 $unset，按最后一次操作覆盖
				if _, exists := finalUnset[field]; exists {
					xlog.Errorf("merge bson docs conflict, field:%s action:set_over_unset", field)
					delete(finalUnset, field)
				}
			}
		}

		// --- 处理 $unset ---
		if fieldsToUnset, ok := normalizeUpdateFields(u["$unset"]); ok {
			for field, value := range fieldsToUnset {
				finalUnset[field] = value
				// 同字段若之前出现在 $set，按最后一次操作覆盖
				if _, exists := finalSet[field]; exists {
					xlog.Errorf("merge bson docs conflict, field:%s action:unset_over_set", field)
					delete(finalSet, field)
				}
			}
		}
	}

	// 构建最终的 mergedUpdate 文档
	mergedUpdate := make(bson.M, 2)
	if len(finalSet) > 0 {
		mergedUpdate["$set"] = finalSet
	}
	if len(finalUnset) > 0 {
		mergedUpdate["$unset"] = finalUnset
	}

	return mergedUpdate, nil
}

// normalizeUpdateFields 尽量把输入转换为 bson.M；失败时返回 false 以便上层忽略该片段。
func normalizeUpdateFields(value any) (bson.M, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case bson.M:
		if len(v) == 0 {
			return nil, false
		}
		return v, true
	case map[string]any:
		if len(v) == 0 {
			return nil, false
		}
		return bson.M(v), true
	case bson.D:
		if len(v) == 0 {
			return nil, false
		}

		fields := make(bson.M, len(v))
		for _, item := range v {
			if item.Key == "" {
				continue
			}
			fields[item.Key] = item.Value
		}
		if len(fields) == 0 {
			return nil, false
		}
		return fields, true
	default:
		// 兼容结构体/结构体指针等输入；无法序列化时直接忽略。
		bsonBytes, err := bson.Marshal(v)
		if err != nil {
			return nil, false
		}

		var fields bson.M
		if err = bson.Unmarshal(bsonBytes, &fields); err != nil {
			return nil, false
		}
		if len(fields) == 0 {
			return nil, false
		}
		return fields, true
	}
}
