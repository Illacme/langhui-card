# 微传名片（三端工程）

企业电子名片小程序：员工扫码/分享进入名片页（含公司信息、经典业绩、业务服务、视频号、埋点），
员工可自助申请开通与修改名片，超管在 PC 后台统一管理内容、审核与员工。

本仓库为 **monorepo**，包含后端、PC 管理后台、微信小程序三端与中文映射文档。

## 目录结构

| 目录 | 说明 |
|---|---|
| `backend/` | Go + Gin + GORM 后端服务（API / 鉴权 / 存储 / 小程序码 / 审核流） |
| `frontend/` | Vue 3 + Element Plus 的 PC 管理后台（超管使用） |
| `miniprogram/` | 原生微信小程序前端（员工 / 访客侧） |
| `backend/migrations/` | MySQL 建表与种子 SQL（含软删除迁移 `0003` 等） |
| `docs/` | OpenAPI 3 接口规范（`openapi.yaml`）· 分支保护协作规范（`branch-protection.md`）· 生产上线 Runbook（`deploy-runbook.md`） |
| `scripts/` | 运维脚本（`preflight.sh`：上线前配置自检；`backup.sh`：MySQL 逻辑备份 + 轮转；`alert_check.sh`：最小告警三规则） |
| `*.md`（根目录） | 需求 / 架构 / 接口字段映射等中文文档 |

## 技术栈

- **后端**：Go 1.26 · Gin · GORM · MySQL（生产）/ SQLite（默认测试）· MinIO / 阿里云 OSS（可选对象存储）
- **前端**：Vue 3.5 · TypeScript · Vite 5 · Element Plus 2 · Pinia · Vue Router 4 · Vitest
- **小程序**：微信小程序原生 + TypeScript

## 快速开始

### 方式一：免 MySQL 本地联调（推荐）
后端用 SQLite 起一个自带演示数据的开发态服务，小程序 / 前端可直接联调：
```bash
cd backend && go run ./cmd/devserver      # 监听 :8080
```
演示账号：超管 `admin / admin123456`；员工手机 `13800000001` / 密码 `demo1234`（staffId=1）。
初始超管由后端按 `ADMIN_USER` / `ADMIN_PASS` 首次启动时 seed（表为空才建）；之后可在
「平台管理 → 账号管理」改用户名或新增平台超管，不必再改环境变量或改库。
> 注意：`go run ./cmd/devserver` 每次启动都用**新的临时 SQLite 库**，改过的密码/新建的账号
> 重启即回到初始演示状态（开发态有意设计）。需要持久化请用 `cmd/server`（MySQL）或 docker-compose。

### 方式二：容器一键起（两种数据库形态任选）
```bash
cp .env.example .env

# 形态一（默认）：零依赖起步——只起 backend + frontend，浏览器开 http://localhost:8080
# 进安装向导选 SQLite 即可，JWT_KEY / ADMIN_PASS / MYSQL_ROOT_PASSWORD 全部不用给
docker compose up -d --build

# 形态二：生产 MySQL——加 --profile mysql 额外起 mysql 容器（MYSQL_ROOT_PASSWORD 必填；
# --profile 是顶层标志，必须放在 up 之前）
docker compose --profile mysql up -d --build
```
> `docker-compose.yml` 里只有 `MYSQL_ROOT_PASSWORD` 是**必填**变量，且只在
> `--profile mysql` 形态校验：未设置则 mysql 容器启动即报错退出（日志有明确提示），
> 不会带默认值悄悄启动；默认 SQLite 形态连 mysql 都不起，完全不受影响；
> `JWT_KEY` / `ADMIN_PASS` 刻意放宽为**可选**——留空即进入安装向导，由向导现场生成强随机值
> 并写进 `/app/data/runtime.env`（详见 `docs/deploy-runbook.md` §2.1）。
> 后端 `cmd/server` 也会拒绝用缺省密钥 / 缺省口令启动（`InsecureGates`）。
> 上线前可跑 `scripts/preflight.sh` 做一次静态自检（含小程序端引导基址是否仍指向 127.0.0.1）。

### 方式三：分别启动（开发态）
```bash
# 后端（需自备 MySQL，配置 DB_DSN / JWT_KEY 等环境变量）
cd backend && go run ./cmd/server

# 前端
cd frontend && npm install && npm run dev   # http://localhost:5173

# 小程序：微信开发者工具导入 miniprogram/ 目录
```

## 系统设置（运行参数后台化）

产品运行参数不再写死在代码里，平台超管可在 PC 后台「平台管理 → 系统设置」界面维护。
配置分**三档生效方式**（界面上用标签标明，保存时接口也会点名提示）：

| 档位 | 含义 | 配置项 |
| --- | --- | --- |
| 即时生效 | 保存即写入内存缓存，无需重启 | 下表「常规」全部 |
| 需重启 | 客户端在启动时构造，重启后才生效 | `storage_*`、`cdn_*`、`upload_dir`、`upload_base_url` |
| 只读 | 界面只**掩码展示当前值**并提示改法，改它要动环境变量后重启 | `db_dsn`、`jwt_key`、`cookie_secure`、`cookie_domain` |

| 分组 | 配置项 | 说明 |
| --- | --- | --- |
| 通用 | `brand_name` | 后台侧边栏 / 登录页 / 页标题品牌名 |
| 小程序 | `miniprogram_base_url` | 小程序端接口基址（须 `http(s)://` 开头；冷启动拉 `/api/public/settings` 覆盖） |
| 小程序 | `pinned_company_id` | 企业独立版锁定租户（>0 锁定且该企业须存在并启用，0 = 平台模式） |
| 上传 | `upload_max_mb` | 单文件上传上限（MB，1~1024） |
| 上传 | `upload_allow_ext` | 允许上传的扩展名（逗号分隔） |
| 上传 | `presign_put_ttl_minutes` / `sign_get_ttl_minutes` | 直传 PUT / 私有桶 GET 签名链接有效期（分钟，最短 1） |
| 微信 | `wxacode_ttl_hours` | 小程序码缓存有效期（小时） |
| 微信 | `wxacode_sign_ttl_minutes` | 私有桶下小程序码签名链接有效期（分钟，最短 1） |
| 微信 | `wx_appid` | 小程序 AppID（留空回退 `WX_APPID`，改完即时生效） |
| 微信 | `wx_appsecret` | 小程序 AppSecret（敏感；留空回退 `WX_APPSECRET`，改完即时生效） |
| 登录与令牌 | `access_token_ttl_minutes` / `refresh_token_ttl_days` | 令牌有效期（最短 1 分钟 / 1 天，非法值回落缺省） |
| 登录与令牌 | `login_rate_limit_per_min` / `refresh_rate_limit_per_min` | 每 IP 每分钟登录 / 刷新次数（防爆破，最小 1） |
| 文件存储与 CDN | `storage_driver` | `local` / `minio` / `oss`（只能取这三个值；留空回退 `STORAGE_DRIVER`，**需重启**） |
| 文件存储与 CDN | `storage_endpoint` / `storage_bucket` | 对象存储端点与桶名（MinIO 与 OSS 共用这两个字段） |
| 文件存储与 CDN | `storage_access_key` / `storage_secret_key` | 对象存储凭据（后者敏感，界面只回传掩码） |
| 文件存储与 CDN | `storage_use_ssl` / `storage_public` | MinIO 走 HTTPS / 是否公开桶（私有桶走限时签名） |
| 文件存储与 CDN | `upload_dir` / `upload_base_url` | 本地存储目录 / 上传文件公开访问基址 |
| 文件存储与 CDN | `cdn_enabled` / `cdn_access_key_id` / `cdn_access_key_secret` / `cdn_endpoint` | CDN 缓存刷新（覆盖写后使边缘缓存失效） |
| 安全与基础设施 | `db_dsn` / `jwt_key` / `cookie_secure` / `cookie_domain` | **只读**，仅掩码展示当前值，修改走环境变量后重启 |
| 功能开关 | `metrics_enabled` | 是否开放 `/metrics`（Prometheus 指标）；留空则跟随 `METRICS_ENABLED` |
| 功能开关 | `independent_lock` | 独立版企业锁定：开启后企业管理页只读（不可新建/编辑/启停企业、不可加企业管理员） |

> **为什么分三档？** 判据是「改错能否自愈」，不是「是否敏感」：
> - 云存储 / CDN 凭据填错最坏是上传失败，**改回来重启即可**，所以允许在界面维护；
> - `DB_DSN` / `JWT_KEY` 改错会把「改回来的入口」一起弄丢（连不上库 → 读不出配置；
>   会话全失效 → 登不进后台），所以只展示不编辑；
> - 敏感项（密钥 / 连接串）接口**只回传掩码**，真实值永不出现在响应里；界面输入框留空 = 不修改。
> - 令牌有效期与签名时长这类「设成 0 就等于把功能关死、甚至把自己锁在外面」的项，
>   代码内保留**安全下限**：界面填 0 / 负数一律回落缺省值。
> - **类型合法 ≠ 取值合法**：对「填错会造成大面积故障」的项，保存入口还会做值域校验并当场拒绝——
>   `upload_max_mb`（1~1024）、`miniprogram_base_url` / `upload_base_url`（必须 `http(s)://` 开头）、
>   `storage_driver`（只能 `local` / `minio` / `oss`）、`pinned_company_id`（必须是存在且启用的企业，0 = 平台模式）。
>   例如上传上限填 0 会让所有上传失败、报错却指向文件；基址漏写协议头会让小程序永远连不上，
>   且「后台下发正确基址」只在种子地址能连通一次之后才生效，等于自愈不了。
>
> 另：云存储驱动初始化失败时会**降级为本地磁盘**而不是让进程退出——否则管理员
> 连后台都进不去，就再也改不回来（见 `cmd/server` 的 `newUploaderWithFallback`）。

接口：`GET/PUT /api/admin/settings`（平台超管；PUT 回包含 `data.restartKeys` 点名需重启的项）、
`GET /api/public/settings`（公开，仅品牌名 / 小程序基址 / 锁定租户）。

## 测试与 CI

CI 见 `.github/workflows/ci.yml`，四个 job：

- **后端 (Go / SQLite)**：`go build` + `go vet` + `go test ./...`，**默认零外部依赖**，用 SQLite 跑完整接口流程、上传内容嗅探与 MinIO/OSS 驱动的 httptest 假服务测试。
- **后端 (Go / MySQL parity)**：可选 job，仅当仓库配置了 `TEST_MYSQL_DSN` 变量时运行，校验生产 MySQL 契约。
- **前端 (Vue)**：`npm ci` + `npm run build` + `npm test`（Vitest；含 PC 后台视图层测试与经前端工程纳管的小程序纯逻辑测试）。
- **前端类型检查 (vue-tsc)**：`vue-tsc --noEmit`，历史 4 个 `@wangeditor/*` 类型错误已修复，**现为阻塞门禁**。

> 分支保护与协作规范见 `docs/branch-protection.md`；接口契约见 `docs/openapi.yaml`（OpenAPI 3.0.3，56 paths）。

## 安全与已知限制

已加固项（详见各子目录 README）：
- 上传文件做 **扩展名白名单 + 文件头魔法字节嗅探**，防止把可执行文件改名图片上传。
- 登录接口按 IP **固定窗口限流**，防爆破。
- 启动自检：JWT 密钥为默认值或超管口令为默认值时打印 **[SECURITY] 告警**。
- 内容模型（分类 / 项目 / 服务）支持 **软删除**（GORM `DeletedAt`）。
- JWT **双令牌 + 两级即时吊销**：access 15 分钟 / refresh 30 天，`POST /api/refresh` 静默续期；账号级走 `token_version` 安全戳（改密 / 禁用即时失效），会话级走 `revoked_sessions` 黑名单（**单设备精确吊销**，登出只失效当前设备）。
- PC 管理后台 **httpOnly Cookie 鉴权**（`HttpOnly; SameSite=Lax`），token 不下发到 JS，防 XSS 窃取；小程序端走 Bearer 头。
- **可观测性**：`X-Request-Id` 贯穿请求（响应头带回，便于报障定位）；每请求一行访问日志（方法/路径/状态/耗时/IP，5xx 单独 `[ACCESS-5XX]` 前缀供告警匹配）；`METRICS_ENABLED=1` 可选开启 `/metrics`（Prometheus 文本格式）。
- **上线前自检已落地**：`scripts/preflight.sh`（零依赖静态检查：JWT 密钥 / 初始口令 / 小程序引导基址 / 存储凭据 / 上传基址 / Cookie / 数据库口令 / 服务探活，输出 PASS·WARN·FAIL·SKIP，有 FAIL 退出码 1；`PREFLIGHT_STRICT=1` 时 WARN 也算失败，便于接发布流水线）。与后端启动门槛是同一套判据的两道闸——门槛负责「跑不起来」，本脚本负责「跑起来之前先告诉你差哪几项」。
- **运维脚本**：`scripts/backup.sh` 做 MySQL 逻辑备份（mysqldump → gzip → 按保留天数轮转，可入 crontab）。
- **测试覆盖**：后端 httptest 集成（SQLite 零外部依赖 + 可选 MySQL parity）+ MinIO/OSS 假服务单测；前端含视图层组件测试；小程序纯逻辑测试经前端 Vitest 运行。
- **操作审计**：后台写操作（改系统设置 / 管理员增删改 / 企业启停 / 改密码）与**登录失败**
  全部记进 `audit_log`，平台超管可在「平台管理 → 操作日志」按动作 / 操作人 / 结果筛选查询。
  敏感值只记「已修改」不落明文（否则审计表会变成新的泄密面）；只提供查询、不提供修改——
  审计记录一旦可改就等于没有审计。
- **平台超管账号管理**：PC 后台「平台管理 → 账号管理」可改用户名、新增平台超管、重置他人密码、删除账号（护栏：不能删自己、至少保留一个平台超管）。改自己密码仍走右上角「修改密码」（需原密码）。
- **管理端列表服务端分页**：员工 / 企业 / 开通申请 / 名片修改申请 / 平台超管账号 / 审计日志
  统一支持 `?page=&size=`（缺省 20、上限 100）与服务端关键字 / 状态筛选，回包
  `{ code:0, data:[...当前页...], total, page, size }`（`data` 仍是数组，总数与页码在根层）。
  筛选必须落在服务端：前端拿到全量再筛会出现「搜得到但翻页后又没了」的错觉。
  分类 / 项目 / 服务（数量有界且需整表拖拽排序）与单个企业下的管理员列表仍不分页。

当前版本**已知边界**（需基建/流程配合，尚未在本仓库落地）：
- 小程序码 / access_token 等内存缓存在多实例下不共享，水平扩展需引入 Redis。
- **最小告警已落地**：`scripts/alert_check.sh`（healthz 探活 / 5xx / 备份新鲜度，零依赖，crontab 每分钟运行，详见《部署与联调》「最小告警配置」）；待运维侧按需扩展的是完整监控栈（Prometheus + Alertmanager）、外部拨测与 PITR（binlog 时间点恢复）；生产需 HTTPS 与反向代理。
- 分类 / 业绩项目 / 业务服务列表未分页（数量有界且需整表拖拽排序）；其余管理端列表已服务端分页（见上）。

## 文档导航

- 需求与架构：《微传名片V1.3_开发内容与工作量确认》《微传名片V1.3_自建技术架构方案》
- 接口 / 字段映射：《首页组件》《跳转页组件》《PC 后台组件与接口字段映射》《三端联调与接口总表》
- 接口规范：`docs/openapi.yaml`（OpenAPI 3.0.3）
- 部署与运维：《部署与联调》（联调环境）· `docs/deploy-runbook.md`（生产上线 Runbook）· `scripts/preflight.sh` · `scripts/backup.sh` · `scripts/alert_check.sh`
- 协作规范：`docs/branch-protection.md`（分支保护 / CI 门禁 / 评审流）
- 子工程说明：`backend/README.md` · `frontend/README.md` · `miniprogram/README.md`

## 许可证

[MIT](./LICENSE)
