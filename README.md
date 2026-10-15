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
| `backend/migrations/` | MySQL 建表与种子 SQL（含软删除迁移 `0003`） |
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

## 测试与 CI

CI 见 `.github/workflows/ci.yml`，三个 job：

- **后端 (Go / SQLite)**：`go build` + `go vet` + `go test ./...`，**默认零外部依赖**，用 SQLite 跑完整接口流程与上传内容嗅探。
- **后端 (Go / MySQL parity)**：可选 job，仅当仓库配置了 `TEST_MYSQL_DSN` 变量时运行，校验生产 MySQL 契约。
- **前端 (Vue)**：`npm ci` + `npm run build` + `npm test`（Vitest）。

> 前端另有一个 **非阻塞** 的 `frontend-typecheck` job 跑 `vue-tsc --noEmit`，用于暴露类型问题（目前受限于 `@wangeditor/*` 在 `moduleResolution: bundler` 下的类型声明，仍报若干错误，不影响 `vite build` 转译，故未设为硬性门禁）。

## 安全与已知限制

已加固项（详见各子目录 README）：
- 上传文件做 **扩展名白名单 + 文件头魔法字节嗅探**，防止把可执行文件改名图片上传。
- 登录接口按 IP **固定窗口限流**，防爆破。
- 启动自检：JWT 密钥为默认值或超管口令为默认值时打印 **[SECURITY] 告警**。
- 内容模型（分类 / 项目 / 服务）支持 **软删除**（GORM `DeletedAt`）。

当前版本**已知边界**（需基建/流程配合，尚未在本仓库落地）：
- JWT 无主动吊销 / 刷新机制，token 存前端 `localStorage`（XSS 风险，建议后续迁 httpOnly Cookie）。
- 小程序码 / access_token 等内存缓存在多实例下不共享，水平扩展需引入 Redis。
- 缺监控 / 日志告警 / 数据库备份策略；生产需 HTTPS 与反向代理。
- MinIO / OSS 真实驱动未含端到端单测；小程序端无自动化单测（需微信 CLI）。
- 列表接口未分页（运营型小数据量，详见 `frontend/README.md`）。

## 文档导航

- 需求与架构：《朗汇机电小程序V1.3_开发内容与工作量确认》《朗汇机电小程序V1.3_自建技术架构方案》
- 接口 / 字段映射：《首页组件》《跳转页组件》《PC 后台组件与接口字段映射》《三端联调与接口总表》
- 部署：《部署与联调》
- 子工程说明：`backend/README.md` · `frontend/README.md` · `miniprogram/README.md`

## 许可证

[MIT](./LICENSE)
