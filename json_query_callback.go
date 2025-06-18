package gorm

import (
	"fmt"
	"reflect"
	"strings"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// JSONQuery 自定义查询回调，处理JSON格式的查询结果
func JSONQuery(db *DB) {
	if db.Error == nil {
		// 构建查询SQL
		buildJSONQuerySQL(db)

		if !db.DryRun && db.Error == nil {
			// 执行查询
			rows, err := db.Statement.ConnPool.QueryContext(db.Statement.Context, db.Statement.SQL.String(), db.Statement.Vars...)
			if err != nil {
				// 检查是否是JSON标记错误
				if jsonMarker, ok := err.(*jsonRowsMarker); ok {
					// 处理JSON数据
					handleJSONResult(db, jsonMarker.jsonStr)
				} else {
					db.AddError(err)
				}
				return
			}
			
			// 如果是正常的sql.Rows，使用标准扫描
			if rows != nil {
				defer func() {
					db.AddError(rows.Close())
				}()
				Scan(rows, db, 0)
			}

			if db.Statement.Result != nil {
				db.Statement.Result.RowsAffected = db.RowsAffected
			}
		}
	}
}

// buildJSONQuerySQL 构建查询SQL（复用标准逻辑）
func buildJSONQuerySQL(db *DB) {
	if db.Statement.Schema != nil {
		for _, c := range db.Statement.Schema.QueryClauses {
			db.Statement.AddClause(c)
		}
	}

	if db.Statement.SQL.Len() == 0 {
		db.Statement.SQL.Grow(100)
		clauseSelect := clause.Select{Distinct: db.Statement.Distinct}

		// 构建SELECT子句（简化版本）
		if len(db.Statement.Selects) > 0 {
			clauseSelect.Columns = make([]clause.Column, len(db.Statement.Selects))
			for idx, name := range db.Statement.Selects {
				if db.Statement.Schema == nil {
					clauseSelect.Columns[idx] = clause.Column{Name: name, Raw: true}
				} else if f := db.Statement.Schema.LookUpField(name); f != nil {
					clauseSelect.Columns[idx] = clause.Column{Name: f.DBName}
				} else {
					clauseSelect.Columns[idx] = clause.Column{Name: name, Raw: true}
				}
			}
		} else if db.Statement.Schema != nil && len(db.Statement.Omits) > 0 {
			selectColumns, _ := db.Statement.SelectAndOmitColumns(false, false)
			clauseSelect.Columns = make([]clause.Column, 0, len(db.Statement.Schema.DBNames))
			for _, dbName := range db.Statement.Schema.DBNames {
				if v, ok := selectColumns[dbName]; (ok && v) || !ok {
					clauseSelect.Columns = append(clauseSelect.Columns, clause.Column{Table: db.Statement.Table, Name: dbName})
				}
			}
		} else if db.Statement.Schema != nil {
			clauseSelect.Columns = make([]clause.Column, len(db.Statement.Schema.DBNames))
			for idx, dbName := range db.Statement.Schema.DBNames {
				clauseSelect.Columns[idx] = clause.Column{Table: db.Statement.Table, Name: dbName}
			}
		}

		db.Statement.AddClauseIfNotExists(clause.From{})
		db.Statement.AddClauseIfNotExists(clauseSelect)
		db.Statement.Build(db.Statement.BuildClauses...)
	}
}

// handleJSONResult 处理JSON查询结果
func handleJSONResult(db *DB, jsonStr string) {
	// 创建JSONRows实例
	jsonRows, err := NewJSONRows(jsonStr)
	if err != nil {
		db.AddError(fmt.Errorf("创建JSONRows失败: %w", err))
		return
	}
	defer jsonRows.Close()

	// 使用自定义扫描逻辑处理JSON数据
	scanJSONRows(jsonRows, db)
}

// scanJSONRows 扫描JSON行数据到目标结构
func scanJSONRows(rows *JSONRows, db *DB) {
	var (
		columns, _   = rows.Columns()
		values       = make([]interface{}, len(columns))
		initialized  = false
	)

	if len(db.Statement.ColumnMapping) > 0 {
		for i, column := range columns {
			v, ok := db.Statement.ColumnMapping[column]
			if ok {
				columns[i] = v
			}
		}
	}

	db.RowsAffected = 0

	switch dest := db.Statement.Dest.(type) {
	case map[string]interface{}, *map[string]interface{}:
		// 扫描到map
		if initialized || rows.Next() {
			prepareJSONValues(values, db, columns)

			db.RowsAffected++
			db.AddError(rows.Scan(values...))

			mapValue, ok := dest.(map[string]interface{})
			if !ok {
				if v, ok := dest.(*map[string]interface{}); ok {
					if *v == nil {
						*v = map[string]interface{}{}
					}
					mapValue = *v
				}
			}
			scanIntoJSONMap(mapValue, values, columns)
		}
	case *[]map[string]interface{}:
		// 扫描到map切片
		for initialized || rows.Next() {
			prepareJSONValues(values, db, columns)

			initialized = false
			db.RowsAffected++
			db.AddError(rows.Scan(values...))

			mapValue := map[string]interface{}{}
			scanIntoJSONMap(mapValue, values, columns)
			*dest = append(*dest, mapValue)
		}
	default:
		// 扫描到结构体
		db.scanIntoJSONStruct(rows, db.Statement.ReflectValue, values, columns)
	}

	if db.RowsAffected == 0 && db.Statement.RaiseErrorOnNotFound && db.Error == nil {
		db.AddError(ErrRecordNotFound)
	}
}

// prepareJSONValues 准备值切片用于扫描
func prepareJSONValues(values []interface{}, db *DB, columns []string) {
	if db.Statement.Schema != nil {
		for idx, name := range columns {
			if field := db.Statement.Schema.LookUpField(name); field != nil {
				values[idx] = field.NewValuePool.Get()
				continue
			}
			values[idx] = new(interface{})
		}
	} else {
		for idx := range columns {
			values[idx] = new(interface{})
		}
	}
}

// scanIntoJSONMap 扫描数据到map
func scanIntoJSONMap(mapValue map[string]interface{}, values []interface{}, columns []string) {
	for idx, column := range columns {
		if values[idx] != nil {
			if ptr, ok := values[idx].(*interface{}); ok && ptr != nil {
				mapValue[column] = *ptr
			} else {
				mapValue[column] = values[idx]
			}
		} else {
			mapValue[column] = nil
		}
	}
}

// scanIntoJSONStruct 扫描JSON数据到结构体
func (db *DB) scanIntoJSONStruct(rows *JSONRows, reflectValue reflect.Value, values []interface{}, columns []string) {
	var (
		fields = make([]*schema.Field, len(columns))
		sch    = db.Statement.Schema
	)

	if sch != nil {
		// 映射列名到字段
		for idx, column := range columns {
			if field := sch.LookUpField(column); field != nil && field.Readable {
				fields[idx] = field
			}
		}
	}

	switch reflectValue.Kind() {
	case reflect.Slice, reflect.Array:
		// 处理切片/数组
		var elem reflect.Value
		isArrayKind := reflectValue.Kind() == reflect.Array

		if !isArrayKind {
			if reflectValue.Cap() == 0 {
				db.Statement.ReflectValue.Set(reflect.MakeSlice(reflectValue.Type(), 0, 20))
			} else {
				reflectValue.SetLen(0)
				db.Statement.ReflectValue.Set(reflectValue)
			}
		}

		elemType := reflectValue.Type().Elem()
		for rows.Next() {
			elem = reflect.New(elemType)
			db.scanJSONIntoStruct(rows, elem, values, fields, columns)

			if !strings.Contains(elemType.String(), "*") {
				elem = elem.Elem()
			}

			if isArrayKind {
				if reflectValue.Len() >= int(db.RowsAffected) {
					reflectValue.Index(int(db.RowsAffected - 1)).Set(elem)
				}
			} else {
				reflectValue = reflect.Append(reflectValue, elem)
			}
		}

		if !isArrayKind {
			db.Statement.ReflectValue.Set(reflectValue)
		}

	case reflect.Struct, reflect.Ptr:
		// 处理单个结构体
		if rows.Next() {
			db.scanJSONIntoStruct(rows, reflectValue, values, fields, columns)
		}
	}
}

// scanJSONIntoStruct 扫描JSON行到结构体
func (db *DB) scanJSONIntoStruct(rows *JSONRows, reflectValue reflect.Value, values []interface{}, fields []*schema.Field, columns []string) {
	for idx, field := range fields {
		if field != nil {
			values[idx] = field.NewValuePool.Get()
		}
	}

	db.RowsAffected++
	db.AddError(rows.Scan(values...))

	for idx, field := range fields {
		if field == nil {
			continue
		}

		db.AddError(field.Set(db.Statement.Context, reflectValue, values[idx]))
		// 释放资源
		field.NewValuePool.Put(values[idx])
	}
}