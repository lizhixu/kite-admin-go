# Kite Admin Backend

基于 Gin、GORM、MySQL 和 JWT 的后台管理系统后端服务，提供账号权限、系统配置、操作审计、定时任务、数据库队列、媒体库、站内消息和邮件模板等管理能力。

## 技术栈

- **Web 框架**: Gin
- **ORM**: GORM
- **数据库**: MySQL
- **认证授权**: JWT + RBAC
- **定时任务**: robfig/cron
- **队列**: 数据库轮询队列
- **文件存储**: 本地存储 / S3 兼容存储
- **API 文档**: swaggo/gin-swagger
- **其他**: bcrypt、captcha、goldmark、godotenv

## 功能模块

- 登录、验证码、退出登录、当前角色切换
- 用户、角色、权限和菜单按钮管理
- 系统配置管理，支持公开读取基础配置
- 操作日志和登录日志查询
- 定时任务管理，支持 HTTP、Shell、内置 FUNC 任务
- 数据库队列管理，支持队列、任务、批量入队、重试/踢出
- 媒体库管理，支持上传、移动、批量删除和文件夹树
- 存储配置管理，支持本地和 S3 兼容存储
- 站内消息管理，支持未读数、批量已读和 SSE 实时推送
- 邮件配置、测试发送、邮件模板预览和保存
- Swagger UI 文档

## 项目结构

```text
backend/
├── main.go              # 应用入口、启动初始化、本地存储静态目录挂载
├── config/              # 环境配置、数据库连接、自动迁移、种子数据
├── controllers/         # Gin HTTP 控制器
├── docs/                # Swagger 生成文件
├── middleware/          # CORS、JWT、RBAC、操作日志中间件
├── models/              # GORM 模型和统一响应结构
├── queue/               # 数据库队列管理器和处理器注册
├── routes/              # 路由注册
├── scheduler/           # 定时任务调度器和内置函数
├── sse/                 # 站内消息 SSE Hub
├── storage/             # 本地 / S3 存储实现
└── utils/               # JWT、密码、验证码等工具
```

## 快速开始

### 1. 安装依赖

```bash
go mod tidy
```

### 2. 配置数据库

创建 MySQL 数据库：

```sql
CREATE DATABASE admin_system CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

在项目根目录创建 `.env`，或使用默认值：

```env
SERVER_PORT=:8080
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=123456
DB_NAME=admin_system
JWT_SECRET=your-secret-key
JWT_EXPIRE_HOURS=24
```

### 3. 运行服务

```bash
go run main.go
```

服务默认监听 `http://localhost:8080`。

### 4. Swagger 文档

启动服务后访问：

```text
http://localhost:8080/swagger/index.html
```

如果修改了 Swagger 注释，可重新生成：

```bash
swag init
```

### 5. 默认账号

- 用户名: `admin`
- 密码: `123456`

## 常用命令

```bash
# 运行服务
go run main.go

# 构建 Windows 可执行文件
go build -o ./tmp/main.exe ./main.go

# 热重载开发（需要先安装 Air）
air

# 整理依赖
go mod tidy

# 重新生成 Swagger 文档
swag init

# 重置数据库并重新自动迁移（仅开发环境使用，会删除数据）
RESET_DB=1 go run main.go
```

> Windows PowerShell 中重置数据库可使用：`$env:RESET_DB = '1'; go run main.go`。

## 启动流程

1. 加载 `.env` 环境变量
2. 初始化 MySQL 连接
3. 对已注册模型执行 GORM `AutoMigrate`
4. 同步代码定义的权限树和初始数据
5. 启动定时任务调度器
6. 启动数据库队列轮询器
7. 挂载本地媒体存储静态目录
8. 注册路由并启动 Gin 服务

## 认证说明

除登录、验证码、退出登录和公开系统配置接口外，其他接口需要在请求头中携带 JWT：

```text
Authorization: Bearer <token>
```

写操作通过 `RequirePermission("权限编码")` 做 RBAC 校验，`SUPER_ADMIN` 角色会绕过权限检查。

## API 概览

### 认证

- `POST /auth/login` - 登录
- `GET /auth/captcha` - 获取验证码
- `POST /auth/logout` - 退出登录
- `GET /auth/system/config` - 获取公开系统配置
- `POST /auth/current-role/switch/:roleCode` - 切换当前角色

### 用户管理

- `GET /user/detail` - 当前用户详情
- `GET /user` - 用户列表
- `GET /user/export` - 导出用户 XLSX
- `GET /user/import/template` - 下载用户导入模板
- `POST /user/import` - 导入用户 XLSX
- `POST /user` - 新增用户
- `PATCH /user/profile/:id` - 修改用户资料
- `PATCH /user/:id` - 修改用户
- `DELETE /user/:id` - 删除用户
- `PATCH /user/password/reset/:id` - 重置密码

### 角色与权限

- `GET /role/page` - 角色分页
- `GET /role` - 所有角色
- `POST /role` - 新增角色
- `PATCH /role/:id` - 修改角色
- `DELETE /role/:id` - 删除角色
- `PATCH /role/users/add/:id` - 分配用户
- `PATCH /role/users/remove/:id` - 移除用户
- `GET /role/permissions/tree` - 角色权限树
- `GET /permission/menu/tree` - 菜单树
- `GET /permission/tree` - 权限树
- `GET /permission/button/:parentId` - 按钮权限
- `POST /permission` - 新增权限
- `PATCH /permission/:id` - 修改权限
- `DELETE /permission/:id` - 删除权限

### 日志与系统配置

- `GET /syslog/list` - 操作日志列表
- `GET /loginlog/list` - 登录日志列表
- `GET /system/config` - 系统配置
- `PUT /system/config` - 保存系统配置

### 定时任务

- `GET /task/page` - 任务分页
- `GET /task/funcs` - 内置函数列表
- `GET /task/stats` - 任务统计
- `GET /task/preview-next` - 预览下次运行时间
- `POST /task` - 新增任务
- `PATCH /task/:id` - 修改任务
- `DELETE /task/:id` - 删除任务
- `PATCH /task/:id/toggle` - 启停任务
- `POST /task/:id/run` - 手动运行任务
- `POST /task/bulk/delete` - 批量删除任务
- `POST /task/bulk/toggle` - 批量启停任务
- `GET /task/log/page` - 任务日志分页

### 队列管理

- `GET /queue/page` - 队列分页
- `GET /queue/stats` - 队列统计
- `GET /queue/handlers` - 队列处理器列表
- `GET /queue/:id` - 队列详情
- `PATCH /queue/:id` - 修改队列
- `DELETE /queue/:id` - 删除队列
- `PATCH /queue/:id/toggle` - 启停队列
- `POST /queue/:id/kick` - 踢出/重试队列任务
- `GET /queue/:id/jobs` - 队列任务列表
- `POST /queue/:id/job` - 新增队列任务
- `POST /queue/:id/jobs/bulk` - 批量新增队列任务
- `DELETE /queue/:id/jobs` - 清空队列任务
- `POST /queue/job/:jobId/kick` - 踢出/重试单个任务
- `DELETE /queue/job/:jobId` - 删除单个任务

### 媒体库与存储

- `POST /media/upload` - 上传媒体
- `GET /media/page` - 媒体分页
- `DELETE /media/:id` - 删除媒体
- `POST /media/bulk/delete` - 批量删除媒体
- `POST /media/move` - 移动媒体
- `GET /media/folder/tree` - 文件夹树
- `GET /media/folder/resolve` - 解析文件夹
- `POST /media/folder` - 创建文件夹
- `PATCH /media/folder/:id` - 重命名文件夹
- `DELETE /media/folder/:id` - 删除文件夹
- `GET /storage/config` - 存储配置列表
- `POST /storage/config` - 新增存储配置
- `PATCH /storage/config/:id` - 修改存储配置
- `DELETE /storage/config/:id` - 删除存储配置
- `PATCH /storage/config/:id/default` - 设置默认存储
- `POST /storage/config/:id/test` - 测试存储配置

### 消息与邮件

- `GET /message/page` - 消息分页
- `POST /message` - 发送消息
- `DELETE /message/:id` - 删除消息
- `POST /message/bulk/delete` - 批量删除消息
- `GET /message/mine` - 当前用户消息
- `GET /message/unread/count` - 当前用户未读数
- `PATCH /message/:id/read` - 标记已读
- `PATCH /message/read/all` - 全部标记已读
- `PATCH /message/bulk/read` - 批量标记已读
- `GET /message/sse` - 消息 SSE 推送
- `GET /email/config` - 邮件配置
- `PUT /email/config` - 保存邮件配置
- `POST /email/config/test` - 测试邮件配置
- `GET /email-template/list` - 邮件模板列表
- `GET /email-template/:id` - 邮件模板详情
- `PUT /email-template/:id` - 保存邮件模板
- `POST /email-template/:id/preview` - 预览邮件模板

## 响应格式

普通响应：

```json
{
  "code": 0,
  "message": "OK",
  "data": {},
  "originUrl": "/api/path"
}
```

分页响应的 `data` 通常包含：

```json
{
  "list": [],
  "total": 0,
  "page": 1,
  "pageSize": 10
}
```

## 开发说明

- 数据库 schema 由 GORM `AutoMigrate` 管理，没有独立迁移文件。
- `config/seed.go` 中的默认权限树是权限菜单的代码事实来源，新增受控接口时应同步补充权限项。
- `config.RegisteredModels()` 是自动迁移模型列表的单一入口。
- `OperationLog` 会异步记录认证路由的请求、响应和耗时；SSE 路由仅使用认证中间件，避免响应被操作日志缓冲。
- 本地存储配置启动时会挂载为 Gin 静态目录，默认目录为 `./uploads`，默认访问前缀为 `/uploads`。
- 项目当前没有测试、lint 或 CI 配置。
