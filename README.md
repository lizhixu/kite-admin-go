# Go 后端服务

基于 Gin + GORM + MySQL + JWT 实现的后端管理系统。

## 技术栈

- **Web 框架**: Gin
- **ORM**: GORM
- **数据库**: MySQL
- **认证**: JWT (golang-jwt/jwt)
- **密码加密**: bcrypt

## 项目结构

```
backend/
├── main.go              # 入口文件
├── config/              # 配置
│   ├── config.go       # 配置结构
│   └── database.go     # 数据库连接
├── models/              # 数据模型
│   ├── user.go
│   ├── role.go
│   ├── permission.go
│   └── response.go
├── controllers/         # 控制器
│   ├── auth.go
│   ├── user.go
│   ├── role.go
│   └── permission.go
├── middleware/          # 中间件
│   ├── auth.go         # JWT 认证
│   └── cors.go         # 跨域处理
├── routes/              # 路由
│   └── routes.go
└── utils/               # 工具函数
    ├── jwt.go
    └── password.go
```

## 快速开始

### 1. 安装依赖

```bash
cd backend
go mod tidy
```

### 2. 配置数据库

创建 MySQL 数据库：

```sql
CREATE DATABASE admin_system CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

修改 `config/config.go` 中的数据库配置。

### 3. 运行服务

```bash
go run main.go
```

服务将在 `http://localhost:8080` 启动。

### 4. 默认账号

- 用户名: `admin`
- 密码: `123456`

## API 接口

### 认证相关

- `POST /auth/login` - 登录
- `GET /auth/captcha` - 获取验证码
- `POST /auth/logout` - 退出登录
- `POST /auth/current-role/switch/:roleCode` - 切换角色

### 用户管理

- `GET /user/detail` - 获取当前用户详情
- `GET /user` - 获取用户列表（分页）
- `POST /user` - 新增用户
- `PATCH /user/:id` - 修改用户
- `DELETE /user/:id` - 删除用户
- `PATCH /user/password/reset/:id` - 重置密码

### 角色管理

- `GET /role/page` - 获取角色列表（分页）
- `GET /role` - 获取所有角色
- `POST /role` - 新增角色
- `PATCH /role/:id` - 修改角色
- `DELETE /role/:id` - 删除角色
- `PATCH /role/users/add/:id` - 分配用户
- `PATCH /role/users/remove/:id` - 取消分配用户

### 权限管理

- `GET /role/permissions/tree` - 获取角色权限树
- `GET /permission/menu/tree` - 获取菜单权限树
- `GET /permission/tree` - 获取所有权限树
- `GET /permission/button/:parentId` - 获取按钮权限
- `POST /permission` - 新增权限
- `PATCH /permission/:id` - 修改权限
- `DELETE /permission/:id` - 删除权限

## 认证说明

除了登录、获取验证码和退出登录接口外，其他接口都需要在请求头中携带 JWT Token：

```
Authorization: Bearer <token>
```

## 响应格式

所有接口统一返回格式：

```json
{
  "code": 0,
  "message": "OK",
  "data": {},
  "originUrl": "/api/path"
}
```

## 开发说明

- 数据库表会在首次启动时自动创建
- 默认会创建一个管理员账号和两个角色
- JWT Token 默认有效期为 24 小时
- 密码使用 bcrypt 加密存储
