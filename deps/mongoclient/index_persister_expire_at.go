package mongoclient

import (
	"errors"
	"game/src/proto/pb"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
	"google.golang.org/protobuf/proto"
)

type ExpireAtDateCodecHook[T proto.Message] struct {
	fieldName  string
	fieldIndex []int
}

func NewExpireAtDateCodecHook[T proto.Message](prototype T) *ExpireAtDateCodecHook[T] {
	fieldName, fieldIndex := resolveExpireAtFieldInfo(prototype)
	if fieldName == "" || len(fieldIndex) != 1 {
		panic("expire_at field not found for ExpireAtDateCodecHook")
	}
	return &ExpireAtDateCodecHook[T]{
		fieldName:  fieldName,
		fieldIndex: fieldIndex,
	}
}

func (h *ExpireAtDateCodecHook[T]) Decode(raw bson.Raw, doc T) error {
	val, err := raw.LookupErr(h.fieldName)
	if err != nil {
		if errors.Is(err, bsoncore.ErrElementNotFound) {
			return bson.Unmarshal(raw, doc)
		}
		return err
	}

	expUnix := parseExpireAtValue(val)

	var temp bson.M
	if err := bson.Unmarshal(raw, &temp); err != nil {
		return err
	}
	delete(temp, h.fieldName)

	data, err := bson.Marshal(temp)
	if err != nil {
		return err
	}
	if err := bson.Unmarshal(data, doc); err != nil {
		return err
	}
	setExpireAtField(doc, h.fieldIndex, expUnix)
	return nil
}

func (h *ExpireAtDateCodecHook[T]) Replace(id int64, doc T) (any, error) {
	return buildReplacementWithExpireAt(id, doc, h.fieldName, h.fieldIndex)
}

var expireAtDateType = reflect.TypeFor[*pb.ExpireAtDate]()

func resolveExpireAtFieldInfo(msg proto.Message) (string, []int) {
	if msg == nil {
		return "", nil
	}
	t := reflect.TypeOf(msg)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return "", nil
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type != expireAtDateType {
			continue
		}
		tag := f.Tag.Get("bson")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "exp_at" {
			return name, f.Index
		}
	}
	return "", nil
}

func parseExpireAtValue(val bson.RawValue) int64 {
	switch val.Type {
	case bson.TypeDateTime:
		return val.DateTime() / 1000
	case bson.TypeInt64:
		return val.Int64()
	case bson.TypeNull:
		return 0
	case bson.TypeEmbeddedDocument:
		var temp struct {
			UnixSec int64 `bson:"unix_sec"`
		}
		if err := val.Unmarshal(&temp); err == nil {
			return temp.UnixSec
		}
	}
	return 0
}

func setExpireAtField(doc any, index []int, expUnix int64) {
	v := reflect.ValueOf(doc)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() {
		return
	}
	field := v.FieldByIndex(index)
	if !field.CanSet() {
		return
	}
	if expUnix > 0 {
		field.Set(reflect.ValueOf(&pb.ExpireAtDate{UnixSec: expUnix}))
		return
	}
	field.Set(reflect.Zero(field.Type()))
}

func buildReplacementWithExpireAt[T proto.Message](id int64, doc T, fieldName string, fieldIndex []int) (any, error) {
	data, err := bson.Marshal(doc)
	if err != nil {
		return nil, err
	}

	var temp bson.M
	if err := bson.Unmarshal(data, &temp); err != nil {
		return nil, err
	}

	expUnix := getExpireAtUnix(doc, fieldIndex)
	if expUnix > 0 {
		temp[fieldName] = bson.DateTime(expUnix * 1000)
	} else {
		delete(temp, fieldName)
	}

	if id > 0 {
		temp["_id"] = id
	}
	return temp, nil
}

func getExpireAtUnix(doc any, index []int) int64 {
	v := reflect.ValueOf(doc)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() {
		return 0
	}
	field := v.FieldByIndex(index)
	if field.IsNil() {
		return 0
	}
	val, ok := field.Interface().(*pb.ExpireAtDate)
	if !ok || val == nil {
		return 0
	}
	return val.UnixSec
}
