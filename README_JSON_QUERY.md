# GORM JSON查询接口

基于GORM库实现的新型MySQL交互方式，直接返回JSON字符串，简化数据处理流程。

## 概述

这个扩展为GORM提供了一组新的方法，可以直接将查询结果转换为JSON字符串返回，无需手动处理结构体到JSON的转换过程。特别适用于API开发、数据导出、前后端分离等场景。

## 核心特性

- **简化交互**：一步完成查询和JSON转换
- **类型安全**：基于结构体定义，保持类型安全
- **链式调用**：完全兼容GORM的链式API
- **多种查询**：支持普通查询、分页查询、聚合查询、原生SQL
- **错误处理**：统一的错误处理机制

## 主要方法

### 1. QueryToJSON - 多条记录查询

```go
jsonStr, err := db.Where("age > ?", 18).QueryToJSON([]User{})
// 返回: '[{"user_id":123, "age": 25}, {"user_id":124, "age": 30}]'
```

### 2. FirstToJSON - 单条记录查询

```go
jsonStr, err := db.Where("user_id = ?", 123).FirstToJSON(User{})
// 返回: '{"user_id":123, "age": 25}'
```

### 3. CountToJSON - 计数查询

```go
jsonStr, err := db.Model(&User{}).Where("status = ?", "active").CountToJSON()
// 返回: '{"count":42}'
```

### 4. RawQueryToJSON - 原生SQL查询

```go
jsonStr, err := db.RawQueryToJSON("SELECT user_id, age FROM users WHERE age > ?", 18)
// 返回: '[{"user_id":123, "age": 25}, {"user_id":124, "age": 30}]'
```

### 5. QueryWithPaginationToJSON - 分页查询

```go
jsonStr, err := db.QueryWithPaginationToJSON([]User{}, 1, 10)
// 返回: '{"data": [...], "pagination": {"page": 1, "page_size": 10, "total": 100}}'
```

## 使用示例

### 基础使用

```go
type User struct {
    UserID int    `json:"user_id" gorm:"primaryKey"`
    Name   string `json:"name"`
    Age    int    `json:"age"`
    Email  string `json:"email"`
}

// 查询所有成年用户
jsonStr, err := db.Where("age >= ?", 18).QueryToJSON([]User{})
if err != nil {
    log.Printf("查询失败: %v", err)
    return
}
fmt.Println(jsonStr)
```

### 复杂查询

```go
// 复合条件查询
jsonStr, err := db.Where("age > ? AND status = ?", 21, "active").
    Or("email LIKE ?", "%@gmail.com").
    Order("age DESC").
    Limit(10).
    QueryToJSON([]User{})
```

### API开发场景

```go
func GetUsersHandler(w http.ResponseWriter, r *http.Request) {
    ageLimit := r.URL.Query().Get("age_limit")
    status := r.URL.Query().Get("status")
    
    jsonStr, err := db.Where("age >= ? AND status = ?", ageLimit, status).
        Order("user_id DESC").
        Limit(50).
        QueryToJSON([]User{})
    
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.Write([]byte(jsonStr))
}
```

### 分页API

```go
func GetUsersWithPaginationHandler(w http.ResponseWriter, r *http.Request) {
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
    
    if page < 1 { page = 1 }
    if pageSize < 1 { pageSize = 10 }
    
    jsonStr, err := db.QueryWithPaginationToJSON([]User{}, page, pageSize)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.Write([]byte(jsonStr))
}
```

### 数据统计

```go
// 用户统计
statsJSON, err := db.RawQueryToJSON(`
    SELECT 
        COUNT(*) as total_users,
        COUNT(CASE WHEN status = 'active' THEN 1 END) as active_users,
        AVG(age) as average_age
    FROM users
`)
```

## 性能优化建议

### 1. 大数据量处理
```go
// 推荐：使用分页查询
jsonStr, err := db.QueryWithPaginationToJSON([]User{}, page, pageSize)

// 避免：一次性查询大量数据
jsonStr, err := db.QueryToJSON([]User{}) // 可能返回数万条记录
```

### 2. 复杂查询优化
```go
// 推荐：使用原生SQL
jsonStr, err := db.RawQueryToJSON(`
    SELECT u.name, u.age, COUNT(o.id) as order_count
    FROM users u
    LEFT JOIN orders o ON u.id = o.user_id
    WHERE u.status = 'active'
    GROUP BY u.id
    HAVING order_count > 5
`, params...)

// 避免：复杂的ORM查询
jsonStr, err := db.Model(&User{}).
    Select("users.name, users.age, COUNT(orders.id) as order_count").
    Joins("LEFT JOIN orders ON users.id = orders.user_id").
    Where("users.status = ?", "active").
    Group("users.id").
    Having("COUNT(orders.id) > ?", 5).
    QueryToJSON([]UserWithOrderCount{})
```

### 3. 字段选择
```go
// 推荐：只选择需要的字段
jsonStr, err := db.Select("user_id, name, age").QueryToJSON([]User{})

// 避免：选择所有字段
jsonStr, err := db.QueryToJSON([]User{}) // 包含所有字段
```

## 错误处理

### 标准错误处理
```go
jsonStr, err := db.Where("user_id = ?", id).FirstToJSON(User{})
if err != nil {
    if errors.Is(err, gorm.ErrRecordNotFound) {
        // 记录不存在的处理
        return "", fmt.Errorf("用户不存在")
    }
    // 其他数据库错误
    return "", fmt.Errorf("查询失败: %w", err)
}
```

### 业务层封装
```go
type UserService struct {
    db *gorm.DB
}

func (s *UserService) GetUser(userID int) (string, error) {
    jsonStr, err := s.db.Where("user_id = ?", userID).FirstToJSON(User{})
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return `{"error": "用户不存在"}`, nil
        }
        log.Printf("查询用户失败: %v", err)
        return "", err
    }
    return jsonStr, nil
}
```

## 与传统方式的对比

### 传统方式
```go
var users []User
err := db.Where("age > ?", 18).Find(&users).Error
if err != nil {
    return "", err
}

jsonBytes, err := json.Marshal(users)
if err != nil {
    return "", err
}

return string(jsonBytes), nil
```

### 新的JSON查询方式
```go
return db.Where("age > ?", 18).QueryToJSON([]User{})
```

## 注意事项

1. **JSON标签**：确保结构体字段有正确的`json`标签
2. **内存使用**：大数据量查询时注意内存占用
3. **索引优化**：WHERE条件字段应该有适当的索引
4. **错误处理**：始终检查并处理返回的错误
5. **类型安全**：虽然返回字符串，但查询仍基于结构体定义

## 兼容性

- **GORM版本**：兼容GORM v1.25+
- **Go版本**：需要Go 1.18+
- **数据库**：支持MySQL、PostgreSQL、SQLite等GORM支持的数据库

## 扩展性

这个接口设计为可扩展的，您可以根据需要添加更多方法：

```go
// 自定义扩展示例
func (db *DB) QueryToXML(dest interface{}, conds ...interface{}) (string, error) {
    // 实现XML格式输出
}

func (db *DB) QueryToCSV(dest interface{}, conds ...interface{}) (string, error) {
    // 实现CSV格式输出
}
```

这个JSON查询接口提供了一种简洁、高效的数据库交互方式，特别适用于现代Web API开发。