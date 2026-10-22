package repository

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"weichuan/internal/dbopen"
	"weichuan/internal/model"
)

// revoked_sessions 的定期清理（main.go 启动时挂的那个后台任务）。
// 用 SQLite 临时库覆盖：清理语义与方言无关（`expires_at < now` 的 DELETE）。

func newSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, _, err := dbopen.Open(filepath.Join(t.TempDir(), "session.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.RevokedSession{}))
	return db
}

func seedSession(t *testing.T, db *gorm.DB, sid string, expiresAt time.Time) {
	t.Helper()
	require.NoError(t, db.Create(&model.RevokedSession{
		SID:       sid,
		UserType:  "admin",
		UserID:    1,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}).Error)
}

func countSessions(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.RevokedSession{}).Count(&n).Error)
	return n
}

// 清理只删过期行；未过期的吊销记录必须留着——它们正是「登出后令牌不可用」的凭据。
func TestPurgeExpiredSessionsKeepsLiveRevocations(t *testing.T) {
	db := newSessionTestDB(t)
	seedSession(t, db, "expired-1", time.Now().Add(-time.Hour))
	seedSession(t, db, "expired-2", time.Now().Add(-24*time.Hour))
	seedSession(t, db, "live-1", time.Now().Add(time.Hour))

	n, err := PurgeExpiredSessions(db)
	require.NoError(t, err)
	require.EqualValues(t, 2, n, "只应删掉两条过期行")
	require.EqualValues(t, 1, countSessions(t, db))

	revoked, err := IsSessionRevoked(db, "live-1")
	require.NoError(t, err)
	require.True(t, revoked, "未过期的吊销记录必须保留")
	revoked, err = IsSessionRevoked(db, "expired-1")
	require.NoError(t, err)
	require.False(t, revoked)
}

// 后台任务：到期后应自动清掉，且 stop() 之后不再继续删（防止测试/热重载泄漏 goroutine）。
func TestStartExpiredSessionPurgeCleansAndStops(t *testing.T) {
	db := newSessionTestDB(t)
	seedSession(t, db, "expired-soon", time.Now().Add(-time.Minute))
	seedSession(t, db, "live-1", time.Now().Add(time.Hour))

	stop := StartExpiredSessionPurge(db, 20*time.Millisecond)
	t.Cleanup(stop)

	deadline := time.Now().Add(2 * time.Second)
	cleared := false
	for time.Now().Before(deadline) {
		if countSessions(t, db) == 1 {
			cleared = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.True(t, cleared, "后台任务应在间隔内清掉过期行（剩余 1 条未过期）")

	// stop 之后新增的过期行不应再被清掉
	stop()
	seedSession(t, db, "expired-after-stop", time.Now().Add(-time.Minute))
	time.Sleep(100 * time.Millisecond)
	require.EqualValues(t, 2, countSessions(t, db), "stop() 后不应继续清理")
}

// interval <= 0 时回退默认间隔，而不是立刻疯狂 tick（0 会打爆数据库）。
func TestStartExpiredSessionPurgeFallsBackToDefault(t *testing.T) {
	db := newSessionTestDB(t)
	seedSession(t, db, "expired", time.Now().Add(-time.Minute))

	stop := StartExpiredSessionPurge(db, 0)
	t.Cleanup(stop)
	// 默认间隔 6 小时，这里只验证「没有立刻把库删空」（即没有以 0 间隔死循环）
	time.Sleep(50 * time.Millisecond)
	require.EqualValues(t, 1, countSessions(t, db))
}

// Redis 之外的兜底：吊销写入 + 清理的完整闭环（登出 → 黑名单生效 → 到期清理）。
func TestRevokeThenPurgeLifecycle(t *testing.T) {
	db := newSessionTestDB(t)
	require.NoError(t, RevokeSession(db, "sid-1", "employee", 42, time.Now().Add(-time.Minute)))
	revoked, err := IsSessionRevoked(db, "sid-1")
	require.NoError(t, err)
	require.True(t, revoked)

	n, err := PurgeExpiredSessions(db)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	revoked, err = IsSessionRevoked(db, "sid-1")
	require.NoError(t, err)
	require.False(t, revoked, "清理后该会话的令牌本就早已过期，不应再判定为吊销")
}
