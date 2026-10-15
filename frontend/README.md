# 朗汇机电 PC 管理后台 · Vue3 前端

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
    │   └── storage.ts        # CDN 刷新 / 私有桶签名
    ├── store/auth.ts         # Pinia 登录态（持久化 localStorage）
    ├── router/index.ts       # 路由表 + 路由守卫
    ├── layout/AdminLayout.vue # 侧边菜单 + 顶栏（改密/退出）
    ├── components/
    │   ├── UploadField.vue   # 通用上传组件（直传预签名 + 回退普通上传）
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
- 登录返回 access + refresh 双令牌：`localStorage.token`（access，15 分钟）+ `localStorage.refreshToken`（refresh，30 天）；axios 请求拦截统一注入 `Authorization: Bearer <access>`。
- 后端鉴权失败一律返回 HTTP 200 + `{ code: 401 | 403, msg }`（见 `backend/internal/middleware/auth.go`），因此前端以 `res.code` 为准。
- **静默刷新**：遇到 `401` 时，响应拦截先用 `refreshToken` 调 `/api/refresh` 换发新 access 并自动重试原请求一次（并发 401 用单飞锁合并）；无 refreshToken 或刷新失败才清 token 跳登录。403 提示无权限。
- **即时吊销**：改密 / 管理员重置密码 / 禁用会使后端 `token_version` 自增，旧令牌立即失效，401 后刷新也会失败 → 重新登录。
- 路由守卫：未登录访问后台页 → 跳 `/login`；已登录访问 `/login` → 跳后台首页。

## 上传组件 UploadField

封装 `el-upload`，优先直传预签名（`GET /api/admin/upload/presign` → `PUT putUrl` 到对象存储），不支持时回退 `POST /api/admin/upload` 普通上传（local 驱动）。成功后通过 `v-model` 回传可访问 URL，用于封面 / LOGO / 头像 / 项目相册。

```vue
<UploadField v-model="form.cover" :multiple="false" accept="image/*" />
<UploadField v-model="form.album" :multiple="true" :limit="9" accept="image/*" />
```

`form.cover` 为 `string`，`form.album` 为 `string[]`。

## 企业内容管理说明

| 页面 | 接口 | 备注 |
|---|---|---|
| 公司信息 | `GET/PUT /api/admin/company-info` | id=1 单行，PUT 自动建首行 |
| 公司简介 | `GET/PUT /api/admin/company-profile` | 文字 + 视频号 ID + 降级 MP4 + 封面 |
| 业绩分类 | `GET/POST/PUT/DELETE /api/admin/categories` | 删除强校验：锁定或存在关联项目拦截 |
| 项目案例 | `GET/POST/PUT/DELETE /api/admin/projects` | 详情含 `<wx-video channel-id>` 视频号块；相册为 URL 数组 |
| 业务服务 | `GET/POST/PUT/DELETE /api/admin/services` | content 含视频号块 |
| 首页布局 | `GET/PUT /api/admin/home-layout` | order 数组控制板块顺序 |
| CDN 刷新 | `POST /api/admin/cdn/refresh` | 需 `CDN_ENABLED=1` |

> 富文本编辑器（项目案例.详情 / 业务服务.内容）已接入 wangEditor v5 可视化编辑，并注册「视频号块」自定义节点；详见下方「富文本编辑器（wangEditor）」。

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

前端配套 Vitest 单测，覆盖关键纯逻辑与契约约定（不依赖真实后端 / 浏览器）：

- `src/composables/useTable.spec.ts`：分页切片、关键字筛选（大小写不敏感）、关键字变化后回到首页等纯逻辑。
- `src/store/auth.spec.ts`：Pinia 登录态 `setAuth` / `logout` 与 `localStorage` 同步。
- `src/api/request.spec.ts`：统一响应拦截的**鉴权契约**——`code=0` 透传、`code=401` 清 token + 跳登录并重 reject、`code=403` 警告、`code!=0` 业务失败报错；以及请求拦截注入 `Bearer` 头。
- `src/editor/wxVideoModule.spec.ts`：`<wx-video channel-id>` 块「仅注册一次」及其序列化/反解析契约（与小程序端 `rich-video` 一致）。

配置见 `vitest.config.ts`（jsdom 环境 + `@` 别名 + vue 插件）。运行：

```bash
cd frontend
npm test            # 等价于 vitest run，跑全部 *.spec.ts
npm run test:watch  # 监听模式
```

> 说明：CI 中 `frontend` job 执行 `npm ci` + `npm run build` + `npm test`。
> 严格类型检查（`npm run type-check` / `vue-tsc --noEmit`）因 `@wangeditor/*` 在 `moduleResolution: bundler` 下的类型声明问题（editor-for-vue 的 `exports` 未暴露 types、自定义节点模块 `registerNodeType` 的类型签名）仍会报若干错误；
> 生产构建（`vite build`）走 esbuild 转译、不做类型检查，故不受影响。CI 已新增独立的 `frontend-typecheck` job（`continue-on-error`），作为「预警」持续暴露这些类型问题，待 wangEditor 类型声明修正后可升级为硬性门禁。
> 已知类型问题集中在 `src/components/RichEditor.vue` 与 `src/editor/wxVideoModule.ts` 两处（运行时行为正常）。

## 已知边界（与后端一致）

- 列表接口未分页（运营型小数据量）。
- 私有桶场景下，前端无需改造：上传/访问经签名通道，URL 由后端返回限时签名地址（见《三端联调与接口总表》§4.4/§4.5）。
- 初始密码仅返回一次（开通/新增/重置），前端弹窗展示，需超管线下通知员工。
