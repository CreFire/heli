package mongoclient

import (
	"errors"
	"fmt"
	"os"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sasha-s/go-deadlock"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
)

// 单文档记录器，记录单文档内的变化更新
type SingleDocRecorder struct {
	ParentField string
	setField    map[string]any
	unsetField  map[string]struct{}
	saveAllDoc  any
	inUse       int32
}

var ErrKeyValueNotMap = errors.New("fieldPath map must be map key")
var ErrKeyValueDeep = errors.New("fieldPath is too deep")

func NewSingleDocRecorder(parentField string) *SingleDocRecorder {
	return &SingleDocRecorder{
		ParentField: parentField,
		setField:    make(map[string]any),
		unsetField:  make(map[string]struct{}),
	}
}

// SetField fieldPath 中没有“.”  value可以是除map的任意类型。 fieldPath 有“.” ，value只能是 map中的filedPath对应value
func (r *SingleDocRecorder) SetField(fieldPath string, value any) error {
	r.enter("SetField")
	defer r.exit()
	if strings.Count(fieldPath, ".") > 1 {
		return ErrKeyValueDeep
	}

	r.setField[fieldPath] = value
	delete(r.unsetField, fieldPath)
	return nil
}

// UnsetField filedPath 只能是map中的key
func (r *SingleDocRecorder) UnsetField(fieldPath string) error {
	r.enter("UnsetField")
	defer r.exit()
	if strings.Count(fieldPath, ".") != 1 {
		return ErrKeyValueNotMap
	}
	r.unsetField[fieldPath] = struct{}{}
	delete(r.setField, fieldPath)
	return nil
}

func (r *SingleDocRecorder) SaveAll(doc any) {
	r.enter("SaveAll")
	defer r.exit()
	r.saveAllDoc = doc
}

func (r *SingleDocRecorder) HasChanges() bool {
	if r == nil {
		return false
	}
	return len(r.setField) > 0 || len(r.unsetField) > 0
}

func (r *SingleDocRecorder) GenUpdateDoc() bson.M {
	r.enter("GenUpdateDoc")
	defer r.exit()
	if r.saveAllDoc != nil {
		ret := bson.M{"$set": bson.M{r.ParentField: r.saveAllDoc}}
		clear(r.setField)
		clear(r.unsetField)
		r.saveAllDoc = nil
		return ret
	}

	if len(r.setField) == 0 && len(r.unsetField) == 0 {
		return nil
	}

	setDoc, unsetDoc := bson.M{}, bson.M{}
	for fieldPath, value := range r.setField {
		setDoc[r.ParentField+"."+fieldPath] = value
	}

	for fieldPath := range r.unsetField {
		unsetDoc[r.ParentField+"."+fieldPath] = 1
	}

	if len(r.setField) > 16 {
		r.setField = make(map[string]any, 100)
	} else {
		clear(r.setField)
	}

	clear(r.unsetField)

	ret := buildSingleDocBsonDoc(setDoc, unsetDoc)
	return ret
}

// buildSingleDocBsonDoc 从 set 和 unset 操作map构建BSON文档。
// 仅当 '$set' 和 '$unset' 字段包含操作时，才会添加它们。
func buildSingleDocBsonDoc(setOps, unsetOps bson.M) bson.M {
	doc := bson.M{}

	if len(setOps) > 0 {
		doc["$set"] = setOps
	}

	if len(unsetOps) > 0 {
		doc["$unset"] = unsetOps
	}
	return doc
}

// 使用deadlock.Opts.Disable 做检测并发开发
func (r *SingleDocRecorder) enter(op string) {
	//todo 生产环境设置成true
	if deadlock.Opts.Disable {
		return
	}

	if !atomic.CompareAndSwapInt32(&r.inUse, 0, 1) {
		// 在panic之前将goroutine信息写入文件
		filename := "./logs/" + time.Now().Format("20060102_150405") + "_goroutine.pprof"

		// 确保logs目录存在
		if _, err := os.Stat("./logs"); os.IsNotExist(err) {
			os.Mkdir("./logs", 0755)
		}

		// 创建文件并写入goroutine信息
		f, err := os.Create(filename)
		if err == nil {
			pprof.Lookup("goroutine").WriteTo(f, 2)
			f.Close()
		}

		panic("mongoclient.SimpleRecorder concurrent access: parent=" + r.ParentField + " op=" + op)
	}
}

func (r *SingleDocRecorder) exit() {
	if deadlock.Opts.Disable {
		return
	}

	time.Sleep(2 * time.Millisecond)
	atomic.StoreInt32(&r.inUse, 0)
}

// MultiDocRecorder tracks document-level upserts and deletes.
type MultiDocRecorder[T proto.Message] struct {
	set   map[int64]T
	unset map[int64]struct{}
	inUse int32
}

func NewMultiDocRecorder[T proto.Message]() *MultiDocRecorder[T] {
	return &MultiDocRecorder[T]{
		set:   make(map[int64]T),
		unset: make(map[int64]struct{}),
	}
}

func (r *MultiDocRecorder[T]) Set(id int64, doc T) {
	if r == nil {
		return
	}
	r.enter("Set")
	defer r.exit()
	r.set[id] = doc
	delete(r.unset, id)
}

func (r *MultiDocRecorder[T]) Unset(id int64) {
	if r == nil {
		return
	}
	r.enter("Unset")
	defer r.exit()
	r.unset[id] = struct{}{}
	delete(r.set, id)
}

func (r *MultiDocRecorder[T]) SaveAll(data map[int64]T) {
	if r == nil {
		return
	}
	r.enter("SaveAll")
	defer r.exit()
	r.set = make(map[int64]T, len(data))
	r.unset = make(map[int64]struct{})
	for id, doc := range data {
		if isInterfaceNil(doc) {
			continue
		}
		r.set[id] = doc
	}
}

func (r *MultiDocRecorder[T]) HasChanges() bool {
	if r == nil {
		return false
	}
	r.enter("HasChanges")
	defer r.exit()
	return len(r.set) > 0 || len(r.unset) > 0
}

func (r *MultiDocRecorder[T]) Clear() {
	if r == nil {
		return
	}
	r.enter("Clear")
	defer r.exit()
	clear(r.set)
	clear(r.unset)
}

func (r *MultiDocRecorder[T]) GenWriteModels() []mongo.WriteModel {
	if r == nil {
		return nil
	}
	r.enter("GenWriteModels")
	defer r.exit()

	if len(r.set) == 0 && len(r.unset) == 0 {
		return nil
	}

	models := make([]mongo.WriteModel, 0, len(r.set)+len(r.unset))
	for id, doc := range r.set {
		if isInterfaceNil(doc) {
			continue
		}
		model := mongo.NewReplaceOneModel().
			SetFilter(bson.M{"_id": id}).
			SetReplacement(doc).
			SetUpsert(true)
		models = append(models, model)
	}

	for id := range r.unset {
		model := mongo.NewDeleteOneModel().SetFilter(bson.M{"_id": id})
		models = append(models, model)
	}

	return models
}

func (r *MultiDocRecorder[T]) GenWriteModelsWithReplacer(replacer func(id int64, doc T) (any, error)) ([]mongo.WriteModel, error) {
	if r == nil {
		return nil, nil
	}
	if replacer == nil {
		return nil, errors.New("multi doc replacer is nil")
	}

	r.enter("GenWriteModelsWithReplacer")
	defer r.exit()

	if len(r.set) == 0 && len(r.unset) == 0 {
		return nil, nil
	}

	models := make([]mongo.WriteModel, 0, len(r.set)+len(r.unset))
	for id, doc := range r.set {
		if isInterfaceNil(doc) {
			continue
		}
		replacement, err := replacer(id, doc)
		if err != nil {
			return nil, fmt.Errorf("build replacement _id:%d error: %w", id, err)
		}
		if replacement == nil {
			return nil, fmt.Errorf("replacement _id:%d is nil", id)
		}
		model := mongo.NewReplaceOneModel().
			SetFilter(bson.M{"_id": id}).
			SetReplacement(replacement).
			SetUpsert(true)
		models = append(models, model)
	}

	for id := range r.unset {
		model := mongo.NewDeleteOneModel().SetFilter(bson.M{"_id": id})
		models = append(models, model)
	}

	return models, nil
}

// 使用deadlock.Opts.Disable 做检测并发开发
func (r *MultiDocRecorder[T]) enter(op string) {
	//todo 生产环境设置成true
	if deadlock.Opts.Disable {
		return
	}

	if !atomic.CompareAndSwapInt32(&r.inUse, 0, 1) {
		// 在panic之前将goroutine信息写入文件
		filename := "./logs/" + time.Now().Format("20060102_150405") + "_goroutine.pprof"

		// 确保logs目录存在
		if _, err := os.Stat("./logs"); os.IsNotExist(err) {
			os.Mkdir("./logs", 0755)
		}

		// 创建文件并写入goroutine信息
		f, err := os.Create(filename)
		if err == nil {
			pprof.Lookup("goroutine").WriteTo(f, 2)
			f.Close()
		}

		panic("mongoclient.MultiDocRecorder concurrent access: op=" + op)
	}
}

func (r *MultiDocRecorder[T]) exit() {
	if deadlock.Opts.Disable {
		return
	}

	time.Sleep(2 * time.Millisecond)
	atomic.StoreInt32(&r.inUse, 0)
}
