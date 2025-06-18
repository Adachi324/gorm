# GORM JSON 交互方式

基于GORM库实现的新型MySQL交互方式，**保持用户API不变**，但底层与数据库的交互改为调用用户自定义的函数返回JSON字符串。

## 核心理念

- **用户API不变**: 继续使用 `db.Find(&users)`, `db.Where().First()` 等熟悉的GORM API
- **底层交互抽象**: 不再直接连接MySQL，而是调用用户提供的 `ReadDB(ctx, sql)` 函数
- **JSON数据转换**: 自动将JSON字符串解析并填充到用户提供的dest结构体中

## 架构图

```
用户代码 (保持不变)
    ↓
db.Where("age > ?", 18).Find(&users)
    ↓
GORM JSON交互层
    ↓ 
ReadDB(ctx, "SELECT * FROM users WHERE age > 18") 
    ↓
返回JSON: '[{"user_id":123, "age":25}]'
    ↓
自动解析到 users 变量中
```

## 使用方法

### 1. 实现ReadDB函数

这是您需要实现的核心抽象方法：

```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    // 您的实现逻辑：
    // - HTTP API调用
    // - gRPC调用  
    // - 消息队列
    // - 其他任何方式
    
    // 示例：HTTP调用
    response, err := http.Post("https://your-api.com/query", 
        "application/sql", strings.NewReader(sql))
    if err != nil {
        return "", err
    }
    defer response.Body.Close()
    
    body, _ := io.ReadAll(response.Body)
    return string(body), nil
}
```

### 2. 初始化GORM

```go
// 使用JSON方言替代传统数据库驱动
db, err := gorm.Open(NewJSONDialector(ReadDB), &gorm.Config{})
if err != nil {
    panic("初始化失败: " + err.Error())
}

// 正常使用GORM - API完全不变！
var users []User
err = db.Where("age > ?", 18).Find(&users).Error
```

### 3. 定义模型

```go
type User struct {
    UserID int    `json:"user_id" gorm:"primaryKey"`
    Name   string `json:"name"`
    Age    int    `json:"age"`
    Email  string `json:"email"`
}
```

## 支持的操作

### 查询操作
```go
// 多条记录查询
var users []User
db.Where("age > ?", 18).Find(&users)

// 单条记录查询  
var user User
db.First(&user)
db.Where("id = ?", 123).First(&user)

// 计数查询
var count int64
db.Model(&User{}).Count(&count)

// 复杂查询
db.Where("age > ?", 18).Or("status = ?", "vip").
   Order("created_at DESC").Limit(10).Find(&users)
```

### JSON格式要求

#### 查询结果格式
```json
[
  {"user_id": 123, "name": "张三", "age": 25},
  {"user_id": 124, "name": "李四", "age": 30}
]
```

#### 计数查询格式
```json
[{"count": 42}]
```

#### 执行结果格式 (INSERT/UPDATE/DELETE)
```json
{
  "rows_affected": 1,
  "last_insert_id": 125
}
```

## 实现示例

### HTTP API方式
```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    client := &http.Client{Timeout: 30 * time.Second}
    
    payload := map[string]string{"sql": sql}
    jsonData, _ := json.Marshal(payload)
    
    req, _ := http.NewRequestWithContext(ctx, "POST", 
        "https://your-api.com/execute", bytes.NewBuffer(jsonData))
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    return string(body), nil
}
```

### gRPC方式
```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    conn, _ := grpc.Dial("your-server:9090", grpc.WithInsecure())
    defer conn.Close()
    
    client := pb.NewDatabaseServiceClient(conn)
    req := &pb.QueryRequest{Sql: sql}
    
    resp, err := client.ExecuteQuery(ctx, req)
    if err != nil {
        return "", err
    }
    
    return resp.JsonResult, nil
}
```

### 模拟实现（用于测试）
```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    fmt.Printf("执行SQL: %s\n", sql)
    
    if strings.Contains(sql, "SELECT") && strings.Contains(sql, "users") {
        return `[
            {"user_id": 123, "name": "张三", "age": 25},
            {"user_id": 124, "name": "李四", "age": 30}
        ]`, nil
    }
    
    if strings.Contains(sql, "COUNT") {
        return `[{"count": 2}]`, nil
    }
    
    return "[]", nil
}
```

## 技术实现

### 核心组件

1. **JSONDialector**: 自定义GORM方言，替换底层连接
2. **JSONConnPool**: 实现ConnPool接口，调用ReadDB函数
3. **JSONQuery**: 自定义查询回调，处理JSON响应
4. **JSONRows**: 实现Rows接口，解析JSON到结构体

### 数据流程

1. 用户调用GORM API：`db.Find(&users)`
2. GORM构建SQL：`SELECT * FROM users`
3. JSONConnPool调用：`ReadDB(ctx, sql)`
4. 解析JSON响应：`[{"user_id":123}]`
5. 自动填充到：`users` 变量

## 优势

### 对用户
- **零学习成本**: API完全不变
- **渐进迁移**: 可逐步替换现有代码
- **易于测试**: 可mock ReadDB函数

### 对架构  
- **完全解耦**: 数据库访问与业务逻辑分离
- **灵活部署**: 支持微服务、Serverless等架构
- **多种协议**: HTTP、gRPC、消息队列等

### 对运维
- **统一入口**: 所有数据库访问通过ReadDB
- **监控简化**: 只需监控ReadDB调用
- **缓存友好**: 可在ReadDB中实现缓存逻辑

## 注意事项

1. **JSON格式**: 必须严格按照规定格式返回
2. **性能考虑**: JSON序列化有一定开销
3. **错误处理**: ReadDB需要妥善处理各种异常
4. **事务限制**: 当前不支持数据库事务
5. **SQL安全**: 需要在ReadDB中处理SQL注入

## 适用场景

✅ **适合的场景**
- 微服务架构中的数据访问层
- 需要统一数据访问接口的系统
- 云原生应用的数据抽象层
- 需要数据库访问审计的场景

❌ **不适合的场景**  
- 需要复杂事务的应用
- 对性能要求极高的场景
- 重度依赖数据库特性的应用

## 扩展性

这个设计是可扩展的，您可以：

- 在ReadDB中实现缓存逻辑
- 添加数据访问权限控制
- 实现读写分离路由
- 添加SQL审计和监控
- 支持多数据源切换

## 总结

这个方案实现了一个**用户无感知的数据库交互方式变更**，保持了GORM原有的易用性，同时提供了极大的架构灵活性。您只需要实现一个 `ReadDB` 函数，就可以将任何数据源抽象为GORM的数据库交互。