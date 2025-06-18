# GORM JSON 交互方式使用示例

这个方案保持GORM用户API不变，但底层与MySQL的交互改为使用JSON方式。

## 核心概念

用户依然使用熟悉的GORM API：
```go
db.Find(&users)
db.Where("age > ?", 18).Find(&users)
db.First(&user)
```

但底层不再直接连接MySQL，而是调用用户提供的 `ReadDB` 函数，该函数返回JSON字符串。

## 使用步骤

### 1. 定义用户的ReadDB函数

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    // 你的HTTP客户端或其他数据源
)

// 实现ReadDB函数，这是您的核心抽象方法
func ReadDB(ctx context.Context, sql string) (string, error) {
    // 这里是您自己的实现逻辑
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
    
    // 示例2: 直接返回模拟数据
    // if strings.Contains(sql, "SELECT * FROM users") {
    //     return `[{"user_id":123, "age":25, "name":"张三"}, {"user_id":124, "age":30, "name":"李四"}]`, nil
    // }
    // return "[]", nil
}
```

### 2. 初始化GORM使用JSON方式

```go
package main

import (
    "gorm.io/gorm"
)

func main() {
    // 使用JSON方言初始化GORM
    db, err := gorm.Open(NewJSONDialector(ReadDB), &gorm.Config{})
    if err != nil {
        panic("初始化数据库失败: " + err.Error())
    }

    // 现在您可以像往常一样使用GORM
    var users []User
    
    // 底层会调用 ReadDB(ctx, "SELECT * FROM users WHERE age > 18")
    err = db.Where("age > ?", 18).Find(&users).Error
    if err != nil {
        fmt.Printf("查询失败: %v\n", err)
        return
    }
    
    fmt.Printf("查询到 %d 个用户\n", len(users))
    for _, user := range users {
        fmt.Printf("用户: %+v\n", user)
    }
}
```

### 3. 定义模型结构

```go
type User struct {
    UserID int    `json:"user_id" gorm:"primaryKey"`
    Name   string `json:"name"`
    Age    int    `json:"age"`
    Email  string `json:"email"`
}

// TableName 指定表名
func (User) TableName() string {
    return "users"
}
```

## ReadDB函数的实现方式

### 方式1: HTTP API调用

```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    client := &http.Client{Timeout: 30 * time.Second}
    
    payload := map[string]string{"sql": sql}
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
```

### 方式3: 消息队列

```go
func ReadDB(ctx context.Context, sql string) (string, error) {
    // 发送查询请求到消息队列
    queryID := generateQueryID()
    
    err := publishQuery(queryID, sql)
    if err != nil {
        return "", err
    }
    
    // 等待响应
    result, err := waitForResult(ctx, queryID)
    if err != nil {
        return "", err
    }
    
    return result, nil
}
```

## 期望的JSON格式

### 查询结果格式
```json
[
  {"user_id": 123, "name": "张三", "age": 25, "email": "zhangsan@example.com"},
  {"user_id": 124, "name": "李四", "age": 30, "email": "lisi@example.com"}
]
```

### 执行结果格式（INSERT/UPDATE/DELETE）
```json
{
  "rows_affected": 1,
  "last_insert_id": 125
}
```

### COUNT查询结果格式
```json
[
  {"count": 42}
]
```

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "gorm.io/gorm"
)

type User struct {
    ID    int    `json:"id" gorm:"primaryKey"`
    Name  string `json:"name"`
    Age   int    `json:"age"`
    Email string `json:"email"`
}

// 模拟的ReadDB实现
func ReadDB(ctx context.Context, sql string) (string, error) {
    // 模拟数据库查询，返回JSON字符串
    fmt.Printf("执行SQL: %s\n", sql)
    
    // 根据SQL返回模拟数据
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

func contains(s, substr string) bool {
    return strings.Contains(strings.ToUpper(s), strings.ToUpper(substr))
}

func main() {
    // 使用JSON方言初始化GORM
    db, err := gorm.Open(NewJSONDialector(ReadDB), &gorm.Config{})
    if err != nil {
        panic("初始化失败: " + err.Error())
    }

    // 1. 查询所有用户
    fmt.Println("=== 查询所有用户 ===")
    var allUsers []User
    err = db.Find(&allUsers).Error
    if err != nil {
        fmt.Printf("查询失败: %v\n", err)
    } else {
        fmt.Printf("查询到 %d 个用户: %+v\n", len(allUsers), allUsers)
    }

    // 2. 条件查询
    fmt.Println("\n=== 条件查询 ===")
    var adultUsers []User
    err = db.Where("age > ?", 18).Find(&adultUsers).Error
    if err != nil {
        fmt.Printf("查询失败: %v\n", err)
    } else {
        fmt.Printf("查询到 %d 个成年用户: %+v\n", len(adultUsers), adultUsers)
    }

    // 3. 查询单个用户
    fmt.Println("\n=== 查询单个用户 ===")
    var user User
    err = db.First(&user).Error
    if err != nil {
        fmt.Printf("查询失败: %v\n", err)
    } else {
        fmt.Printf("查询到用户: %+v\n", user)
    }

    // 4. 计数查询
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

## 优势

1. **API不变**: 用户可以继续使用熟悉的GORM API
2. **灵活交互**: 底层可以是HTTP、gRPC、消息队列等任意方式
3. **解耦架构**: 数据库访问逻辑与业务逻辑分离
4. **易于测试**: 可以轻松mock ReadDB函数进行单元测试
5. **渐进迁移**: 可以逐步迁移现有代码

## 注意事项

1. **JSON格式**: 确保ReadDB返回的JSON格式正确
2. **错误处理**: ReadDB函数需要适当处理各种错误情况
3. **性能考虑**: JSON序列化/反序列化可能有性能开销
4. **事务支持**: 当前方案不支持数据库事务
5. **连接管理**: 需要在ReadDB中自行管理连接池等资源