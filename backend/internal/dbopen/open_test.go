package dbopen

import (
	"path/filepath"
	"strings"
	"testing"

	"weichuan/internal/model"
)

// TestIsSQLiteDSN 钉死路由判据：判错比不判更糟——把生产 MySQL 读成空 SQLite 库，
// 或把 SQLite 路径丢给 MySQL 驱动，两者都是启动期就炸的事故。
func TestIsSQLiteDSN(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
		want bool
	}{
		{"默认 MySQL DSN", "root:root@tcp(127.0.0.1:3306)/weichuan?charset=utf8mb4&parseTime=true", false},
		{"compose 里的 MySQL DSN", "root:pw@tcp(mysql:3306)/weichuan?charset=utf8mb4&parseTime=true", false},
		{"sqlite:// 前缀", "sqlite:///data/weichuan.db", true},
		{"sqlite: 前缀", "sqlite:/data/weichuan.db", true},
		{"file: URI", "file:/data/weichuan.db", true},
		{"裸路径 .db", "/data/weichuan.db", true},
		{"裸路径带查询串", "/data/weichuan.db?_pragma=busy_timeout(1000)", true},
		{".sqlite3 结尾", "/data/weichuan.sqlite3", true},
		{"扩展名不在白名单的裸路径不算 SQLite", "/data/weichuan", false},
		{"空串", "", false},
		{"空白串", "   ", false},
	}
	for _, c := range cases {
		if got := IsSQLiteDSN(c.dsn); got != c.want {
			t.Errorf("%s：IsSQLiteDSN(%q) = %v，want %v", c.name, c.dsn, got, c.want)
		}
	}
}

// TestNormalizeSQLiteDSN 三项默认 pragma 必须补上，但用户显式指定的同名项优先。
// WAL + busy_timeout 是 SQLite 上生产可用的底线：缺了会出现「并发读被写挡住」
// 与「并发写直接报 database is locked」。
func TestNormalizeSQLiteDSN(t *testing.T) {
	got := NormalizeSQLiteDSN("sqlite:///data/weichuan.db")
	for _, want := range []string{
		"/data/weichuan.db",
		"_pragma=journal_mode(WAL)",
		"_pragma=busy_timeout(5000)",
		"_pragma=foreign_keys(1)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少 %q，实际：%s", want, got)
		}
	}
	if strings.HasPrefix(got, "sqlite:") {
		t.Errorf("前缀应被剥离，实际：%s", got)
	}

	// 用户指定的 busy_timeout 优先，其余两项仍要补
	got = NormalizeSQLiteDSN("sqlite:/data/weichuan.db?_pragma=busy_timeout(1000)")
	if !strings.Contains(got, "busy_timeout(1000)") {
		t.Errorf("应保留用户值，实际：%s", got)
	}
	if strings.Contains(got, "busy_timeout(5000)") {
		t.Errorf("不应覆盖用户值，实际：%s", got)
	}
	if !strings.Contains(got, "_pragma=journal_mode(WAL)") {
		t.Errorf("未指定的项仍应补默认值，实际：%s", got)
	}

	// 已有查询串时用 & 连接，不能出现两个 ?
	got = NormalizeSQLiteDSN("file:/data/weichuan.db?mode=rwc")
	if n := strings.Count(got, "?"); n != 1 {
		t.Errorf("查询串分隔符拼接错误（? 出现 %d 次）：%s", n, got)
	}
}

// TestOpenSQLite 端到端：DSN 走完 Open 后，库真的可用且 pragma 生效。
// 这是「生产上 SQLite」这条路径唯一能自动化验证的环节——驱动选错、
// pragma 没吃进去，都在这里暴露，而不是等到线上报 database is locked。
func TestOpenSQLite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "weichuan.db")
	db, driver, err := Open("sqlite://" + p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if driver != DriverSQLite {
		t.Fatalf("driver = %q，want %q", driver, DriverSQLite)
	}
	if err := db.AutoMigrate(model.All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	var jm string
	if err := db.Raw("PRAGMA journal_mode").Scan(&jm).Error; err != nil {
		t.Fatalf("pragma journal_mode: %v", err)
	}
	if jm != "wal" {
		t.Errorf("journal_mode = %q，want wal（未生效则并发读会被写锁挡住）", jm)
	}
	var bt int64
	if err := db.Raw("PRAGMA busy_timeout").Scan(&bt).Error; err != nil {
		t.Fatalf("pragma busy_timeout: %v", err)
	}
	if bt != 5000 {
		t.Errorf("busy_timeout = %d，want 5000", bt)
	}

	// 表必须真的建出来：AutoMigrate 静默跳过会让运行期才报 no such table
	var cnt int64
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='employees'").Scan(&cnt).Error; err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if cnt == 0 {
		t.Error("employees 表未建出——model.All() 漏了 Employee？")
	}
}

// TestOpenFileURI 保证 file: URI 写法同样可用（文档里给了这种写法）。
func TestOpenFileURI(t *testing.T) {
	p := filepath.Join(t.TempDir(), "weichuan.db")
	db, driver, err := Open("file:" + p)
	if err != nil {
		t.Fatalf("open file: URI: %v", err)
	}
	if driver != DriverSQLite {
		t.Fatalf("driver = %q，want sqlite", driver)
	}
	if err := db.Exec("CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
}
