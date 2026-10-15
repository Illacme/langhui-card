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
- 即时吊销：用户模型含 `token_version`（安全戳）。改密 / 管理员重置密码 / 禁用启用时自增版本号，
  鉴权中间件校验令牌内 `vid` 与当前版本一致，**旧令牌立即失效**（账号级吊销，无需黑名单表，多实例 / 重启均安全）。
- 上传：扩展名白名单 + 文件头魔法字节嗅探，单文件 ≤10MB；私有桶支持限时签名访问。
- 登录限流：按客户端 IP 固定窗口限流，防爆破（刷新接口单独限流）。
- 软删除：内容模型支持软删除，避免误删。

## 已知待加固项（尚未落地）

- access / refresh token 仍存于前端 `localStorage`，存在 XSS 窃取风险（可改 httpOnly Cookie，见 A2）。
- 吊销粒度为「账号级」：改密 / 禁用会令该账号所有会话失效；单设备精确吊销需引入 refresh 令牌的 jti 黑名单（后续）。
- 无集中式密钥管理 / 审计日志 / 监控告警；生产需 HTTPS 与反向代理。
- 多实例部署下，小程序码 / access_token 等内存缓存不共享，需引入 Redis。

> 上线前务必修改默认 `JWT_KEY` 与初始超管口令 `ADMIN_PASS`（服务启动时会打印 `[SECURITY]` 告警）。
