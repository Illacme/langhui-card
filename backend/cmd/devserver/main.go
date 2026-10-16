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
// 真机 / 局域网联调时设 DEV_UPLOAD_BASE_URL=http://<本机IP>:8080/uploads，
// 否则返回给端的头像 URL 写死 127.0.0.1，手机取不到图。
package main

import (
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
	"langhui/internal/storage"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// devModels devserver 的建表清单，须与 model 包的全部模型保持一致。
// 新增 GORM 模型时必须同步追加——漏加不会编译报错，只会在运行期
// 查询时报「no such table」（曾漏加 RevokedSession 导致 devserver
// 下每次鉴权请求都被误判为「会话已吊销」而 401，见 main_test.go 的守卫）。
var devModels = []interface{}{
	&model.Employee{}, &model.CompanyInfo{}, &model.CompanyProfile{},
	&model.AchievementCategory{}, &model.ProjectCase{}, &model.BizService{},
	&model.HomeLayout{}, &model.StatEvent{}, &model.Admin{},
	&model.OpenApply{}, &model.CardModifyApply{},
	&model.RevokedSession{},
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

	cfg := config.Config{
		UploadDir:     mustDir(filepath.Join(os.TempDir(), "langhui-dev-uploads")),
		UploadBaseURL: uploadBaseURL(port),
		StorageDriver: "local",
		StoragePublic: true,
		JWTKey:        "dev-jwt-key",
		AdminUser:     "admin",
		AdminPass:     "admin123456",
	}
	log.Printf("upload base url: %s", cfg.UploadBaseURL)
	handler.SeedAdmin(db, cfg)

	seedDemoData(db)

	r := handler.New(db)
	// 与生产一致的可观测性中间件（请求 ID + 访问日志）
	r.Use(middleware.RequestID(), middleware.AccessLog())
	handler.RegisterAuth(r, db, cfg)
	handler.RegisterApply(r, db)
	handler.RegisterContent(r, db)

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
	handler.RegisterWechat(r, cfg, up)

	log.Printf("dev server listening on :%s (storage=local, db=sqlite)", port)
	log.Printf("演示账号：超管 admin/admin123456；员工手机 13800000001 / 密码 demo1234")
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

// uploadBaseURL 上传基址：默认 http://127.0.0.1:<port>/uploads（同机测试可直接用）；
// 真机 / 局域网联调时用 DEV_UPLOAD_BASE_URL 指向本机 IP 或域名——
// 否则后端返回给端的头像 / 相册 URL 写死 127.0.0.1，手机取不到图。
func uploadBaseURL(port string) string {
	if v := strings.TrimSpace(os.Getenv("DEV_UPLOAD_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:" + port + "/uploads"
}

// seedDemoData 注入一套可联调的演示数据（仅开发态；生产由 migrations SQL + PC 后台录入）
func seedDemoData(db *gorm.DB) {
	// 公司信息 / 简介
	db.Where("id = ?", 1).FirstOrCreate(&model.CompanyInfo{
		ID: 1, FullName: "朗汇机电设备有限公司", Address: "上海市浦东新区XX路88号",
		Tel: "021-88888888", Logo: "https://placehold.co/96x96?text=LH",
	})
	db.Where("id = ?", 1).FirstOrCreate(&model.CompanyProfile{
		ID: 1, IntroText: "朗汇机电专注机电设备销售、安装与维保，服务政务、医疗、商业等多场景。",
		VideoChannelID: "", Cover: "https://placehold.co/640x360?text=company",
	})

	// 经典业绩 7 分类（与迁移种子一致）
	cats := []model.AchievementCategory{
		{ID: 1, Name: "居住住宅", Sort: 1},
		{ID: 2, Name: "政务办公", Sort: 2},
		{ID: 3, Name: "医院、学校", Sort: 3},
		{ID: 4, Name: "商业、酒店", Sort: 4},
		{ID: 5, Name: "公共交通", Sort: 5},
		{ID: 6, Name: "生产制造、仓储物流", Sort: 6},
		{ID: 7, Name: "私人住宅", Sort: 7},
	}
	for _, c := range cats {
		db.Where("id = ?", c.ID).FirstOrCreate(&c)
	}

	// 1 个精选项目（含视频号块，验证 rich-video 渲染路径）
	db.Where("id = ?", 1).FirstOrCreate(&model.ProjectCase{
		ID: 1, CategoryID: 2, Name: "某政务中心机电总包", Summary: "空调+给排水+强弱电一体化交付",
		Detail: "<p>项目概况与交付亮点。</p><wx-video channel-id=\"gh_demo\"></wx-video><p>更多图文说明。</p>",
		Cover:  "https://placehold.co/640x360?text=project", Album: `["https://placehold.co/300x200?text=1"]`,
		IsHomeFeatured: true, Enabled: true, Sort: 1,
	})

	// 1 个业务服务（含视频号块）
	db.Where("id = ?", 1).FirstOrCreate(&model.BizService{
		ID: 1, Title: "机电维保", Content: "<p>7x24 应急维保服务。</p><wx-video channel-id=\"gh_demo\"></wx-video>", Sort: 1,
	})

	// 首页布局
	db.Where("id = ?", 1).FirstOrCreate(&model.HomeLayout{ID: 1, Order: `["achievement","service"]`})

	// 1 个演示员工（手机号登录用）
	var cnt int64
	db.Model(&model.Employee{}).Where("phone = ?", "13800000001").Count(&cnt)
	if cnt == 0 {
		hash, _ := auth.HashPassword("demo1234")
		emp := model.Employee{
			Name: "演示员工", Title: "客户经理", Phone: "13800000001", Email: "demo@langhui.com",
			Avatar: "https://placehold.co/96x96?text=EMP", PasswordHash: hash, Status: "enabled",
		}
		db.Create(&emp)
		var got model.Employee
		db.Where("phone = ?", "13800000001").First(&got)
		fmt.Printf("演示员工 staffId=%d\n", got.StaffID)
	}
}
