# Kite Admin API 文档

## 概述

- 基础路径: `http://localhost:{port}`（默认端口见 `.env` 中 `SERVER_PORT`）
- 认证方式: JWT Bearer Token（`Authorization: Bearer <token>`）
- 统一响应格式:

```json
{
  "code": 0,
  "message": "OK",
  "data": {},
  "originUrl": "/path"
}
```

`code = 0` 表示成功，非 0 为错误码。

---

## 一、认证 `/auth`

### 1.1 获取验证码

```
GET /auth/captcha
```

**响应**: 直接返回 PNG 图片（`image/png`），同时通过 Cookie 设置 `captcha_id`（有效期 300 秒）。

---

### 1.2 登录

```
POST /auth/login
```

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |
| captcha | string | 是 | 验证码 |

需要携带上一步返回的 Cookie `captcha_id`。

**成功响应** (`code = 0`):

```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

**错误码**:

| code | 说明 |
|------|------|
| 10003 | 验证码已过期或错误 |
| 10004 | 账号或密码错误 |

---

### 1.3 登出

```
POST /auth/logout
```

**响应**: `{ "code": 0, "data": true }`（前端自行清除 Token）

---

### 1.4 切换角色

```
POST /auth/current-role/switch/:roleCode
```

**路径参数**: `roleCode` — 目标角色编码

**成功响应**: 返回新的 `accessToken`（包含切换后的角色信息）

---

## 二、用户管理 `/user`

> 以下接口均需认证（`Authorization: Bearer <token>`）

### 2.1 获取当前用户详情

```
GET /user/detail
```

**响应**:

```json
{
  "code": 0,
  "data": {
    "id": 1,
    "username": "admin",
    "enable": true,
    "createTime": "2024-01-01T00:00:00Z",
    "updateTime": "2024-01-01T00:00:00Z",
    "profile": {
      "id": 1,
      "nickName": "管理员",
      "gender": "male",
      "avatar": "https://...",
      "address": null,
      "email": null,
      "userId": 1
    },
    "roles": [{ "id": 1, "code": "SUPER_ADMIN", "name": "超级管理员", "enable": true }],
    "currentRole": { "id": 1, "code": "SUPER_ADMIN", "name": "超级管理员", "enable": true }
  }
}
```

---

### 2.2 用户列表（分页）

```
GET /user?pageNo=1&pageSize=10&username=keyword
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码，默认 1 |
| pageSize | int | 否 | 每页条数，默认 10 |
| username | string | 否 | 用户名模糊搜索 |

**响应**:

```json
{
  "code": 0,
  "data": {
    "pageData": [
      {
        "id": 1,
        "username": "admin",
        "enable": true,
        "createTime": "...",
        "updateTime": "...",
        "roles": [...],
        "gender": "male",
        "avatar": "...",
        "address": null,
        "email": null
      }
    ],
    "total": 1
  }
}
```

---

### 2.3 创建用户

```
POST /user
```

**权限**: `AddUser`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |
| enable | bool | 否 | 是否启用，默认 true |
| roleIds | uint[] | 否 | 关联角色 ID 列表 |
| nickName | string | 否 | 昵称 |
| gender | int | 否 | 性别（0: 未知, 1: 男, 2: 女） |
| avatar | string | 否 | 头像 URL |
| address | string | 否 | 地址 |
| email | string | 否 | 邮箱 |

---

### 2.4 更新用户

```
PATCH /user/:id
```

**权限**: `EditUser`

**路径参数**: `id` — 用户 ID

**请求体**（所有字段均可选）:

| 字段 | 类型 | 说明 |
|------|------|------|
| username | string | 用户名 |
| enable | bool | 是否启用 |
| roleIds | uint[] | 角色 ID 列表（全量替换） |
| nickName | string | 昵称 |
| gender | int | 性别（0: 未知, 1: 男, 2: 女） |
| avatar | string | 头像 URL |
| address | string | 地址 |
| email | string | 邮箱 |

---

### 2.5 更新个人资料

```
PATCH /user/profile/:id
```

仅允许修改自己的资料（`id` 必须等于当前登录用户 ID）。

**请求体**（所有字段均可选）:

| 字段 | 类型 | 说明 |
|------|------|------|
| nickName | string | 昵称 |
| gender | int | 性别（0: 未知, 1: 男, 2: 女） |
| avatar | string | 头像 URL |
| address | string | 地址 |
| email | string | 邮箱 |

---

### 2.6 重置密码

```
PATCH /user/password/reset/:id
```

**权限**: `ResetPassword`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| password | string | 是 | 新密码 |

---

### 2.7 删除用户

```
DELETE /user/:id
```

**权限**: `DeleteUser`

---

## 三、角色管理 `/role`

### 3.1 角色列表（分页）

```
GET /role/page?pageNo=1&pageSize=10&name=keyword
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码，默认 1 |
| pageSize | int | 否 | 每页条数，默认 10 |
| name | string | 否 | 角色名模糊搜索 |

**响应**:

```json
{
  "code": 0,
  "data": {
    "pageData": [
      {
        "id": 1,
        "code": "SUPER_ADMIN",
        "name": "超级管理员",
        "enable": true,
        "permissionIds": [1, 2, 3]
      }
    ],
    "total": 1
  }
}
```

---

### 3.2 获取所有启用角色

```
GET /role
```

返回所有 `enable = true` 的角色列表（不分页）。

---

### 3.3 创建角色

```
POST /role
```

**权限**: `AddRole`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 角色名称 |
| code | string | 是 | 角色编码（唯一） |
| enable | bool | 否 | 是否启用，默认 true |
| permissionIds | uint[] | 否 | 权限 ID 列表 |

---

### 3.4 更新角色

```
PATCH /role/:id
```

**权限**: `EditRole`

**请求体**（所有字段均可选）:

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 角色名称 |
| code | string | 角色编码 |
| enable | bool | 是否启用 |
| permissionIds | uint[] | 权限 ID 列表（全量替换） |

---

### 3.5 删除角色

```
DELETE /role/:id
```

**权限**: `DeleteRole`

> `SUPER_ADMIN` 角色不可删除。

---

### 3.6 给角色添加用户

```
PATCH /role/users/add/:id
```

**权限**: `AssignPermission`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| userIds | uint[] | 是 | 用户 ID 列表 |

---

### 3.7 从角色移除用户

```
PATCH /role/users/remove/:id
```

**权限**: `AssignPermission`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| userIds | uint[] | 是 | 用户 ID 列表 |

---

## 四、权限管理 `/permission`

### 4.1 获取当前角色的权限树

```
GET /role/permissions/tree
```

根据当前登录用户的角色返回权限树。`SUPER_ADMIN` 返回全部权限。

**响应**: 嵌套的权限树结构，`children` 字段为子节点数组。

---

### 4.2 获取菜单树

```
GET /permission/menu/tree
```

返回所有 `type = MENU` 的权限树（不含 BUTTON 类型）。

---

### 4.3 获取完整权限树

```
GET /permission/tree
```

返回所有权限（MENU + BUTTON）的完整树结构。

---

### 4.4 获取指定父级下的按钮权限

```
GET /permission/button/:parentId
```

返回指定 `parentId` 下所有 `type = BUTTON` 的权限列表（扁平）。

---

### 4.5 创建权限

```
POST /permission
```

**权限**: `AddResource`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 权限名称 |
| code | string | 是 | 权限编码（唯一） |
| type | string | 是 | 类型：`MENU` / `BUTTON` |
| parentId | uint | 否 | 父级 ID |
| path | string | 否 | 路由路径 |
| redirect | string | 否 | 重定向路径 |
| icon | string | 否 | 图标 |
| component | string | 否 | 前端组件路径 |
| layout | string | 否 | 布局 |
| keepAlive | bool | 否 | 是否缓存 |
| method | string | 否 | HTTP 方法 |
| description | string | 否 | 描述 |
| show | bool | 否 | 是否显示，默认 true |
| enable | bool | 否 | 是否启用，默认 true |
| order | int | 否 | 排序，默认 0 |

---

### 4.6 更新权限

```
PATCH /permission/:id
```

**权限**: `EditResource`

请求体所有字段均可选，同创建接口。

---

### 4.7 删除权限

```
DELETE /permission/:id
```

**权限**: `DeleteResource`

> 有子权限时不可删除，需先删除子权限。

---

## 五、操作日志 `/syslog`

### 5.1 日志列表（分页）

```
GET /syslog/list?pageNo=1&pageSize=10&username=keyword&method=GET&statusCode=200
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码，默认 1 |
| pageSize | int | 否 | 每页条数，默认 10 |
| username | string | 否 | 操作人用户名模糊搜索 |
| method | string | 否 | HTTP 方法精确匹配 |
| statusCode | string | 否 | 状态码精确匹配 |

**响应**:

```json
{
  "code": 0,
  "data": {
    "pageData": [
      {
        "id": 1,
        "userId": 1,
        "username": "admin",
        "method": "POST",
        "path": "/user",
        "params": "...",
        "response": "...",
        "ip": "127.0.0.1",
        "userAgent": "...",
        "statusCode": 200,
        "latency": 12,
        "createTime": "..."
      }
    ],
    "total": 1
  }
}
```

---

## 六、定时任务 `/task`

### 6.1 任务列表（分页）

```
GET /task/page?pageNo=1&pageSize=10&name=keyword&type=HTTP
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码 |
| pageSize | int | 否 | 每页条数 |
| name | string | 否 | 任务名模糊搜索 |
| type | string | 否 | 任务类型：`HTTP` / `SHELL` / `FUNC` |

---

### 6.2 任务统计

```
GET /task/stats
```

**响应**:

```json
{
  "code": 0,
  "data": {
    "total": 10,
    "enabled": 8,
    "disabled": 2,
    "totalToday": 50,
    "successToday": 48,
    "failedToday": 1,
    "timeoutToday": 1,
    "recent": [ ... ]  // 最近 10 条执行记录
  }
}
```

---

### 6.3 获取内置函数列表

```
GET /task/funcs
```

返回所有注册的内置函数名称（用于前端 FUNC 类型任务选择）。

---

### 6.4 预览 Cron 表达式

```
GET /task/preview-next?spec=*/5 * * * *&n=5
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| spec | string | 是 | 5 字段 Cron 表达式 |
| n | int | 否 | 预览次数，默认 5 |

**响应**: 时间戳数组

---

### 6.5 创建任务

```
POST /task
```

**权限**: `AddTask`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 任务名称 |
| spec | string | 是 | 5 字段 Cron 表达式 |
| type | string | 是 | `HTTP` / `SHELL` / `FUNC` |
| command | string | 是 | 执行命令（URL / Shell 命令 / 函数名） |
| httpMethod | string | 否 | HTTP 请求方法（仅 HTTP 类型） |
| httpHeaders | string | 否 | HTTP 请求头（仅 HTTP 类型） |
| httpBody | string | 否 | HTTP 请求体（仅 HTTP 类型） |
| timeout | int | 否 | 超时时间（秒） |
| enabled | bool | 否 | 是否启用，默认 true |
| description | string | 否 | 描述 |

---

### 6.6 更新任务

```
PATCH /task/:id
```

**权限**: `EditTask`

请求体同创建接口。

---

### 6.7 删除任务

```
DELETE /task/:id
```

**权限**: `DeleteTask`

---

### 6.8 切换任务启用/停用

```
PATCH /task/:id/toggle
```

**权限**: `EditTask`

---

### 6.9 立即执行任务

```
POST /task/:id/run
```

**权限**: `RunTask`

---

### 6.10 批量删除

```
POST /task/bulk/delete
```

**权限**: `DeleteTask`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | uint[] | 是 | 任务 ID 列表 |

---

### 6.11 批量启用/停用

```
POST /task/bulk/toggle
```

**权限**: `EditTask`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | uint[] | 是 | 任务 ID 列表 |
| enabled | bool | 是 | 目标状态 |

---

### 6.12 任务执行日志（分页）

```
GET /task/log/page?pageNo=1&pageSize=10&taskId=1&status=SUCCESS&trigger=MANUAL
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码 |
| pageSize | int | 否 | 每页条数 |
| taskId | string | 否 | 任务 ID |
| status | string | 否 | 状态：`SUCCESS` / `FAILED` / `TIMEOUT` |
| trigger | string | 否 | 触发方式：`CRON` / `MANUAL` |

---

## 七、队列管理 `/queue`

### 7.1 队列列表（分页）

```
GET /queue/page?pageNo=1&pageSize=10&name=keyword&status=RUNNING
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码 |
| pageSize | int | 否 | 每页条数 |
| name | string | 否 | 队列名模糊搜索 |
| status | string | 否 | 状态：`RUNNING` / `PAUSED` |

---

### 7.2 队列统计

```
GET /queue/stats
```

**响应**:

```json
{
  "code": 0,
  "data": {
    "total": 3,
    "running": 2,
    "paused": 1,
    "jobTotal": 100,
    "jobPending": 5,
    "jobRunning": 2,
    "jobSuccess": 90,
    "jobFailed": 3,
    "successToday": 10,
    "failedToday": 1
  }
}
```

---

### 7.3 获取已注册的 Handler 列表

```
GET /queue/handlers
```

返回代码侧已注册的队列处理器名称列表。

---

### 7.4 获取单个队列详情

```
GET /queue/:id
```

---

### 7.5 更新队列

```
PATCH /queue/:id
```

**权限**: `EditQueue`

**请求体**（所有字段均可选）:

| 字段 | 类型 | 说明 |
|------|------|------|
| description | string | 描述 |
| concurrency | int | 并发数 |
| timeout | int | 超时（秒） |
| maxRetries | int | 最大重试次数 |
| status | string | 状态：`RUNNING` / `PAUSED` |

> `name`、`handler` 等代码侧字段不可修改。

---

### 7.6 删除队列

```
DELETE /queue/:id
```

**权限**: `DeleteQueue`

同时删除该队列下所有任务。

---

### 7.7 切换队列运行/暂停

```
PATCH /queue/:id/toggle
```

**权限**: `EditQueue`

---

### 7.8 复活队列所有失败任务

```
POST /queue/:id/kick
```

**权限**: `KickQueueJob`

将该队列内所有 `FAILED` 任务重置为 `PENDING`。

**响应**: `{ "code": 0, "data": { "affected": 5 } }`

---

### 7.9 队列任务列表（分页）

```
GET /queue/:id/jobs?pageNo=1&pageSize=10&status=PENDING&from=2024-01-01&to=2024-12-31
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码 |
| pageSize | int | 否 | 每页条数 |
| status | string | 否 | 状态：`PENDING` / `RUNNING` / `SUCCESS` / `FAILED` |
| from | string | 否 | 起始时间（RFC3339 / `2006-01-02 15:04:05` / 毫秒时间戳） |
| to | string | 否 | 结束时间 |

---

### 7.10 投递单个任务

```
POST /queue/:id/job
```

**权限**: `AddQueueJob`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| payload | string | 否 | 任务载荷 |
| maxRetries | int | 否 | 最大重试次数（默认使用队列配置） |

---

### 7.11 批量投递任务

```
POST /queue/:id/jobs/bulk
```

**权限**: `AddQueueJob`

**请求体**:

```json
{
  "items": [
    { "payload": "task1", "maxRetries": 3 },
    { "payload": "task2" }
  ]
}
```

---

### 7.12 清空队列任务

```
DELETE /queue/:id/jobs?status=FAILED&before=2024-01-01
```

**权限**: `DeleteQueue`

**查询参数**（均可选）:

| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | 只清除指定状态的任务 |
| before | string | 只清除该时间之前完成的任务 |

---

### 7.13 复活单个失败任务

```
POST /queue/job/:jobId/kick
```

**权限**: `KickQueueJob`

将单个 `FAILED` 任务重置为 `PENDING`。

---

### 7.14 删除单个任务

```
DELETE /queue/job/:jobId
```

**权限**: `DeleteQueue`

---

## 八、媒体库 `/media`

### 8.1 上传文件

```
POST /media/upload
```

**权限**: `UploadMedia`

**Content-Type**: `multipart/form-data`

**表单字段**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| file | file | 是 | 上传文件 |
| configId | string | 否 | 存储配置 ID（不传则使用默认存储） |
| folderId | string | 否 | 目标文件夹 ID（不传则上传到根目录） |

**响应**: 返回 Media 对象，包含 `url`、`storageKey`、`mimeType` 等字段。

---

### 8.2 媒体列表（分页）

```
GET /media/page?pageNo=1&pageSize=24&filename=keyword&mimePrefix=image/&storageType=LOCAL&configId=1&folderId=1&scope=all
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码，默认 1 |
| pageSize | int | 否 | 每页条数，默认 24 |
| filename | string | 否 | 文件名模糊搜索 |
| mimePrefix | string | 否 | MIME 类型前缀过滤（如 `image/`、`video/`） |
| storageType | string | 否 | 存储类型：`LOCAL` / `S3` |
| configId | string | 否 | 存储配置 ID |
| folderId | string | 否 | 文件夹 ID |
| scope | string | 否 | `mine`（默认）/ `all`（需 ViewAllMedia 权限） |

> 默认仅显示当前用户上传的文件。拥有 `ViewAllMedia` 权限的用户传 `scope=all` 可查看所有文件。

---

### 8.3 删除媒体

```
DELETE /media/:id
```

**权限**: `DeleteMedia`

> 仅文件所有者或拥有 `ViewAllMedia` 权限的用户可删除。

---

### 8.4 批量删除

```
POST /media/bulk/delete
```

**权限**: `DeleteMedia`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | uint[] | 是 | 媒体 ID 列表 |

---

### 8.5 移动媒体到文件夹

```
POST /media/move
```

**权限**: `UploadMedia`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | uint[] | 是 | 媒体 ID 列表 |
| folderId | uint | 是 | 目标文件夹 ID（传 0 表示移到根目录） |

---

## 九、媒体文件夹 `/media/folder`

### 9.1 文件夹列表

```
GET /media/folder/tree?configId=1
```

**查询参数**: `configId` — 存储配置 ID（可选）

返回扁平列表，前端自行构建树。

---

### 9.2 按路径解析文件夹

```
GET /media/folder/resolve?configId=1&path=avatars/2024&autoCreate=1
```

按路径逐级查找文件夹，不存在时可自动创建。

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| configId | string | 是 | 存储配置 ID |
| path | string | 是 | 文件夹路径（如 `avatars/2024`） |
| autoCreate | string | 否 | 传 `1` 时自动创建不存在的层级 |

**响应**: 最终匹配（或创建）的文件夹对象。

---

### 9.3 创建文件夹

```
POST /media/folder
```

**权限**: `ManageFolder`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 文件夹名称（不允许 `/` `\`） |
| configId | uint | 是 | 存储配置 ID |
| parentId | uint | 否 | 父文件夹 ID（不传则为根级） |

---

### 9.4 重命名文件夹

```
PATCH /media/folder/:id
```

**权限**: `ManageFolder`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 新名称 |

> 会联动更新所有后代文件夹的路径以及关联媒体的 `folder_path`。

---

### 9.5 删除文件夹

```
DELETE /media/folder/:id?cascade=1
```

**权限**: `ManageFolder`

**查询参数**: `cascade` — 传 `1` 级联删除（含子文件夹和文件），不传则仅允许删除空文件夹。

---

## 十、存储配置 `/storage/config`

### 10.1 存储配置列表

```
GET /storage/config
```

返回所有存储配置，按默认优先排序。

---

### 10.2 创建存储配置

```
POST /storage/config
```

**权限**: `ManageStorage`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 配置名称 |
| type | string | 是 | `LOCAL` / `S3` |
| endpoint | string | 否 | S3 端点 |
| region | string | 否 | S3 区域 |
| bucket | string | 否 | S3 桶名 |
| accessKey | string | 否 | S3 Access Key |
| secretKey | string | 否 | S3 Secret Key |
| useSSL | bool | 否 | 是否使用 SSL，默认 true |
| customDomain | string | 否 | 自定义域名 |
| localDir | string | 否 | 本地存储目录（仅 LOCAL 类型） |
| publicPrefix | string | 否 | 公开访问前缀 |
| allowExtensions | string | 否 | 允许的扩展名（逗号分隔，如 `jpg,png,gif`） |
| maxSizeMB | int | 否 | 最大文件大小（MB），默认 50 |
| enabled | bool | 否 | 是否启用，默认 true |
| isDefault | bool | 否 | 是否为默认存储 |

---

### 10.3 更新存储配置

```
PATCH /storage/config/:id
```

**权限**: `ManageStorage`

请求体同创建接口。`secretKey` 传空字符串则保留原值。

---

### 10.4 删除存储配置

```
DELETE /storage/config/:id
```

**权限**: `ManageStorage`

> 默认配置不可删除。有媒体文件引用时不可删除。

---

### 10.5 设置默认存储

```
PATCH /storage/config/:id/default
```

**权限**: `ManageStorage`

将指定配置设为默认（其他配置自动取消默认）。

---

### 10.6 测试存储配置

```
POST /storage/config/:id/test
```

**权限**: `ManageStorage`

执行一次写入+删除探测操作，验证存储连通性。

**响应**:

```json
{
  "code": 0,
  "data": {
    "ok": true,
    "elapsedMs": 120
  }
}
```

---

## 十一、消息管理 `/message`

### 11.1 消息列表（分页）

```
GET /message/page?pageNo=1&pageSize=10&title=keyword&type=SYSTEM
```

**查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pageNo | int | 否 | 页码，默认 1 |
| pageSize | int | 否 | 每页条数，默认 10 |
| title | string | 否 | 标题模糊搜索 |
| type | string | 否 | 消息类型：`SYSTEM` / `NOTICE` / `ANNOUNCEMENT` |

---

### 11.2 发送消息

```
POST /message
```

**权限**: `SendMessage`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| title | string | 是 | 消息标题 |
| content | string | 是 | 消息内容 |
| type | string | 是 | 消息类型：`SYSTEM` / `NOTICE` / `ANNOUNCEMENT` |
| targetType | string | 是 | 目标类型：`ALL`（全体用户）/ `USER`（指定用户） |
| userIds | uint[] | 否 | 目标用户 ID 列表（targetType 为 `USER` 时必填） |

发送后自动通过 SSE 推送通知，并异步投递邮件任务。

---

### 11.3 删除消息

```
DELETE /message/:id
```

**权限**: `DeleteMessage`

---

### 11.4 批量删除消息

```
POST /message/bulk/delete
```

**权限**: `DeleteMessage`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | uint[] | 是 | 消息 ID 列表 |

---

### 11.5 获取我的消息（分页）

```
GET /message/mine?pageNo=1&pageSize=10
```

返回当前用户收到的消息，包含已读状态。

**响应**:

```json
{
  "code": 0,
  "data": {
    "pageData": [
      {
        "id": 1,
        "title": "系统通知",
        "content": "...",
        "type": "SYSTEM",
        "senderId": 1,
        "senderName": "admin",
        "targetType": "ALL",
        "status": "SENT",
        "createTime": "...",
        "isRead": false,
        "readAt": null
      }
    ],
    "total": 1
  }
}
```

---

### 11.6 获取未读消息数量

```
GET /message/unread/count
```

**响应**:

```json
{
  "code": 0,
  "data": { "count": 5 }
}
```

---

### 11.7 标记单条消息已读

```
PATCH /message/:id/read
```

---

### 11.8 标记所有消息已读

```
PATCH /message/read/all
```

---

### 11.9 批量标记已读

```
PATCH /message/bulk/read
```

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| ids | uint[] | 是 | 消息 ID 列表 |

---

## 十二、消息 SSE 推送

### 12.1 建立 SSE 连接

```
GET /message/sse
```

**Content-Type**: `text/event-stream`

建立长连接后，服务器会实时推送消息事件。

**初始事件**（连接建立时发送）:

```
event: init
data: {"unreadCount":5}
```

**消息通知事件**（有新消息时推送）:

```
data: {"messageId":1,"title":"系统通知","type":"SYSTEM"}
```

**心跳**（每 30 秒发送一次）:

```
data: ping
```

---

## 十三、邮件配置 `/email/config`

### 13.1 获取邮件配置

```
GET /email/config
```

返回当前邮件配置（密码字段不返回）。

---

### 13.2 保存邮件配置

```
PUT /email/config
```

**权限**: `EmailConfigMgt`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| host | string | 是 | SMTP 服务器地址 |
| port | int | 是 | SMTP 端口（通常 465/587） |
| username | string | 是 | SMTP 用户名 |
| password | string | 否 | SMTP 密码（空字符串则保留原值） |
| fromName | string | 是 | 发件人名称 |
| fromEmail | string | 是 | 发件人邮箱 |
| enabled | bool | 否 | 是否启用 |

全局仅一条配置，首次调用创建，后续调用更新。

---

### 13.3 测试邮件配置

```
POST /email/config/test
```

**权限**: `EmailConfigMgt`

向当前登录用户的邮箱发送一封测试邮件（需先在个人资料中绑定邮箱）。

---

## 十四、邮件模板 `/email-template`

### 14.1 模板列表

```
GET /email-template/list
```

返回所有邮件模板，按场景（scene）排序。

---

### 14.2 获取单个模板

```
GET /email-template/:id
```

---

### 14.3 更新模板

```
PUT /email-template/:id
```

**权限**: `EmailTemplateMgt`

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 模板名称 |
| subject | string | 是 | 邮件主题（支持变量占位符） |
| content | string | 是 | 邮件 HTML 内容（支持变量占位符） |

> 内置模板（`isBuiltin = true`）的 `scene` 字段不可修改。

---

### 14.4 预览模板

```
POST /email-template/:id/preview
```

**请求体**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| vars | map[string]string | 否 | 模板变量（不传则使用默认示例值） |

**响应**:

```json
{
  "code": 0,
  "data": {
    "subject": "渲染后的主题",
    "htmlBody": "渲染后的 HTML 内容"
  }
}
```

支持的变量: `title`、`content`、`username`、`siteURL`、`currentTime`（具体取决于模板定义）。

---

## 附录

### 权限编码列表

系统启动时自动从代码同步到数据库，以下为已注册的权限编码：

| 编码 | 说明 |
|------|------|
| AddUser | 新增用户 |
| EditUser | 编辑用户 |
| DeleteUser | 删除用户 |
| ResetPassword | 重置密码 |
| AddRole | 新增角色 |
| EditRole | 编辑角色 |
| DeleteRole | 删除角色 |
| AssignPermission | 分配权限/用户 |
| AddResource | 新增权限 |
| EditResource | 编辑权限 |
| DeleteResource | 删除权限 |
| AddTask | 新增任务 |
| EditTask | 编辑任务 |
| DeleteTask | 删除任务 |
| RunTask | 执行任务 |
| EditQueue | 编辑队列 |
| DeleteQueue | 删除队列 |
| AddQueueJob | 投递任务 |
| KickQueueJob | 复活任务 |
| UploadMedia | 上传/移动媒体 |
| DeleteMedia | 删除媒体 |
| ManageFolder | 管理文件夹 |
| ManageStorage | 管理存储配置 |
| ViewAllMedia | 查看所有用户的媒体 |
| SendMessage | 发送消息 |
| DeleteMessage | 删除消息 |
| EmailConfigMgt | 管理邮件配置 |
| EmailTemplateMgt | 管理邮件模板 |

> `SUPER_ADMIN` 角色自动跳过所有权限检查。
