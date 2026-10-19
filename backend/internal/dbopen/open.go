// Package dbopen 按 DSN 形态选择数据库驱动，让同一个生产二进制同时支持 MySQL 与 SQLite。
//
// 为什么需要这个包：业务层是方言无关的（`TestAPIFlow` 在 SQLite 上跑完整流程，
// `TestAPIFlowMySQL` 用真实 MySQL 跑完全相同断言做 parity），唯一写死 MySQL 的地方
// 是 `cmd/server` 的 `gorm.Open(mysql.Open(...))`。把驱动选择收敛到这里，
// 部署时就能只靠一个 `DB_DSN` 环境变量决定用哪个库，不必换二进制。
//
// 调用方还需要按返回的驱动名决定建表方式：`migrations/*.sql` 是 MySQL 方言
// （ENGINE=InnoDB / AUTO_INCREMENT / 反引号），SQLite 侧只能走 GORM AutoMigrate。
package dbopen

import (
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 驱动名常量，供调用方分支（建表方式、日志、运维提示）。
const (
	DriverMySQL  = "mysql"
	DriverSQLite = "sqlite"
)

// Open 按 DSN 形态打开数据库连接，第二个返回值是实际使用的驱动名。
//
//	err = db 为 nil 时的错误（调用方直接 log.Fatal 即可）。
func Open(dsn string) (*gorm.DB, string, error) {
	if IsSQLiteDSN(dsn) {
		db, err := gorm.Open(sqlite.Open(NormalizeSQLiteDSN(dsn)), &gorm.Config{})
		if err != nil {
			return nil, DriverSQLite, err
		}
		return db, DriverSQLite, nil
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, DriverMySQL, err
	}
	return db, DriverMySQL, nil
}

// IsSQLiteDSN 判定 DSN 是否指向 SQLite。
//
// 识别三种写法：`sqlite://<路径>`、`sqlite:<路径>`、`file:<URI>`，
// 以及裸路径兜底（`.db` / `.sqlite` / `.sqlite3` 结尾，可带 `?_pragma=` 查询串）。
// 兜底只看扩展名而不「非 MySQL 即 SQLite」：猜错会把生产库读成空库，
// 宁可让老实的 MySQL DSN 继续走 MySQL。
func IsSQLiteDSN(dsn string) bool {
	s := strings.TrimSpace(dsn)
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for _, p := range []string{"sqlite://", "sqlite:", "file:"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	path := lower
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	for _, ext := range []string{".db", ".sqlite", ".sqlite3"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// NormalizeSQLiteDSN 把用户写的 DSN 规整成驱动可直接消费的形式，并补上生产必需的 pragma。
//
// 三项默认值（用户显式写了同名 pragma 时以其为准，不被覆盖）：
//   - journal_mode(WAL)：读写不再互斥。默认的 rollback journal 下，一次写会挡住所有读，
//     名片页与后台管理并发时表现为大面积卡顿。
//   - busy_timeout(5000)：拿不到写锁时最多等 5 秒再报 `database is locked`。
//     SQLite 是单写者，没有这个等待时间，并发写会直接失败。
//   - foreign_keys(1)：SQLite 出于兼容默认**关闭**外键约束，不打开则约束形同虚设。
//
// 已实测：`github.com/glebarez/sqlite`（modernc，SQLite 3.41.2）支持 `_pragma=` 查询参数。
func NormalizeSQLiteDSN(dsn string) string {
	s := strings.TrimSpace(dsn)
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "sqlite://"):
		s = s[len("sqlite://"):]
	case strings.HasPrefix(lower, "sqlite:"):
		s = s[len("sqlite:"):]
	}
	if s == "" {
		return s
	}
	sep := "?"
	if strings.Contains(s, "?") {
		sep = "&"
	}
	for _, p := range []struct{ key, val string }{
		{"journal_mode", "WAL"},
		{"busy_timeout", "5000"},
		{"foreign_keys", "1"},
	} {
		if strings.Contains(s, "_pragma="+p.key) {
			continue
		}
		s += sep + "_pragma=" + p.key + "(" + p.val + ")"
		sep = "&"
	}
	return s
}
