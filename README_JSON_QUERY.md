# GORM JSON 交互方式

基于GORM库实现的新型MySQL交互方式，**保持用户API不变**，但底层与数据库的交互改为调用用户自定义的函数。现在支持读取(ReadDB)和写入(WriteDB)操作。

## 核心理念

- **用户API不变**: 继续使用 `db.Find(&users)`, `db.Create(&user)`, `db.Update()` 等熟悉的GORM API
- **读写分离抽象**: 读取操作调用 `ReadDB(ctx, sql)` 返回JSON，写入操作调用 `WriteDB(ctx, sql)` 返回结果
- **自动数据转换**: JSON自动解析并填充到用户提供的dest结构体中，写入结果自动处理

## 架构图

```
用户代码 (保持不变)
    ↓
db.Find(&users)              → ReadDB(ctx, sql)  → JSON → 解析到users
db.Create(&user)             → WriteDB(ctx, sql) → UpdateRet → 设置user.ID
db.Where("age > ?", 18).Find(&users) → ReadDB → JSON → 解析到users  
db.Update("age", 26)         → WriteDB(ctx, sql) → UpdateRet → 设置影响行数
```

## 使用方法

### 1. 实现ReadDB和WriteDB函数

```go
// ReadDB 处理查询操作，返回JSON字符串
func ReadDB(ctx context.Context, sql string) (string, error) {
    // 您的查询实现：HTTP API、gRPC、消息队列等
    response, err := http.Post("https://your-api.com/query", 
        "application/sql", strings.NewReader(sql))
    if err != nil {
        return "", err
    }
    defer response.Body.Close()
    
    body, _ := io.ReadAll(response.Body)
    return string(body), nil
}

// WriteDB 处理写入操作，返回UpdateRet结构
func WriteDB(ctx context.Context, sql string) (*gorm.UpdateRet, error) {
    // 您的写入实现：HTTP API、gRPC、消息队列等
    payload := map[string]string{"sql": sql}
    jsonData, _ := json.Marshal(payload)
    
    response, err := http.Post("https://your-api.com/execute", 
        "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, err
    }
    defer response.Body.Close()
    
    body, _ := io.ReadAll(response.Body)
    
    var result gorm.UpdateRet
    err = json.Unmarshal(body, &result)
    return &result, err
}
```

### 2. 初始化GORM

```go
// 使用JSON方言替代传统数据库驱动
db, err := gorm.Open(gorm.NewJSONDialector(ReadDB, WriteDB), &gorm.Config{})
if err != nil {
    panic("初始化失败: " + err.Error())
}

// 正常使用GORM - API完全不变！
// 查询操作
var users []User
err = db.Where("age > ?", 18).Find(&users).Error

// 写入操作  
user := User{Name: "张三", Age: 25}
err = db.Create(&user).Error              // user.ID 会自动设置
err = db.Model(&user).Update("age", 26).Error
err = db.Delete(&user).Error
```

### 3. 定义模型

```go
type User struct {
    ID     int    `json:"id" gorm:"primaryKey;autoIncrement"`
    Name   string `json:"name"`
    Age    int    `json:"age"`
    Email  string `json:"email"`
}
```

## 支持的操作

### 查询操作 (调用ReadDB)
```go
// 基本查询
var users []User
db.Find(&users)
db.First(&user)
db.Where("age > ?", 18).Find(&users)

// 复杂查询
db.Where("age > ?", 18).Or("status = ?", "vip").
   Order("created_at DESC").Limit(10).Find(&users)

// 计数查询
var count int64
db.Model(&User{}).Count(&count)

// 原生SQL
db.Raw("SELECT * FROM users WHERE age > ?", 18).Scan(&users)
```

### 写入操作 (调用WriteDB)
```go
// 创建
db.Create(&user)                    // 单个创建
db.Create(&users)                   // 批量创建

// 更新
db.Save(&user)                      // 保存所有字段
db.Model(&user).Update("age", 26)   // 更新单个字段
db.Model(&user).Updates(User{Age: 26, Name: "新名字"})  // 更新多个字段
db.Where("age < ?", 18).Updates(User{Status: "minor"}) // 条件更新

// 删除
db.Delete(&user)                    // 按主键删除
db.Delete(&User{}, 1)               // 按ID删除
db.Where("age < ?", 13).Delete(&User{})  // 条件删除

// Upsert
db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&user)
```

## 数据格式

### ReadDB返回格式

```json
[
  {"id": 123, "name": "张三", "age": 25, "email": "zhangsan@example.com"},
  {"id": 124, "name": "李四", "age": 30, "email": "lisi@example.com"}
]
```

计数查询：
```json
[{"count": 42}]
```

### WriteDB返回格式

```go
type UpdateRet struct {
    InsertId     int64  `json:"insert_id"`      // 新插入记录的ID
    AffectedRows int64  `json:"affected_rows"`  // 受影响的行数
    ServerStatus int32  `json:"server_status"`  // 服务器状态
    WarningCount int64  `json:"warning_count"`  // 警告数量
    Message      string `json:"message"`        // 消息
}
```

INSERT操作：
```json
{
  "insert_id": 125,
  "affected_rows": 1,
  "server_status": 2,
  "warning_count": 0,
  "message": ""
}
```

UPDATE/DELETE操作：
```json
{
  "insert_id": 0,
  "affected_rows": 3,
  "server_status": 2,
  "warning_count": 0,
  "message": ""
}
```

## 实现示例

### HTTP API方式
```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    client := &http.Client{Timeout: 30 * time.Second}
    
    req, _ := http.NewRequestWithContext(ctx, "POST", 
        "https://your-api.com/query", strings.NewReader(sql))
    req.Header.Set("Content-Type", "application/sql")
    req.Header.Set("Authorization", "Bearer your-token")
    
    resp, err := client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    return string(body), nil
}

func WriteDB(ctx context.Context, sql string) (*gorm.UpdateRet, error) {
    client := &http.Client{Timeout: 30 * time.Second}
    
    payload := map[string]string{"sql": sql}
    jsonData, _ := json.Marshal(payload)
    
    req, _ := http.NewRequestWithContext(ctx, "POST", 
        "https://your-api.com/execute", bytes.NewBuffer(jsonData))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer your-token")
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result gorm.UpdateRet
    json.NewDecoder(resp.Body).Decode(&result)
    return &result, nil
}
```

### 模拟实现（测试用）
```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    if strings.Contains(sql, "SELECT") && strings.Contains(sql, "users") {
        return `[
            {"id": 1, "name": "张三", "age": 25},
            {"id": 2, "name": "李四", "age": 30}
        ]`, nil
    }
    if strings.Contains(sql, "COUNT") {
        return `[{"count": 2}]`, nil
    }
    return "[]", nil
}

func WriteDB(ctx context.Context, sql string) (*gorm.UpdateRet, error) {
    result := &gorm.UpdateRet{ServerStatus: 2}
    
    if strings.Contains(sql, "INSERT") {
        result.InsertId = 123
        result.AffectedRows = 1
    } else if strings.Contains(sql, "UPDATE") || strings.Contains(sql, "DELETE") {
        result.AffectedRows = 1
    }
    
    return result, nil
}
```

## 技术实现

### 核心组件

1. **JSONDialector**: 自定义GORM方言，替换底层连接
2. **JSONConnPool**: 实现ConnPool接口，分别调用ReadDB和WriteDB
3. **JSONQuery**: 查询回调，处理JSON响应并解析到结构体
4. **JSONCreate/Update/Delete**: 写入回调，调用WriteDB并处理结果

### 数据流程

**查询流程**:
1. 用户调用: `db.Find(&users)`
2. GORM构建SQL: `SELECT * FROM users`
3. JSONConnPool调用: `ReadDB(ctx, sql)`
4. 解析JSON: `[{"id":123}]`
5. 自动填充: `users` 变量

**写入流程**:
1. 用户调用: `db.Create(&user)`
2. GORM构建SQL: `INSERT INTO users ...`
3. JSONConnPool调用: `WriteDB(ctx, sql)`
4. 处理结果: `UpdateRet{InsertId: 123}`
5. 自动设置: `user.ID = 123`

## 优势

### 对用户
- **零学习成本**: API完全不变
- **完整功能**: 支持所有CRUD操作
- **类型安全**: 保持GORM的类型安全特性
- **自动映射**: 自动处理JSON到结构体的转换

### 对架构  
- **完全解耦**: 数据库访问与业务逻辑分离
- **读写分离**: ReadDB和WriteDB可以连接不同服务
- **灵活部署**: 支持微服务、Serverless等架构
- **多种协议**: HTTP、gRPC、消息队列等

### 对运维
- **统一入口**: 所有数据库访问通过ReadDB/WriteDB
- **监控简化**: 只需监控两个函数调用
- **缓存友好**: 可在函数中实现缓存逻辑
- **审计便利**: 统一的SQL审计和权限控制

## 注意事项

1. **数据格式**: 必须严格按照规定格式返回JSON和UpdateRet
2. **ID回填**: INSERT操作会自动将insert_id设置到结构体主键字段
3. **错误处理**: ReadDB和WriteDB需要妥善处理各种异常
4. **并发安全**: 确保两个函数都是并发安全的
5. **事务限制**: 当前不支持数据库事务（可在函数内部实现）
6. **性能考虑**: JSON序列化有一定开销，适合大部分业务场景

## 适用场景

✅ **适合的场景**
- 微服务架构中的数据访问层
- 需要统一数据访问接口的系统  
- 云原生应用的数据抽象层
- 需要数据库访问审计的场景
- API网关后的数据服务
- 读写分离的架构

❌ **不适合的场景**  
- 需要复杂事务的应用
- 对性能要求极高的场景
- 重度依赖数据库特性的应用

## 扩展性

这个设计是高度可扩展的：

- **缓存层**: 在ReadDB中实现查询缓存
- **权限控制**: 在函数中添加SQL权限检查
- **读写分离**: ReadDB和WriteDB连接不同的服务
- **负载均衡**: 函数内部实现多服务负载均衡
- **监控审计**: 统一的SQL执行监控和审计
- **数据转换**: 函数中实现数据格式转换

## 总结

这个方案实现了**用户无感知的数据库交互方式变更**，保持了GORM原有的易用性和完整功能，同时提供了极大的架构灵活性。您只需要实现 `ReadDB` 和 `WriteDB` 两个函数，就可以将任何数据源抽象为完整的GORM数据库交互，支持所有CRUD操作。