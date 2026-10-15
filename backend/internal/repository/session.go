package repository

import (
	"time"

	"langhui/internal/model"

	"gorm.io/gorm"
)

// IsSessionRevoked 判断会话 sid 是否已被吊销（登出 / 踢下线时写入黑名单）。
// sid 为空（历史令牌，签发时无 sid）时按未吊销处理，由调用方结合其他校验兜底。
func IsSessionRevoked(db *gorm.DB, sid string) (bool, error) {
	if sid == "" {
		return false, nil
	}
	var cnt int64
	if err := db.Model(&model.RevokedSession{}).Where("sid = ?", sid).Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// RevokeSession 把会话 sid 写入黑名单（幂等：重复登出不会重复生效）。
// expiresAt 传该会话 refresh 令牌的过期时间，供后台按时间清理过期记录；
// 吊销判定只看 sid 是否存在，与 expiresAt 无关。
//
// 可移植性说明：刻意不用 clause.OnConflict{DoNothing}——它生成
// 「ON CONFLICT DO NOTHING」，MySQL 不支持该语法（仅 SQLite / PG 支持）。
// 这里用「先查后插」：同一会话的并发登出只会有一个 Insert 成功，
// 且两者写入的是同一个 sid，无论谁成功吊销判定都生效；失败方的
// duplicate-key 错误由调用方忽略（登出接口本身始终返回成功）。
func RevokeSession(db *gorm.DB, sid, userType string, userID int64, expiresAt time.Time) error {
	if sid == "" {
		return nil
	}
	exists, err := IsSessionRevoked(db, sid)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	rs := model.RevokedSession{
		SID:       sid,
		UserType:  userType,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return db.Create(&rs).Error
}
