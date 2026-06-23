package configdoc

import (
	"reflect"
	"unsafe"

	cfg "game/src/proto/docpb"
)

func calcChangedTables(oldDoc, newDoc *DocPbConfig) *cfg.Tables {
	if oldDoc == nil || newDoc == nil || oldDoc.Tables == nil || newDoc.Tables == nil {
		return nil
	}
	return diffTables(oldDoc.Tables, newDoc.Tables)
}

// diffTables returns only modified records for each table field.
// Added or removed keys are ignored.
func diffTables(oldTables, newTables *cfg.Tables) *cfg.Tables {
	oldVal := reflect.ValueOf(oldTables).Elem()
	newVal := reflect.ValueOf(newTables).Elem()
	changed := &cfg.Tables{}
	changedVal := reflect.ValueOf(changed).Elem()

	typ := oldVal.Type()
	for i := 0; i < typ.NumField(); i++ {
		diff := diffTableData(oldVal.Field(i), newVal.Field(i))
		if diff.IsValid() {
			changedVal.Field(i).Set(diff)
		}
	}
	if isEmptyTables(changedVal) {
		return nil
	}
	return changed
}

func diffTableData(oldField, newField reflect.Value) reflect.Value {
	if oldField.IsNil() || newField.IsNil() {
		return reflect.Value{}
	}
	oldMap, oldOk := getDataMap(oldField)
	newMap, newOk := getDataMap(newField)
	if oldOk && newOk {
		return diffMap(oldField.Type(), oldMap, newMap)
	}
	if reflect.DeepEqual(oldField.Interface(), newField.Interface()) {
		return reflect.Value{}
	}
	return newField
}

func getDataMap(field reflect.Value) (reflect.Value, bool) {
	if field.IsNil() {
		return reflect.Value{}, false
	}
	method := field.MethodByName("GetDataMap")
	if !method.IsValid() || method.Type().NumIn() != 0 || method.Type().NumOut() != 1 {
		return reflect.Value{}, false
	}
	out := method.Call(nil)[0]
	if out.Kind() != reflect.Map {
		return reflect.Value{}, false
	}
	return out, true
}

func diffMap(tableType reflect.Type, oldMap, newMap reflect.Value) reflect.Value {
	if oldMap.IsNil() || newMap.IsNil() {
		return reflect.Value{}
	}
	changedMap := reflect.MakeMapWithSize(oldMap.Type(), 0)
	changedList, hasList := newDataList(tableType, 0, 0)
	iter := oldMap.MapRange()
	for iter.Next() {
		key := iter.Key()
		oldVal := iter.Value()
		newVal := newMap.MapIndex(key)
		if !newVal.IsValid() {
			continue
		}
		if !reflect.DeepEqual(oldVal.Interface(), newVal.Interface()) {
			changedMap.SetMapIndex(key, newVal)
			if hasList {
				changedList = reflect.Append(changedList, newVal)
			}
		}
	}
	if changedMap.Len() == 0 {
		return reflect.Value{}
	}
	return buildTableWithData(tableType, changedMap, changedList)
}

func newDataList(tableType reflect.Type, lenVal, capVal int) (reflect.Value, bool) {
	elem := tableType.Elem()
	field, ok := elem.FieldByName("_dataList")
	if !ok || field.Type.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	return reflect.MakeSlice(field.Type, lenVal, capVal), true
}

func buildTableWithData(tableType reflect.Type, dataMap, dataList reflect.Value) reflect.Value {
	newTable := reflect.New(tableType.Elem())
	setUnexportedField(newTable.Elem().FieldByName("_dataMap"), dataMap)
	if dataList.IsValid() {
		setUnexportedField(newTable.Elem().FieldByName("_dataList"), dataList)
	}
	return newTable
}

func setUnexportedField(field, value reflect.Value) {
	if !field.IsValid() {
		return
	}
	if field.Type() != value.Type() {
		if value.Type().ConvertibleTo(field.Type()) {
			value = value.Convert(field.Type())
		} else {
			return
		}
	}
	if field.CanSet() {
		field.Set(value)
		return
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(value)
}

func isEmptyTables(val reflect.Value) bool {
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := val.Field(i)
		if field.Kind() == reflect.Pointer && !field.IsNil() {
			return false
		}
	}
	return true
}
