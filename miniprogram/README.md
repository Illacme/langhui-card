# 微传名片 · 微信小程序前端（原生 + TS）骨架

按《首页组件与接口字段映射》《跳转页组件与接口字段映射》两份文档落地的前端代码骨架。后端见 `backend/`。

## 目录
```
miniprogram/
├── app.json / app.ts / app.wxss     # 全局配置 / 入口 / 全局样式
├── project.config.json / tsconfig.json / sitemap.json
├── utils/
│   ├── request.ts      # wx.request 封装（自动注入 empToken、统一 code 处理）
│   ├── tenant.ts       # 多租户：企业独立版锁定（pinnedCompanyId）/ 平台模式扫码分发与粘滞
│   ├── types.ts        # 与后端 JSON 对齐的 TS 类型
│   ├── richtext.ts     # <wx-video> 富文本拆分（视频号块 ↔ channel-video）
│   ├── wechat.ts       # 微信身份：wx.login 换 openid / getUserProfile 授权昵称
│   └── contact.ts      # 保存名片到通讯录（员工名片区 / 底部栏共用）
├── components/         # 7 个可复用组件（均开启 addGlobalClass）
│   ├── employee-card / company-video / achievement-grid
│   ├── featured-projects / service-grid / action-bar
│   └── rich-video       # 视频号渲染控件：有 feedId 用 <channel-video feed-id>，否则回退 finder-user-name
├── pages/
│   ├── index/          # 名片首页（自定义顶栏含「我的」入口 + 员工卡 + 公司简介卡片 + 业绩 + 服务；底部 tabBar 已移除）
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

## 首页背景图（商业化美化包装，可选）
- PC 后台「首页布局」可设置首页背景图（支持从内置素材库「背景」分类直接选用 750×1334 竖幅素材），并可调节遮罩浓度 0~100。后端经 `GET /api/home-layout` 在**响应根级**下发 `bgImage` / `bgMask`（与 `order` 同级）。
- 首页 `pages/index` 收到非空 `bgImage` 时：铺一层全屏背景（`mode="aspectFill"`），其上叠 `bgMask/100` 透明度的黑色蒙层，白色内容卡片浮于其上形成层次；`bgImage` 为空即保持默认灰底布局，不渲染背景层。
- 背景层与蒙层用 `position: fixed` + 负 `z-index` 铺在最底层，内容按常规流绘制天然在其上方，不会互相遮挡；有背景时顶栏改为半透明白、板块卡片加圆角阴影（`.has-bg`）。

## 小程序码 + 海报生成
- 后端 `GET /api/wechat/wxacode?staffId=` 调用微信 `getwxacodeunlimit` 生成小程序码（scene=`staffId=12`），按员工磁盘缓存到 `uploads/wxacode/<id>.jpg` 并返回可访问 URL（复用 `/uploads` 静态托管），避免反复消耗微信配额；access_token 在服务端内存缓存并在失效时自动刷新重试。
- 小程序端 `pages/poster/poster` 用 canvas 2d 把「头像 / 姓名 / 职位 / 公司 / 地址 / 电话 / 小程序码」合成为海报图，`wx.canvasToTempFilePath` 导出后 `wx.saveImageToPhotosAlbum` 保存到相册。
- 首页「生成分享海报」与「我的」页「生成海报」均跳转到 poster 页（携带 staffId）。
- 通过小程序码扫码进入时，`index.onLoad` 解析 `query.scene`（形如 `staffId=12`）定位到对应员工名片。
- 后端需配置 `WX_APPID` / `WX_APPSECRET`（环境变量）；未配置时接口返回明确错误。

## 多租户与企业独立版（第二期）

小程序按企业分发，`utils/tenant.ts` 是唯一决策点，两种部署形态：

1. **企业独立小程序版本**（`tenantConfig.pinnedCompanyId` 设为企业 id，在 PC 后台「企业管理」查看编号）：
   整个小程序锁定为该企业的独立版本——内容 / 申请 / 分享恒带该 companyId，
   且**不读任何外部来源**（扫码 scene、分享参数、Storage 残留都不能改写），
   不存在切换多租户的行为，呈现为「该企业专属小程序」。
   > 锁定值可由后台下发，无需改代码重新提审：在 PC 后台「系统设置 → 小程序 →
   > `pinned_company_id`」填企业 id，小程序冷启动拉 `/api/public/settings` 覆盖
   > （见 `utils/config.ts`）。缺省 0。
2. **平台模式**（`pinned_company_id = 0`，默认）：
   访客扫企业专属小程序码（scene 携带 `companyId=N`，或分享链接带 `companyId`）
   首次进入时绑定该企业并写入 Storage 粘滞，之后本机所有入口（直接打开 / 分享）均落该企业；
   无任何来源时不传 companyId，由后端缺省落 1 号租户（朗汇）。

机制说明：
- `request.ts` 对内容类 GET（`/api/achievements|services|projects|home-layout|company`）与申请 POST（`/api/apply/open`）自动注入 companyId；`/api/card` 按员工归属企业取，`/api/employee/*`、登录刷新等与租户无关，一律不注入。
- `app.ts` `onLaunch` 与 `index.onLoad` 都会调 `bindCompanyFromScene`（幂等，覆盖冷启动与热启动两条路径）。
- 申请页（`pages/apply/open`）顶部展示「申请加入：XX公司」（`GET /api/company`），访客可确认申请目标；申请由该企业的管理员在 PC 后台审核。
- 员工登录态与名片内容始终按员工归属企业走（token / staffId 维度），不受本模块影响。
- 后端公开接口的 `companyId` 参数、企业停用拦截（`code=1`）与两级后台见 `backend/README.md`。

## 运行
1. 微信开发者工具 → 导入项目 → 选择 `miniprogram/` 目录（appid 可用测试号）。
2. 把 `utils/config.ts` 的 `DEFAULT_BASE_URL` 改成后端地址（这是「还没拿到后台配置时」的引导
   种子）；开发阶段 `project.config.json` 已设 `urlCheck:false` 允许 http。
   联调起来后即可由 PC 后台「系统设置 → 小程序 → `miniprogram_base_url`」下发正式基址，
   不必再改代码打包（目标域名仍需是已配置的 request 合法域名）。
3. 生产需在小程序后台配置 request 合法域名（https）。

## 本地联调（推荐：免 MySQL 的 devserver）
后端在 `backend/cmd/devserver` 提供了一个纯 SQLite（免 cgo）的开发态服务，复用与生产**完全一致**的路由装配，启动即自动 seed 演示数据，无需 MySQL / Docker 即可一键联调：
```bash
cd backend
go run ./cmd/devserver          # 监听 :8080（可用 PORT 环境变量覆盖）
```
内置演示数据：平台超管 `admin / admin123456`；演示员工 手机 `13832603521` / 密码 `langhui123`（高立新，业务经理，staffId=1）；公司信息、7 个业绩分类、多个项目案例、4 个业务服务、首页布局。
小程序侧联调步骤：
1. 微信开发者工具导入 `miniprogram/`（测试号 appid，`urlCheck:false` 已开）。
2. 把 `utils/config.ts` 的 `DEFAULT_BASE_URL` 改成开发机地址：模拟器用 `http://127.0.0.1:8080`，真机用电脑局域网 IP（同网段、关防火墙）。
3. 首页直接打开会弹「输入员工编号」，输入 `1` 进入演示员工名片；或经 PC 后台生成的分享链接 / 小程序码进入（scene=`staffId=1`）。
4. 员工登录页用 `13832603521 / langhui123` 登录，可改头像（上传走 `/api/employee/upload`）、提交名片修改（待平台/企业管理员审核）。
5. 多租户联调：平台超管在 PC 后台「企业管理」开通企业并建企业管理员 → 企业管理员登录管理自己企业的内容 → 小程序申请页填 `companyId` 或用 `?companyId=` 访问公开接口验证隔离（见「多租户与企业独立版」）。

> 接口约定：登录返回 `token` 在**根级**（`{code:0, token}`），`login.ts` 按 `r.token` 读取（与 PC 后台 `adminLogin` 一致）；其余资源在 `data` 下。视频号 `<channel-video>` 需真机 + 已绑定视频号的小程序 AppID 才能渲染，测试号下该块留空、`company-video` 自动回退封面/MP4；小程序码接口需配置 `WX_APPID/WX_APPSECRET` 才返回真实码，未配置时返回 `code:1`（联调预期内）。

## 技术要点
- **视频号渲染控件 `rich-video`**：抽出的可复用组件，内部按优先级三分支渲染 `<channel-video>`——`feed-id`（同主体，可自动播放）→ `feed-token`（跨主体，强制不能自动播放）→ `finder-user-name`（兼容存量富文本，只展示视频号主页；历史块里只存了视频号 ID，直接丢弃会让内容整块消失）。详情页（`pages/project/detail`、`pages/service/detail`）在富文本分段后，遇到视频号段改用 `<rich-video feedId="{{item.feedId}}" channelId="{{item.channelId}}" />` 渲染，不再内联 `<channel-video>`，避免页面各自重复条件判断。
- **富文本 + 视频号混排**：`utils/richtext.ts` 用正则 `/<wx-video\s+([^>]*?)\/?>(\s*<\/wx-video>)?/g` 整段捕获属性后再按名提取 `feed-id` / `feed-token` / `channel-id`（属性不再写死进正则，顺序无关，**成对写法时闭合标签一并消费**，不会漏进后续 html 片段），拆分 `<wx-video feed-id="…" feed-token="…" channel-id="sph_…">`；三个属性都为空的标签不拆分，留给 `rich-text` 原样渲染。视频号段用 `rich-video` 组件、其余用 `rich-text`，保证图文/视频号位置正确（对应需求难点）。PC 富文本编辑器插入视频号时调用 `buildWxVideo(feedId, channelId, feedToken)` 统一输出 `<wx-video feed-id="${feedId}" feed-token="${feedToken}" channel-id="${channelId}"></wx-video>`（缺失属性省略），保证前后端片段规则一致。
- **视频号 ID 语义**：视频号 ID 以 **`sph` 开头**（`gh_` 是公众号 ID，早期注释写错了，勿沿用）。`<channel-video>` 的 `finder-user-name` 只用于展示视频号主页，**播放指定视频必须用 `feed-id`**：公司简介档位 `channels` 的 `feedId` 来自视频号助手 → 动态管理 → 复制 feedId，且要求视频号与小程序**同主体**（跨主体需改用 `feed-token`，且强制不能自动播放）。
- **公司简介视频多来源**：`components/company-video` 按后端 `profile.videoSource` 选档——`channels` 渲染 `<channel-video>`（**同主体用 `feed-id`、跨主体用 `feed-token`**，`feed-id` 优先）；`upload` / `url` 渲染 `<video src=videoUrl>`；其余回封面占位。选档是**显式的**（不做隐式自动降级链），只有视频号主源 `bind:error` 且后台开了 `videoFallbackUrl` 时才回落到 `videoUrl`。跨主体必须走 `feed-token`：官方明确 `feed-id`「仅同主体时生效」，平台模式下客户视频号与我们小程序必然不同主体，代价是强制 `autoplay=false`。`tencent`（腾讯视频插件）是预留档：插件必须在 `app.json` 静态声明，统一声明会让未通过插件授权的客户小程序直接报「插件未授权」，故默认既不声明也不渲染。
- **员工 token**：登录后存 `wx.storage('empToken')`（access，15 分钟）+ `empRefreshToken`（refresh，30 天），`request.ts` 自动注入；`code=401` 先用 refresh 令牌静默换发并重试一次，失败再清 token 引导重新登录。改密 / 被禁用会令旧令牌立即失效。（小程序走 Bearer 头鉴权；PC 管理后台则改用 httpOnly Cookie，见前端 README。）
- **状态语义**：员工 `status=disabled` 时首页整页提示「已停用」；名片修改走「提交申请 → 超管审核通过后公开生效」。

## 组件开发规范与样式自查清单

所有组件均开启 `addGlobalClass: true`，组件目录内**没有独立 wxss**，样式统一写在 `app.wxss`。由此带来一条必须遵守的约束（踩过真实线上样式 bug）：

1. **组件内禁用裸通用类名**：`cell` / `row` / `label` / `val` / `name` / `title` / `info` / `head` / `avatar` / `logo` 等，一律加组件前缀（如 employee-card 用 `ec-`）。
   - 原因：全局 `app.wxss` 里任何同名独立类（例如给「我的」页写的 `.cell { border-bottom: 1rpx solid #f0f0f0 }`）都会命中组件内部元素，泄漏出凭空多出的边框 / 边距，且症状与业务代码无关、极难排查。
   - 组件自身根类名（`.emp-card`、`.cat-grid` 等）属于项目约定，写在 `app.wxss` 中不受此限。
2. **分隔线 / 装饰线用真实元素**（`<view class="xx-line" />`）而非 `::before` 伪元素，结构在 WXML 可见，便于比对与排查。
3. **新增组件或全局类后跑一次审计**（秒级，无需开发者工具）：

```bash
python3 miniprogram/tests/audit_class_collision.py
```

脚本会做两件事：① 找出「被组件与其他页面/组件同时使用」的全局独立类（真正的跨模块污染，应为 0）；② 列出组件内残留的通用字类名（未来风险提示）。

### 排查「预览里凭空多出的线 / 边距」的方法
截图肉眼判断不可靠，直接用像素扫描定位：取截图逐行找**连续横向线段**，输出其起止 x、宽度占比与主色——据此即可反查是哪个元素、什么颜色、多宽（例：曾据此定位到 `#f0f0f0`、宽 50% 的线来自全局 `.cell` 的 `border-bottom`，而非组件代码）。

## 单元测试（经前端 Vitest 运行）

`tests/` 收录小程序**纯逻辑**单测（不依赖微信开发者工具 / 真机）。小程序侧没有独立测试工具链，统一由前端工程的 Vitest 纳管（`frontend/vitest.config.ts` 通过 `server.fs.allow` 放行仓库根外路径 + `vi.resetModules()` + 动态 import），并对 `getApp` / `wx` 全局做桩：

- `tests/richtext.spec.ts`：富文本与视频号拆分——自闭合 `<wx-video ... />`、成对 `<wx-video ...></wx-video>`（闭合标签必须一并消费，不能漏进后续 html 片段）、属性顺序无关、无视频号时整体作为一段 html、含 `channel-id` 缺失时不误判等。
- `tests/request.spec.ts`：`wx.request` 封装——自动注入 `Authorization: Bearer`、`code=0` 直接 resolve、`code=401` 先用 refresh 令牌静默换发并重试一次（成功则放行）、刷新失败清 token 并 reject、`code!=0` 业务错误直接 reject 等。采用「同步挂起 + `respond(i, v)` 显式应答」模式驱动 Promise 链，不依赖真实计时器。
- `tests/tenant.spec.ts`：多租户解析——平台模式扫码 scene / 分享 query 绑定并粘滞、无来源缺省 0、非法值拦截、`resetCompanyBinding` 清绑定；企业独立版恒返回锁定值且拒绝一切外部绑定（scene / query / Storage 残留）。

```bash
cd frontend && npm test        # 前端 + 小程序 spec 一并运行
```

> 视图层 / 页面渲染不在覆盖范围（需微信 CLI 或真机自动化），由联调步骤保证。

## 待补（相对完整产品）
- 真实视频号 feed-id 映射、图片懒加载/失败占位细化、下拉刷新等体验项。
