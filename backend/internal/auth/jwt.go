package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"weichuan/internal/config"
	"weichuan/internal/setting"

	"github.com/golang-jwt/jwt/v5"
)

// 吊销分两层，互补使用：
//   - 账号级（token_version）：改密 / 禁用使用户所有设备的所有令牌立即失效（见 middleware/auth.go）；
//   - 会话级（sid 黑名单）：登出只失效当前设备的会话，不影响同一账号的其他登录（见 repository/session.go）。

// 令牌有效期的**编译期缺省值兼安全下限**（分钟 / 天）。
// 有效期由系统设置 `access_token_ttl_minutes` / `refresh_token_ttl_days` 驱动（后台可改、即时生效），
// 但界面填 0 或负数时一律回落到这里——否则 access 有效期 0 会让所有人一登录就掉线，
// 连后台都进不去改回来（「改错可自愈」的底线，见 setting 包注释）。
const (
	defaultAccessTokenMinutes = 15
	defaultRefreshTokenDays   = 30
)

// AccessTokenTTL access 令牌有效期：取系统设置，非法回落缺省值。
func AccessTokenTTL() time.Duration {
	m := setting.GetInt("access_token_ttl_minutes", defaultAccessTokenMinutes)
	if m < 1 {
		m = defaultAccessTokenMinutes
	}
	return time.Duration(m) * time.Minute
}

// RefreshTokenTTL refresh 令牌有效期：取系统设置，非法回落缺省值。
func RefreshTokenTTL() time.Duration {
	d := setting.GetInt("refresh_token_ttl_days", defaultRefreshTokenDays)
	if d < 1 {
		d = defaultRefreshTokenDays
	}
	return time.Duration(d) * 24 * time.Hour
}

// Claims 自定义 JWT 载荷
type Claims struct {
	StaffID   int64  `json:"staffId"`   // 员工 staff_id 或管理员 admin.id
	Role      string `json:"role"`      // employee / admin
	CompanyID int64  `json:"companyId"` // 租户：员工/企业管理员归属企业；平台超管为 0
	JTI       string `json:"jti"`       // 单令牌唯一标识，便于审计
	SID       string `json:"sid"`       // 会话 ID：一次登录生成一个，access/refresh 共享；登出写入黑名单实现单设备精确吊销
	Typ       string `json:"typ"`       // "access" | "refresh"
	VID       int    `json:"vid"`       // 签发时的 token_version（安全戳）；改密/禁用时自增即令旧令牌失效
	jwt.RegisteredClaims
}

// IssueTokens 新会话签发 access + refresh 令牌（生成新 sid）。
// version 为用户当前的 token_version；任何密码变更 / 账号禁用都会使其自增，
// 从而让本次签发的令牌立即失效（账号级吊销）。
// companyID 为租户标识：employee 取员工归属企业，admin 取 company_id（0=平台超管）。
func IssueTokens(staffID int64, role string, version int, companyID int64) (access, refresh string, expiresIn int, err error) {
	return issueTokens(staffID, role, version, companyID, mustSessionID())
}

// IssueTokensWithSID 复用已有会话 ID 重新签发双令牌（/api/refresh 用）。
// 刷新不换 sid：一次登录始终对应同一个会话，登出吊销该 sid 即终结整条令牌链
// （含此前轮换掉的旧 refresh 令牌）。
func IssueTokensWithSID(staffID int64, role string, version int, companyID int64, sid string) (access, refresh string, expiresIn int, err error) {
	return issueTokens(staffID, role, version, companyID, sid)
}

func issueTokens(staffID int64, role string, version int, companyID int64, sid string) (access, refresh string, expiresIn int, err error) {
	now := time.Now()
	accessClaims := Claims{
		StaffID:   staffID,
		Role:      role,
		CompanyID: companyID,
		JTI:       mustJTI(),
		SID:       sid,
		Typ:       "access",
		VID:       version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL())),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	refreshClaims := Claims{
		StaffID:   staffID,
		Role:      role,
		CompanyID: companyID,
		JTI:       mustJTI(),
		SID:       sid,
		Typ:       "refresh",
		VID:       version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(RefreshTokenTTL())),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	access, err = jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(config.Load().JWTKey))
	if err != nil {
		return "", "", 0, err
	}
	refresh, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(config.Load().JWTKey))
	if err != nil {
		return "", "", 0, err
	}
	return access, refresh, int(AccessTokenTTL().Seconds()), nil
}

// ParseToken 校验签名与过期，返回载荷（不校验令牌类型 / 版本，后者由调用方按需判断）。
func ParseToken(t string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(t, claims, func(tk *jwt.Token) (interface{}, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.Load().JWTKey), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// randRead 抽成变量是为了让单测能覆盖「熵源失效」这条支线（真实环境下不可控）。
var randRead = rand.Read

// RandomJTI 生成 16 字节十六进制随机串，作为令牌唯一标识。
func RandomJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func mustJTI() string {
	jti, err := RandomJTI()
	if err != nil {
		// 熵耗尽属于极端情况，退化为时间戳+随机数避免panic
		return time.Now().Format("20060102150405.000000000")
	}
	return jti
}

// mustSessionID 生成会话 ID（与 JTI 同规格随机串）。
// 一次登录调用一次，生成的 sid 同时写入 access 与 refresh，
// 作为「一次登录 = 一个会话」的身份标识（登出吊销的最小粒度）。
func mustSessionID() string {
	sid, err := RandomJTI()
	if err != nil {
		return "sid-" + time.Now().Format("20060102150405.000000000")
	}
	return sid
}
