# 智能名片 PC 管理后台 · Vue3 前端

Vue3 + Element Plus 的 PC 管理端（超管使用）。对接《三端联调与接口总表》中全部 `admin` 接口：登录 / 开通审核 / 名片修改审核 / 员工管理 / 企业内容 CRUD / 上传 / CDN 刷新 / 超管改密。

> 小程序端（员工侧）在 `../miniprogram`，后端在 `../backend`。

## 技术栈

- Vue 3.5（`<script setup>`）+ TypeScript
- Vite 5（开发代理 `/api` → 后端 8080）
- Element Plus 2.8 + 图标
- Pinia（登录态）+ Vue Router 4（路由守卫）

## 目录结构

```
frontend/
├── index.html
├── package.json
├── vite.config.ts            # /api 代理到后端 8080
├── tsconfig.json / tsconfig.node.json
├── env.d.ts
└── src/
    ├── main.ts               # 入口：挂载 Pinia / Router / Element Plus
    ├── App.vue
    ├── api/
    │   ├── request.ts        # axios 实例 + 统一响应拦截（code=401 跳登录 / 403 提示 / 非0 报错）
    │   ├── auth.ts           # 登录 / 改密
    │   ├── apply.ts          # 开通审核 / 名片修改审核
    │   ├── employees.ts      # 员工管理
    │   ├── content.ts        # 公司信息/简介/分类/项目/服务/首页布局
    │   ├── storage.ts        # CDN 刷新 / 私有桶签名
    │   └── assets.ts         # 开源素材库：清单 / 采用 / 预览图 URL
    ├── store/auth.ts         # Pinia 登录态（持久化 localStorage）
    ├── router/index.ts       # 路由表 + 路由守卫
    ├── layout/AdminLayout.vue # 侧边菜单 + 顶栏（改密/退出）
    ├── components/
    │   ├── UploadField.vue   # 通用上传组件（直传预签名 + 回退普通上传 + 素材库入口）
    │   ├── AssetPicker.vue   # 开源素材库选择器（分类 tab + 风格行 + 搜索 + 分页 + 单选/多选采用）
    │   └── ChangePasswordDialog.vue
    └── views/
        ├── Login.vue
        ├── ApplyOpen.vue     # 开通审核（通过弹初始密码 / 驳回）
        ├── ApplyModify.vue   # 名片修改审核（snapshot 旧值→新值对比）
        ├── Employees.vue     # 员工管理（新增/启用停用/重置密码）
        └── content/
            ├── CompanyInfo.vue
            ├── CompanyProfile.vue
            ├── Categories.vue
            ├── Projects.vue   # 富文本+视频号块 / 封面+相册 / 精选+上架
            ├── Services.vue
            ├── HomeLayout.vue
            └── CdnRefresh.vue
```

## 本地运行

```bash
cd frontend
npm install
npm run dev        # 启动开发服务器（默认 http://localhost:5173）
```

- 开发期 `vite.config.ts` 已将 `/api` 代理到 `http://127.0.0.1:8080`；
  如需指向其他后端地址，用 `VITE_BACKEND_URL=http://x.x.x.x:8080 npm run dev`。
- 生产构建：`npm run build`（产物在 `dist/`，交由 Nginx 托管，API 走反向代理或同源）。

## 登录与鉴权

- 超管账号来自后端 `ADMIN_USER/ADMIN_PASS`（默认 `admin/admin123456`，**上线必改**），首启动由后端 seed。
- **httpOnly Cookie 鉴权**：登录成功后服务端写入 `access_token` / `refresh_token` 两个 `HttpOnly; SameSite=Lax` Cookie，axios 以 `withCredentials` 随请求自动携带，**JS 全程不接触令牌**（防 XSS 窃取）。前端仅在 localStorage 存非敏感的 `admin_user` / `admin_loggedin`，用于路由守卫与界面展示。
- 后端鉴权失败一律返回 HTTP 200 + `{ code: 401 | 403, msg }`（见 `backend/internal/middleware/auth.go`），因此前端以 `res.code` 为准。
- **静默刷新**：遇到 `401` 时，响应拦截先调 `/api/refresh`（凭 refresh Cookie，服务端轮换下发新 Cookie）并自动重试原请求一次（并发 401 用单飞锁合并，`__skipRefresh` 防递归）；刷新失败才清登录态跳登录。403 提示无权限。
- **即时吊销**：改密 / 管理员重置密码 / 禁用会使后端 `token_version` 自增，旧 Cookie 立即失效；超管改密后前端自动登出并回登录页。
- **登出**：`AdminLayout` 调 `POST /api/logout` 清 Cookie 后清本地登录态；路由守卫据 `loggedIn` 标记跳转（令牌在 Cookie 中，JS 以非敏感标记判断）。

## 上传组件 UploadField

封装 `el-upload`，优先直传预签名（`GET /api/admin/upload/presign` → `PUT putUrl` 到对象存储），不支持时回退 `POST /api/admin/upload` 普通上传（local 驱动）。成功后通过 `v-model` 回传可访问 URL，用于封面 / LOGO / 头像 / 项目相册。

```vue
<UploadField v-model="form.cover" :multiple="false" accept="image/*" />
<UploadField v-model="form.album" :multiple="true" :limit="9" accept="image/*" />
```

`form.cover` 为 `string`，`form.album` 为 `string[]`。

## 开源素材库（减少找图/处理图成本）

所有需要图片的表单（业绩分类图标、业务服务图标、项目封面/相册、公司简介封面、公司信息 LOGO、员工头像、**小程序首页背景**）在 `UploadField` 上方都提供「从素材库选择」入口，点击弹出 `AssetPicker.vue` 选择器：**分类 tab**（全部 + 图标/封面/LOGO/头像/背景）+ **风格行**（简约清晰/温暖自然/轻奢尊贵/柔美清新/光影流动）+ 关键字搜索 + 卡片网格 + **底部分页器**，**单选即采用并回填，多选累计后批量添加**（超出 `limit` 自动截断并提示）。

- **两个筛选维度正交**：分类 tab = "用在哪"，风格行 = "什么气质"。风格行只渲染当前条件下 `count>0` 的档位（服务端回传 `styleCounts`），只有一档可选时整行隐藏——例如图标分类下视觉语言统一，不会摆一排点不出东西的筛选项。已选调性在新条件下无素材时（如切分类、搜索收窄）自动回落「全部」，不会停在空结果页。
- 素材清单走**服务端分页**：`GET /api/admin/assets/library?page=&size=&type=&style=&keyword=`，返回 `{items,total,page,size,pages,styleCounts}`；翻页 / 切分类 / 切风格 / 搜索（300ms 防抖）都会重新请求并重置到第 1 页，已选中的素材跨页保留。
- 当前内置 **347 项**（187 图标 / 48 封面 / 40 LOGO / 24 头像 / 48 背景，含深浅两色系），预览图走公开只读端点 `GET /api/assets/file/*path`（immutable 长缓存）。
- 采用走 `POST /api/admin/assets/adopt`：后端把内嵌素材复制进当前上传存储，返回与手动上传**完全同构**的 URL，后续展示/CDN 链路零差异。
- 按位置传入 `library-type`（`icon` / `cover` / `logo` / `avatar` / `background`）控制默认定位的分类；素材资产与许可证见后端 `internal/assets/library/`（Lucide 图标 ISC 许可 + 程序化生成图形）。

```vue
<!-- 分类/服务：图标库；项目：封面库；公司简介：封面库；公司信息：LOGO 库；员工：头像库；首页布局：背景库 -->
<UploadField v-model="form.icon" library-type="icon" :multiple="false" />
<UploadField v-model="form.cover" library-type="cover" :multiple="false" />
<UploadField v-model="layout.bgImage" library-type="background" :multiple="false" />
```

## 企业内容管理说明

| 页面 | 接口 | 备注 |
|---|---|---|
| 公司信息 | `GET/PUT /api/admin/company-info` | id=1 单行，PUT 自动建首行 |
| 公司简介 | `GET/PUT /api/admin/company-profile` | 文字 + 视频号 ID + 降级 MP4 + 封面 |
| 业绩分类 | `GET/POST/PUT/DELETE /api/admin/categories` | 删除强校验：锁定或存在关联项目拦截 |
| 项目案例 | `GET/POST/PUT/DELETE /api/admin/projects` | 详情含 `<wx-video channel-id>` 视频号块；相册为 URL 数组 |
| 业务服务 | `GET/POST/PUT/DELETE /api/admin/services` | content 含视频号块 |
| 首页布局 | `GET/PUT /api/admin/home-layout` | 板块顺序 + **首页背景图**（可从素材库「背景」分类直接选用）+ 遮罩浓度滑块与效果预览 |

> 富文本编辑器（项目案例.详情 / 业务服务.内容）已接入 wangEditor v5 可视化编辑，并注册「视频号块」自定义节点；详见下方「富文本编辑器（wangEditor）」。

## 平台管理说明（仅平台超级管理员）

| 页面 | 接口 | 备注 |
|---|---|---|
| 企业管理 | `GET/POST/PUT /api/admin/companies` 等 | 多租户：开通/停用企业、企业管理员管理、企业简称；企业管理员访问会被后端 403 拒绝 |
| CDN 刷新 | `POST /api/admin/cdn/refresh` | 平台级基础设施操作，需 `CDN_ENABLED=1`；上传后已自动 purge，此处为覆盖写场景的兜底 |

## 列表分页 / 搜索

六个列表页（开通审核 / 名片修改审核 / 员工管理 / 业绩分类 / 项目案例 / 业务服务）均接入了统一的 `src/composables/useTable.ts` 组合式函数，提供：

- **关键字搜索**：顶部搜索框，按各页核心字段本地筛选（如员工按姓名/手机、项目按名称 + 分类下拉）。
- **分页**：`el-pagination`，默认每页 10 条，可切 10/20/50；切换关键字/分类时自动回到第一页。

> 采用**前端分页 + 搜索**（对已在内存中的列表做筛选/切片），与《三端联调与接口总表》§2.3「当前版本列表接口不分页（数据量小，运营型后台可接受）」的定位一致，无需改动后端接口契约。若日后数据量显著增长，可平滑改为服务端分页（后端加 `?page&size&keyword` 并返回 `{list,total}`），前端仅替换 `useTable` 的数据源即可。

## 富文本编辑器（wangEditor）

`项目案例.详情` 与 `业务服务.内容` 使用可视化富文本编辑器 `src/components/RichEditor.vue`（基于 wangEditor v5 + `@wangeditor/editor-for-vue`），替换了原先的纯 textarea。

- 工具栏开箱即用（加粗/标题/列表/图片/链接等），并通过 `excludeKeys` 排除了自带视频上传。
- **视频号块节点**：通过 `src/editor/wxVideoModule.ts` 注册了自定义元素 `wxVideo`，编辑区以绿色占位块显示（视频号图标 + channel-id），保存时序列化为 `<wx-video channel-id="gh_xxx"></wx-video>`，与小程序端 `rich-video` 组件约定一致；回显时 `<wx-video>` 标签会被解析还原为编辑区节点。
- 编辑器上方有一个「插入视频号块」按钮，弹窗输入 channel-id 后插入（void 原子块，内部不可编辑）。
- `v-model` 双向绑定 HTML 字符串，保存时直接写入 `detail` / `content` 字段（后端按字符串原样存储）。

> 富文本中的视频号块约定、小程序端渲染（`<wx-video>` → `<channel-video>`）详见《首页/跳转页组件与接口字段映射》。

## 单元测试（Vitest）

前端配套 Vitest 单测，覆盖关键纯逻辑与契约约定（不依赖真实后端 / 浏览器）。**同时纳管小程序端的纯逻辑测试**（小程序本身无独立测试工具链，`vitest.config.ts` 通过 `server.fs.allow` 放行仓库根外的 spec 路径）。

前端（`src/`）：
- `src/composables/useTable.spec.ts`：分页切片、关键字筛选（大小写不敏感）、关键字变化后回到首页等纯逻辑。
- `src/store/auth.spec.ts`：Pinia 登录态 `setAuth` / `logout` 与 `localStorage` 同步。
- `src/api/request.spec.ts`：统一响应拦截的**鉴权契约**——`code=0` 透传、`code=401` 清 token + 跳登录并重 reject、`code=403` 警告、`code!=0` 业务失败报错；以及请求拦截注入 `Bearer` 头。
- `src/editor/wxVideoModule.spec.ts`：`<wx-video channel-id>` 块「仅注册一次」及其序列化/反解析契约（与小程序端 `rich-video` 一致）。
- `src/views/Login.spec.ts`：登录页视图层用例——表单校验提示、登录成功后写 Pinia + 跳转、`code=401` 时展示错误并不写登录态（jsdom + Element Plus 组件挂载）。
- `src/views/Employees.spec.ts`：员工管理页视图层用例——列表渲染、启用/停用/重置密码的弹窗确认与接口调用、`el-table` 行上下文（provide/inject 复刻列槽 `{ row }`）等。
- `src/components/AssetPicker.spec.ts`：素材选择器用例——清单渲染、`defaultCategory` 定位、单选即采用、多选批量、超限拦截、**翻页带新页码重新请求**、**搜索防抖后走服务端过滤**、**风格行只渲染 count>0 的档位**、**切换风格带 style 请求**、**单一调性分类隐藏风格行**、**已选调性无素材时自动回落全部**（11 例）。
- `src/components/UploadField.spec.ts`：上传组件用例——直传/回退分支、素材库入口与 `library-type` 透传、单选替换、多选追加、超限截断。

小程序纯逻辑（`miniprogram/tests/`，经本目录 Vitest 运行，桩化 `getApp`/`wx` 全局）：
- `miniprogram/tests/richtext.spec.ts`：`<wx-video>` 富文本拆分（自闭合 / 成对 / 属性顺序无关）、无视频号时整体走 `rich-text` 等。
- `miniprogram/tests/request.spec.ts`：`wx.request` 封装的 token 注入、`code=401` 静默 refresh 重试一次、失败清 token 等。

配置见 `vitest.config.ts`（jsdom 环境 + `@` 别名 + vue 插件 + 小程序 spec 纳管）。运行：

```bash
cd frontend
npm test            # 等价于 vitest run，跑前端 + 小程序全部 spec
npm run test:watch  # 监听模式
```

> 说明：CI 中 `frontend` job 执行 `npm ci` + `npm run build` + `npm test`（前端与小程序单测一并覆盖）。
> 严格类型检查（`npm run type-check` / `vue-tsc --noEmit`）历史上有 4 个 `@wangeditor/*` 相关错误（editor-for-vue 的 `exports` 未暴露 types、自定义节点 `registerNodeType` 的类型签名），已通过 tsconfig `paths` 指向其真实 d.ts + 精准断言收敛修复，**当前 0 错误**；CI 的 `frontend-typecheck` job 已从「预警（continue-on-error）」升级为**阻塞门禁**，后续不得引入新的类型错误。

## 已知边界（与后端一致）

- 列表接口未分页（运营型小数据量）。
- 私有桶场景下，前端无需改造：上传/访问经签名通道，URL 由后端返回限时签名地址（见《三端联调与接口总表》§4.4/§4.5）。
- 初始密码仅返回一次（开通/新增/重置），前端弹窗展示，需超管线下通知员工。
