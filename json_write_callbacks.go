package gorm

import (
	"fmt"
	"reflect"

	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// JSONCreate 自定义创建回调，处理INSERT操作
func JSONCreate(db *DB) {
	if db.Error == nil {
		if db.Statement.Schema != nil && !db.Statement.Unscoped {
			for _, c := range db.Statement.Schema.CreateClauses {
				db.Statement.AddClause(c)
			}
		}

		if db.Statement.SQL.Len() == 0 {
			buildJSONCreateSQL(db)
		}

		if !db.DryRun && db.Error == nil {
			// 执行写入操作
			result, err := db.Statement.ConnPool.ExecContext(db.Statement.Context, db.Statement.SQL.String(), db.Statement.Vars...)
			if err != nil {
				db.AddError(err)
				return
			}

			// 设置影响的行数
			if rowsAffected, err := result.RowsAffected(); err == nil {
				db.RowsAffected = rowsAffected
			}

			// 设置插入ID
			if lastInsertId, err := result.LastInsertId(); err == nil && lastInsertId > 0 {
				setJSONInsertID(db, lastInsertId)
			}

			if db.Statement.Result != nil {
				db.Statement.Result.RowsAffected = db.RowsAffected
			}
		}
	}
}

// JSONUpdate 自定义更新回调，处理UPDATE操作
func JSONUpdate(db *DB) {
	if db.Error == nil {
		if db.Statement.Schema != nil && !db.Statement.Unscoped {
			for _, c := range db.Statement.Schema.UpdateClauses {
				db.Statement.AddClause(c)
			}
		}

		if db.Statement.SQL.Len() == 0 {
			buildJSONUpdateSQL(db)
		}

		if !db.DryRun && db.Error == nil {
			// 执行更新操作
			result, err := db.Statement.ConnPool.ExecContext(db.Statement.Context, db.Statement.SQL.String(), db.Statement.Vars...)
			if err != nil {
				db.AddError(err)
				return
			}

			// 设置影响的行数
			if rowsAffected, err := result.RowsAffected(); err == nil {
				db.RowsAffected = rowsAffected
			}

			if db.Statement.Result != nil {
				db.Statement.Result.RowsAffected = db.RowsAffected
			}
		}
	}
}

// JSONDelete 自定义删除回调，处理DELETE操作
func JSONDelete(db *DB) {
	if db.Error == nil {
		if db.Statement.Schema != nil && !db.Statement.Unscoped {
			for _, c := range db.Statement.Schema.DeleteClauses {
				db.Statement.AddClause(c)
			}
		}

		if db.Statement.SQL.Len() == 0 {
			buildJSONDeleteSQL(db)
		}

		if !db.DryRun && db.Error == nil {
			// 执行删除操作
			result, err := db.Statement.ConnPool.ExecContext(db.Statement.Context, db.Statement.SQL.String(), db.Statement.Vars...)
			if err != nil {
				db.AddError(err)
				return
			}

			// 设置影响的行数
			if rowsAffected, err := result.RowsAffected(); err == nil {
				db.RowsAffected = rowsAffected
			}

			if db.Statement.Result != nil {
				db.Statement.Result.RowsAffected = db.RowsAffected
			}
		}
	}
}

// buildJSONCreateSQL 构建INSERT SQL
func buildJSONCreateSQL(db *DB) {
	if db.Statement.Schema != nil {
		for _, c := range db.Statement.Schema.CreateClauses {
			db.Statement.AddClause(c)
		}
	}

	if db.Statement.SQL.Len() == 0 {
		db.Statement.SQL.Grow(180)
		
		// 构建INSERT子句
		var columns []clause.Column
		var values [][]clause.Expression

		switch value := db.Statement.Dest.(type) {
		case map[string]interface{}:
			// 处理map类型
			columns, values = buildColumnsAndValuesFromMap(db, value)
		case []map[string]interface{}:
			// 处理map切片
			columns, values = buildColumnsAndValuesFromMaps(db, value)
		default:
			// 处理结构体
			columns, values = buildColumnsAndValuesFromStruct(db)
		}

		if len(columns) > 0 {
			db.Statement.AddClause(clause.Insert{
				Table:   clause.Table{Name: db.Statement.Table},
				Columns: columns,
			})
			db.Statement.AddClause(clause.Values{
				Columns: columns,
				Values:  values,
			})
		}

		db.Statement.AddClauseIfNotExists(clause.Insert{Table: clause.Table{Name: db.Statement.Table}})
		db.Statement.Build(db.Statement.BuildClauses...)
	}
}

// buildJSONUpdateSQL 构建UPDATE SQL
func buildJSONUpdateSQL(db *DB) {
	if db.Statement.Schema != nil {
		for _, c := range db.Statement.Schema.UpdateClauses {
			db.Statement.AddClause(c)
		}
	}

	if db.Statement.SQL.Len() == 0 {
		db.Statement.SQL.Grow(180)

		// 构建SET子句
		set := clause.Set{}
		
		switch value := db.Statement.Dest.(type) {
		case map[string]interface{}:
			// 处理map类型
			for column, val := range value {
				set.Assignments = append(set.Assignments, clause.Assignment{
					Column: clause.Column{Name: column},
					Value:  val,
				})
			}
		default:
			// 处理结构体
			if db.Statement.Schema != nil {
				selectColumns, restricted := db.Statement.SelectAndOmitColumns(true, !db.Statement.SkipHooks)
				for _, field := range db.Statement.Schema.FieldsByDBName {
					if !field.PrimaryKey && (!restricted || (restricted && selectColumns[field.DBName])) && field.Updatable {
						if v, isZero := field.ValueOf(db.Statement.Context, db.Statement.ReflectValue); !isZero || !field.NotNull {
							set.Assignments = append(set.Assignments, clause.Assignment{
								Column: clause.Column{Name: field.DBName},
								Value:  v,
							})
						}
					}
				}
			}
		}

		if len(set.Assignments) > 0 {
			db.Statement.AddClause(set)
		}

		db.Statement.AddClauseIfNotExists(clause.Update{Table: clause.Table{Name: db.Statement.Table}})
		db.Statement.Build(db.Statement.BuildClauses...)
	}
}

// buildJSONDeleteSQL 构建DELETE SQL
func buildJSONDeleteSQL(db *DB) {
	if db.Statement.Schema != nil {
		for _, c := range db.Statement.Schema.DeleteClauses {
			db.Statement.AddClause(c)
		}
	}

	if db.Statement.SQL.Len() == 0 {
		db.Statement.SQL.Grow(100)

		// 构建WHERE子句（基于主键）
		if db.Statement.ReflectValue.IsValid() {
			reflectValue := db.Statement.ReflectValue
			if reflectValue.Kind() == reflect.Interface {
				reflectValue = reflectValue.Elem()
			}

			if reflectValue.Kind() == reflect.Struct && db.Statement.Schema != nil {
				var conds []clause.Expression
				for _, primaryField := range db.Statement.Schema.PrimaryFields {
					if v, isZero := primaryField.ValueOf(db.Statement.Context, reflectValue); !isZero {
						conds = append(conds, clause.Eq{
							Column: clause.Column{Table: db.Statement.Table, Name: primaryField.DBName},
							Value:  v,
						})
					}
				}

				if len(conds) > 0 {
					db.Statement.AddClause(clause.Where{Exprs: conds})
				}
			}
		}

		db.Statement.AddClauseIfNotExists(clause.Delete{Table: clause.Table{Name: db.Statement.Table}})
		db.Statement.Build(db.Statement.BuildClauses...)
	}
}

// buildColumnsAndValuesFromMap 从map构建列和值
func buildColumnsAndValuesFromMap(db *DB, data map[string]interface{}) ([]clause.Column, [][]clause.Expression) {
	var columns []clause.Column
	var values []clause.Expression

	for column, value := range data {
		columns = append(columns, clause.Column{Name: column})
		values = append(values, clause.Expr{SQL: "?", Vars: []interface{}{value}})
	}

	return columns, [][]clause.Expression{values}
}

// buildColumnsAndValuesFromMaps 从map切片构建列和值
func buildColumnsAndValuesFromMaps(db *DB, data []map[string]interface{}) ([]clause.Column, [][]clause.Expression) {
	if len(data) == 0 {
		return nil, nil
	}

	// 从第一个map获取列名
	var columns []clause.Column
	var columnNames []string
	for column := range data[0] {
		columns = append(columns, clause.Column{Name: column})
		columnNames = append(columnNames, column)
	}

	// 构建每行的值
	var allValues [][]clause.Expression
	for _, row := range data {
		var values []clause.Expression
		for _, column := range columnNames {
			values = append(values, clause.Expr{SQL: "?", Vars: []interface{}{row[column]}})
		}
		allValues = append(allValues, values)
	}

	return columns, allValues
}

// buildColumnsAndValuesFromStruct 从结构体构建列和值
func buildColumnsAndValuesFromStruct(db *DB) ([]clause.Column, [][]clause.Expression) {
	if db.Statement.Schema == nil {
		return nil, nil
	}

	var columns []clause.Column
	var values []clause.Expression

	reflectValue := db.Statement.ReflectValue
	if reflectValue.Kind() == reflect.Interface {
		reflectValue = reflectValue.Elem()
	}

	switch reflectValue.Kind() {
	case reflect.Slice, reflect.Array:
		// 处理结构体切片
		return buildColumnsAndValuesFromStructSlice(db, reflectValue)
	case reflect.Struct:
		// 处理单个结构体
		selectColumns, restricted := db.Statement.SelectAndOmitColumns(false, !db.Statement.SkipHooks)
		
		for _, field := range db.Statement.Schema.FieldsByDBName {
			if !restricted || (restricted && selectColumns[field.DBName]) {
				if v, isZero := field.ValueOf(db.Statement.Context, reflectValue); !isZero || !field.NotNull {
					columns = append(columns, clause.Column{Name: field.DBName})
					values = append(values, clause.Expr{SQL: "?", Vars: []interface{}{v}})
				}
			}
		}
	}

	if len(columns) == 0 {
		return nil, nil
	}

	return columns, [][]clause.Expression{values}
}

// buildColumnsAndValuesFromStructSlice 从结构体切片构建列和值
func buildColumnsAndValuesFromStructSlice(db *DB, reflectValue reflect.Value) ([]clause.Column, [][]clause.Expression) {
	var columns []clause.Column
	var allValues [][]clause.Expression
	
	selectColumns, restricted := db.Statement.SelectAndOmitColumns(false, !db.Statement.SkipHooks)
	
	// 构建列名（从第一个元素）
	if reflectValue.Len() > 0 {
		for _, field := range db.Statement.Schema.FieldsByDBName {
			if !restricted || (restricted && selectColumns[field.DBName]) {
				columns = append(columns, clause.Column{Name: field.DBName})
			}
		}
	}

	// 构建每行的值
	for i := 0; i < reflectValue.Len(); i++ {
		elem := reflectValue.Index(i)
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}

		var values []clause.Expression
		for _, field := range db.Statement.Schema.FieldsByDBName {
			if !restricted || (restricted && selectColumns[field.DBName]) {
				if v, isZero := field.ValueOf(db.Statement.Context, elem); !isZero || !field.NotNull {
					values = append(values, clause.Expr{SQL: "?", Vars: []interface{}{v}})
				} else {
					values = append(values, clause.Expr{SQL: "?", Vars: []interface{}{nil}})
				}
			}
		}
		allValues = append(allValues, values)
	}

	return columns, allValues
}

// setJSONInsertID 设置插入ID到结构体
func setJSONInsertID(db *DB, insertID int64) {
	if db.Statement.Schema == nil || db.Statement.Schema.PrioritizedPrimaryField == nil {
		return
	}

	reflectValue := db.Statement.ReflectValue
	if reflectValue.Kind() == reflect.Interface {
		reflectValue = reflectValue.Elem()
	}

	switch reflectValue.Kind() {
	case reflect.Slice, reflect.Array:
		// 对于切片，设置最后一个元素的ID
		if reflectValue.Len() > 0 {
			elem := reflectValue.Index(reflectValue.Len() - 1)
			if elem.Kind() == reflect.Ptr {
				elem = elem.Elem()
			}
			setIDToStruct(db, elem, insertID)
		}
	case reflect.Struct:
		// 对于单个结构体，直接设置ID
		setIDToStruct(db, reflectValue, insertID)
	}
}

// setIDToStruct 设置ID到结构体
func setIDToStruct(db *DB, reflectValue reflect.Value, insertID int64) {
	if db.Statement.Schema.PrioritizedPrimaryField != nil {
		field := db.Statement.Schema.PrioritizedPrimaryField
		if field.AutoIncrement {
			err := field.Set(db.Statement.Context, reflectValue, insertID)
			if err != nil {
				db.AddError(fmt.Errorf("设置插入ID失败: %w", err))
			}
		}
	}
}