package mongoclient

import (
	"context"
	"errors"
	"fmt"
	"game/deps/xlog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
	"google.golang.org/protobuf/proto"
)

const defaultIndexPersisterTimeout = 5 * time.Second
const defaultIndexPersisterLimit = int64(100)

type IndexPersisterCodecHook[T proto.Message] interface {
	Decode(raw bson.Raw, doc T) error
	Replace(id int64, doc T) (any, error)
}

// IndexPersister loads all documents by a non-unique index field.
type IndexPersister[T proto.Message] struct {
	col        *mongo.Collection
	indexField string
	indexValue any
	dataType   proto.Message
	timeout    time.Duration
	limit      int64
	data       map[int64]T
	record     *MultiDocRecorder[T]
	loaded     bool
	mode       PersisterMode
	codecHook  IndexPersisterCodecHook[T]
}

func NewIndexPersister[T proto.Message](prototype T, indexField string, col *mongo.Collection, indexValue any, limit int64) *IndexPersister[T] {

	if indexField == "" {
		panic("indexField is empty")
	}

	if limit <= 0 {
		limit = defaultIndexPersisterLimit
	}

	return &IndexPersister[T]{
		col:        col,
		indexField: indexField,
		indexValue: indexValue,
		dataType:   prototype,
		timeout:    defaultIndexPersisterTimeout,
		limit:      limit,
		data:       make(map[int64]T),
		record:     NewMultiDocRecorder[T](),
	}
}

func (p *IndexPersister[T]) SetTimeout(d time.Duration) {
	if d <= 0 {
		p.timeout = defaultIndexPersisterTimeout
		return
	}
	p.timeout = d
}

func (p *IndexPersister[T]) SetIndexValue(v any) {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly, index: %s=%v", p.indexField, p.indexValue)
		return
	}
	p.indexValue = v
	p.loaded = false
	if p.data == nil {
		p.data = make(map[int64]T)
	} else {
		clear(p.data)
	}
	if p.record != nil {
		p.record.Clear()
	}
}

func (p *IndexPersister[T]) SetLimit(limit int64) {
	if limit <= 0 {
		p.limit = defaultIndexPersisterLimit
		return
	}
	p.limit = limit
}

// SetMode sets persister mode (default/read/nodefault).
func (p *IndexPersister[T]) SetMode(mode PersisterMode) {
	p.mode = mode
	if p.mode.noDefaultLoad() && p.data == nil {
		p.data = make(map[int64]T)
	}
}

// SetNoDefaultLoad enables a mode that skips automatic full-load in Data().
// Only newly added data is saved; single records can be fetched via Get(id).
func (p *IndexPersister[T]) SetNoDefaultLoad() {
	p.SetMode(PersisterModeNoDefaultLoad)
}

func (p *IndexPersister[T]) SetReadOnly() {
	p.SetMode(PersisterModeReadOnly)
}

func (p *IndexPersister[T]) SetCodecHook(h IndexPersisterCodecHook[T]) *IndexPersister[T] {
	p.codecHook = h
	return p
}

func (p *IndexPersister[T]) Load() error {
	if p.loaded {
		return nil
	}
	if p.indexValue == nil {
		return errors.New("index persister indexValue is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	_, err := p.LoadByIndexWithContext(ctx, p.indexValue)
	return err
}

func (p *IndexPersister[T]) LoadByIndex(indexValue any, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	if p.col == nil {
		return nil, errors.New("index persister collection is nil")
	}
	if p.indexField == "" {
		return nil, errors.New("index persister indexField is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	p.indexValue = indexValue
	return p.LoadByIndexWithContext(ctx, indexValue, opts...)
}

func (p *IndexPersister[T]) LoadByIndexWithContext(ctx context.Context, indexValue any, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	if p.col == nil {
		return nil, errors.New("index persister collection is nil")
	}
	if p.indexField == "" {
		return nil, errors.New("index persister indexField is empty")
	}

	filter := bson.M{p.indexField: indexValue}
	findOpts := make([]options.Lister[options.FindOptions], 0, len(opts)+1)
	findOpts = append(findOpts, opts...)
	findOpts = append(findOpts, p.defaultFindOptions())
	cursor, err := p.col.Find(ctx, filter, findOpts...)
	if err != nil {
		return nil, fmt.Errorf("find by index %s=%v error: %w", p.indexField, indexValue, err)
	}
	defer cursor.Close(ctx)

	data := make(map[int64]T, 16)
	result := make([]T, 0, 16)
	for cursor.Next(ctx) {
		raw := cursor.Current
		if len(raw) == 0 {
			var rawDoc bson.Raw
			if err := cursor.Decode(&rawDoc); err != nil {
				return nil, fmt.Errorf("decode raw by index %s=%v error: %w", p.indexField, indexValue, err)
			}
			raw = rawDoc
		}

		id, err := p.extractDocID(raw)
		if err != nil {
			return nil, err
		}

		doc := proto.Clone(p.dataType).(T)
		if err := p.decodeRaw(raw, doc); err != nil {
			return nil, fmt.Errorf("decode by index %s=%v error: %w", p.indexField, indexValue, err)
		}
		data[id] = doc
		result = append(result, doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor by index %s=%v error: %w", p.indexField, indexValue, err)
	}
	p.data = data
	if p.record == nil {
		p.record = NewMultiDocRecorder[T]()
	} else {
		p.record.Clear()
	}
	p.indexValue = indexValue
	p.loaded = true
	return result, nil
}

func (p *IndexPersister[T]) Save() error {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly, index: %s=%v", p.indexField, p.indexValue)
		return nil
	}
	if !p.loaded && !p.mode.noDefaultLoad() {
		xlog.Warnf("data not loaded, save index %s=%v", p.indexField, p.indexValue)
		return nil
	}
	if p.record == nil || !p.record.HasChanges() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	models, err := p.buildWriteModels()
	if err != nil {
		return fmt.Errorf("gen write models index %s=%v error: %w", p.indexField, p.indexValue, err)
	}
	if len(models) == 0 {
		return nil
	}
	_, err = p.col.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return fmt.Errorf("bulk write index %s=%v error: %w", p.indexField, p.indexValue, err)
	}
	p.record.Clear()
	return nil
}

func (p *IndexPersister[T]) SetLoaded() {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly, index: %s=%v", p.indexField, p.indexValue)
		return
	}
	p.loaded = true
}

func (p *IndexPersister[T]) IsLoaded() bool {
	return p.loaded
}

func (p *IndexPersister[T]) SaveDocs() bson.M {
	if !p.loaded || p.mode.readOnly() {
		return nil
	}
	return nil
}

func (p *IndexPersister[T]) SaveAllDoc() {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly, index: %s=%v", p.indexField, p.indexValue)
		return
	}
	p.loaded = true
	if p.record == nil {
		p.record = NewMultiDocRecorder[T]()
	}
	p.record.SaveAll(p.data)
}

func (p *IndexPersister[T]) AddUpdateOp(id int64, doc T) {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly, index: %s=%v", p.indexField, p.indexValue)
		return
	}
	if isInterfaceNil(doc) {
		xlog.Errorf("add update op failed, index %s=%v _id:%d error: nil doc", p.indexField, p.indexValue, id)
		return
	}
	if p.record == nil {
		p.record = NewMultiDocRecorder[T]()
	}
	if p.data == nil {
		p.data = make(map[int64]T)
	}
	p.data[id] = doc
	p.record.Set(id, doc)
}

func (p *IndexPersister[T]) AddUnsetOp(id int64) {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly, index: %s=%v", p.indexField, p.indexValue)
		return
	}
	if p.record == nil {
		p.record = NewMultiDocRecorder[T]()
	}
	if p.data == nil {
		p.data = make(map[int64]T)
	}
	delete(p.data, id)
	p.record.Unset(id)
}

func (p *IndexPersister[T]) Data() map[int64]T {
	if p.mode.noDefaultLoad() {
		if p.data == nil {
			p.data = make(map[int64]T)
		}
		return p.data
	}
	if !p.loaded {
		start := time.Now()
		if err := p.Load(); err != nil {
			xlog.Errorf("load data failed, index %s=%v error: %v", p.indexField, p.indexValue, err)
			return nil
		}
		if xlog.GetLogLevel() == xlog.LOG_LEVEL_DEBUG {
			xlog.Debugf("try load persist data, index %s=%v dur %v", p.indexField, p.indexValue, time.Since(start))
		}
	}
	return p.data
}

// Get returns the record by id. If not in memory, it loads a single document from DB.
// It returns an error when the record does not exist.
func (p *IndexPersister[T]) Get(id int64) (T, error) {
	var zero T
	if p.data != nil {
		if doc, ok := p.data[id]; ok {
			return doc, nil
		}
	}
	if p.col == nil {
		return zero, errors.New("index persister collection is nil")
	}
	if p.indexField == "" {
		return zero, errors.New("index persister indexField is empty")
	}
	if p.indexValue == nil {
		return zero, errors.New("index persister indexValue is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	filter := bson.M{"_id": id, p.indexField: p.indexValue}
	res := p.col.FindOne(ctx, filter)
	if err := res.Err(); err != nil {
		return zero, err
	}

	var raw bson.Raw
	if err := res.Decode(&raw); err != nil {
		return zero, fmt.Errorf("decode by id %d error: %w", id, err)
	}
	doc := proto.Clone(p.dataType).(T)
	if err := p.decodeRaw(raw, doc); err != nil {
		return zero, fmt.Errorf("unmarshal by id %d error: %w", id, err)
	}
	if p.data == nil {
		p.data = make(map[int64]T)
	}
	p.data[id] = doc
	return doc, nil
}

func (p *IndexPersister[T]) RawData() proto.Message {
	return nil
}

func (p *IndexPersister[T]) SetData(data map[int64]T) {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly, index: %s=%v", p.indexField, p.indexValue)
		return
	}
	p.loaded = true
	p.data = data
	p.SaveAllDoc()
}

func (p *IndexPersister[T]) Clear() {
	if p.mode.readOnly() {
		xlog.Warnf("data is readonly, index: %s=%v", p.indexField, p.indexValue)
		return
	}
	p.data = nil
	if p.record != nil {
		p.record.Clear()
	}
	p.loaded = false
}

func (p *IndexPersister[T]) defaultFindOptions() options.Lister[options.FindOptions] {
	limit := p.limit
	if limit <= 0 {
		limit = defaultIndexPersisterLimit
	}
	return options.Find().
		SetSort(bson.D{{Key: "_id", Value: -1}}).
		SetLimit(limit)
}

// _id must be int64 to satisfy the ordered/limit load requirement.
func (p *IndexPersister[T]) extractDocID(raw bson.Raw) (int64, error) {
	val, err := raw.LookupErr("_id")
	if err != nil {
		if errors.Is(err, bsoncore.ErrElementNotFound) {
			return 0, fmt.Errorf("index persister doc missing _id")
		}
		return 0, fmt.Errorf("lookup _id error: %w", err)
	}

	if val.Type != bson.TypeInt64 {
		return 0, fmt.Errorf("index persister _id type %v not int64", val.Type)
	}
	return val.Int64(), nil
}

func (p *IndexPersister[T]) decodeRaw(raw bson.Raw, doc T) error {
	if p.codecHook != nil {
		return p.codecHook.Decode(raw, doc)
	}
	return bson.Unmarshal(raw, doc)
}

func (p *IndexPersister[T]) buildWriteModels() ([]mongo.WriteModel, error) {
	if p.record == nil || !p.record.HasChanges() {
		return nil, nil
	}
	if p.codecHook != nil {
		return p.record.GenWriteModelsWithReplacer(p.codecHook.Replace)
	}
	return p.record.GenWriteModels(), nil
}

// isInterfaceNil 检查接口类型值是否为nil
func isInterfaceNil(v any) bool {
	return v == nil
}
