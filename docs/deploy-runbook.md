# 生产上线 Runbook（从零到发布）

> 适用场景：首次部署到生产服务器。联调 / 日常开发见《部署与联调.md》，本文只讲生产路径。
> **没有 Docker，或想先跑起来测试？** 直接看下方「无 Docker 部署」一节（三条路径，A 路径十分钟可跑通）；就绪后再按第 0 节起的 Docker 主流程上生产。
> 预计耗时：服务器与域名就绪后 1–2 小时（不含微信审核等待）。

## 无 Docker 部署（先测试 / 无 Docker 环境）

本项目不依赖 Docker 也能完整运行：后端 `cmd/devserver` 用纯 Go SQLite（免 cgo、免 MySQL），前端 vite dev server 自带 `/api` 代理。
前置：**Go ≥ 1.26**（`go.mod` 要求，`go version` 确认）+ **Node ≥ 20**。

### A. 先跑起来测（同机，约 10 分钟）

```bash
# 终端 1：后端 —— SQLite + 演示数据，不需要 MySQL
cd backend && go run ./cmd/devserver          # 监听 :8080

# 终端 2：PC 管理后台 —— vite 自动把 /api 代理到 8080
cd frontend && npm install && npm run dev     # 打开 http://localhost:5173
```

- 演示账号：超管 `admin/admin123456`；员工手机 `13800000001` / 密码 `demo1234`
- **小程序**：开发者工具导入 `miniprogram/`，`app.ts` 的 `baseUrl` 指向 `http://<电脑局域网IP>:8080`（`urlCheck:false` 已配，http 可直接连）
- **手机看 PC 后台**：同一 WiFi 下开 `http://<电脑IP>:5173`（vite 绑定 `0.0.0.0`）
- **真机测上传**：设 `DEV_UPLOAD_BASE_URL=http://<电脑IP>:8080/uploads` 再启动 devserver——否则返回给端的头像 URL 写死 `127.0.0.1`，手机取不到图（启动日志里的 `upload base url:` 会显示当前生效值）
- 已知限制：SQLite 库在系统临时目录，**重启即丢数据**（每次重新 seed 演示数据）——功能测试无所谓，要持久化走 B/C

### B. 编译二进制 + nginx（贴近生产的运行形态，仍免 MySQL）

```bash
cd backend && go build -o langhui ./cmd/devserver   # 产出单文件二进制
cd frontend && npm run build                         # 产物在 frontend/dist
```

nginx（即 Docker 版 `frontend/nginx.conf` 的配置，仅把 `backend:8080` 换成 `127.0.0.1:8080`、root 指向 dist）：

```nginx
server {
    listen 80;
    root /path/to/langhui-card/frontend/dist;
    index index.html;
    client_max_body_size 50m;
    location / { try_files $uri $uri/ /index.html; }
    location /api/     { proxy_pass http://127.0.0.1:8080; proxy_set_header Host $host; }
    location /uploads/ { proxy_pass http://127.0.0.1:8080; proxy_set_header Host $host; }
}
```

后端常驻，二选一：

```bash
nohup /path/to/langhui-card/backend/langhui > /var/log/langhui.log 2>&1 &   # 最简
```

```ini
# /etc/systemd/system/langhui.service（推荐：崩溃自拉起、开机自启）
[Unit]
Description=langhui backend (dev mode)
After=network.target

[Service]
WorkingDirectory=/path/to/langhui-card/backend
Environment=DEV_UPLOAD_BASE_URL=http://192.168.1.10:8080/uploads
ExecStart=/path/to/langhui-card/backend/langhui
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload && sudo systemctl enable --now langhui
```

- 数据持久化同 A（临时库重启丢）；要真持久化用路径 C。日后如需 devserver 持久库，可加 `DEV_DB_PATH` 环境变量（当前未实现）。
- `DEV_UPLOAD_BASE_URL` 设为局域网 IP 后，内网同事 / 真机的上传与头像均正常。

### C. 完整生产（MySQL 原生 + systemd + HTTPS，不用 Docker）

1. 安装 MySQL 8.0（`apt install mysql-server` 或 yum），建库 `langhui`，手动应用迁移（命令见第 4 节，与 Docker 无关完全适用）。
2. `go build -o langhui-server ./cmd/server`，环境变量：`DB_DSN` / `JWT_KEY` / `ADMIN_USER` / `ADMIN_PASS` / `UPLOAD_BASE_URL` / `COOKIE_SECURE=true`。
3. systemd 常驻（unit 同 B，`ExecStart` 换二进制、补 `Environment=` 行）。
4. nginx 反代（同 B）+ HTTPS：前置 Caddy 自动证书（配置见第 5 节）。
5. **MySQL parity 验证照常执行（第 6 节）**——与是否用 Docker 无关，生产用 MySQL 就必须跑。

### 三条路径对比

| | A 先跑起来 | B 二进制 + nginx | C 完整生产 |
|---|---|---|---|
| MySQL | 不用 | 不用 | 要（原生安装） |
| 数据持久 | 重启丢 | 重启丢 | 持久 |
| 真机 / 内网上传 | ✅ 设 `DEV_UPLOAD_BASE_URL` | ✅ 同 A | ✅ `UPLOAD_BASE_URL` 可配 |
| HTTPS | 无 | 无（可加 Caddy） | ✅ |
| 适合 | 功能测试 / 演示 | 内网部署 / 给同事看 | 上线 |

## 0. 前置条件

| 项 | 要求 | 说明 |
|---|---|---|
| 服务器 | Linux x86_64，2C4G+，磁盘 40G+ | Go 后端 + nginx + MySQL 8.0 三容器，名片类应用流量小，2C4G 足够 |
| Docker | Docker 24+ 与 compose 插件 | `docker compose version` 验证 |
| 域名 | **已 ICP 备案**，A 记录指向服务器 | 微信小程序合法域名的硬性要求，未备案域名配不了 |
| 端口 | 80 / 443 对外 | 3306 **不要**对公网开放（compose 里 `3306:3306` 生产建议删掉或绑 127.0.0.1） |
| 微信 | 正式 AppID（提审阶段必须） | 联调期可用测试号；`channel-video` 视频号渲染还需绑定视频号 |

## 1. 获取代码与配置

```bash
git clone git@github.com:Illacme/langhui-card.git
cd langhui-card
cp .env.example .env
```

生成强随机值（**不要用默认值**，启动时的 `[SECURITY]` 告警必须全部消除）：

```bash
openssl rand -base64 48   # → JWT_KEY
openssl rand -base64 24   # → MYSQL_ROOT_PASSWORD
openssl rand -base64 12   # → ADMIN_PASS（或自定义强口令）
```

编辑 `.env`，生产必改项：`JWT_KEY`、`MYSQL_ROOT_PASSWORD`、`ADMIN_PASS`、`COOKIE_SECURE=true`（HTTPS 生产必须，见第 5 节）。可选按需：`WX_APPID/WX_APPSECRET`（小程序码）、`MINIO_*`/`OSS_*`（对象存储）、`METRICS_ENABLED=1`。

> **三处「忘了改就出事」的项，已由代码强制把关**（不是靠人记住）：
> 1. `JWT_KEY` / `ADMIN_PASS` / `MYSQL_ROOT_PASSWORD` 在 `docker-compose.yml` 里是**必填变量**
>    （`${VAR:?提示}` 语法），未设置时 `docker compose up` 直接报错、不会带默认值悄悄启动；
> 2. 后端 `cmd/server` 启动时若检测到 `JWT_KEY` 仍是缺省值 `change-me`（或长度 < 16）、
>    或 `ADMIN_PASS` 仍是内置缺省口令，会**拒绝启动**并打印 `[SECURITY][FATAL]`
>    （这两个默认值连同演示账号写进了公开文档，等于已知凭据）；
>    本地联调确需放行加 `ALLOW_INSECURE=1`，**切勿用于生产**；
> 3. 部署前可先跑一遍静态自检（服务不必先启动），它会逐项列出还差什么：
>    ```bash
>    scripts/preflight.sh                 # 有 FAIL 则退出码 1
>    PREFLIGHT_STRICT=1 scripts/preflight.sh   # WARN 也算失败（接发布流水线用）
>    ```

### 小程序端必改（容易漏，且只影响真机）

`miniprogram/utils/config.ts` 的 `DEFAULT_BASE_URL` 默认是 `http://127.0.0.1:8080`。
它是小程序**首次**拉配置用的引导种子——真机上 127.0.0.1 是手机自身、不可达，
首连失败后就永远拿不到后台下发的正确基址（后台配置只有在种子能连通一次之后才生效）。

- 真机联调：改成本机局域网 IP（如 `http://192.168.1.20:8080`）
- 生产：改成 https 域名（如 `https://card.example.com`）

`preflight.sh` 第 3 项会检查这一处；未改时判 FAIL。

## 2. 启动

```bash
docker compose up -d --build
docker compose ps        # mysql / backend / frontend 三服务均 healthy
```

首启时 MySQL 容器自动按序执行 `backend/migrations/0001–0005` 建表；`backend` 等 mysql healthy 后启动，自动 seed 初始超管。

## 3. 首次验证清单（按顺序过一遍）

- [ ] `curl http://127.0.0.1:8090/healthz` → `{"status":"ok"}`（8090 是后端直连调试端口）
- [ ] `docker compose logs backend | grep SECURITY` **无输出**（有输出说明密钥还是默认值，回第 1 步）
- [ ] 浏览器开 `http://<服务器IP>:8080`，用 `.env` 的 `ADMIN_USER/ADMIN_PASS` 登录 PC 后台，**立即改密码**
- [ ] 表结构齐全：`docker exec -i langhui-mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" langhui -e "show tables;"` —— 应包含 `revoked_sessions`（0005）等全部迁移产物
- [ ] 上传一张图片（公司信息/封面），确认返回 URL 可访问（`uploads_data` 卷已挂载）

## 4. 存量数据库手动应用迁移（仅「数据库已存在」时）

compose 的自动迁移只在 `mysql_data` 卷**为空**（首启）时执行。对已有数据库：

```bash
for f in backend/migrations/000*.sql; do
  docker exec -i langhui-mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" langhui < "$f"
done
```

迁移向后兼容、只增不改；若报 `1060 Duplicate column` / `1050 table already exists`，说明该变更已应用过，可忽略。**执行前先 `mysqldump` 备份**。

## 5. HTTPS（生产必须，小程序合法域名的前提）

compose 自身对外是 http。推荐前置 Caddy 自动申请证书（最省事，无需手动续期）：

```yaml
# 追加到 docker-compose.yml
caddy:
  image: caddy:2
  restart: unless-stopped
  ports: ["80:80", "443:443"]
  volumes:
    - ./Caddyfile:/etc/caddy/Caddyfile:ro
    - caddy_data:/data
  depends_on: [frontend]
```

```
# Caddyfile（把域名换成实际域名，Caddy 自动签发 + 续期 Let's Encrypt 证书）
your-domain.com {
  reverse_proxy frontend:80
}
```

同时确认 `.env` 里 `COOKIE_SECURE=true`——否则 httpOnly Cookie 不带 `Secure` 标志，HTTPS 下浏览器虽仍会发送，但丧失了「永不走明文」的保证（A2 加固在生产打折）。

> 也可用云厂商 SLB/ALB 或自建 nginx 终结 TLS，效果等价；`COOKIE_SECURE=true` 同样必须。

## 6. MySQL parity 验证（上线前必做，约 10 分钟）

项目测试默认跑 SQLite，而生产是 MySQL——**本仓库有真实前科**：`ON CONFLICT DO NOTHING` 在 SQLite 测试全绿，但 MySQL 不支持该语法（生产登出会 500）。上线前必须用真实 MySQL 把 parity 跑绿：

```bash
# 起一个一次性 MySQL 8.0（跑完即可删；也可指向测试库）
docker run -d --name parity-mysql -p 13306:3306 \
  -e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=langhui_test mysql:8.0

cd backend
TEST_MYSQL_DSN='root:root@tcp(127.0.0.1:13306)/langhui_test?charset=utf8mb4&parseTime=true' \
  go test ./... -v
```

预期：`TestAPIFlowMySQL` / `TestMySQLMigrations` 由 SKIP 转为 **PASS**，其余用例不变。全绿再上线。

> 本机需 Go ≥ 1.26（见 `backend/go.mod`）。想让 CI 常驻该门禁：在仓库 Settings → Variables 配 `TEST_MYSQL_DSN`，指向一个 GitHub runner 公网可达的 MySQL（runner 无内网访问）。

## 7. 挂上运维任务

```cron
# 每天凌晨 3 点全量备份（mysqldump → gzip → 保留 7 天）
0 3 * * * cd /path/to/repo && scripts/backup.sh >> /var/log/langhui-backup.log 2>&1

# 每分钟告警检查（healthz / 5xx / 备份新鲜度，webhook 换成实际机器人地址）
* * * * * cd /path/to/repo && ALERT_WEBHOOK="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxxx" \
  HEALTH_URL=http://127.0.0.1:8080/healthz \
  LOG_FILE="$(docker inspect -f '{{.LogPath}}' langhui-backend)" \
  BACKUP_DIR=/path/to/repo/backups \
  scripts/alert_check.sh >> /var/log/langhui-alert.log 2>&1
```

建议再补一条外部拨测（UptimeRobot 免费档即可）盯 `https://你的域名/healthz`——它覆盖「服务器外部不可达」这一本机探活看不到的场景，是投入产出比最高的一条。

## 8. 小程序侧切换（提审前）

1. `miniprogram/app.ts` 的 `globalData.baseUrl` 改为 `https://你的域名`。
2. 微信公众平台 → 开发管理 → 开发设置 → 服务器域名：
   - `request` 合法域名：`https://你的域名`
   - `uploadFile` 合法域名：`https://你的域名`（头像上传）
   - `downloadFile` 合法域名：同上（如用到）
   - **域名必须已备案；每月修改次数有限，一次填对。**
3. 开发管理 → 用户隐私保护指引：声明「相册选择（chooseMedia）」用于员工头像上传——微信审核必查项，漏了会被打回。
4. 视频号渲染（`<channel-video>`）需真机 + 小程序绑定视频号；测试号下该块留空属预期内。

## 9. 回滚

- **代码回滚**：`git checkout <上一个提交> && docker compose up -d --build`。数据库不动——迁移只增不删、向后兼容，旧版本代码能读新表结构跑。
- **切勿对生产执行 `docker compose down -v`**——这会清空 `mysql_data` / `uploads_data` 卷。回滚前先确认备份可恢复。

## 常见问题

| 现象 | 原因 / 处理 |
|---|---|
| backend 反复重启 | 多在等 mysql healthy；`docker compose logs backend` 看 DB_DSN 与 mysql 状态 |
| 登录后立刻 401 循环 | `COOKIE_SECURE` 与协议不匹配——http 访问却设了 `true`（或反之） |
| 上传报 413 | 请求体超限：`frontend/nginx.conf` 已设 50m；前置 Caddy 默认不限 |
| 小程序码接口 code=1 | 未配 `WX_APPID/WX_APPSECRET`，联调期属预期 |
| 视频号块空白 | 测试号 / 未绑视频号 / 模拟器——需真机 + 绑定视频号的正式 AppID |
