package auth

import (
	"errors"
	"time"

	"langhui/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 自定义 JWT 载荷
type Claims struct {
	StaffID int64  `json:"staffId"` // 员工 staff_id 或超管 admin.id
	Role    string `json:"role"`    // employee / admin
	jwt.RegisteredClaims
}

// GenerateToken 签发 7 天有效期 token
func GenerateToken(staffID int64, role string) (string, error) {
	claims := Claims{
		StaffID: staffID,
		Role:    role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.Load().JWTKey))
}

// ParseToken 校验并返回载荷
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
