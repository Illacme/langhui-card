# 朗汇机电电子名片小程序 — 后端骨架（Go + Gin + GORM + MySQL）

对应《首页组件与接口字段映射》《跳转页组件与接口字段映射》两份文档。骨架已覆盖需求中的读操作与写操作（员工/超管登录、开通申请、名片修改审核流、员工管理）。

## 目录结构
```
backend/
├── cmd/server/main.go           # 入口：连 DB、注册路由、启动（含 purge 启动清理 / 可观测中间件）
├── cmd/devserver/main.go        # 开发态入口：SQLite（免 cgo）+ 自动 seed 演示数据
├── internal/
│   ├── config/config.go         # 环境变量注入（DB_DSN / PORT / JWT_KEY / 上传 / 小程序码 / 超管 / TTL / METRICS_ENABLED）
│   ├── model/models.go          # GORM 模型（员工/企业/分类/项目/服务/布局/统计/超管/两类申请/吊销会话）
│   ├── repository/repository.go # 数据访问层
│   ├── repository/session.go    # 会话黑名单读写 + 过期清理（revoked_sessions）
│   ├── auth/jwt.go              # JWT 双令牌签发 / 解析（access 15m + refresh 30d，sid 会话维度）
│   ├── auth/password.go         # bcrypt 哈希 / 校验
│   ├── handler/handler.go       # 首页相关 HTTP 接口（读）
│   ├── handler/auth.go          # 员工/超管登录，签发 JWT + 刷新 + 登出
│   ├── handler/apply.go         # 开通申请 / 名片修改审核 / 员工管理 / 统计（写 + 审核流）
│   ├── handler/content.go       # 企业内容管理（分类/项目/服务/简介…）含分类删除强校验
│   ├── handler/cookie.go        # httpOnly Cookie 读写（PC 后台鉴权）
│   ├── handler/seed.go          # 首次启动自动创建初始超管（SeedAdmin）
│   ├── handler/wechat.go        # 小程序码生成 + 缓存（写入当前存储驱动）
│   ├── storage/storage.go        # Uploader 接口 + LocalUploader（默认）
│   ├── storage/minio.go          # MinIO / S3 兼容实现
│   ├── storage/oss.go            # 阿里云 OSS 实现
│   └── middleware/
│       ├── auth.go              # JWT 鉴权 + 角色守卫 + 会话黑名单校验
│       ├── ratelimit.go         # 登录 IP 限流
│       └── observability.go     # 可观测三件套：RequestID / AccessLog / Metrics（/metrics）
├── migrations/0001_init.sql      # 建表 + 经典业绩 7 分类种子
├── migrations/0002_apply.sql    # 审核流表 + employee.staff_id 自增
├── migrations/0003_soft_delete.sql   # 软删除列
├── migrations/0004_token_version.sql # token_version 安全戳（账号级吊销）
├── migrations/0005_revoked_sessions.sql # 会话黑名单表（单设备精确吊销）
├── Dockerfile                   # 多阶段构建，单二进制镜像
└── go.mod
```

## 本地运行
```bash
# 1. 建库并导入全部迁移（含 7 分类种子 / 审核流表 / 软删除 / token_version / 会话黑名单）
for f in migrations/000*.sql; do mysql -u root -p langhui < "$f"; done

# 2. 配置环境变量（JWT_KEY 必填，用于 token 签名）
export DB_DSN="root:root@tcp(127.0.0.1:3306)/langhui?charset=utf8mb4&parseTime=true"
export PORT=8080
export JWT_KEY="change-me-to-a-long-random-string"
# 文件上传（可选，有默认值）
export UPLOAD_DIR="./uploads"
export UPLOAD_BASE_URL="http://127.0.0.1:8080/uploads"
# 文件存储驱动：local(默认) | minio | oss；上传/小程序码均写入该驱动
export STORAGE_DRIVER="local"
# —— MinIO / S3 兼容（STORAGE_DRIVER=minio）——
export MINIO_ENDPOINT="play.min.io"
export MINIO_ACCESS_KEY="your-access-key"
export MINIO_SECRET_KEY="your-secret-key"
export MINIO_BUCKET="langhui"
export MINIO_USE_SSL="true"
# —— 阿里云 OSS（STORAGE_DRIVER=oss）——
export OSS_ENDPOINT="oss-cn-hangzhou.aliyuncs.com"
export OSS_ACCESS_KEY_ID="your-access-key-id"
export OSS_ACCESS_KEY_SECRET="your-secret"
export OSS_BUCKET="langhui"
# 注：MINIO/OSS 模式下 UPLOAD_BASE_URL 改为对象存储的公开访问基址（如 https://cdn.xxx 或 bucket 公网域名）
# 存储可见性：true=公开桶（默认）；false=私有桶，需配合限时签名访问（STORAGE_PUBLIC=false）
export STORAGE_PUBLIC="true"
# CDN 刷新（覆盖写后使边缘缓存失效）：CDN_ENABLED=1 + 阿里云 CDN 凭据（可与 OSS 相同）
export CDN_ENABLED="false"
export CDN_ACCESS_KEY_ID=""
export CDN_ACCESS_KEY_SECRET=""
export CDN_ENDPOINT="cdn.aliyuncs.com"
# 微信小程序码（wxacode）所需凭据
export WX_APPID="wxXXXXXXXXXXXXXXXX"
export WX_APPSECRET="your-app-secret"
# 小程序码磁盘缓存有效期（小时，默认 720 = 30 天，到期自动重新生成）
export WXACODE_TTL_HOURS="720"

# 初始超管（首启动若 admin 表为空则自动 seed，上线务必修改为强密码）
export ADMIN_USER="admin"
export ADMIN_PASS="admin123456"

# 3. 安装依赖并启动
go mod tidy
go run ./cmd/server
```

> 启动时会执行 `SeedAdmin`：若 `admin` 表为空，则用 `ADMIN_USER`/`ADMIN_PASS`（bcrypt 哈希后）创建初始超管并打印登录提示；已存在则跳过。避免手工写库。

## 容器运行
```bash
docker build -t langhui-backend ./backend
docker run -e DB_DSN="$DB_DSN" -e JWT_KEY="$JWT_KEY" -e UPLOAD_DIR="./uploads" -e UPLOAD_BASE_URL="$UPLOAD_BASE_URL" -e STORAGE_DRIVER="$STORAGE_DRIVER" -e MINIO_ENDPOINT="$MINIO_ENDPOINT" -e MINIO_ACCESS_KEY="$MINIO_ACCESS_KEY" -e MINIO_SECRET_KEY="$MINIO_SECRET_KEY" -e MINIO_BUCKET="$MINIO_BUCKET" -e MINIO_USE_SSL="$MINIO_USE_SSL" -e OSS_ENDPOINT="$OSS_ENDPOINT" -e OSS_ACCESS_KEY_ID="$OSS_ACCESS_KEY_ID" -e OSS_ACCESS_KEY_SECRET="$OSS_ACCESS_KEY_SECRET" -e OSS_BUCKET="$OSS_BUCKET" -e WX_APPID="$WX_APPID" -e WX_APPSECRET="$WX_APPSECRET" -e WXACODE_TTL_HOURS="$WXACODE_TTL_HOURS" -e ADMIN_USER="$ADMIN_USER" -e ADMIN_PASS="$ADMIN_PASS" -p 8080:8080 langhui-backend
```

## 本地联调（SQLite devserver，免 MySQL）
`cmd/devserver` 是仅用于联调/演示的开发态入口：用 SQLite（纯 Go，免 cgo）替代 MySQL，复用与 `cmd/server` **完全一致**的路由装配，启动即 seed 演示数据（超管 / 公司信息 / 7 分类 / 1 精选项目 / 1 服务 / 首页布局 / 1 演示员工）。适合小程序端本地联调，免去起 MySQL / Docker。
```bash
go run ./cmd/devserver          # 监听 :8080（PORT 可覆盖）
```
演示账号：超管 `admin/admin123456`；员工手机 `13800000001` / 密码 `demo1234`（staffId=1）。生产部署仍走 `cmd/server`（MySQL + docker-compose）。

两个联调注意点：
- **真机 / 局域网上传**：上传基址默认 `http://127.0.0.1:<port>/uploads`（同机测试可直接用）；手机取不到图时设 `DEV_UPLOAD_BASE_URL=http://<本机IP>:8080/uploads` 再启动（启动日志 `upload base url:` 显示生效值）。
- **数据不持久**：SQLite 库在系统临时目录，重启即丢并重新 seed；要持久化运行见仓库根 `docs/deploy-runbook.md`「无 Docker 部署」路径 B/C。

## 生产文件存储（MinIO / OSS）
- 通过 `STORAGE_DRIVER` 切换，业务代码（handler / 前端 / 小程序）完全不变，仅 `main.go` 的 `newUploader` 工厂按环境变量选择实现，统一满足 `storage.Uploader` 接口：
  - `local`（默认）：写本地 `UPLOAD_DIR` 并由 `/uploads` 静态路由托管，适用于开发/单机。
  - `minio`：写入 MinIO（或任意 S3 兼容对象存储，含阿里云 OSS 的 S3 兼容端点），需 `MINIO_*` 环境变量；`UPLOAD_BASE_URL` 设为 bucket 的公开访问基址或 CDN 域名。
  - `oss`：写入阿里云 OSS，需 `OSS_*` 环境变量；`UPLOAD_BASE_URL` 设为 bucket 公网域名或绑定的自定义域名/CDN。
- 三种实现都做了**统一的大小 / 类型白名单校验**（`checkFile`），对象命名布局一致（用户上传放 `uploads/<rand>`，小程序码放 `wxacode/<staffId>.jpg`），便于从本地平滑迁移到对象存储。
- **小程序码**也走同一抽象（`SaveBytes`），写入当前驱动并带内存 TTL 缓存（按 `WXACODE_TTL_HOURS`），到期自动重新生成；不再依赖本地文件系统 ModTime。
- 前置要求：bucket 需设为**公开读**（或经 CDN 回源公开），这样返回的 URL 才能被小程序/H5 直接访问。
- 依赖：`github.com/minio/minio-go/v7`、`github.com/aliyun/aliyun-oss-go-sdk`。因 SDK 要求，**项目 Go 版本需 ≥ 1.26**（见 `go.mod` 与 `Dockerfile` 的 `golang:1.26-alpine`）。

## 存储增强（直传预签名 / 私有桶签名 / CDN 刷新）
三项增强均基于 `storage.Uploader` 之上的可选能力接口，业务代码不变：
- `Presigner`：`PresignPut(key, contentType, ttl)` → 预签名 PUT URL（MinIO/OSS 实现；本地磁盘无此能力，返回 code=1 由前端回退普通上传）。
- `Signer`：`SignGetURL(key, ttl)` → 限时 GET URL（私有桶用）。
- `CDNRefresher`：`Refresh(urls)` → CDN 缓存刷新（`AliyunCDNRefresher` 纯标准库实现，无额外依赖）。

### 1. 上传直传预签名 URL
- 流程：前端先 `GET /api/admin/upload/presign?ext=png&contentType=image/png`（员工用 `/api/employee/...`）拿到 `putUrl` + `url`；再用 `PUT putUrl` 把文件直接传到对象存储；`url` 即最终可访问地址，写回表单。
- 好处：大文件不流经应用服务器，省带宽、减存储压力。
- 本地驱动不支持预签名，`presignHandler` 返回 `code=1`，`UploadField.vue` 自动回退到普通 `/api/admin/upload` 上传。
- 注意（OSS）：预签名 URL 已绑定 `Content-Type`，前端 `PUT` 时必须带**相同** `Content-Type`（组件已处理）；MinIO 对 PUT Content-Type 不强制。
- 小程序端：原生 `wx.uploadFile` 只能 multipart POST，不便做 raw PUT；小程序仍走普通 `/api/employee/upload` 服务端上传（能力已具备），直传预签名主要在 PC 后台生效。

### 2. CDN 刷新
- 启用：`CDN_ENABLED=1` + `CDN_ACCESS_KEY_ID` / `CDN_ACCESS_KEY_SECRET`（可与 OSS 凭据相同），可选 `CDN_ENDPOINT`（默认 `cdn.aliyuncs.com`，阿里云 CDN）。
- 行为：每次 `POST /api/admin/upload` 成功后 best-effort 调用 `Refresh(urls)`，使边缘节点旧缓存失效（覆盖写头像/封面/相册时避免回源拿到旧图）；失败仅记录日志，不影响上传结果。
- 也可手动触发：`POST /api/admin/cdn/refresh` body `{"urls":["https://cdn.xxx/a.jpg", ...]}`（单次最多 1000 条，自动分批）。
- 仅实现阿里云 CDN；其它 CDN 只需新增一个 `CDNRefresher` 实现并在 `main.go` 注入。

### 3. 私有桶 + 限时签名访问
- 启用：`STORAGE_PUBLIC=false`（默认 `true`）。对象存储 Bucket 设为私有读。
- 存储：DB 仍按现有字段保存「公开形态 URL」（`BaseURL/key`），对象本身私有。
- 访问：前端在需要展示私有文件时，调用 `POST /api/admin/storage/sign`（员工用 `/api/employee/...`）提交已存 URL 列表，后端按 `BaseURL` 前缀反推对象 key，返回 **30 分钟有效**的限时签名 URL（`SignGetURL`），前端用签名 URL 取图。
- 本地私有：若 `local` 驱动 + `STORAGE_PUBLIC=false`，文件不再经 `/uploads` 静态公开，改用 `GET /api/storage/file?key=...`（鉴权后流式返回，防目录穿越）。
- 小程序码已接入私有桶签名通道：`STORAGE_PUBLIC=false` 且存储支持签名时，`/api/wechat/wxacode` 返回 **1 小时有效**的限时签名 URL（每次请求重新签发），二维码内容本身仍按 `WXACODE_TTL_HOURS` 缓存、到期才重新调微信生成；公开桶/本地驱动仍返回公开 URL。

## 已实现接口
### 读（首页 + 跳转页）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET  | /api/card?staffId= | 聚合名片（员工+企业+简介），禁用返回 code=2 |
| GET  | /api/achievements | 分类 + 精选项目 |
| GET  | /api/projects?categoryId= | 项目列表（enabled=1） |
| GET  | /api/projects/:id | 项目详情（解析 album JSON） |
| GET  | /api/services | 业务服务列表 |
| GET  | /api/services/:id | 业务服务详情 |
| GET  | /api/home-layout | 首页板块顺序 |
| POST | /api/stat/track | 埋点上报（staffId + eventType） |

### 写 + 审核流（员工 / 超管，账号密码，JWT）
| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | /api/employee/login | 否 | 员工登录（手机号+密码）→ 返回 token + refreshToken（小程序 Bearer） |
| POST | /api/admin/login | 否 | 超管登录（用户名+密码）→ 写 httpOnly Cookie，响应体仅 expiresIn |
| POST | /api/refresh | 否 | 换发新 access（PC 读 refresh Cookie；小程序 body 传 refreshToken） |
| POST | /api/logout | 否 | 吊销当前会话 sid（单设备精确吊销）+ 清除 httpOnly 鉴权 Cookie（PC 后台登出） |
| POST | /api/apply/open | 否 | 员工自助提交开通申请（pending） |
| GET  | /api/employee/me | employee | 本人名片信息 |
| PUT  | /api/employee/me | employee | 提交名片修改申请（待审，对外仍显旧信息） |
| GET  | /api/employee/me/stat | employee | 本人访问/拨号/保存统计 |
| PUT  | /api/employee/password | employee | 修改本人密码 |
| GET  | /api/admin/apply/open | admin | 开通申请列表 |
| POST | /api/admin/apply/open/:id/approve | admin | 通过 → 自动建员工账号并生成初始密码 |
| POST | /api/admin/apply/open/:id/reject | admin | 驳回 |
| GET  | /api/admin/apply/modify | admin | 名片修改申请列表 |
| POST | /api/admin/apply/modify/:id/approve | admin | 通过 → 应用快照到员工 |
| POST | /api/admin/apply/modify/:id/reject | admin | 驳回 |
| GET  | /api/admin/employees | admin | 员工列表 |
| POST | /api/admin/employees | admin | 新增员工（生成账号+初始密码） |
| POST | /api/admin/employees/:id/disable | admin | 停用账号 |
| POST | /api/admin/employees/:id/enable | admin | 启用账号 |
| POST | /api/admin/employees/:id/reset-password | admin | 重置密码（返回新初始密码） |
| PUT  | /api/admin/password | admin | 超管修改自己密码 |

### 企业内容管理（超管）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET  | /api/admin/company-info | 公司基础信息（id=1 单行） |
| PUT  | /api/admin/company-info | 更新公司信息 |
| GET  | /api/admin/company-profile | 公司简介 + 视频号 |
| PUT  | /api/admin/company-profile | 更新简介 + 视频号 |
| GET  | /api/admin/categories | 业绩分类列表 |
| POST | /api/admin/categories | 新增分类 |
| PUT  | /api/admin/categories/:id | 编辑分类 |
| DELETE | /api/admin/categories/:id | 删除分类（强校验：关联项目存在则拦截 / 锁定分类拦截，均返回 code=1） |
| GET  | /api/admin/projects?categoryId= | 项目案例列表（含下架） |
| POST | /api/admin/projects | 新增项目案例 |
| PUT  | /api/admin/projects/:id | 编辑项目案例 |
| DELETE | /api/admin/projects/:id | 删除项目案例 |
| GET  | /api/admin/services | 业务服务列表 |
| POST | /api/admin/services | 新增业务服务 |
| PUT  | /api/admin/services/:id | 编辑业务服务 |
| DELETE | /api/admin/services/:id | 删除业务服务 |
| GET  | /api/admin/home-layout | 首页板块顺序 |
| PUT  | /api/admin/home-layout | 更新板块顺序 |
| POST | /api/admin/upload | 文件上传（图片/mp4），返回可访问 URL（填回封面/相册/头像/LOGO） |
| POST | /api/employee/upload | 员工上传头像（员工鉴权），返回可访问 URL |
| GET  | /api/admin/upload/presign | 直传预签名：返回 `putUrl`（直接 PUT 到对象存储）+ 最终 `url`（仅 minio/oss；local 返回 code=1 由前端回退普通上传） |
| GET  | /api/employee/upload/presign | 同上（员工鉴权） |
| POST | /api/admin/storage/sign | 私有桶：提交已存 URL 列表，返回限时签名 URL（仅 minio/oss 私有桶） |
| POST | /api/employee/storage/sign | 同上（员工鉴权） |
| POST | /api/admin/cdn/refresh | 触发 CDN 缓存刷新（body: `{"urls":[...]}`），需 `CDN_ENABLED=1` |

### 小程序码（公开，分享海报用）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET  | /api/wechat/wxacode?staffId= | 按员工生成小程序码（wxacodeunlimit，scene=staffId=），磁盘缓存后返回 URL |
| GET  | /api/wechat/wxacode?staffId=&page= | 指定落地页（默认 pages/index/index） |

> 小程序码缓存 TTL：生成的图片通过当前存储驱动的 `SaveBytes` 写入（local → `uploads/wxacode/<staffId>.jpg`；minio/oss → 对应对象）。返回策略：公开桶/本地直接返回公开 URL；**私有桶**（`STORAGE_PUBLIC=false` 且存储支持签名）返回**每次重新签发的 1 小时限时签名 URL**。二维码内容按 `WXACODE_TTL_HOURS`（默认 720h=30 天）做内存 TTL 缓存，过期才重新调微信生成，既避免反复消耗微信配额，也支持变更样式/落地页后自然刷新。`access_token` 服务端内存缓存并在 40001/42001 时自动刷新重试。
> 鉴权方式：请求头 `Authorization: Bearer <token>`（admin 接口）。小程序码接口为公开（分享用），未配置 `WX_APPID/WX_APPSECRET` 时返回明确错误。

## 安全加固

本骨架已落地以下安全基线（详见对应源码）：

- **上传内容嗅探**：`storage.checkFile` 在校验扩展名白名单后，进一步按文件头（魔法字节）校验真实类型（PNG / JPG / GIF / WEBP / MP4），防止把可执行文件改名图片上传。`backend/internal/storage/storage_test.go` 覆盖各类型的合法/伪造场景。
- **登录限流**：`middleware.NewLoginRateLimiter` 按客户端 IP 做固定窗口限流（默认每 IP 每分钟 10 次登录尝试），防爆破；超限返回 `{code:429}`（与现有鉴权约定一致）。`backend/internal/middleware/ratelimit_test.go` 覆盖计数与隔离逻辑。
- **启动自检告警**：`config.InsecureWarnings()` 在 `JWT_KEY` 为默认值或长度不足 16、或初始超管口令仍为 `admin123456` 时，由 `cmd/server/main.go` 打印 `[SECURITY]` 告警，提醒上线前修改。
- **健康检查**：`GET /healthz` 返回 `{status:"ok"}`，供容器编排 / 负载均衡探活；`docker-compose.yml` 已据此为 backend 配置 `healthcheck`，frontend 的 `depends_on` 升级为 `service_healthy`。
- **软删除**：`ProjectCase` / `BizService` / `AchievementCategory` 已接入 GORM `DeletedAt`，删除改为置空 `deleted_at`，列表接口默认不返回已删除记录；MySQL 侧迁移见 `migrations/0003_soft_delete.sql`。
- **JWT 双令牌 + 即时吊销**：`auth.IssueTokens` 同时签发 access（15 分钟）与 refresh（30 天）；`POST /api/refresh` 用 refresh 换发新 access。`Employee` / `Admin` 含 `token_version`（安全戳，迁移 `migrations/0004_token_version.sql`），改密 / 管理员重置密码 / 禁用启用时自增，`middleware.AuthMiddleware` 校验令牌内 `vid` 与当前版本一致，**旧令牌立即失效**（账号级吊销，无需黑名单表）。覆盖测试见 `TestTokenRefresh` 与「超管改密与令牌吊销」。
- **PC 管理后台 httpOnly Cookie 鉴权**：`/api/admin/login` 将 access/refresh 写入 `HttpOnly; SameSite=Lax` 的 Cookie（`access_token` / `refresh_token`，见 `handler/cookie.go`），响应体不再回传令牌；`AuthMiddleware` 支持「Authorization 头 或 `access_token` Cookie」两种来源（小程序走 Bearer，PC 走 Cookie）；`POST /api/logout` 清除 Cookie。`COOKIE_SECURE`（生产 HTTPS 置 true）/ `COOKIE_DOMAIN`（跨子域才设）可配。覆盖测试见 `TestCookieAuth`。
- **单设备精确吊销（会话黑名单）**：`Claims.SID` 为会话 ID——一次登录生成一个，access/refresh 共享，**刷新不换 sid**（`auth.IssueTokensWithSID`）。`POST /api/logout` 从 Bearer / Cookie 解析当前会话，把 sid 写入黑名单表 `revoked_sessions`（迁移 `migrations/0005_revoked_sessions.sql`，读写见 `repository/session.go`）；`AuthMiddleware` 与 `/api/refresh` 均校验 sid 不在黑名单内，**登出后该设备的所有令牌立即失效，且不能靠刷新绕过**。与 `token_version`（账号级）互补：登出只失效当前设备，同一账号的其他登录不受影响。覆盖测试见 `TestSingleDeviceRevocation`。
- **黑名单过期清理**：`repository.PurgeExpiredSessions(db)` 按 `expires_at` 删除已过期的吊销记录（这些行的令牌自身也已过期，留着无意义）；`cmd/server` 与 `cmd/devserver` 启动时机会性执行一次并打印日志，生产可另配定时 SQL（`DELETE FROM revoked_sessions WHERE expires_at < NOW();`）。
- **可观测性三件套**（`middleware/observability.go`，零第三方依赖）：
  - `RequestID`：注入 / 透传 `X-Request-Id` 响应头，报障时可引用，串联日志排障；
  - `AccessLog`：每请求一行（方法 / 路径 / 状态 / 耗时 / IP），5xx 以 `[ACCESS-5XX]` 前缀标出，便于日志告警规则直接匹配；panic 抛穿本中间件，由链上 `gin.Recovery`（`handler.New` 已用 `gin.New()+Recovery` 替代 `gin.Default()`）记录含栈日志；
  - `Metrics`：内存计数器 + Prometheus 文本格式 `/metrics`（**默认关闭**，`METRICS_ENABLED=1` 显式开启，避免公网暴露流量规模；生产建议仅内网 / 反代白名单可达）。
- **数据库备份**：仓库根 `scripts/backup.sh` 做 MySQL 逻辑备份（`docker exec mysqldump --single-transaction` → gzip → 按 `KEEP_DAYS` 轮转），可入 crontab；逻辑备份不含 binlog，需 PITR 时另行开启。

### 已知待加固项（需流程 / 基建配合，未在本仓库落地）
- 小程序码 / access_token 等内存缓存多实例不共享，水平扩展需引入 Redis。
- **最小告警已落地**：仓库根 `scripts/alert_check.sh` 消费 `[ACCESS-5XX]` 访问日志与 `/healthz`（healthz 探活 / 5xx / 备份新鲜度三条规则，零依赖，配 crontab 即可运行，配置见《部署与联调》「最小告警配置」）；待运维侧按需扩展完整监控栈（Prometheus + Alertmanager）、外部拨测与 PITR（binlog 时间点恢复）；生产需 HTTPS 与反向代理。

## 自动化接口测试
接口层（`/api/*`）已配套 `httptest` 集成测试，覆盖读接口、登录鉴权、审核流、员工管理、分类/项目强校验、业务服务、企业内容、文件上传与存储增强、小程序码等全部主路径，并对「本地驱动不支持预签名→code:1」「CDN 未配置→code:1」「分类下有项目/锁定→删除拦截 code=1」等回退分支做了断言。

- 测试文件：`internal/handler/handler_test.go`。入口：
  - `TestAPIFlow`（默认）：SQLite 跑完整流程，按「公开接口 → 超管登录 → 审核流 → 员工管理 → 强校验 → 服务 → 内容 → 上传存储 → 改密」分段执行。
  - `TestStorageEnhanceSuccess`：用 fake `Presigner`/`Signer`/`CDNRefresher` 覆盖存储增强**成功分支**（预签名返回 `putUrl/url/key`、私有签名返回 `signed` 映射、CDN 刷新被调用）。
  - `TestTokenRefresh` / `TestCookieAuth` / `TestSingleDeviceRevocation`：分别覆盖 refresh 换发与账号级吊销、PC 端 httpOnly Cookie 鉴权与登出、单设备精确吊销（会话黑名单）。
  - `TestAPIFlowMySQL`（生产契约 parity）：设置 `TEST_MYSQL_DSN` 后，连真实 MySQL、应用 `migrations/*.sql`，再跑与 SQLite **完全相同的断言**，用以校验 AutoMigrate 与生产建表 SQL 行为一致。
- 运行：
  ```bash
  cd backend
  go test ./internal/handler/ -v
  # 生产契约校验（需可用 MySQL）：
  TEST_MYSQL_DSN='root:root@tcp(127.0.0.1:3306)/langhui_test?charset=utf8mb4&parseTime=true' \
    go test ./internal/handler/ -run TestAPIFlowMySQL -v
  ```
- 鉴权约定：中间件返回的是 **HTTP 200 + `{code:401|403}`**（非 HTTP 状态），测试统一断言 `body["code"]`。
- 测试 DB：默认无需外部 MySQL。通过 `handler.New(db)` 注入一个**临时文件的 SQLite 库**（纯 Go 驱动 `github.com/glebarez/sqlite`，免 cgo），用 GORM `AutoMigrate` 按模型建表，并 `SeedAdmin` 初始超管后把完整路由跑起来。SQLite 把模型里的 `JSON` 列按 `TEXT` 存储，与 MySQL `JSON` 列在字符串读写语义上等价，足以验证 handler 业务正确性。MySQL parity 用例通过 `runtime.Caller` 定位源码目录、回溯读取 `migrations/`，不依赖测试进程 CWD。
- 依赖影响：新增依赖 `github.com/glebarez/sqlite`（测试用，纯 Go）与 `github.com/stretchr/testify`（断言，已转 direct）**仅作用于 `_test.go`**，生产二进制（`go build ./...`）不引入它们，也不连任何外部服务。

## 其它测试（按包）

除 handler 集成测试外，各内部包均有针对性单测（全部零外部依赖，`go test ./...` 一并运行）：

- `internal/storage/minio_test.go`：用 `httptest` 起**假 S3 服务**跑 MinIO 驱动真实 SDK 调用链（SaveBytes / Presign / SignGetURL / Delete）。要点：minio-go 需显式 `credentials.NewStaticV4`（否则触发 IAM 凭据链查询）与固定 `Region`（否则会打 `GET /bucket?location` 区域探测）。
- `internal/storage/oss_test.go`：用 `httptest` 起假 OSS 服务（`oss.ForcePathStyle(true)` 才能打 IP:PORT 端点），覆盖上传 / 预签名 / 签名 / 删除与构造期 fail-fast（空 endpoint/bucket 直接报错，不拖到上传才失败）。
- `internal/storage/storage_test.go`：`checkFile` 扩展名白名单 + 文件头魔法字节嗅探的各类型合法 / 伪造场景。
- `internal/middleware/observability_test.go`：RequestID 透传 / 生成、AccessLog 前缀（5xx 走 `[ACCESS-5XX]`）、Metrics 计数与 Prometheus 文本格式。
- `internal/middleware/ratelimit_test.go`：登录限流计数与 IP 隔离。
- `internal/repository/session.go`（黑名单读写 / 清理）无独立单测，由 handler 集成测试 `TestSingleDeviceRevocation` 覆盖其完整链路。

CI：见仓库根 `.github/workflows/ci.yml`，共四个 job：
  - `backend`（**默认必跑，零外部依赖**）：`go build` + `go vet` + `go test ./...`。
    未设置 `TEST_MYSQL_DSN` 时，两个 MySQL 用例（`TestAPIFlowMySQL` / `TestMySQLMigrations`）自动 `t.Skip`，
    因此本 job **不需要任何数据库账号**，靠 SQLite 覆盖全部业务流程。
  - `backend-mysql`（**可选**）：仅当仓库配置了变量 `TEST_MYSQL_DSN` 时才运行，
    先跑「连接 + 建表」再跑完整 parity，用于校验生产 MySQL 契约。
    配置路径：仓库 Settings → Secrets and variables → Actions → Variables 新增 `TEST_MYSQL_DSN`
    （例：`root:pwd@tcp(host:3306)/langhui_test?charset=utf8mb4&parseTime=true`）。
  - `frontend`：`npm ci` + `npm run build` + `npm run test`（Vitest，含 PC 后台视图层测试与经前端工程纳管的小程序纯逻辑测试）。
  - `frontend-typecheck`（**阻塞门禁**）：`vue-tsc --noEmit`。历史 4 个 `@wangeditor/*` 类型错误已通过 tsconfig `paths` 指向真实 d.ts + 精准断言修复，当前 0 错误。

## 待补全（下一步，均为可选增强）
- 富文本中视频号块渲染：已落地 `rich-video` 组件 + `utils/richtext.ts` 鲁棒解析（见 miniprogram README）
- 存储增强已落地：直传预签名 / CDN 刷新 / 私有桶限时签名（见上文「存储增强」）
- 内容模型软删除已落地（见「安全加固」），如需「回收站 / 恢复」可在此基础上扩展
- 可观测性已落地：RequestID / AccessLog / Metrics + 黑名单过期清理 + 备份脚本（见「安全加固」与仓库根 `scripts/backup.sh`）
- 后续可按需补充：更多 CDN 厂商、上传进度/断点续传、多实例会话共享（Redis）、完整监控栈（Prometheus + Alertmanager）、PITR（binlog）等

## 说明
- 需求使用「账号 + 密码」登录，不走微信 openid 免鉴权，因此自研部署完全够用；若日后迁云托管，仅需容器化 + 配置外置（已在 config 中预留），不依赖微信云能力。
- 密码使用 bcrypt 哈希，token 使用 HS256 签名（JWT_KEY 环境变量）。
