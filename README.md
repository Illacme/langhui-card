# 朗汇机电 · 电子名片小程序（三端工程）

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
| `scripts/` | 运维脚本（`backup.sh`：MySQL 逻辑备份 + 轮转；`alert_check.sh`：最小告警三规则） |
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

### 方式二：容器一键起（MySQL + 后端 + 前端）
```bash
cp .env.example .env          # 按需修改 JWT_KEY / ADMIN_PASS 等
docker compose up -d --build  # 访问 http://localhost:8080
```

### 方式三：分别启动（开发态）
```bash
# 后端（需自备 MySQL，配置 DB_DSN / JWT_KEY 等环境变量）
cd backend && go run ./cmd/server

# 前端
cd frontend && npm install && npm run dev   # http://localhost:5173

# 小程序：微信开发者工具导入 miniprogram/ 目录
```

## 系统设置（运行参数后台化）

产品运行参数不再写死在代码里，平台超管可在 PC 后台「平台管理 → 系统设置」界面维护，
**保存即时生效、无需重启**：

| 分组 | 配置项 | 说明 |
| --- | --- | --- |
| 通用 | `brand_name` | 后台侧边栏 / 登录页 / 页标题品牌名 |
| 小程序 | `miniprogram_base_url` | 小程序端接口基址（冷启动拉 `/api/public/settings` 覆盖） |
| 小程序 | `pinned_company_id` | 企业独立版锁定租户（>0 锁定，0 = 平台模式） |
| 上传 | `upload_max_mb` | 单文件上传上限（MB） |
| 上传 | `upload_allow_ext` | 允许上传的扩展名（逗号分隔） |
| 微信 | `wxacode_ttl_hours` | 小程序码缓存有效期（小时） |
| 微信 | `wx_appid` | 小程序 AppID（留空则回退环境变量 `WX_APPID`） |
| 功能开关 | `metrics_enabled` | 是否开放 `/metrics`（Prometheus 指标）；留空则跟随环境变量 `METRICS_ENABLED` |
| 功能开关 | `independent_lock` | 独立版企业锁定：开启后企业管理页只读（不可新建/编辑/启停企业、不可加企业管理员） |

> **密钥类不进界面**：数据库地址、JWT 密钥、云存储 / CDN 凭据、微信 AppSecret 等
> 始终走环境变量（见 `.env.example`），不落库、不可在界面编辑，避免敏感信息泄漏。

接口：`GET/PUT /api/admin/settings`（平台超管）、`GET /api/public/settings`（公开，仅上表前 3 项）。

## 测试与 CI

CI 见 `.github/workflows/ci.yml`，四个 job：

- **后端 (Go / SQLite)**：`go build` + `go vet` + `go test ./...`，**默认零外部依赖**，用 SQLite 跑完整接口流程、上传内容嗅探与 MinIO/OSS 驱动的 httptest 假服务测试。
- **后端 (Go / MySQL parity)**：可选 job，仅当仓库配置了 `TEST_MYSQL_DSN` 变量时运行，校验生产 MySQL 契约。
- **前端 (Vue)**：`npm ci` + `npm run build` + `npm test`（Vitest；含 PC 后台视图层测试与经前端工程纳管的小程序纯逻辑测试）。
- **前端类型检查 (vue-tsc)**：`vue-tsc --noEmit`，历史 4 个 `@wangeditor/*` 类型错误已修复，**现为阻塞门禁**。

> 分支保护与协作规范见 `docs/branch-protection.md`；接口契约见 `docs/openapi.yaml`（OpenAPI 3.0.3，47 paths）。

## 安全与已知限制

已加固项（详见各子目录 README）：
- 上传文件做 **扩展名白名单 + 文件头魔法字节嗅探**，防止把可执行文件改名图片上传。
- 登录接口按 IP **固定窗口限流**，防爆破。
- 启动自检：JWT 密钥为默认值或超管口令为默认值时打印 **[SECURITY] 告警**。
- 内容模型（分类 / 项目 / 服务）支持 **软删除**（GORM `DeletedAt`）。
- JWT **双令牌 + 两级即时吊销**：access 15 分钟 / refresh 30 天，`POST /api/refresh` 静默续期；账号级走 `token_version` 安全戳（改密 / 禁用即时失效），会话级走 `revoked_sessions` 黑名单（**单设备精确吊销**，登出只失效当前设备）。
- PC 管理后台 **httpOnly Cookie 鉴权**（`HttpOnly; SameSite=Lax`），token 不下发到 JS，防 XSS 窃取；小程序端走 Bearer 头。
- **可观测性**：`X-Request-Id` 贯穿请求（响应头带回，便于报障定位）；每请求一行访问日志（方法/路径/状态/耗时/IP，5xx 单独 `[ACCESS-5XX]` 前缀供告警匹配）；`METRICS_ENABLED=1` 可选开启 `/metrics`（Prometheus 文本格式）。
- **运维脚本**：`scripts/backup.sh` 做 MySQL 逻辑备份（mysqldump → gzip → 按保留天数轮转，可入 crontab）。
- **测试覆盖**：后端 httptest 集成（SQLite 零外部依赖 + 可选 MySQL parity）+ MinIO/OSS 假服务单测；前端含视图层组件测试；小程序纯逻辑测试经前端 Vitest 运行。

当前版本**已知边界**（需基建/流程配合，尚未在本仓库落地）：
- 小程序码 / access_token 等内存缓存在多实例下不共享，水平扩展需引入 Redis。
- **最小告警已落地**：`scripts/alert_check.sh`（healthz 探活 / 5xx / 备份新鲜度，零依赖，crontab 每分钟运行，详见《部署与联调》「最小告警配置」）；待运维侧按需扩展的是完整监控栈（Prometheus + Alertmanager）、外部拨测与 PITR（binlog 时间点恢复）；生产需 HTTPS 与反向代理。
- 列表接口未分页（运营型小数据量，详见 `frontend/README.md`）。

## 文档导航

- 需求与架构：《朗汇机电小程序V1.3_开发内容与工作量确认》《朗汇机电小程序V1.3_自建技术架构方案》
- 接口 / 字段映射：《首页组件》《跳转页组件》《PC 后台组件与接口字段映射》《三端联调与接口总表》
- 接口规范：`docs/openapi.yaml`（OpenAPI 3.0.3）
- 部署与运维：《部署与联调》（联调环境）· `docs/deploy-runbook.md`（生产上线 Runbook）· `scripts/backup.sh` · `scripts/alert_check.sh`
- 协作规范：`docs/branch-protection.md`（分支保护 / CI 门禁 / 评审流）
- 子工程说明：`backend/README.md` · `frontend/README.md` · `miniprogram/README.md`

## 许可证

[MIT](./LICENSE)
