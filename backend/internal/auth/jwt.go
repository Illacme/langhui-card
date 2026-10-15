package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"langhui/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

// 令牌时效：access 短时效（降低泄露风险），refresh 长时效（避免频繁重登）。
// 吊销不依赖令牌自身过期，而是靠 Claims.VID 与用户 token_version 的比对（见 middleware/auth.go）。
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
)

// Claims 自定义 JWT 载荷
type Claims struct {
	StaffID int64  `json:"staffId"` // 员工 staff_id 或超管 admin.id
	Role    string `json:"role"`    // employee / admin
	JTI     string `json:"jti"`     // 唯一标识，便于审计 / 未来做 jti 黑名单
	Typ     string `json:"typ"`     // "access" | "refresh"
	VID     int    `json:"vid"`     // 签发时的 token_version（安全戳）；改密/禁用时自增即令旧令牌失效
	jwt.RegisteredClaims
}

// IssueTokens 同时签发 access + refresh 令牌。
// version 为用户当前的 token_version；任何密码变更 / 账号禁用都会使其自增，
// 从而让本次签发的令牌立即失效（吊销）。
func IssueTokens(staffID int64, role string, version int) (access, refresh string, expiresIn int, err error) {
	now := time.Now()
	accessClaims := Claims{
		StaffID: staffID,
		Role:    role,
		JTI:     mustJTI(),
		Typ:     "access",
		VID:     version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	refreshClaims := Claims{
		StaffID: staffID,
		Role:    role,
		JTI:     mustJTI(),
		Typ:     "refresh",
		VID:     version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(RefreshTokenTTL)),
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
	return access, refresh, int(AccessTokenTTL.Seconds()), nil
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

// RandomJTI 生成 16 字节十六进制随机串，作为令牌唯一标识。
func RandomJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
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
