package gorm

import (
	"encoding/json"
	"reflect"
)

// JSONQuery 一个新的与MySQL交互的方式，直接返回JSON字符串
// 这个方法抽象了查询过程，用户只需要传入目标类型，就能获得JSON格式的查询结果
type JSONQueryInterface interface {
	// QueryToJSON 执行查询并返回JSON字符串
	// dest: 目标结构体类型的零值，用于确定查询的数据结构
	// conds: 查询条件，与Find方法的条件参数相同
	// 返回值：JSON字符串和错误信息
	QueryToJSON(dest interface{}, conds ...interface{}) (string, error)
	
	// FirstToJSON 查询第一条记录并返回JSON字符串
	// dest: 目标结构体类型的零值
	// conds: 查询条件
	// 返回值：JSON字符串和错误信息
	FirstToJSON(dest interface{}, conds ...interface{}) (string, error)
	
	// CountToJSON 执行COUNT查询并返回JSON格式的计数结果
	// 返回值：JSON字符串，格式如 '{"count":123}'
	CountToJSON() (string, error)
	
	// RawQueryToJSON 执行原生SQL查询并返回JSON字符串
	// sql: 原生SQL语句
	// values: SQL参数
	// 返回值：JSON字符串和错误信息
	RawQueryToJSON(sql string, values ...interface{}) (string, error)
}

// QueryToJSON 执行Find查询并将结果转换为JSON字符串
// 这是主要的交互方法，支持查询多条记录
// 
// 示例用法：
//   type User struct {
//       UserID int `json:"user_id"`
//       Age    int `json:"age"`
//   }
//   
//   jsonStr, err := db.Where("age > ?", 18).QueryToJSON([]User{})
//   // 返回: '[{"user_id":123, "age": 25}, {"user_id":124, "age": 30}]'
func (db *DB) QueryToJSON(dest interface{}, conds ...interface{}) (string, error) {
	// 创建目标类型的实例用于接收查询结果
	destType := reflect.TypeOf(dest)
	var result interface{}
	
	// 根据传入的dest类型确定查询结果的容器类型
	if destType.Kind() == reflect.Slice {
		// 如果dest是切片类型，直接使用切片接收结果
		result = reflect.New(destType).Interface()
	} else {
		// 如果dest是单个结构体，创建对应的切片类型
		sliceType := reflect.SliceOf(destType)
		result = reflect.New(sliceType).Interface()
	}
	
	// 执行查询
	tx := db.Find(result, conds...)
	if tx.Error != nil {
		return "", tx.Error
	}
	
	// 获取实际的查询结果
	resultValue := reflect.ValueOf(result).Elem().Interface()
	
	// 转换为JSON字符串
	jsonBytes, err := json.Marshal(resultValue)
	if err != nil {
		return "", err
	}
	
	return string(jsonBytes), nil
}

// FirstToJSON 查询第一条记录并将结果转换为JSON字符串
// 适用于只需要查询单条记录的场景
//
// 示例用法：
//   jsonStr, err := db.Where("user_id = ?", 123).FirstToJSON(User{})
//   // 返回: '{"user_id":123, "age": 25}'
func (db *DB) FirstToJSON(dest interface{}, conds ...interface{}) (string, error) {
	// 创建目标类型的指针实例
	destType := reflect.TypeOf(dest)
	result := reflect.New(destType).Interface()
	
	// 执行First查询
	tx := db.First(result, conds...)
	if tx.Error != nil {
		return "", tx.Error
	}
	
	// 获取实际值（去除指针）
	resultValue := reflect.ValueOf(result).Elem().Interface()
	
	// 转换为JSON字符串
	jsonBytes, err := json.Marshal(resultValue)
	if err != nil {
		return "", err
	}
	
	return string(jsonBytes), nil
}

// CountToJSON 执行COUNT查询并返回JSON格式的计数结果
// 返回格式化的计数JSON字符串
//
// 示例用法：
//   jsonStr, err := db.Model(&User{}).Where("age > ?", 18).CountToJSON()
//   // 返回: '{"count":42}'
func (db *DB) CountToJSON() (string, error) {
	var count int64
	
	// 执行Count查询
	tx := db.Count(&count)
	if tx.Error != nil {
		return "", tx.Error
	}
	
	// 创建结果结构
	result := map[string]int64{"count": count}
	
	// 转换为JSON字符串
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	
	return string(jsonBytes), nil
}

// RawQueryToJSON 执行原生SQL查询并返回JSON字符串
// 支持任意的SQL查询，结果以map[string]interface{}的切片形式返回
//
// 示例用法：
//   jsonStr, err := db.RawQueryToJSON("SELECT user_id, age FROM users WHERE age > ?", 18)
//   // 返回: '[{"user_id":123, "age": 25}, {"user_id":124, "age": 30}]'
func (db *DB) RawQueryToJSON(sql string, values ...interface{}) (string, error) {
	var results []map[string]interface{}
	
	// 执行原生SQL查询
	rows, err := db.Raw(sql, values...).Rows()
	if err != nil {
		return "", err
	}
	defer rows.Close()
	
	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}
	
	// 处理查询结果
	for rows.Next() {
		// 创建扫描目标
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		
		// 扫描行数据
		if err := rows.Scan(valuePtrs...); err != nil {
			return "", err
		}
		
		// 构建结果map
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			rowMap[col] = values[i]
		}
		
		results = append(results, rowMap)
	}
	
	// 检查扫描过程中是否有错误
	if err := rows.Err(); err != nil {
		return "", err
	}
	
	// 转换为JSON字符串
	jsonBytes, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	
	return string(jsonBytes), nil
}

// AggregateToJSON 执行聚合查询并返回JSON字符串
// 支持SUM、AVG、MAX、MIN等聚合函数
//
// 示例用法：
//   jsonStr, err := db.Model(&User{}).Select("AVG(age) as avg_age, COUNT(*) as total").AggregateToJSON()
//   // 返回: '[{"avg_age": 25.5, "total": 100}]'
func (db *DB) AggregateToJSON() (string, error) {
	return db.RawQueryToJSON(db.Statement.SQL.String(), db.Statement.Vars...)
}

// QueryWithPaginationToJSON 支持分页的查询方法
// page: 页码（从1开始）
// pageSize: 每页大小
// 返回格式：'{"data": [...], "pagination": {"page": 1, "page_size": 10, "total": 100}}'
func (db *DB) QueryWithPaginationToJSON(dest interface{}, page, pageSize int, conds ...interface{}) (string, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	
	// 计算总数
	var total int64
	countTx := db.Session(&Session{}).Model(dest).Count(&total)
	if countTx.Error != nil {
		return "", countTx.Error
	}
	
	// 计算偏移量
	offset := (page - 1) * pageSize
	
	// 执行分页查询
	destType := reflect.TypeOf(dest)
	var result interface{}
	
	if destType.Kind() == reflect.Slice {
		result = reflect.New(destType).Interface()
	} else {
		sliceType := reflect.SliceOf(destType)
		result = reflect.New(sliceType).Interface()
	}
	
	tx := db.Offset(offset).Limit(pageSize).Find(result, conds...)
	if tx.Error != nil {
		return "", tx.Error
	}
	
	// 构建分页结果
	resultValue := reflect.ValueOf(result).Elem().Interface()
	paginationResult := map[string]interface{}{
		"data": resultValue,
		"pagination": map[string]interface{}{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	}
	
	// 转换为JSON字符串
	jsonBytes, err := json.Marshal(paginationResult)
	if err != nil {
		return "", err
	}
	
	return string(jsonBytes), nil
}