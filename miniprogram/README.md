# 朗汇机电电子名片 · 微信小程序前端（原生 + TS）骨架

按《首页组件与接口字段映射》《跳转页组件与接口字段映射》两份文档落地的前端代码骨架。后端见 `backend/`。

## 目录
```
miniprogram/
├── app.json / app.ts / app.wxss     # 全局配置 / 入口 / 全局样式
├── project.config.json / tsconfig.json / sitemap.json
├── utils/
│   ├── request.ts      # wx.request 封装（自动注入 empToken、统一 code 处理）
│   ├── types.ts        # 与后端 JSON 对齐的 TS 类型
│   ├── richtext.ts     # <wx-video> 富文本拆分（视频号块 ↔ channel-video）
│   └── contact.ts      # 保存名片到通讯录（员工名片区 / 底部栏共用）
├── components/         # 7 个可复用组件（均开启 addGlobalClass）
│   ├── employee-card / company-video / achievement-grid
│   ├── featured-projects / service-grid / action-bar
│   └── rich-video       # 视频号渲染控件：封装 <channel-video finder-user-name="{{channelId}}">
├── pages/
│   ├── index/          # 名片首页（员工卡 + 公司简介视频 + 业绩 + 服务 + 底部栏）
│   ├── project/list/   # 项目列表（按分类）
│   ├── project/detail/ # 项目详情（富文本+视频号拆分+相册）
│   ├── service/detail/ # 服务详情（富文本+视频号拆分）
│   └── employee/       # 员工登录 login + 名片修改 edit（提交审核）
└── tests/              # 纯逻辑单元测试（richtext 拆分 / request 刷新重试）
```

## 与后端接口对应
| 页面 | 接口 |
|---|---|
| index | GET /api/card, /api/achievements, /api/services, /api/home-layout, POST /api/stat/track(visit) |
| project/list | GET /api/projects?categoryId= |
| project/detail | GET /api/projects/:id |
| service/detail | GET /api/services/:id |
| employee/login → edit | POST /api/employee/login, GET /api/employee/me, PUT /api/employee/me, POST /api/employee/upload(头像) |
| action-bar | wx.makePhoneCall + POST /api/stat/track(call/save) |
| poster | GET /api/card, GET /api/wechat/wxacode?staffId= |

## 员工头像上传
- 员工修改页 `pages/employee/edit` 的头像由「URL 文本输入」改为「选图上传」：用 `wx.chooseMedia` 选图，经 `wx.uploadFile` 调后端 `POST /api/employee/upload`（员工鉴权，复用同一 storage.Uploader），返回 URL 写回 `form.avatar`，再随 `PUT /api/employee/me` 提交审核。与 PC 端 `UploadField` 调用 `/api/admin/upload` 同一思路、同一存储层，仅鉴权角色不同。

## 小程序码 + 海报生成
- 后端 `GET /api/wechat/wxacode?staffId=` 调用微信 `getwxacodeunlimit` 生成小程序码（scene=`staffId=12`），按员工磁盘缓存到 `uploads/wxacode/<id>.jpg` 并返回可访问 URL（复用 `/uploads` 静态托管），避免反复消耗微信配额；access_token 在服务端内存缓存并在失效时自动刷新重试。
- 小程序端 `pages/poster/poster` 用 canvas 2d 把「头像 / 姓名 / 职位 / 公司 / 地址 / 电话 / 小程序码」合成为海报图，`wx.canvasToTempFilePath` 导出后 `wx.saveImageToPhotosAlbum` 保存到相册。
- 首页「生成分享海报」与「我的」页「生成海报」均跳转到 poster 页（携带 staffId）。
- 通过小程序码扫码进入时，`index.onLoad` 解析 `query.scene`（形如 `staffId=12`）定位到对应员工名片。
- 后端需配置 `WX_APPID` / `WX_APPSECRET`（环境变量）；未配置时接口返回明确错误。

## 运行
1. 微信开发者工具 → 导入项目 → 选择 `miniprogram/` 目录（appid 可用测试号）。
2. 修改 `app.ts` 的 `globalData.baseUrl` 指向后端地址；开发阶段 `project.config.json` 已设 `urlCheck:false` 允许 http。
3. 生产需在小程序后台配置 request 合法域名（https）。

## 本地联调（推荐：免 MySQL 的 devserver）
后端在 `backend/cmd/devserver` 提供了一个纯 SQLite（免 cgo）的开发态服务，复用与生产**完全一致**的路由装配，启动即自动 seed 演示数据，无需 MySQL / Docker 即可一键联调：
```bash
cd backend
go run ./cmd/devserver          # 监听 :8080（可用 PORT 环境变量覆盖）
```
内置演示数据：超管 `admin / admin123456`；演示员工 手机 `13800000001` / 密码 `demo1234`（staffId=1）；公司信息、7 个业绩分类、1 个精选项目（详情含 `<wx-video>`）、1 个业务服务、首页布局。
小程序侧联调步骤：
1. 微信开发者工具导入 `miniprogram/`（测试号 appid，`urlCheck:false` 已开）。
2. 把 `app.ts` 的 `globalData.baseUrl` 改成开发机地址：模拟器用 `http://127.0.0.1:8080`，真机用电脑局域网 IP（同网段、关防火墙）。
3. 首页直接打开会弹「输入员工编号」，输入 `1` 进入演示员工名片；或经 PC 后台生成的分享链接 / 小程序码进入（scene=`staffId=1`）。
4. 员工登录页用 `13800000001 / demo1234` 登录，可改头像（上传走 `/api/employee/upload`）、提交名片修改（待超管审核）。

> 接口约定：登录返回 `token` 在**根级**（`{code:0, token}`），`login.ts` 按 `r.token` 读取（与 PC 后台 `adminLogin` 一致）；其余资源在 `data` 下。视频号 `<channel-video>` 需真机 + 已绑定视频号的小程序 AppID 才能渲染，测试号下该块留空、`company-video` 自动回退封面/MP4；小程序码接口需配置 `WX_APPID/WX_APPSECRET` 才返回真实码，未配置时返回 `code:1`（联调预期内）。

## 技术要点
- **视频号渲染控件 `rich-video`**：抽出的可复用组件，内部渲染 `<channel-video wx:if="{{channelId}}" finder-user-name="{{channelId}}" class="rich-video" />`，外裹 `.rich-video-wrap` 便于统一样式。详情页（`pages/project/detail`、`pages/service/detail`）在富文本分段后，遇到视频号段改用 `<rich-video channelId="{{item.content}}" />` 渲染，不再内联 `<channel-video>`，避免页面各自重复条件判断。
- **富文本 + 视频号混排**：`utils/richtext.ts` 用鲁棒正则 `/<wx-video\s+[^>]*?channel-id="([^"]+)"[^>]*?\/?>(\s*<\/wx-video>)?/g` 拆分 `<wx-video channel-id="gh_xxx">`（兼容自闭合与成对、属性顺序无关，**成对写法时闭合标签一并消费**，不会漏进后续 html 片段），视频号段用 `rich-video` 组件、其余用 `rich-text`，保证图文/视频号位置正确（对应需求难点）。PC 富文本编辑器插入视频号时调用 `buildWxVideo(channelId)` 统一输出 `<wx-video channel-id="${channelId}"></wx-video>`，保证前后端片段规则一致。
- **视频号 ID 语义**：后端 `videoChannelId` 存的是视频号原始 ID（gh_xxx），已映射到 `<channel-video>` 的 `finder-user-name`；若要播放「指定视频」需换成该视频的 `feed-id`（生产对接时补全）。
- **员工 token**：登录后存 `wx.storage('empToken')`（access，15 分钟）+ `empRefreshToken`（refresh，30 天），`request.ts` 自动注入；`code=401` 先用 refresh 令牌静默换发并重试一次，失败再清 token 引导重新登录。改密 / 被禁用会令旧令牌立即失效。（小程序走 Bearer 头鉴权；PC 管理后台则改用 httpOnly Cookie，见前端 README。）
- **状态语义**：员工 `status=disabled` 时首页整页提示「已停用」；名片修改走「提交申请 → 超管审核通过后公开生效」。

## 单元测试（经前端 Vitest 运行）

`tests/` 收录小程序**纯逻辑**单测（不依赖微信开发者工具 / 真机）。小程序侧没有独立测试工具链，统一由前端工程的 Vitest 纳管（`frontend/vitest.config.ts` 通过 `server.fs.allow` 放行仓库根外路径 + `vi.resetModules()` + 动态 import），并对 `getApp` / `wx` 全局做桩：

- `tests/richtext.spec.ts`：富文本与视频号拆分——自闭合 `<wx-video ... />`、成对 `<wx-video ...></wx-video>`（闭合标签必须一并消费，不能漏进后续 html 片段）、属性顺序无关、无视频号时整体作为一段 html、含 `channel-id` 缺失时不误判等。
- `tests/request.spec.ts`：`wx.request` 封装——自动注入 `Authorization: Bearer`、`code=0` 直接 resolve、`code=401` 先用 refresh 令牌静默换发并重试一次（成功则放行）、刷新失败清 token 并 reject、`code!=0` 业务错误直接 reject 等。采用「同步挂起 + `respond(i, v)` 显式应答」模式驱动 Promise 链，不依赖真实计时器。

```bash
cd frontend && npm test        # 前端 + 小程序 spec 一并运行
```

> 视图层 / 页面渲染不在覆盖范围（需微信 CLI 或真机自动化），由联调步骤保证。

## 待补（相对完整产品）
- 真实视频号 feed-id 映射、图片懒加载/失败占位细化、下拉刷新等体验项。
