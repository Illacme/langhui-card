package main

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUploadBaseURL(t *testing.T) {
	t.Setenv("DEV_UPLOAD_BASE_URL", "")
	if got := uploadBaseURL("8080"); got != "http://127.0.0.1:8080/uploads" {
		t.Fatalf("未设置时应回退本机默认值，got %s", got)
	}

	// 覆盖为局域网地址（末尾斜杠应被裁剪，避免 URL 出现双斜杠）
	t.Setenv("DEV_UPLOAD_BASE_URL", "http://192.168.1.10:8080/uploads/")
	if got := uploadBaseURL("8080"); got != "http://192.168.1.10:8080/uploads" {
		t.Fatalf("设置后应使用覆盖值并裁剪末尾斜杠，got %s", got)
	}

	// 仅空白字符等同于未设置
	t.Setenv("DEV_UPLOAD_BASE_URL", "   ")
	if got := uploadBaseURL("9090"); got != "http://127.0.0.1:9090/uploads" {
		t.Fatalf("空白值应回退默认值，got %s", got)
	}
}

// TestDevModelsCoverAllTables 守卫：devModels 必须覆盖 model 包的全部模型。
// 历史事故：A3 新增 RevokedSession（会话黑名单）时漏加进 devserver 的建表
// 清单——漏加不报编译错，只在运行期查 revoked_sessions 时报 no such table，
// 又被中间件「查询出错按已吊销处理」放大成「登录成功后所有请求 401」。
// 新增 GORM 模型时，请同步更新 devModels 与本列表。
func TestDevModelsCoverAllTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "guard.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(devModels...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	want := []string{
		"employees", "company_infos", "company_profiles", "achievement_categories",
		"project_cases", "biz_services", "home_layouts", "stat_events", "admins",
		"open_applies", "card_modify_applies", "revoked_sessions",
	}
	for _, name := range want {
		var cnt int64
		if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&cnt).Error; err != nil {
			t.Fatalf("query sqlite_master: %v", err)
		}
		if cnt == 0 {
			t.Errorf("表 %s 不存在——devModels 漏加了对应模型？", name)
		}
	}
}
