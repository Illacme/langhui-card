# 上线前核对清单

> 生产部署前逐项过一遍；另有零依赖静态自检：`scripts/preflight.sh`
> （接 CI / 发布流水线加 `PREFLIGHT_STRICT=1`，使 WARN 也算失败）。
> 后端启动门槛 `InsecureGates` 与本清单是同一套判据的两道闸——脚本负责「跑之前告诉你差什么」，
> 门槛负责「跑不起来」。

## 一、阻断级（不改 = 起不来 / 任何人可登录 / 功能全挂）

| 项 | 改哪里 | 验证已生效 | preflight 覆盖 |
|---|---|---|---|
| 初始超管口令 `ADMIN_PASS` | 环境变量 / `data/runtime.env`（留空走安装向导现场生成） | 不再是内置缺省值 `admin123456`（该值已随演示账号写进公开文档）；建议 `openssl rand -base64 12` | ✅ 查缺省值 / 长度 |
| 签名密钥 `JWT_KEY` | 同上 | 不再是内置缺省值；长度 ≥ 32（`openssl rand -base64 32`） | ✅ 查缺省值 / 长度 |
| 部署形态 | 用 `cmd/server`（MySQL）或 `docker-compose`；**不要**用 `cmd/devserver` | devserver 每次建临时 SQLite，重启即丢 | ⚠️ 需人工确认启动方式 |
| HTTPS + 反向代理前置 | 网关 / 反代配置 | 对外只暴露 https，后台 `COOKIE_SECURE=true` | ✅ 查 `COOKIE_SECURE`；设 `PUBLIC_BASE_URL` 后还查外部可达性与证书有效期 |
| 小程序种子基址 `DEFAULT_BASE_URL` | 小程序源码常量 / 部署注入 | 是线上 https 域名；**不能**是 `127.0.0.1` / `localhost` / `http`（真机上 127 是手机自身，首次请求失败后再也拿不到后台下发的正确基址） | ✅ 查 127/localhost/http |
| `COOKIE_SECURE` | 环境变量 / `runtime.env`（只读项，只能改环境） | 生产 HTTPS 下为 `true` | ✅ |

## 二、功能级（不改 = 线上功能残缺，但服务能起）

| 项 | 改哪里 | 验证已生效 | preflight 覆盖 |
|---|---|---|---|
| 对象存储 `STORAGE_DRIVER` + 凭据 | 「系统设置 → 文件存储与 CDN」或环境变量 `STORAGE_*` | 取值只可是 `local` / `minio` / `oss`；选非 local 时 `STORAGE_ENDPOINT` / `STORAGE_BUCKET` / 凭据已填 | ✅ 查合法值 / 凭据非空 |
| 上传基址 `UPLOAD_BASE_URL` | 同上 `UPLOAD_BASE_URL` | 是真实可访问的 https 基址（也可在后台系统设置覆盖并重启） | ✅ 查本机地址 / http |
| 微信 `wx_appid` / `wx_appsecret` | 后台「系统设置」覆盖（留空回退 `WX_*` 环境变量） | 真机扫码能拉起、小程序码能生成 | ✅ 查是否配置与格式；`PREFLIGHT_WX_LIVE=1` 时真调微信接口验证是否配对 |
| 数据库迁移已应用到最新（MySQL 升级） | 启动时自动补齐（`AUTO_MIGRATE=0` 关闭，改跑 `server migrate`） | 启动日志是「已应用 N 个迁移」或「数据库已是最新」，**无** `[MIGRATE][FATAL]` | ❌ 需看启动日志（详见 runbook §4） |
| CDN 刷新凭据 `CDN_*` | 同上「文件存储与 CDN」或环境变量 | 需刷新时选了厂商（`CDN_PROVIDER`，五家之一）并填全该厂商要求的凭据、开 `CDN_ENABLED` | ❌ 需人工确认 |
| `MYSQL_ROOT_PASSWORD` | `docker-compose.yml` / 环境 | 仅 `--profile mysql` 形态必填；改强随机值（默认 SQLite 形态无所谓） | ✅ 查示例值 / 必填 |

## 三、运行级（上线后看护）

- **部署前跑一遍** `bash scripts/preflight.sh`：期望全 `PASS`；`PREFLIGHT_STRICT=1` 时 `WARN` 也返回退出码 1。
- **接最小告警** `scripts/alert_check.sh`（crontab 每分钟）：healthz 本地探活 / 5xx / 备份新鲜度 /
  **外部拨测**（配 `EXTERNAL_URL`，覆盖反代 / 证书 / 防火墙这类本机探活看不见的故障）。
- 数据库：MySQL 形态请确认 `DB_DSN` 指向的实例可达、已建库、迁移已应用到最新、备份策略就位
  （SQLite 形态库文件勿放 NFS，单机单进程，备份用 `VACUUM INTO`）。
- 代码层兜底已就位：内置缺省密钥 / 口令会被 `InsecureGates` 拒绝启动，不会带着默认凭据悄悄上线。

## 关联文档
- 生产上线 Runbook：`docs/deploy-runbook.md`
- 部署与联调：`部署与联调.md`
- 安全边界：`SECURITY.md`
