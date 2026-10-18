// devserver 是一个仅用于本地联调 / 演示的开发态后端：
//   - 用 SQLite（纯 Go，免 cgo）替代 MySQL，无需任何外部依赖即可 `go run` 起来；
//   - 复用与生产完全一致的路由装配（handler.New + 全部 Register*），保证联调即验证生产契约；
//   - 首次启动自动 seed 演示数据（超管 / 公司信息 / 7 分类 / 1 精选项目 / 1 服务 / 首页布局 / 1 演示员工）。
//
// 生产部署仍走 cmd/server（MySQL + docker-compose），本文件不参与生产构建路径。
//
// 运行：
//
//	cd backend && go run ./cmd/devserver
//
// 然后小程序 app.ts 的 baseUrl 指向 http://<本机IP>:8080 （开发者工具需勾选"不校验合法域名"）。
// 上传/采用接口返回的是相对路径 /uploads/<name>，各端自行拼绝对——
// 浏览器/管理后台经 vite 代理或反向代理解析，小程序经 assetUrl 拼 baseUrl，
// 因此不再需要为手机端单独配 DEV_UPLOAD_BASE_URL（旧方案会把 127.0.0.1 写死进库导致裂图）。
package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"langhui/internal/auth"
	"langhui/internal/config"
	"langhui/internal/handler"
	"langhui/internal/middleware"
	"langhui/internal/model"
	"langhui/internal/setting"
	"langhui/internal/storage"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// seedAssetFS 内嵌设计还原用的蓝色主题 PNG 占位素材（由 gen_seed_assets.py 生成）。
// 这些仅用于本地联调/演示占位；正式上线请在 PC 后台上传真实图片替换。
//
//go:embed seedassets/*.png
var seedAssetFS embed.FS

// devModels devserver 的建表清单，须与 model 包的全部模型保持一致。
// 新增 GORM 模型时必须同步追加——漏加不会编译报错，只会在运行期
// 查询时报「no such table」（曾漏加 RevokedSession 导致 devserver
// 下每次鉴权请求都被误判为「会话已吊销」而 401，见 main_test.go 的守卫）。
var devModels = []interface{}{
	&model.Employee{}, &model.CompanyInfo{}, &model.CompanyProfile{},
	&model.AchievementCategory{}, &model.ProjectCase{}, &model.BizService{},
	&model.HomeLayout{}, &model.StatEvent{}, &model.Admin{},
	&model.OpenApply{}, &model.CardModifyApply{},
	&model.RevokedSession{}, &model.Company{},
	&setting.Setting{}, // 系统设置表（后台「系统设置」界面维护运行参数）
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 临时 SQLite 文件库（比内存库更稳，避免共享缓存连接断开后数据丢失）
	tmpDB, err := os.CreateTemp("", "langhui-dev-*.db")
	if err != nil {
		log.Fatalf("create temp db: %v", err)
	}
	dbPath := tmpDB.Name()
	tmpDB.Close()
	log.Printf("dev db: %s", dbPath)

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(devModels...); err != nil {
		log.Fatalf("auto migrate: %v", err)
	}

	// 系统设置：seed 缺省值 + 载入内存缓存（后台「系统设置」界面维护运行参数）
	if err := setting.Init(db); err != nil {
		log.Fatalf("init settings: %v", err)
	}
	// 上传限制从系统设置读取（后台改上限即时生效、无需重启）
	storage.UploadLimitBytes = func() int64 {
		return int64(setting.GetInt("upload_max_mb", 10)) * 1024 * 1024
	}
	storage.AllowExtProvider = func() map[string]bool {
		sl := setting.GetStringSlice("upload_allow_ext", nil)
		if len(sl) == 0 {
			return storage.DefaultAllowExt
		}
		m := make(map[string]bool, len(sl))
		for _, e := range sl {
			m[e] = true
		}
		return m
	}

	// 复用与 cmd/server 一致的环境变量加载（ADMIN_USER / ADMIN_PASS 等可由 env 覆盖，
	// 避免把演示账号口令写死在代码里），仅本地联调相关字段（SQLite 上传目录 / 本地驱动 /
	// dev JWT 密钥）在此显式覆盖。
	cfg := config.Load()
	cfg.UploadDir = mustDir(filepath.Join(os.TempDir(), "langhui-dev-uploads"))
	cfg.UploadBaseURL = uploadBaseURL(port)
	cfg.StorageDriver = "local"
	cfg.StoragePublic = true
	cfg.JWTKey = "dev-jwt-key"
	log.Printf("dev server listening on :%s (storage=local, 上传返回相对路径 /uploads/<name>，由各端按源拼绝对)", port)
	handler.SeedAdmin(db, cfg)

	seedDemoData(db, cfg)

	r := handler.New(db)
	// 与生产一致的可观测性中间件（请求 ID + 访问日志）
	r.Use(middleware.RequestID(), middleware.AccessLog())
	handler.RegisterAuth(r, db, cfg)
	handler.RegisterApply(r, db)
	handler.RegisterContent(r, db)
	handler.RegisterCompanies(r, db) // 企业管理（平台超管）

	up := &storage.LocalUploader{
		Dir:      cfg.UploadDir,
		BaseURL:  cfg.UploadBaseURL,
		MaxBytes: 10 * 1024 * 1024,
		AllowExt: storage.DefaultAllowExt,
	}
	var refresher storage.CDNRefresher // 开发态不接 CDN
	r.Static("/uploads", cfg.UploadDir)
	handler.RegisterUpload(r, db, up, refresher)
	handler.RegisterStorage(r, db, up, refresher, cfg)
	handler.RegisterAssets(r, db, up) // 常用素材库（内嵌图标/封面/LOGO）
	handler.RegisterWechat(r, cfg, up)
	handler.RegisterSettings(r, db) // 系统设置：GET/PUT /api/admin/settings + GET /api/public/settings

	log.Printf("dev server listening on :%s (storage=local, db=sqlite)", port)
	log.Printf("演示账号：平台超管 %s/%s；员工手机 13832603521 / 密码 langhui123（高立新，业务经理）", cfg.AdminUser, cfg.AdminPass)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}

func mustDir(name string) string {
	if err := os.MkdirAll(name, 0755); err != nil {
		log.Fatalf("mkdir %s: %v", name, err)
	}
	abs, _ := filepath.Abs(name)
	return abs
}

// uploadBaseURL 仅作为配置字段 UploadBaseURL 的填充值（本地驱动已不依赖它生成 URL）。
// 保留以兼容生产云存储（MinIO/OSS 用 BaseURL 拼绝对 CDN）与反向代理场景；
// 本地联调因上传返回相对路径 /uploads/<name>，DEV_UPLOAD_BASE_URL 已不再影响手机端取图。
func uploadBaseURL(port string) string {
	if v := strings.TrimSpace(os.Getenv("DEV_UPLOAD_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:" + port + "/uploads"
}

// seedDemoData 注入一套可联调的演示数据（仅开发态；生产由 migrations SQL + PC 后台录入）。
// 数据内容与小程序 V1.3 设计稿一致：廊坊市朗汇机电设备有限公司 / 员工高立新 / 7 分类 / 2 精选项目 / 4 业务服务。
// 图片素材（LOGO/头像/分类图标/项目封面/服务图标等）由 writeSeedAssets 写入上传目录，
// 并以 cfg.UploadBaseURL 为基址回写，保证本机与真机（配 DEV_UPLOAD_BASE_URL）都能取到图。
func seedDemoData(db *gorm.DB, cfg config.Config) {
	assets := writeSeedAssets(cfg.UploadDir)

	// 1 号租户：朗汇机电（存量演示数据全部归属；多租户第一期的小程序缺省也落这里）
	db.Where("id = ?", 1).FirstOrCreate(&model.Company{ID: 1, Name: "廊坊市朗汇机电设备有限公司", ShortName: "朗汇机电", Status: "enabled"})

	// 公司信息 / 简介（company_id=1）
	db.Where("company_id = ?", 1).FirstOrCreate(&model.CompanyInfo{
		CompanyID: 1, FullName: "廊坊市朗汇机电设备有限公司", ShortName: "朗汇机电", Address: "廊坊市广阳区紫金城商住楼1-2-1911",
		Tel: "0316-823950", Logo: assets["logo.png"],
	})
	db.Where("company_id = ?", 1).FirstOrCreate(&model.CompanyProfile{
		CompanyID: 1, IntroText: "朗汇机电设备有限公司专注于电梯销售、安装、维修保养与改造服务，为住宅、政务中心、医疗、商业、公共交通、生产制造与仓储物流等多场景提供机电一体化解决方案。",
		VideoChannelID: "", BackupMp4URL: "", Cover: assets["cover-company.png"],
	})

	// 经典业绩 7 分类（与设计稿一致，客户已确认锁定）
	cats := []model.AchievementCategory{
		{ID: 1, CompanyID: 1, Name: "居住住宅", Icon: assets["cat-1.png"], Sort: 1, IsLocked: true},
		{ID: 2, CompanyID: 1, Name: "政务办公", Icon: assets["cat-2.png"], Sort: 2, IsLocked: true},
		{ID: 3, CompanyID: 1, Name: "医院、学校", Icon: assets["cat-3.png"], Sort: 3, IsLocked: true},
		{ID: 4, CompanyID: 1, Name: "商业、酒店", Icon: assets["cat-4.png"], Sort: 4, IsLocked: true},
		{ID: 5, CompanyID: 1, Name: "公共交通", Icon: assets["cat-5.png"], Sort: 5, IsLocked: true},
		{ID: 6, CompanyID: 1, Name: "生产制造、仓储物流", Icon: assets["cat-6.png"], Sort: 6, IsLocked: true},
		{ID: 7, CompanyID: 1, Name: "私人住宅", Icon: assets["cat-7.png"], Sort: 7, IsLocked: true},
	}
	for _, c := range cats {
		db.Where("id = ?", c.ID).FirstOrCreate(&c)
	}

	// 项目案例：2 个首页精选（设计稿）+ 其余分类各 1 个（让客户验收时点开任意分类都有内容）
	projects := []model.ProjectCase{
		{ID: 101, CompanyID: 1, CategoryID: 1, Name: "大兴临空服务中心", Summary: "临空经济区地标机电总包",
			Detail: "<p>大兴临空服务中心项目涵盖多栋客货梯与自动扶梯的供货、安装与维保，满足高客流交通枢纽的可靠运行需求。</p>",
			Cover: assets["cover-daxing.png"], Album: `["` + assets["album-1.png"] + `","` + assets["album-2.png"] + `"]`,
			IsHomeFeatured: true, Enabled: true, Sort: 1},
		{ID: 102, CompanyID: 1, CategoryID: 3, Name: "某医院电梯项目", Summary: "医疗楼宇专用电梯解决方案",
			Detail: "<p>某医院电梯项目提供医疗专用电梯的定制、安装与运维，兼顾洁净、安静与应急响应。</p>",
			Cover: assets["cover-hospital.png"], Album: `["` + assets["album-1.png"] + `"]`,
			IsHomeFeatured: true, Enabled: true, Sort: 2},
		{ID: 103, CompanyID: 1, CategoryID: 1, Name: "某高层住宅电梯项目", Summary: "住宅客梯批量交付",
			Detail: "<p>为高层住宅提供客梯的批量供货与安装，统一调度的群控方案提升高峰运力。</p>",
			Cover: assets["cover-p1b.png"], Album: `["` + assets["album-1.png"] + `"]`,
			IsHomeFeatured: false, Enabled: true, Sort: 2},
		{ID: 104, CompanyID: 1, CategoryID: 2, Name: "市民服务中心电梯工程", Summary: "政务大厅无机房客梯",
			Detail: "<p>市民服务中心采用无机房客梯，节省机房空间，满足政务服务大厅的无障碍通行需求。</p>",
			Cover: assets["cover-p2.png"], Album: `["` + assets["album-2.png"] + `"]`,
			IsHomeFeatured: false, Enabled: true, Sort: 1},
		{ID: 105, CompanyID: 1, CategoryID: 3, Name: "某学校电梯项目", Summary: "校园电梯安全改造",
			Detail: "<p>针对校园既有电梯的安全改造，升级制动与监控，保障师生乘梯安全。</p>",
			Cover: assets["cover-p3b.png"], Album: `["` + assets["album-1.png"] + `"]`,
			IsHomeFeatured: false, Enabled: true, Sort: 3},
		{ID: 106, CompanyID: 1, CategoryID: 4, Name: "商业综合体扶梯项目", Summary: "商场自动扶梯群控",
			Detail: "<p>商业综合体部署自动扶梯群控系统，结合客流分析优化运行节奏，节能高效。</p>",
			Cover: assets["cover-p4.png"], Album: `["` + assets["album-2.png"] + `"]`,
			IsHomeFeatured: false, Enabled: true, Sort: 1},
		{ID: 107, CompanyID: 1, CategoryID: 5, Name: "地铁站点电梯项目", Summary: "轨道交通垂直交通",
			Detail: "<p>地铁站点垂直交通方案，重载电梯与扶梯满足大客流高频率运行。</p>",
			Cover: assets["cover-p5.png"], Album: `["` + assets["album-1.png"] + `"]`,
			IsHomeFeatured: false, Enabled: true, Sort: 1},
		{ID: 108, CompanyID: 1, CategoryID: 6, Name: "智能仓储货梯项目", Summary: "物流仓储货梯定制",
			Detail: "<p>为智能仓储定制货梯，对接 AGV 与 WMS，实现物料垂直转运自动化。</p>",
			Cover: assets["cover-p6.png"], Album: `["` + assets["album-2.png"] + `"]`,
			IsHomeFeatured: false, Enabled: true, Sort: 1},
		{ID: 109, CompanyID: 1, CategoryID: 7, Name: "别墅家用电梯项目", Summary: "私人住宅别墅电梯",
			Detail: "<p>私人别墅家用电梯，定制轿厢与静音驱动，兼顾美观与乘坐舒适。</p>",
			Cover: assets["cover-p7.png"], Album: `["` + assets["album-1.png"] + `"]`,
			IsHomeFeatured: false, Enabled: true, Sort: 1},
	}
	for _, p := range projects {
		db.Where("id = ?", p.ID).FirstOrCreate(&p)
	}

	// 4 个业务服务（设计稿示例：电梯销售/安装/维修保养/改造）
	svcs := []model.BizService{
		{ID: 1, CompanyID: 1, Title: "电梯销售", Icon: assets["svc-1.png"], Content: "<p>全系列电梯设备销售与选型咨询，匹配不同楼宇场景需求。</p>", Sort: 1},
		{ID: 2, CompanyID: 1, Title: "电梯安装", Icon: assets["svc-2.png"], Content: "<p>专业安装团队，标准化施工与验收交付，保障工期与质量。</p>", Sort: 2},
		{ID: 3, CompanyID: 1, Title: "电梯维修保养", Icon: assets["svc-3.png"], Content: "<p>7x24 应急响应与按需定制的保养计划，延长设备寿命。</p>", Sort: 3},
		{ID: 4, CompanyID: 1, Title: "电梯改造", Icon: assets["svc-4.png"], Content: "<p>旧梯升级改造，提升安全性、乘运体验与能效表现。</p>", Sort: 4},
	}
	for _, s := range svcs {
		db.Where("id = ?", s.ID).FirstOrCreate(&s)
	}

	// 首页布局（company_id=1）
	db.Where("company_id = ?", 1).FirstOrCreate(&model.HomeLayout{CompanyID: 1, Order: `["achievement","service"]`})

	// 1 个演示员工（设计稿员工：高立新 / 业务经理；手机号登录用）
	var cnt int64
	db.Model(&model.Employee{}).Where("phone = ?", "13832603521").Count(&cnt)
	if cnt == 0 {
		hash, _ := auth.HashPassword("langhui123")
		emp := model.Employee{
			CompanyID: 1,
			Name: "高立新", Title: "业务经理", Phone: "13832603521", Landline: "0316-823950",
			Email:  "138266603521@163.com",
			Avatar: assets["avatar-gaolixin.png"], PasswordHash: hash, Status: "enabled",
		}
		db.Create(&emp)
		var got model.Employee
		db.Where("phone = ?", "13832603521").First(&got)
		fmt.Printf("演示员工 staffId=%d\n", got.StaffID)
	}
}

// writeSeedAssets 把内嵌的 PNG 占位素材写入上传目录，并返回「文件名→相对 URL」映射。
// 每次启动都重写（上传目录为临时目录，重启即清空），保证素材始终可用。
// 返回相对 /uploads/<name>（与各端按源拼绝对的统一约定一致）。
func writeSeedAssets(uploadDir string) map[string]string {
	urls := map[string]string{}
	entries, err := seedAssetFS.ReadDir("seedassets")
	if err != nil {
		log.Printf("读取内嵌素材失败: %v", err)
		return urls
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := seedAssetFS.ReadFile("seedassets/" + e.Name())
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(uploadDir, e.Name()), data, 0644); err != nil {
			log.Printf("写入素材 %s 失败: %v", e.Name(), err)
			continue
		}
		urls[e.Name()] = "/uploads/" + e.Name()
	}
	return urls
}
