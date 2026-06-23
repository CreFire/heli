package mongoclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMergeBsonDocs(t *testing.T) {
	// 测试用例1: 空输入
	t.Run("EmptyInput", func(t *testing.T) {
		result, err := MergeBsonDocs()
		assert.NoError(t, err)
		assert.Equal(t, bson.M{}, result)
	})

	// 测试用例2: 单个更新文档，只包含$set
	t.Run("SingleSetOnly", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"name": "Alice", "age": 30}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{"$set": bson.M{"name": "Alice", "age": 30}}
		assert.Equal(t, expected, result)
	})

	// 测试用例3: 单个更新文档，只包含$unset
	t.Run("SingleUnsetOnly", func(t *testing.T) {
		updates := []bson.M{
			{"$unset": bson.M{"deprecatedField": ""}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{"$unset": bson.M{"deprecatedField": ""}}
		assert.Equal(t, expected, result)
	})

	// 测试用例4: 单个更新文档，同时包含$set和$unset
	t.Run("SingleSetAndUnset", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"name": "Bob", "score": 95}, "$unset": bson.M{"tempField": ""}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{"$set": bson.M{"name": "Bob", "score": 95}, "$unset": bson.M{"tempField": ""}}
		assert.Equal(t, expected, result)
	})

	// 测试用例5: 多个更新文档合并，只有$set操作
	t.Run("MultipleSetsOnly", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"name": "Charlie"}},
			{"$set": bson.M{"age": 25}},
			{"$set": bson.M{"score": 88.5}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{"$set": bson.M{"name": "Charlie", "age": 25, "score": 88.5}}
		assert.Equal(t, expected, result)
	})

	// 测试用例6: 多个更新文档合并，只有$unset操作
	t.Run("MultipleUnsetsOnly", func(t *testing.T) {
		updates := []bson.M{
			{"$unset": bson.M{"field1": ""}},
			{"$unset": bson.M{"field2": ""}},
			{"$unset": bson.M{"field3": ""}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{"$unset": bson.M{"field1": "", "field2": "", "field3": ""}}
		assert.Equal(t, expected, result)
	})

	// 测试用例7: 多个更新文档合并，包含$set和$unset混合操作
	t.Run("MultipleMixedOperations", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"name": "David"}},
			{"$unset": bson.M{"oldField": ""}},
			{"$set": bson.M{"age": 35}},
			{"$unset": bson.M{"anotherOldField": ""}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$set":   bson.M{"name": "David", "age": 35},
			"$unset": bson.M{"oldField": "", "anotherOldField": ""},
		}
		assert.Equal(t, expected, result)
	})

	// 测试用例8: 后面的$set会覆盖前面相同字段的值
	t.Run("SetFieldOverwrite", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"name": "Eve", "age": 30}},
			{"$set": bson.M{"name": "Eva", "score": 92}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{"$set": bson.M{"name": "Eva", "age": 30, "score": 92}}
		assert.Equal(t, expected, result)
	})

	// 测试用例9: 复杂嵌套结构的合并
	t.Run("ComplexNestedStructure", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{
				"user.name":     "Frank",
				"user.profile":  bson.M{"age": 28, "city": "Shanghai"},
				"user.settings": bson.M{"theme": "dark"},
			}},
			{"$set": bson.M{
				"user.profile.email":     "frank@example.com",
				"user.settings.language": "zh-CN",
			}},
			{"$unset": bson.M{
				"user.tempData": "",
			}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$set": bson.M{
				"user.name":              "Frank",
				"user.profile":           bson.M{"age": 28, "city": "Shanghai"},
				"user.settings":          bson.M{"theme": "dark"},
				"user.profile.email":     "frank@example.com",
				"user.settings.language": "zh-CN",
			},
			"$unset": bson.M{
				"user.tempData": "",
			},
		}
		assert.Equal(t, expected, result)
	})

	// 测试用例10: 使用结构体作为$set值
	t.Run("StructAsSetValue", func(t *testing.T) {
		type User struct {
			Name string `bson:"name"`
			Age  int32  `bson:"age"` // 使用int32以匹配BSON序列化行为
		}

		user := User{Name: "Grace", Age: 27}

		updates := []bson.M{
			{"$set": user},
			{"$set": bson.M{"score": 85}},
		}
		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$set": bson.M{
				"name":  "Grace",
				"age":   int32(27), // 明确指定类型
				"score": 85,
			},
		}
		assert.Equal(t, expected, result)
	})

	// 测试用例11: 非法$set值应被忽略，不返回错误
	t.Run("IgnoreInvalidSetValue", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"name": "Helen"}},
			{"$set": func() {}},
			{"$set": bson.M{"score": 99}},
		}

		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$set": bson.M{
				"name":  "Helen",
				"score": 99,
			},
		}
		assert.Equal(t, expected, result)
	})

	// 测试用例12: 非法$unset值应被忽略，不返回错误
	t.Run("IgnoreInvalidUnsetValue", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"name": "Ivy"}},
			{"$unset": 12345},
		}

		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$set": bson.M{
				"name": "Ivy",
			},
		}
		assert.Equal(t, expected, result)
	})

	// 测试用例13: 同字段$set和$unset冲突时，按最后一次操作生效
	t.Run("SetUnsetConflictLastWriteWins", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"profile.name": "old"}},
			{"$unset": bson.M{"profile.name": ""}},
			{"$set": bson.M{"profile.name": "new"}},
		}

		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$set": bson.M{
				"profile.name": "new",
			},
		}
		assert.Equal(t, expected, result)
	})

	// 测试用例14: 兼容 public_gamer_data 场景，单个 nil 文档应返回空更新
	t.Run("SingleNilDoc", func(t *testing.T) {
		result, err := MergeBsonDocs(nil)
		assert.NoError(t, err)
		assert.Equal(t, bson.M{}, result)
	})

	// 测试用例15: 混合 nil/空文档/有效文档时，忽略无效项并保留有效操作
	t.Run("MixedNilEmptyAndValidDocs", func(t *testing.T) {
		updates := []bson.M{
			nil,
			{},
			{"$set": bson.M{"mail_data.unread": int32(2)}},
			{"$unset": bson.M{"mail_data.expired": 1}},
		}

		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$set":   bson.M{"mail_data.unread": int32(2)},
			"$unset": bson.M{"mail_data.expired": 1},
		}
		assert.Equal(t, expected, result)
	})

	// 测试用例16: 同一文档内同字段$set/$unset冲突时，当前实现中$unset生效
	t.Run("SameDocSetUnsetConflictUnsetWins", func(t *testing.T) {
		updates := []bson.M{
			{"$set": bson.M{"player_data.items.potion": 5}, "$unset": bson.M{"player_data.items.potion": 1}},
		}

		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$unset": bson.M{"player_data.items.potion": 1},
		}
		assert.Equal(t, expected, result)
	})

	// 测试用例17: 未知操作符应被忽略
	t.Run("IgnoreUnknownOperators", func(t *testing.T) {
		updates := []bson.M{
			{"$inc": bson.M{"score": 1}},
			{"$rename": bson.M{"a": "b"}},
		}

		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		assert.Equal(t, bson.M{}, result)
	})

	// 测试用例18: bson.D 作为$set输入时，空key忽略、重复key后者覆盖前者
	t.Run("SetWithBsonDBoundary", func(t *testing.T) {
		updates := []bson.M{
			{
				"$set": bson.D{
					{Key: "player_data.name", Value: "Tom"},
					{Key: "", Value: "ignored"},
					{Key: "player_data.name", Value: "Jerry"},
				},
			},
		}

		result, err := MergeBsonDocs(updates...)
		assert.NoError(t, err)
		expected := bson.M{
			"$set": bson.M{"player_data.name": "Jerry"},
		}
		assert.Equal(t, expected, result)
	})
}
