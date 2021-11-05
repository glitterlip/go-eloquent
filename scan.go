package goeloquent

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
)

type ScanResult struct {
	Count int
}

func (ScanResult) LastInsertId() (int64, error) {
	return 0, errors.New("no insert in select")
}

func (v ScanResult) RowsAffected() (int64, error) {
	return int64(v.Count), nil
}
func ScanAll(rows *sql.Rows, dest interface{}) (result ScanResult) {
	realDest := reflect.Indirect(reflect.ValueOf(dest))
	if realDest.Kind() == reflect.Slice {
		slice := realDest.Type()
		sliceItem := slice.Elem()
		if sliceItem.Kind() == reflect.Map {
			return scanMapSlice(rows, dest)
		} else if sliceItem.Kind() == reflect.Struct {
			return scanStructSlice(rows, dest)
		} else if sliceItem.Kind() == reflect.Ptr {
			if sliceItem.Elem().Kind() == reflect.Struct {
				return scanStructSlice(rows, dest)
			}
		} else {
			return scanValues(rows, dest)
		}
	} else if _, ok := dest.(*reflect.Value); ok {
		return scanRelations(rows, dest)
	} else if realDest.Kind() == reflect.Struct {
		return scanStruct(rows, dest)
	} else if realDest.Kind() == reflect.Map {
		return scanMap(rows, dest)
	} else {
		for rows.Next() {
			result.Count++
			err := rows.Scan(dest)
			if err != nil {
				panic(err.Error())
			}
		}
	}
	return
}

func scanMapSlice(rows *sql.Rows, dest interface{}) (result ScanResult) {
	columns, _ := rows.Columns()
	realDest := reflect.Indirect(reflect.ValueOf(dest))
	for rows.Next() {
		scanArgs := make([]interface{}, len(columns))
		element := make(map[string]interface{})
		result.Count++
		for i, _ := range columns {
			scanArgs[i] = new(interface{})
		}
		err := rows.Scan(scanArgs...)
		if err != nil {
			panic(err.Error())
		}
		for i, column := range columns {
			element[column] = reflect.ValueOf(scanArgs[i]).Elem().Interface()
		}
		realDest.Set(reflect.Append(realDest, reflect.ValueOf(element)))
	}
	return
}
func scanStructSlice(rows *sql.Rows, dest interface{}) (result ScanResult) {
	realDest := reflect.Indirect(reflect.ValueOf(dest))
	columns, _ := rows.Columns()
	slice := realDest.Type()
	sliceItem := slice.Elem()
	itemIsPtr := sliceItem.Kind() == reflect.Ptr
	model := GetParsedModel(dest)
	scanArgs := make([]interface{}, len(columns))
	var needProcessPivot bool
	var pivotColumnMap = make(map[string]int, 2)
	for rows.Next() {
		result.Count++
		var v, vp reflect.Value
		if itemIsPtr {
			vp = reflect.New(sliceItem.Elem())
			v = reflect.Indirect(vp)
		} else {
			vp = reflect.New(sliceItem)
			v = reflect.Indirect(vp)
		}
		for i, column := range columns {
			if f, ok := model.FieldsByDbName[column]; ok {
				scanArgs[i] = v.Field(f.Index).Addr().Interface()
			} else {
				if strings.Contains(column, "goelo_pivot_") {
					if !needProcessPivot {
						needProcessPivot = true
					}
					pivotColumnMap[column] = i
					scanArgs[i] = new(sql.NullString)
				} else {
					scanArgs[i] = new(interface{})
				}
			}
		}
		err := rows.Scan(scanArgs...)
		if err != nil {
			panic(err.Error())
		}
		if needProcessPivot {
			t := make(map[string]sql.NullString, 2)
			for columnName, index := range pivotColumnMap {
				t[columnName] = *(scanArgs[index].(*sql.NullString))
			}
			v.Field(model.PivotFieldIndex[0]).Field(model.PivotFieldIndex[1]).Set(reflect.ValueOf(t))
		}
		if itemIsPtr {
			realDest.Set(reflect.Append(realDest, vp))
		} else {
			realDest.Set(reflect.Append(realDest, v))
		}
	}
	return
}
func scanRelations(rows *sql.Rows, dest interface{}) (result ScanResult) {
	columns, _ := rows.Columns()
	destValue := dest.(*reflect.Value)
	//relation reflect results
	slice := destValue.Type()
	sliceItem := slice.Elem()
	//itemIsPtr := base.Kind() == reflect.Ptr
	model := GetParsedModel(sliceItem)
	scanArgs := make([]interface{}, len(columns))
	vp := reflect.New(sliceItem)
	v := reflect.Indirect(vp)
	var needProcessPivot bool
	var pivotColumnMap = make(map[string]int, 2)
	for rows.Next() {
		result.Count++
		for i, column := range columns {
			if f, ok := model.FieldsByDbName[column]; ok {
				scanArgs[i] = v.Field(f.Index).Addr().Interface()
			} else if strings.Contains(column, "goelo_pivot_") {
				if !needProcessPivot {
					needProcessPivot = true
				}
				pivotColumnMap[column] = i
				scanArgs[i] = new(sql.NullString)
			} else {
				scanArgs[i] = new(interface{})
			}
		}
		err := rows.Scan(scanArgs...)
		if err != nil {
			panic(err.Error())
		}
		if needProcessPivot {
			t := make(map[string]sql.NullString, 2)
			for columnName, index := range pivotColumnMap {
				t[columnName] = *(scanArgs[index].(*sql.NullString))
			}
			v.Field(model.PivotFieldIndex[0]).Field(model.PivotFieldIndex[1]).Set(reflect.ValueOf(t))
		}
		*destValue = reflect.Append(*destValue, v)
	}
	return
}
func scanStruct(rows *sql.Rows, dest interface{}) (result ScanResult) {
	realDest := reflect.Indirect(reflect.ValueOf(dest))
	model := GetParsedModel(dest)
	columns, _ := rows.Columns()
	scanArgs := make([]interface{}, len(columns))
	vp := reflect.New(realDest.Type())
	v := reflect.Indirect(vp)

	for rows.Next() {
		result.Count++
		for i, column := range columns {
			if f, ok := model.FieldsByDbName[column]; ok {
				scanArgs[i] = v.Field(f.Index).Addr().Interface()
			} else {
				scanArgs[i] = new(interface{})
			}
		}
		err := rows.Scan(scanArgs...)
		if err != nil {
			panic(err.Error())
		}
		if realDest.Kind() == reflect.Ptr {
			realDest.Set(vp)
		} else {
			realDest.Set(v)
		}
	}
	return
}
func scanMap(rows *sql.Rows, dest interface{}) (result ScanResult) {
	columns, _ := rows.Columns()
	realDest := reflect.Indirect(reflect.ValueOf(dest))
	scanArgs := make([]interface{}, len(columns))
	for rows.Next() {
		result.Count++
		for i, _ := range columns {
			scanArgs[i] = new(interface{})
		}
		err := rows.Scan(scanArgs...)
		if err != nil {
			panic(err.Error())
		}
		for i, column := range columns {
			//If elem is the zero Value, SetMapIndex deletes the key from the map.
			realDest.SetMapIndex(reflect.ValueOf(column), reflect.ValueOf(reflect.ValueOf(scanArgs[i]).Elem().Interface()))
		}
	}
	return
}
func scanValues(rows *sql.Rows, dest interface{}) (result ScanResult) {
	columns, _ := rows.Columns()
	realDest := reflect.Indirect(reflect.ValueOf(dest))
	scanArgs := make([]interface{}, len(columns))
	for rows.Next() {
		result.Count++
		scanArgs[0] = reflect.New(realDest.Type().Elem()).Interface()
		err := rows.Scan(scanArgs...)
		if err != nil {
			panic(err.Error())
		}
		realDest.Set(reflect.Append(realDest, reflect.ValueOf(reflect.ValueOf(scanArgs[0]).Elem().Interface())))
	}
	return
}
