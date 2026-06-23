package xjson

import (
	"bytes"
	"encoding/json"
	"game/deps/basal"
	"io"
	"math"
	"os"
)

const APPEND = -1          //追加
const APPEND_IN_FRONT = -2 //追加在前面
const INDEX_LAST = -3      //最后的位置
const INDEX_FIRST = -4     //第一的位置

type INumber = basal.JsonINumber

type Json struct {
	data any
}

func (my *Json) String() string {
	if my == nil {
		return ""
	}
	//s, _ := TryDump(my.data, false)
	//return s
	s, _ := my.ToString(false)
	return s
}

func (my *Json) Interface() any {
	return my.data
}

func (my *Json) IsNil() bool {
	return my.data == nil
}

func (my *Json) ToString(indent bool) (string, error) {
	return basal.ToString(my.data, indent)
}

func (my *Json) ToInt64() (int64, error) {
	return basal.ToInt64(my.data)
}

func (my *Json) ToInt32() (int32, error) {
	return basal.ToInt32(my.data)
}

func (my *Json) ToFloat64() (float64, error) {
	return basal.ToFloat64(my.data)
}

func (my *Json) ToFloat32() (float32, error) {
	return basal.ToFloat32(my.data)
}

func (my *Json) ToBool() (bool, error) {
	v, err := basal.ToInt64(my.data)
	if err != nil {
		return false, err
	}
	return v != 0, err
}

func (my *Json) TryFloat64() (float64, error) {
	if number, ok := my.data.(basal.JsonINumber); ok {
		v, err := number.Float64()
		if err != nil {
			return 0, err
		}
		return v, nil
	}
	return 0, basal.NewError("json.Number value type error: %v", basal.Type(my.data))
}

func (my *Json) TryFloat32() (float32, error) {
	if v, err := my.TryFloat64(); err == nil {
		if v > math.MaxFloat32 || v < -math.MaxFloat32 {
			return 0, basal.NewError("json.Number overflow float32: %v", my.data)
		}
		return float32(v), nil
	}
	return 0, basal.NewError("json.Number value type error: %v", basal.Type(my.data))
}

func (my *Json) TryInt64() (int64, error) {
	if number, ok := my.data.(INumber); ok {
		v, err := number.Int64()
		if err != nil {
			return 0, err
		}
		return v, nil
	}
	return 0, basal.NewError("json.Number value type error: %v", basal.Type(my.data))
}

func (my *Json) TryInt32() (int32, error) {
	v, err := my.TryInt64()
	if err != nil {
		return 0, err
	}
	if v > math.MaxInt32 || v < math.MinInt32 {
		return 0, basal.NewError("overflow int32 value error: %v", my.data)
	}
	return int32(v), nil
}

func (my *Json) TryInt() (int, error) {
	v, err := my.TryInt64()
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func (my *Json) TryBool() (v bool, ok bool) {
	v, ok = my.data.(bool)
	return
}

func (my *Json) TrySlice() ([]any, error) {
	v, ok := my.data.([]any)
	if ok {
		return v, nil
	} else {
		return nil, basal.NewError("[]interface{} value type error: %v", basal.Type(my.data))
	}
}

func (my *Json) TryMap() (map[string]any, error) {
	v, ok := my.data.(map[string]any)
	if ok {
		return v, nil
	} else {
		return nil, basal.NewError("map[string]interface{} value type error: %v", basal.Type(my.data))
	}
}

func (my *Json) TryBytes() ([]byte, error) {
	if my.data == nil {
		return nil, basal.NewError("json is nil")
	}
	js, err := TryDump(my.data, false)
	return []byte(js), err
}

func (my *Json) GetJson(keys ...any) *Json {
	return &Json{my.Get(keys...)}
}

func (my *Json) Get(keys ...any) any {
	var v = my.data
	var ok bool
	for _, key := range keys {
		switch k := key.(type) {
		case string:
			v, ok, _ = findMapKey(v, k)
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			index, _ := basal.ToInt(k)
			v, ok, _ = findSliceIndex(v, index)
		default:
			ok = false
		}
		if !ok {
			return nil
		}
	}
	return v
}

func (my *Json) Bool() bool {
	v, ok := my.TryBool()
	if !ok {
		panic(basal.NewError("bool value type error: %v", basal.Type(my.data)))
	}
	return v
}

func (my *Json) Int64() int64 {
	v, err := my.TryInt64()
	if err != nil {
		panic(err)
	}
	return v
}

func (my *Json) Int32() int32 {
	return int32(my.Int64())
}

func (my *Json) Slice() []any {
	v, ok := my.data.([]any)
	if ok {
		return v
	} else {
		panic(basal.NewError("[]interface{} value type error: %v", basal.Type(my.data)))
	}
}

func (my *Json) RangeSliceJson(f func(i int, elem *Json) bool) {
	for i, v := range my.Slice() {
		if !f(i, &Json{v}) {
			return
		}
	}
}

func (my *Json) Load(js any) error {
	obj, err := TryLoad(js)
	if err != nil {
		return err
	}
	my.data = obj.data
	return nil
}

func (my *Json) create(keys []any) (any, error) {
	length := len(keys)
	if length < 2 {
		return nil, basal.NewError("json create error: keys num less 2, keys=%v", keys)
	}

	var lastRoot any
	pos := length - 1
	lastRoot = keys[pos]
	for i := pos - 1; i >= 0; i-- {
		switch k := keys[i].(type) {
		case string:
			parent := map[string]any{}
			parent[k] = lastRoot
			lastRoot = parent
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			index, _ := basal.ToInt(k)
			if index == APPEND || index == APPEND_IN_FRONT {
				index = 0
			}
			if index == 0 {
				lastRoot = []any{lastRoot}
			} else {
				return nil, basal.NewError("json create error: slice out of range, keys=%v, index=%v", keys, i)
			}
		default:
			return nil, basal.NewError("json create error: not found key type, keys=%v, index=%v, type=%v", keys, i, basal.Type(keys[i]))
		}
	}
	return lastRoot, nil
}

func (my *Json) set(root any, args []any) (any, error) {
	if len(args) < 2 {
		return nil, basal.NewError("json set error: args num less 2")
	}
	switch data := root.(type) {
	case *any:
		switch v := (*data).(type) {
		case []any:
			var index int
			switch idx := args[0].(type) {
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				index, _ = basal.ToInt(idx)
			default:
				return nil, basal.NewError("json set error: key not is index %v", args)
			}
			maxLen := len(v)
			if index > maxLen || index < INDEX_FIRST {
				return nil, basal.NewError("json set error: out of range[%v, %v] error: index=%v", INDEX_FIRST, maxLen, index)
			}
			if len(args) == 2 {
				value := args[1]
				if index == APPEND || index == maxLen {
					v = append(v, value)
				} else if index == APPEND_IN_FRONT {
					v = append([]any{value}, v...)
				} else if index == INDEX_LAST {
					if maxLen > 0 {
						v[maxLen-1] = value
					}
				} else if index == INDEX_FIRST {
					v[0] = value
				} else {
					v[index] = value
				}
			} else {
				if index == APPEND || index == maxLen {
					value, err := my.create(args[1:])
					if err != nil {
						return nil, err
					}
					v = append(v, value)
				} else if index == APPEND_IN_FRONT {
					value, err := my.create(args[1:])
					if err != nil {
						return nil, err
					}
					v = append([]any{value}, v...)
				} else if index == INDEX_LAST {
					value, err := my.set(&v[maxLen-1], args[1:])
					if err != nil {
						return nil, err
					}
					v[maxLen-1] = value
				} else if index == INDEX_FIRST {
					value, err := my.set(&v[0], args[1:])
					if err != nil {
						return nil, err
					}
					v[0] = value
				} else {
					value, err := my.set(&v[index], args[1:])
					if err != nil {
						return nil, err
					}
					v[index] = value
				}
			}
			return v, nil

		case map[string]any:
			key, ok := args[0].(string)
			if !ok {
				return nil, basal.NewError("json set error: key not is string %v", args)
			}
			if len(args) == 2 {
				v[key] = args[1]
			} else {
				next, ok := v[key]
				if ok {
					value, err := my.set(&next, args[1:])
					if err != nil {
						return nil, err
					}
					v[key] = value
				} else {
					value, err := my.create(args[1:])
					if err != nil {
						return nil, err
					}
					v[key] = value
				}
			}
			return v, nil
		}
	}
	return nil, basal.NewError("json set type error: args=%v", args)
}

func (my *Json) Set(args ...any) error {
	if my.data == nil {
		value, err := my.create(args)
		if err != nil {
			return basal.NewError("json root is nil create error: %v", err)
		}
		my.data = value
		return nil
	} else {
		value, err := my.set(&my.data, args)
		if err != nil {
			return err
		}
		my.data = value
		return nil
	}
}

func (my *Json) delete(root any, keys []any) (any, bool, bool) {
	if len(keys) == 0 {
		return nil, false, true
	}
	switch data := root.(type) {
	case *any:
		switch v := (*data).(type) {
		case []any:
			switch idx := keys[0].(type) {
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				index, err := basal.ToInt(idx)
				if err == nil {
					maxLen := len(v)
					if index == INDEX_LAST {
						index = maxLen - 1
					} else if index == INDEX_FIRST {
						index = 0
					}
					if index >= 0 && index < maxLen {
						value, success, found := my.delete(&v[index], keys[1:])
						if value != nil {
							v[index] = value
						}
						if found {
							v = append(v[:index], v[index+1:]...)
							return v, true, false
						} else {
							return nil, success, false
						}
					}
				}
			}
		case map[string]any:
			switch key := keys[0].(type) {
			case string:
				next, ok := v[key]
				if ok {
					value, success, found := my.delete(&next, keys[1:])
					if value != nil {
						v[key] = value
					}
					if found {
						delete(v, key)
						return v, true, false
					} else {
						return nil, success, false
					}
				}
			}
		}
	}
	return nil, false, false
}

func (my *Json) Delete(keys ...any) bool {
	if my.data != nil {
		value, success, _ := my.delete(&my.data, keys)
		if value != nil {
			my.data = value
		}
		return success
	}
	return false
}

func (my *Json) Clear() {
	my.data = nil
}

func findMapKey(data any, key string) (v any, ok bool, err error) {
	switch m := data.(type) {
	case map[string]any:
		v, ok = m[key]
	default:
		ok = false
		err = basal.NewError("not is map[string]interface{}, type=%v", basal.Type(m))
	}
	return
}

func findSliceIndex(data any, index int) (any, bool, error) {
	if data == nil {
		return nil, false, nil
	}
	slice, ok := data.([]any)
	if !ok {
		return nil, false, basal.NewError("not is []interface{}, type=%v", basal.Type(data))
	}
	maxLen := len(slice)
	if index >= 0 && index < maxLen {
		return slice[index], true, nil
	} else if index == INDEX_LAST && maxLen > 0 {
		return slice[maxLen-1], true, nil
	} else if index == INDEX_FIRST && maxLen > 0 {
		return slice[0], true, nil
	} else {
		return nil, false, nil
	}
}

func loadJson(v []byte) (*Json, error) {
	js := &Json{}
	if err := LoadBytesTo(v, &js.data); err != nil {
		return nil, err
	}
	return js, nil
}

func linkJson(js any) (*Json, error) {
	switch v := js.(type) {
	case map[string]any, []any:
		return &Json{v}, nil
	}
	return nil, basal.NewError("link json type error: %v", basal.Type(js))
}

func TryLoad(js any) (*Json, error) {
	switch v := js.(type) {
	case string:
		return loadJson([]byte(v))
	case []byte:
		return loadJson(v)
	case map[string]any, []any:
		return linkJson(v)
	case *os.File:
		bs, err := io.ReadAll(v)
		if err != nil {
			return nil, err
		}
		return loadJson(bs)
	}
	return nil, basal.NewError("new json type error: %v", basal.Type(js))
}

// Load
//
//	@Description: 加载为json对象
//	@param js  map[string] interface, []interface, struct, json string, json []byte, *os.File
//	@return *Json
func Load(js any) *Json {
	v, err := TryLoad(js)
	if err != nil {
		panic(err)
	}
	return v
}

func LoadFileTo(jsFileName string, toPtr any) error {
	data, err := os.ReadFile(jsFileName)
	if err != nil {
		return err
	}
	return LoadBytesTo(data, toPtr)
}

func LoadBytesTo(js []byte, toPtr any) error {
	decoder := json.NewDecoder(bytes.NewBuffer(js))
	decoder.UseNumber()
	switch xjs := toPtr.(type) {
	case *Json:
		return decoder.Decode(&xjs.data)
	}
	return decoder.Decode(toPtr)
}

func LoadStringTo(js string, toPtr any) error {
	return LoadBytesTo([]byte(js), toPtr)
}

func TryDump(v any, indent bool) (string, error) {
	if indent {
		buf := &bytes.Buffer{}
		encoder := json.NewEncoder(buf)
		encoder.SetIndent("", "\t")
		encoder.SetEscapeHTML(false)
		err := encoder.Encode(v)
		return buf.String(), err
	}
	buf, err := json.Marshal(v)
	return string(buf), err
}

func Dump(v any, indent bool) string {
	if js, err := TryDump(v, indent); err != nil {
		panic(err)
	} else {
		return js
	}
}
