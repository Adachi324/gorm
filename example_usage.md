# GORM JSON 交互方式使用示例

这个方案保持GORM用户API不变，但底层与MySQL的交互改为使用JSON方式。现在支持读取(ReadDB)和写入(WriteDB)操作。

## 核心概念

用户依然使用熟悉的GORM API：
```go
db.Find(&users)                    // 查询操作 -> 调用ReadDB
db.Create(&user)                   // 创建操作 -> 调用WriteDB
db.Where("age > ?", 18).Find(&users)  // 条件查询 -> 调用ReadDB
db.Save(&user)                     // 保存操作 -> 调用WriteDB
```

但底层不再直接连接MySQL，而是调用用户提供的 `ReadDB` 和 `WriteDB` 函数。

## 使用步骤

### 1. 定义ReadDB和WriteDB函数

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "gorm.io/gorm"
    // 你的HTTP客户端或其他数据源
)

// ReadDB 处理查询操作，返回JSON字符串
func ReadDB(ctx context.Context, sql string) (string, error) {
    // 这里是您的查询实现逻辑
    // 可以是HTTP调用、消息队列、或其他任何方式
    
    // 示例1: HTTP调用
    response, err := httpClient.Post("https://your-api.com/query", "application/sql", strings.NewReader(sql))
    if err != nil {
        return "", err
    }
    defer response.Body.Close()
    
    body, err := io.ReadAll(response.Body)
    if err != nil {
        return "", err
    }
    
    return string(body), nil
}

// WriteDB 处理写入操作（INSERT、UPDATE、DELETE），返回UpdateRet结构
func WriteDB(ctx context.Context, sql string) (*gorm.UpdateRet, error) {
    // 这里是您的写入实现逻辑
    
    // 示例1: HTTP调用
    payload := map[string]string{"sql": sql}
    jsonData, _ := json.Marshal(payload)
    
    response, err := httpClient.Post("https://your-api.com/execute", "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, err
    }
    defer response.Body.Close()
    
    body, err := io.ReadAll(response.Body)
    if err != nil {
        return nil, err
    }
    
    var result gorm.UpdateRet
    err = json.Unmarshal(body, &result)
    if err != nil {
        return nil, err
    }
    
    return &result, nil
}
```

### 2. 初始化GORM使用JSON方式

```go
package main

import (
    "gorm.io/gorm"
)

func main() {
    // 使用JSON方言初始化GORM，传入ReadDB和WriteDB函数
    db, err := gorm.Open(gorm.NewJSONDialector(ReadDB, WriteDB), &gorm.Config{})
    if err != nil {
        panic("初始化数据库失败: " + err.Error())
    }

    // 现在您可以像往常一样使用GORM
    
    // 查询操作 - 调用ReadDB
    var users []User
    err = db.Where("age > ?", 18).Find(&users).Error
    if err != nil {
        fmt.Printf("查询失败: %v\n", err)
        return
    }
    
    // 创建操作 - 调用WriteDB
    user := User{Name: "张三", Age: 25, Email: "zhangsan@example.com"}
    err = db.Create(&user).Error
    if err != nil {
        fmt.Printf("创建失败: %v\n", err)
        return
    }
    fmt.Printf("创建用户成功，ID: %d\n", user.ID)
    
    // 更新操作 - 调用WriteDB
    err = db.Model(&user).Update("age", 26).Error
    if err != nil {
        fmt.Printf("更新失败: %v\n", err)
        return
    }
    
    // 删除操作 - 调用WriteDB
    err = db.Delete(&user).Error
    if err != nil {
        fmt.Printf("删除失败: %v\n", err)
        return
    }
}
```

### 3. 定义模型结构

```go
type User struct {
    ID     int    `json:"id" gorm:"primaryKey;autoIncrement"`
    Name   string `json:"name"`
    Age    int    `json:"age"`
    Email  string `json:"email"`
}

// TableName 指定表名
func (User) TableName() string {
    return "users"
}
```

## ReadDB和WriteDB函数的实现方式

### 方式1: HTTP API调用

```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    client := &http.Client{Timeout: 30 * time.Second}
    
    payload := map[string]string{"sql": sql, "type": "query"}
    jsonData, _ := json.Marshal(payload)
    
    req, err := http.NewRequestWithContext(ctx, "POST", 
        "https://your-database-api.com/execute", 
        bytes.NewBuffer(jsonData))
    if err != nil {
        return "", err
    }
    
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer your-token")
    
    resp, err := client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("API调用失败: %d", resp.StatusCode)
    }
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", err
    }
    
    return string(body), nil
}

func WriteDB(ctx context.Context, sql string) (*gorm.UpdateRet, error) {
    client := &http.Client{Timeout: 30 * time.Second}
    
    payload := map[string]string{"sql": sql, "type": "execute"}
    jsonData, _ := json.Marshal(payload)
    
    req, err := http.NewRequestWithContext(ctx, "POST", 
        "https://your-database-api.com/execute", 
        bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer your-token")
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("API调用失败: %d", resp.StatusCode)
    }
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }
    
    var result gorm.UpdateRet
    err = json.Unmarshal(body, &result)
    if err != nil {
        return nil, fmt.Errorf("解析响应失败: %w", err)
    }
    
    return &result, nil
}
```

### 方式2: gRPC调用

```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    conn, err := grpc.Dial("your-grpc-server:9090", grpc.WithInsecure())
    if err != nil {
        return "", err
    }
    defer conn.Close()
    
    client := pb.NewDatabaseServiceClient(conn)
    
    req := &pb.QueryRequest{Sql: sql}
    resp, err := client.ExecuteQuery(ctx, req)
    if err != nil {
        return "", err
    }
    
    return resp.JsonResult, nil
}

func WriteDB(ctx context.Context, sql string) (*gorm.UpdateRet, error) {
    conn, err := grpc.Dial("your-grpc-server:9090", grpc.WithInsecure())
    if err != nil {
        return nil, err
    }
    defer conn.Close()
    
    client := pb.NewDatabaseServiceClient(conn)
    
    req := &pb.ExecuteRequest{Sql: sql}
    resp, err := client.ExecuteUpdate(ctx, req)
    if err != nil {
        return nil, err
    }
    
    return &gorm.UpdateRet{
        InsertId:     resp.InsertId,
        AffectedRows: resp.AffectedRows,
        ServerStatus: resp.ServerStatus,
        WarningCount: resp.WarningCount,
        Message:      resp.Message,
    }, nil
}
```

### 方式3: 模拟实现（用于测试）

```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    fmt.Printf("执行查询SQL: %s\n", sql)
    
    if strings.Contains(sql, "SELECT") && strings.Contains(sql, "users") {
        if strings.Contains(sql, "age > 18") {
            return `[
                {"id": 1, "name": "张三", "age": 25, "email": "zhangsan@example.com"},
                {"id": 2, "name": "李四", "age": 30, "email": "lisi@example.com"}
            ]`, nil
        }
        return `[
            {"id": 1, "name": "张三", "age": 25, "email": "zhangsan@example.com"},
            {"id": 2, "name": "李四", "age": 30, "email": "lisi@example.com"},
            {"id": 3, "name": "王五", "age": 16, "email": "wangwu@example.com"}
        ]`, nil
    }
    
    if strings.Contains(sql, "COUNT") {
        return `[{"count": 3}]`, nil
    }
    
    return "[]", nil
}

func WriteDB(ctx context.Context, sql string) (*gorm.UpdateRet, error) {
    fmt.Printf("执行写入SQL: %s\n", sql)
    
    result := &gorm.UpdateRet{
        ServerStatus: 2,
        WarningCount: 0,
        Message:      "",
    }
    
    if strings.Contains(sql, "INSERT") {
        result.InsertId = 123     // 模拟插入ID
        result.AffectedRows = 1
    } else if strings.Contains(sql, "UPDATE") {
        result.AffectedRows = 1
    } else if strings.Contains(sql, "DELETE") {
        result.AffectedRows = 1
    }
    
    return result, nil
}
```

## 期望的JSON格式

### ReadDB返回格式

#### 查询结果格式
```json
[
  {"id": 123, "name": "张三", "age": 25, "email": "zhangsan@example.com"},
  {"id": 124, "name": "李四", "age": 30, "email": "lisi@example.com"}
]
```

#### COUNT查询结果格式
```json
[
  {"count": 42}
]
```

### WriteDB返回格式

WriteDB应该返回`*gorm.UpdateRet`结构：

```go
type UpdateRet struct {
    InsertId     int64  `json:"insert_id"`      // 插入操作的新记录ID
    AffectedRows int64  `json:"affected_rows"`  // 受影响的行数
    ServerStatus int32  `json:"server_status"`  // 服务器状态
    WarningCount int64  `json:"warning_count"`  // 警告数量
    Message      string `json:"message"`        // 消息
}
```

#### INSERT操作返回示例
```json
{
  "insert_id": 125,
  "affected_rows": 1,
  "server_status": 2,
  "warning_count": 0,
  "message": ""
}
```

#### UPDATE/DELETE操作返回示例
```json
{
  "insert_id": 0,
  "affected_rows": 3,
  "server_status": 2,
  "warning_count": 0,
  "message": ""
}
```

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "strings"
    "gorm.io/gorm"
)

type User struct {
    ID    int    `json:"id" gorm:"primaryKey;autoIncrement"`
    Name  string `json:"name"`
    Age   int    `json:"age"`
    Email string `json:"email"`
}

// 模拟的ReadDB实现
func ReadDB(ctx context.Context, sql string) (string, error) {
    fmt.Printf("执行查询SQL: %s\n", sql)
    
    if contains(sql, "SELECT") && contains(sql, "users") {
        if contains(sql, "age > 18") {
            return `[
                {"id": 1, "name": "张三", "age": 25, "email": "zhangsan@example.com"},
                {"id": 2, "name": "李四", "age": 30, "email": "lisi@example.com"}
            ]`, nil
        }
        return `[
            {"id": 1, "name": "张三", "age": 25, "email": "zhangsan@example.com"},
            {"id": 2, "name": "李四", "age": 30, "email": "lisi@example.com"},
            {"id": 3, "name": "王五", "age": 16, "email": "wangwu@example.com"}
        ]`, nil
    }
    
    if contains(sql, "COUNT") {
        return `[{"count": 3}]`, nil
    }
    
    return "[]", nil
}

// 模拟的WriteDB实现
func WriteDB(ctx context.Context, sql string) (*gorm.UpdateRet, error) {
    fmt.Printf("执行写入SQL: %s\n", sql)
    
    result := &gorm.UpdateRet{
        ServerStatus: 2,
        WarningCount: 0,
        Message:      "",
    }
    
    if contains(sql, "INSERT") {
        result.InsertId = 4       // 模拟新插入的ID
        result.AffectedRows = 1
    } else if contains(sql, "UPDATE") {
        result.AffectedRows = 1
    } else if contains(sql, "DELETE") {
        result.AffectedRows = 1
    }
    
    return result, nil
}

func contains(s, substr string) bool {
    return strings.Contains(strings.ToUpper(s), strings.ToUpper(substr))
}

func main() {
    // 使用JSON方言初始化GORM
    db, err := gorm.Open(gorm.NewJSONDialector(ReadDB, WriteDB), &gorm.Config{})
    if err != nil {
        panic("初始化失败: " + err.Error())
    }

    // 1. 查询操作
    fmt.Println("=== 查询所有用户 ===")
    var allUsers []User
    err = db.Find(&allUsers).Error
    if err != nil {
        fmt.Printf("查询失败: %v\n", err)
    } else {
        fmt.Printf("查询到 %d 个用户: %+v\n", len(allUsers), allUsers)
    }

    // 2. 创建操作
    fmt.Println("\n=== 创建新用户 ===")
    newUser := User{Name: "赵六", Age: 28, Email: "zhaoliu@example.com"}
    err = db.Create(&newUser).Error
    if err != nil {
        fmt.Printf("创建失败: %v\n", err)
    } else {
        fmt.Printf("创建成功，新用户ID: %d, 完整信息: %+v\n", newUser.ID, newUser)
    }

    // 3. 批量创建
    fmt.Println("\n=== 批量创建用户 ===")
    users := []User{
        {Name: "钱七", Age: 24, Email: "qianqi@example.com"},
        {Name: "孙八", Age: 29, Email: "sunba@example.com"},
    }
    err = db.Create(&users).Error
    if err != nil {
        fmt.Printf("批量创建失败: %v\n", err)
    } else {
        fmt.Printf("批量创建成功: %+v\n", users)
    }

    // 4. 更新操作
    fmt.Println("\n=== 更新用户 ===")
    err = db.Model(&User{}).Where("id = ?", 1).Update("age", 26).Error
    if err != nil {
        fmt.Printf("更新失败: %v\n", err)
    } else {
        fmt.Printf("更新成功，影响行数: %d\n", db.RowsAffected)
    }

    // 5. 删除操作
    fmt.Println("\n=== 删除用户 ===")
    err = db.Delete(&User{}, 3).Error
    if err != nil {
        fmt.Printf("删除失败: %v\n", err)
    } else {
        fmt.Printf("删除成功，影响行数: %d\n", db.RowsAffected)
    }

    // 6. 计数查询
    fmt.Println("\n=== 计数查询 ===")
    var count int64
    err = db.Model(&User{}).Count(&count).Error
    if err != nil {
        fmt.Printf("计数失败: %v\n", err)
    } else {
        fmt.Printf("用户总数: %d\n", count)
    }
}
```

## 支持的操作

### 查询操作（调用ReadDB）
```go
// 基本查询
db.Find(&users)
db.First(&user)
db.Last(&user)
db.Take(&user)

// 条件查询
db.Where("age > ?", 18).Find(&users)
db.Where("name = ?", "张三").First(&user)

// 复杂查询
db.Where("age > ?", 18).Or("status = ?", "vip").
   Order("created_at DESC").Limit(10).Find(&users)

// 计数查询
db.Model(&User{}).Count(&count)

// 原生SQL查询
db.Raw("SELECT * FROM users WHERE age > ?", 18).Scan(&users)
```

### 写入操作（调用WriteDB）
```go
// 创建
db.Create(&user)
db.Create(&users)           // 批量创建

// 更新
db.Save(&user)              // 保存（更新所有字段）
db.Model(&user).Updates(map[string]interface{}{"age": 26})
db.Model(&user).Update("age", 26)
db.Where("age < ?", 18).Updates(map[string]interface{}{"status": "minor"})

// 删除
db.Delete(&user)
db.Delete(&User{}, 1)       // 按ID删除
db.Where("age < ?", 13).Delete(&User{})  // 条件删除

// Upsert (ON DUPLICATE KEY UPDATE)
db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&user)
```

## 优势

1. **API不变**: 用户可以继续使用熟悉的GORM API
2. **灵活交互**: 底层可以是HTTP、gRPC、消息队列等任意方式
3. **读写分离**: ReadDB和WriteDB可以连接不同的服务
4. **类型安全**: 保持GORM的类型安全和自动映射特性
5. **完整支持**: 支持所有GORM操作：查询、创建、更新、删除
6. **易于测试**: 可以轻松mock ReadDB和WriteDB函数

## 注意事项

1. **JSON格式**: 确保ReadDB和WriteDB返回正确的格式
2. **错误处理**: 函数需要适当处理各种错误情况
3. **性能考虑**: JSON序列化/反序列化可能有性能开销
4. **事务支持**: 当前方案不支持数据库事务
5. **ID回填**: INSERT操作会自动将insert_id设置到结构体的主键字段
6. **并发安全**: 确保ReadDB和WriteDB函数是并发安全的