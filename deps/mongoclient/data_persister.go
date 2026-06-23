package mongoclient

import (
	"context"
	"errors"
	"fmt"
	"game/deps/xlog"
	"reflect"
	"strings"
	"time"

	"github.com/sasha-s/go-deadlock"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
	"google.golang.org/protobuf/proto"
)

/*
mongo 落地辅助类。外部包装结构如下，

	type ColData struct{
		ID int64 `bson:"_id"`
		data1 DataPersister[*pb.xxxdata1] `bson:"data1"`
		data2 DataPersister[*pb.xxxdata2] `bson:"data2"`
	}
*/

type MongoPersister interface {
	Load() (err error)
	Save() error
	SetLoaded()
	SaveDocs() bson.M
	RawData() proto.Message
}

type DataPersister[T proto.Message] struct {
	data    T                  `bson:",inline"`
	record  *SingleDocRecorder `bson:"-"`
	loaded  bool               `bson:"-"`
	dataTag string             `bson:"-"`
	col     *mongo.Collection  `bson:"-"`
	id      int64              `bson:"-"`
	mode    PersisterMode      `bson:"-"`
}

func (p *DataPersister[T]) UnmarshalBSON(data []byte) error {
	// The data here is the content of the "currency_data" field.
	// We need to unmarshal it into p.data.
	// p.data is of type T, which is *pb.GamerCurrencyData.
	// p.data is already initialized in NewDataPersister.
	return bson.Unmarshal(data, p.data)
}

func NewDataPersister[T proto.Message](data T, tag string, col *mongo.Collection, id int64) *DataPersister[T] {

	return &DataPersister[T]{
		data:    data,
		record:  NewSingleDocRecorder(tag),
		loaded:  false,
		dataTag: tag,
		col:     col,
		id:      id,
	}
}

func (p *DataPersister[T]) Load() (err error) {
	if p.loaded {
		return nil
	}
	opt := options.FindOne().SetProjection(bson.M{p.dataTag: 1, "_id": 0})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	si := p.col.FindOne(ctx, bson.M{"_id": p.id}, opt)
	if err := si.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// 没文档属于正常（新号/延迟创建），不打错误日志
			p.loaded = true
			return nil
		}
		return fmt.Errorf("find one _id:%d tag: %s error: %w", p.id, p.dataTag, err)
	}
	raw, err := si.Raw()
	if err != nil {
		return fmt.Errorf("decode raw bytes _id:%d tag: %s error: %w", p.id, p.dataTag, err)
	}

	subDocValue, err := raw.LookupErr(p.dataTag)
	if err != nil {
		if errors.Is(err, bsoncore.ErrElementNotFound) {
			p.loaded = true
			return nil
		}
		return fmt.Errorf("lookup tag '%s' in raw bson _id:%d error: %w", p.dataTag, p.id, err)
	}
	if err := subDocValue.Unmarshal(p.data); err != nil {
		return fmt.Errorf("unmarshal sub-document to target _id:%d tag: %s error: %w", p.id, p.dataTag, err)
	}

	p.loaded = true
	return nil
}

func (p *DataPersister[T]) Save() error {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly,tag: %s gid: %d ", p.dataTag, p.id)
		return nil
	}
	if !p.loaded {
		xlog.Warnf("data not loaded, save %s  gid: %d", p.dataTag, p.id)
		return nil
	}
	docs := p.record.GenUpdateDoc()
	if len(docs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	_, err := p.col.UpdateOne(ctx, bson.M{"_id": p.id}, docs, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("update doc _id:%d tag: %s error: %w", p.id, p.dataTag, err)
	}
	return nil
}

func (p *DataPersister[T]) SetLoaded() {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly,tag: %s gid: %d ", p.dataTag, p.id)
		return
	}
	p.loaded = true
}

func (p *DataPersister[T]) IsLoaded() bool {
	return p.loaded
}

func (p *DataPersister[T]) SaveDocs() bson.M {
	if !p.loaded || p.mode.readOnly() {
		return nil
	}
	return p.record.GenUpdateDoc()
}

func (p *DataPersister[T]) SaveAllDoc() {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly,tag: %s gid: %d ", p.dataTag, p.id)
		return
	}
	p.loaded = true
	p.record.SaveAll(p.data)
}

// AddUpdateOp keyPath 不包含“.” 只能是data的一级成员，并且不能是map，keyPath包含 ".",只能是map 中的key ，且keyPath 为“map.key”
func (p *DataPersister[T]) AddUpdateOp(keyPath string, doc any) {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly,tag: %s gid: %d ", p.dataTag, p.id)
		return
	}
	if err := p.validateUpdatePath(keyPath); err != nil {
		xlog.Errorf("add update op failed, tag %s, gid %d error: %v", p.dataTag, p.id, err)
		return
	}
	err := p.record.SetField(keyPath, doc)
	if err != nil {
		xlog.Errorf("add update op failed, tag %s, gid %d error: %v", p.dataTag, p.id, err)
		return
	}
}

// AddUnsetOp keyPath 必须包含 "."， 只能是map 中的key ，且keyPath 为“map.key”
func (p *DataPersister[T]) AddUnsetOp(keyPath string) {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly,tag: %s gid: %d ", p.dataTag, p.id)
		return
	}
	if err := p.validateNestedMapPath(keyPath); err != nil {
		xlog.Errorf("add unset op failed, tag %s, gid %d error: %v", p.dataTag, p.id, err)
		return
	}
	err := p.record.UnsetField(keyPath)
	if err != nil {
		xlog.Errorf("add unset op failed, tag %s, gid %d error: %v", p.dataTag, p.id, err)
		return
	}
}

func (p *DataPersister[T]) Data() T {
	if !p.loaded {
		start := time.Now()
		if err := p.Load(); err != nil {
			xlog.Errorf("load data failed, tag %s, gid %d error: %v", p.dataTag, p.id, err)
			var zero T
			return zero
		}
		if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
			xlog.Debugf("try load persist data, load %s  gid: %d dur %v ", p.dataTag, p.id, time.Since(start))
		}
	}
	return p.data
}

func (p *DataPersister[T]) RawData() proto.Message {
	return p.Data()
}
func (p *DataPersister[T]) SetData(data T) {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly,tag: %s gid: %d ", p.dataTag, p.id)
		return
	}
	p.loaded = true
	p.data = data
	p.SaveAllDoc()
}

func (p *DataPersister[T]) SetMode(mode PersisterMode) {
	p.mode = mode
}

func (p *DataPersister[T]) SetReadOnly() {
	p.SetMode(PersisterModeReadOnly)
}

func (p *DataPersister[T]) Clear() {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly,tag: %s gid: %d ", p.dataTag, p.id)
		return
	}
	var zero T
	p.data = zero
	p.loaded = false
}

func (p *DataPersister[T]) validateUpdatePath(keyPath string) error {
	if deadlock.Opts.Disable {
		return nil
	}
	if strings.Count(keyPath, ".") == 1 {
		return p.validateNestedMapPath(keyPath)
	}
	return nil
}

func (p *DataPersister[T]) validateNestedMapPath(keyPath string) error {
	if deadlock.Opts.Disable {
		return nil
	}
	parentField := strings.SplitN(keyPath, ".", 2)[0]
	if p.isMapField(parentField) {
		return nil
	}
	return fmt.Errorf("fieldPath parent must be map field: %s", keyPath)
}

func (p *DataPersister[T]) isMapField(fieldPath string) bool {
	typ := reflect.TypeOf(p.data)
	if typ == nil {
		return false
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.Map {
			continue
		}
		if field.Name == fieldPath {
			return true
		}
		bsonTag := field.Tag.Get("bson")
		if bsonTag == "" {
			continue
		}
		tagName := strings.Split(bsonTag, ",")[0]
		if tagName == fieldPath {
			return true
		}
	}

	return false
}
