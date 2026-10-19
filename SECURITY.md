# 安全政策 (Security Policy)

## 支持的版本

| 版本 | 状态 |
|---|---|
| main | ✅ 维护中 |

## 上报安全漏洞

如发现安全漏洞（如越权、注入、敏感信息泄露等），**请勿在公开 issue 中披露**，
请通过以下方式私信项目维护者，并尽量提供复现步骤与影响范围：

- 邮件 / 站内信联系仓库 Owner
- 或在 GitHub 通过 **Private vulnerability reporting**（仓库 Settings → Security → Private
  vulnerability reporting）提交

我们会在收到后 72 小时内确认，并协商修复与披露时间线。

## 安全设计要点（当前实现）

- 登录鉴权：账号 + 密码（bcrypt 哈希），JWT（HS256，密钥来自 `JWT_KEY` 环境变量）。
- JWT 双令牌：access（15 分钟）+ refresh（30 天）；`POST /api/refresh` 用 refresh 换发新 access，避免频繁重登。
- **PC 管理后台：httpOnly Cookie 鉴权**——access/refresh 写入 `HttpOnly; SameSite=Lax` 的 Cookie（`access_token` / `refresh_token`），
  由浏览器自动携带、JS 不可读，规避 XSS 窃取令牌；`SameSite=Lax` 阻止跨站写操作携带 Cookie，缓解 CSRF。`POST /api/logout` 清除 Cookie。
- **小程序端：Bearer 头鉴权**——token 由 JS 显式携带（微信沙箱内代码受审核，无 DOM XSS 面，风险可接受）。
- 即时吊销（两级）：
  - **账号级**（`token_version` 安全戳）：改密 / 管理员重置密码 / 禁用启用时自增版本号，鉴权中间件校验令牌内 `vid` 与当前版本一致，**旧令牌立即失效**（该账号所有设备，无需黑名单表，多实例 / 重启均安全）。
  - **会话级**（`revoked_sessions` 黑名单）：令牌含 `sid`（会话 ID，一次登录一个，access/refresh 共享，刷新不换）。`POST /api/logout` 把当前会话 sid 写入黑名单，该设备所有令牌立即失效且无法靠刷新绕过；**不影响同一账号的其他设备登录**。
- 上传：扩展名白名单 + 文件头魔法字节嗅探，单文件 ≤10MB；私有桶支持限时签名访问。
- 登录限流：按客户端 IP 固定窗口限流，防爆破（刷新接口单独限流）。
- 软删除：内容模型支持软删除，避免误删。

## 已知待加固项（尚未落地）

- Cookie 为 `SameSite=Lax`（同源部署已足够缓解 CSRF）；若未来跨域部署或需更强保证，可加 double-submit CSRF Token。
- 生产须设 `COOKIE_SECURE=true`（HTTPS）并前置反向代理；跨子域共享登录态才需设 `COOKIE_DOMAIN`。
- 黑名单表 `revoked_sessions` 暂无自动清理任务，可后续加定时 SQL（按 `expires_at` 删除过期行）。
- ~~无集中式密钥管理 / 审计日志 / 监控告警~~：**审计日志已落地**（`audit_log` 表 + 「平台管理 → 操作日志」），
  覆盖后台写操作与登录失败，敏感值脱敏不落明文、只提供查询。仍未落地的是集中式密钥管理与完整监控告警栈。
- 多实例部署下，小程序码 / access_token 等内存缓存不共享，需引入 Redis。

> 上线前务必修改默认 `JWT_KEY` 与初始超管口令 `ADMIN_PASS`（这两个默认值连同演示账号写进了公开文档，属已知凭据）。
> 已由代码强制把关，不依赖人记住：`docker-compose.yml` 中三者为必填变量（未设置则 compose 报错）；
> `cmd/server` 启动检测到缺省 / 过短的 `JWT_KEY` 或缺省口令时**拒绝启动**并打印 `[SECURITY][FATAL]`（本地联调加 `ALLOW_INSECURE=1` 放行，切勿用于生产）。
> 部署前可先跑 `scripts/preflight.sh` 做静态自检（含小程序端 `DEFAULT_BASE_URL` 种子是否仍指向 127.0.0.1）。
