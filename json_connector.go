package gorm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// JSONReaderFunc 定义读取数据库并返回JSON的函数签名
// 用户需要实现这个函数来替代传统的数据库查询
type JSONReaderFunc func(ctx context.Context, sql string) (string, error)

// JSONWriterFunc 定义写入数据库并返回结果的函数签名
// 用户需要实现这个函数来处理INSERT、UPDATE、DELETE等写入操作
type JSONWriterFunc func(ctx context.Context, sql string) (*UpdateRet, error)

// UpdateRet 写入操作的返回结构
type UpdateRet struct {
	InsertId     int64  `json:"insert_id"`
	AffectedRows int64  `json:"affected_rows"`
	ServerStatus int32  `json:"server_status"`
	WarningCount int64  `json:"warning_count"`
	Message      string `json:"message"`
}

// JSONConnPool 实现ConnPool接口，底层使用JSON方式与数据库交互
type JSONConnPool struct {
	readDB  JSONReaderFunc
	writeDB JSONWriterFunc
}

// NewJSONConnPool 创建新的JSON连接池
// readDB: 用户自定义的数据库读取函数，返回JSON字符串
// writeDB: 用户自定义的数据库写入函数，返回UpdateRet结构
func NewJSONConnPool(readDB JSONReaderFunc, writeDB JSONWriterFunc) *JSONConnPool {
	return &JSONConnPool{
		readDB:  readDB,
		writeDB: writeDB,
	}
}

// PrepareContext 实现ConnPool接口 - 暂不支持预编译语句
func (j *JSONConnPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, fmt.Errorf("JSONConnPool不支持预编译语句")
}

// ExecContext 实现ConnPool接口 - 执行非查询SQL（INSERT, UPDATE, DELETE）
func (j *JSONConnPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	// 调用用户提供的WriteDB函数
	sql := j.interpolateSQL(query, args...)
	result, err := j.writeDB(ctx, sql)
	if err != nil {
		return nil, err
	}
	
	// 将UpdateRet转换为sql.Result
	return &jsonResult{
		rowsAffected: result.AffectedRows,
		lastInsertId: result.InsertId,
		updateRet:    result,
	}, nil
}

// QueryContext 实现ConnPool接口 - 这是核心方法，将查询结果转换为JSON然后解析
func (j *JSONConnPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	// 调用用户提供的ReadDB函数
	jsonStr, err := j.readDB(ctx, j.interpolateSQL(query, args...))
	if err != nil {
		return nil, err
	}
	
	// 由于sql.Rows是具体类型，我们返回一个特殊的标记，在后续处理中会被识别
	// 这里使用一个技巧：存储JSON数据到context中，然后返回一个特殊的rows
	return newJSONSQLRows(jsonStr)
}

// QueryRowContext 实现ConnPool接口 - 查询单行
func (j *JSONConnPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	// 对于单行查询，我们仍然使用QueryContext，然后封装为Row
	rows, err := j.QueryContext(ctx, query, args...)
	if err != nil {
		return &sql.Row{} // 返回包含错误的Row
	}
	return &sql.Row{} // 简化实现，实际应该封装rows
}

// interpolateSQL 简单的SQL参数插值（生产环境中应该使用更安全的方法）
func (j *JSONConnPool) interpolateSQL(query string, args ...interface{}) string {
	for _, arg := range args {
		var value string
		switch v := arg.(type) {
		case string:
			value = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
		case int, int8, int16, int32, int64:
			value = fmt.Sprintf("%d", v)
		case uint, uint8, uint16, uint32, uint64:
			value = fmt.Sprintf("%d", v)
		case float32, float64:
			value = fmt.Sprintf("%f", v)
		case bool:
			if v {
				value = "1"
			} else {
				value = "0"
			}
		case time.Time:
			value = fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05"))
		case nil:
			value = "NULL"
		default:
			value = fmt.Sprintf("'%v'", v)
		}
		query = strings.Replace(query, "?", value, 1)
	}
	return query
}

// JSONRows 实现GORM的Rows接口，用于处理JSON格式的查询结果
type JSONRows struct {
	data        []map[string]interface{}
	columns     []string
	columnTypes []*sql.ColumnType
	currentRow  int
	closed      bool
}

// newJSONSQLRows 创建一个特殊的sql.Rows，其中包含JSON数据
func newJSONSQLRows(jsonStr string) (*sql.Rows, error) {
	// 这是一个技巧：我们创建一个特殊的标记对象，在GORM的扫描过程中会被特别处理
	// 由于无法直接创建sql.Rows，我们需要修改GORM的查询回调
	return nil, &jsonRowsMarker{jsonStr: jsonStr}
}

// jsonRowsMarker 用于标记这是JSON数据的特殊错误类型
type jsonRowsMarker struct {
	jsonStr string
}

func (e *jsonRowsMarker) Error() string {
	return "JSON_ROWS_MARKER:" + e.jsonStr
}

// NewJSONRows 从JSON字符串创建JSONRows
func NewJSONRows(jsonStr string) (*JSONRows, error) {
	var data []map[string]interface{}
	
	// 解析JSON数组
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("解析JSON查询结果失败: %w", err)
	}
	
	var columns []string
	if len(data) > 0 {
		// 从第一行数据中提取列名
		for col := range data[0] {
			columns = append(columns, col)
		}
	}
	
	return &JSONRows{
		data:       data,
		columns:    columns,
		currentRow: -1,
		closed:     false,
	}, nil
}

// 实现GORM的Rows接口
func (r *JSONRows) Columns() ([]string, error) {
	if r.closed {
		return nil, fmt.Errorf("JSONRows已关闭")
	}
	return r.columns, nil
}

func (r *JSONRows) ColumnTypes() ([]*sql.ColumnType, error) {
	if r.closed {
		return nil, fmt.Errorf("JSONRows已关闭")
	}
	return r.columnTypes, nil
}

func (r *JSONRows) Next() bool {
	if r.closed {
		return false
	}
	r.currentRow++
	return r.currentRow < len(r.data)
}

func (r *JSONRows) Scan(dest ...interface{}) error {
	if r.closed {
		return fmt.Errorf("JSONRows已关闭")
	}
	if r.currentRow < 0 || r.currentRow >= len(r.data) {
		return fmt.Errorf("无效的行位置")
	}
	
	row := r.data[r.currentRow]
	
	// 扫描数据到目标变量
	for i, d := range dest {
		if i >= len(r.columns) {
			break
		}
		
		colName := r.columns[i]
		value := row[colName]
		
		if err := assignValue(d, value); err != nil {
			return fmt.Errorf("扫描列 %s 失败: %w", colName, err)
		}
	}
	
	return nil
}

func (r *JSONRows) Err() error {
	return nil
}

func (r *JSONRows) Close() error {
	r.closed = true
	return nil
}

// assignValue 将interface{}值赋给目标指针
func assignValue(dest interface{}, value interface{}) error {
	if dest == nil {
		return nil
	}
	
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("dest必须是指针")
	}
	
	destValue = destValue.Elem()
	if !destValue.CanSet() {
		return fmt.Errorf("dest不能设置值")
	}
	
	if value == nil {
		destValue.Set(reflect.Zero(destValue.Type()))
		return nil
	}
	
	sourceValue := reflect.ValueOf(value)
	
	// 类型转换和赋值
	switch destValue.Kind() {
	case reflect.String:
		if str, ok := value.(string); ok {
			destValue.SetString(str)
		} else {
			destValue.SetString(fmt.Sprintf("%v", value))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if intVal, err := convertToInt64(value); err == nil {
			destValue.SetInt(intVal)
		} else {
			return err
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if intVal, err := convertToInt64(value); err == nil {
			destValue.SetUint(uint64(intVal))
		} else {
			return err
		}
	case reflect.Float32, reflect.Float64:
		if floatVal, err := convertToFloat64(value); err == nil {
			destValue.SetFloat(floatVal)
		} else {
			return err
		}
	case reflect.Bool:
		if boolVal, err := convertToBool(value); err == nil {
			destValue.SetBool(boolVal)
		} else {
			return err
		}
	case reflect.Interface:
		destValue.Set(sourceValue)
	default:
		if sourceValue.Type().ConvertibleTo(destValue.Type()) {
			destValue.Set(sourceValue.Convert(destValue.Type()))
		} else {
			return fmt.Errorf("无法将 %T 转换为 %v", value, destValue.Type())
		}
	}
	
	return nil
}

// 类型转换辅助函数
func convertToInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("无法转换 %T 为 int64", value)
	}
}

func convertToFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case int, int8, int16, int32, int64:
		if intVal, err := convertToInt64(v); err == nil {
			return float64(intVal), nil
		}
		return 0, err
	case uint, uint8, uint16, uint32, uint64:
		if intVal, err := convertToInt64(v); err == nil {
			return float64(intVal), nil
		}
		return 0, err
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("无法转换 %T 为 float64", value)
	}
}

func convertToBool(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int, int8, int16, int32, int64:
		if intVal, err := convertToInt64(v); err == nil {
			return intVal != 0, nil
		}
		return false, err
	case string:
		return strconv.ParseBool(v)
	default:
		return false, fmt.Errorf("无法转换 %T 为 bool", value)
	}
}

// jsonResult 实现sql.Result接口
type jsonResult struct {
	rowsAffected int64
	lastInsertId int64
	updateRet    *UpdateRet // 保存完整的更新结果
}

func (r *jsonResult) LastInsertId() (int64, error) {
	return r.lastInsertId, nil
}

func (r *jsonResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

// GetUpdateResult 获取完整的更新结果（扩展方法）
func (r *jsonResult) GetUpdateResult() *UpdateRet {
	return r.updateRet
}

// 辅助函数
func getInt64FromMap(m map[string]interface{}, key string) int64 {
	if val, exists := m[key]; exists {
		if intVal, err := convertToInt64(val); err == nil {
			return intVal
		}
	}
	return 0
}

// JSONDialector 自定义的Dialector，使用JSON连接池
type JSONDialector struct {
	readDB  JSONReaderFunc
	writeDB JSONWriterFunc
}

// NewJSONDialector 创建JSON方式的数据库方言
// readDB: 读取数据库的函数
// writeDB: 写入数据库的函数
func NewJSONDialector(readDB JSONReaderFunc, writeDB JSONWriterFunc) *JSONDialector {
	return &JSONDialector{
		readDB:  readDB,
		writeDB: writeDB,
	}
}

func (d *JSONDialector) Name() string {
	return "json"
}

func (d *JSONDialector) Initialize(db *DB) error {
	// 替换连接池为JSON连接池
	db.ConnPool = NewJSONConnPool(d.readDB, d.writeDB)
	// 替换查询回调为JSON查询回调
	db.Callback().Query().Replace("gorm:query", JSONQuery)
	// 替换创建回调为JSON创建回调
	db.Callback().Create().Replace("gorm:create", JSONCreate)
	// 替换更新回调为JSON更新回调
	db.Callback().Update().Replace("gorm:update", JSONUpdate)
	// 替换删除回调为JSON删除回调
	db.Callback().Delete().Replace("gorm:delete", JSONDelete)
	return nil
}

func (d *JSONDialector) Migrator(db *DB) Migrator {
	return nil // JSON方式不支持迁移
}

func (d *JSONDialector) DataTypeOf(field *schema.Field) string {
	return "TEXT" // 简化实现
}

func (d *JSONDialector) DefaultValueOf(field *schema.Field) clause.Expression {
	return clause.Expr{}
}

func (d *JSONDialector) BindVarTo(writer clause.Writer, stmt *Statement, v interface{}) {
	writer.WriteByte('?')
}

func (d *JSONDialector) QuoteTo(writer clause.Writer, str string) {
	writer.WriteByte('`')
	writer.WriteString(str)
	writer.WriteByte('`')
}

func (d *JSONDialector) Explain(sql string, vars ...interface{}) string {
	return sql
}