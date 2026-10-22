package auth

import (
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"weichuan/internal/dbopen"
	"weichuan/internal/setting"
)

// 测试统一用固定签名密钥：JWT 的验证对象是 config.Load().JWTKey，
// 环境变量是它的唯一注入口，t.Setenv 即可隔离（进程内其它用例读到的也是这个值）。
const testJWTKey = "unit-test-jwt-key-0123456789abcdef"

func setKey(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_KEY", testJWTKey)
}

// newSettingDB 起一个临时 SQLite 库并初始化系统设置（令牌有效期由它驱动）。
func newSettingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, _, err := dbopen.Open(filepath.Join(t.TempDir(), "setting.db"))
	require.NoError(t, err)
	require.NoError(t, setting.Init(db))
	return db
}

// ---- 签发与解析 ----

func TestIssueTokensRoundTrip(t *testing.T) {
	setKey(t)

	access, refresh, expiresIn, err := IssueTokens(42, "employee", 3, 7)
	require.NoError(t, err)
	require.Equal(t, int(AccessTokenTTL().Seconds()), expiresIn)

	ac, err := ParseToken(access)
	require.NoError(t, err)
	require.EqualValues(t, 42, ac.StaffID)
	require.Equal(t, "employee", ac.Role)
	require.EqualValues(t, 7, ac.CompanyID)
	require.Equal(t, "access", ac.Typ)
	require.Equal(t, 3, ac.VID)
	require.NotEmpty(t, ac.JTI)
	require.NotEmpty(t, ac.SID)

	rc, err := ParseToken(refresh)
	require.NoError(t, err)
	require.Equal(t, "refresh", rc.Typ)
	// access 与 refresh 共享同一个 sid：一次登录 = 一个会话，
	// 登出吊销 sid 才能同时终结两条令牌链（否则 refresh 还能换新 access）
	require.Equal(t, ac.SID, rc.SID)
	require.NotEqual(t, ac.JTI, rc.JTI, "每条令牌的 jti 应各自独立，便于按条审计")
	// refresh 必须比 access 活得久，否则刷新窗口为零
	require.True(t, rc.ExpiresAt.Time.After(ac.ExpiresAt.Time))
}

// 刷新不换 sid：换 sid 会让「登出吊销当前会话」失效——旧 sid 被吊销，
// 但新令牌带着新 sid 继续可用，等于登出只吊销了一半。
func TestIssueTokensWithSIDKeepsSession(t *testing.T) {
	setKey(t)
	_, _, _, err := IssueTokens(1, "admin", 0, 0)
	require.NoError(t, err)

	a1, r1, _, err := IssueTokensWithSID(9, "admin", 1, 0, "sid-fixed")
	require.NoError(t, err)
	ac, err := ParseToken(a1)
	require.NoError(t, err)
	require.Equal(t, "sid-fixed", ac.SID)
	rc, err := ParseToken(r1)
	require.NoError(t, err)
	require.Equal(t, "sid-fixed", rc.SID)
}

func TestIssueTokensGeneratesDistinctSessionIDs(t *testing.T) {
	setKey(t)
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		a, _, _, err := IssueTokens(1, "admin", 0, 0)
		require.NoError(t, err)
		ac, err := ParseToken(a)
		require.NoError(t, err)
		require.False(t, seen[ac.SID], "会话 ID 重复：%s", ac.SID)
		seen[ac.SID] = true
	}
}

// ---- 解析必须拒绝的令牌 ----

func TestParseTokenRejectsTampered(t *testing.T) {
	setKey(t)
	access, _, _, err := IssueTokens(1, "admin", 0, 0)
	require.NoError(t, err)

	// 改动签名段中间的一个字符。
	// 注意不能改最后一个字符：base64 末位只承载部分比特，解码时多余比特会被忽略，
	// 换个字符有可能解出**完全相同**的签名字节——那样测试会随机通过/失败。
	parts := strings.Split(access, ".")
	require.Len(t, parts, 3)
	sig := []byte(parts[2])
	i := len(sig) / 2
	if sig[i] == 'A' {
		sig[i] = 'B'
	} else {
		sig[i] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(sig)
	_, err = ParseToken(tampered)
	require.Error(t, err, "篡改过的令牌必须拒绝")

	// 改载荷更要拒：典型攻击是把 role 改成 admin / 换 staffId 冒充他人
	forgedClaims := Claims{StaffID: 999, Role: "admin", Typ: "access", CompanyID: 0,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	repacked, err := jwt.NewWithClaims(jwt.SigningMethodHS256, forgedClaims).SignedString([]byte("other-key-0123456789abcdef"))
	require.NoError(t, err)
	_, err = ParseToken(repacked)
	require.Error(t, err, "换密钥重签的令牌必须拒绝")
}

func TestParseTokenRejectsWrongKey(t *testing.T) {
	setKey(t)
	access, _, _, err := IssueTokens(1, "admin", 0, 0)
	require.NoError(t, err)
	claims, err := ParseToken(access)
	require.NoError(t, err)

	// 用另一个密钥重签同一份载荷
	other := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	forged, err := other.SignedString([]byte("attacker-key-0123456789"))
	require.NoError(t, err)
	_, err = ParseToken(forged)
	require.Error(t, err, "他人密钥签的令牌必须拒绝")
}

// alg=none 是 JWT 的经典绕过：攻击者把签名算法改成 none 并去掉签名，
// 早期实现会「无需校验」直接放行。这里必须拒绝。
func TestParseTokenRejectsAlgNone(t *testing.T) {
	setKey(t)

	// 手工拼一个 alg=none、无签名的令牌
	header := `{"alg":"none","typ":"JWT"}`
	payload := `{"staffId":1,"role":"admin","companyId":0,"typ":"access","vid":0}`
	tok := b64(header) + "." + b64(payload) + "."
	_, err := ParseToken(tok)
	require.Error(t, err, "alg=none 的无签名令牌必须拒绝")
}

// b64 JWT 用的 base64url（无填充）
func b64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// 过期令牌必须被拒（jwt 库会校验 exp，这里钉住我们确实打开了校验）
func TestParseTokenRejectsExpired(t *testing.T) {
	setKey(t)
	claims := Claims{
		StaffID: 1, Role: "admin", Typ: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
		},
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTKey))
	require.NoError(t, err)
	_, err = ParseToken(expired)
	require.Error(t, err)
}

func TestParseTokenRejectsGarbage(t *testing.T) {
	setKey(t)
	for _, s := range []string{"", "not-a-token", "a.b.c"} {
		_, err := ParseToken(s)
		require.Error(t, err, "非法输入 %q 必须报错", s)
	}
}

// ---- 有效期：系统设置驱动 + 非法值回落 ----

func TestTokenTTLDefaults(t *testing.T) {
	setKey(t)
	// 未初始化系统设置（进程内没有值）时走编译期缺省
	require.Equal(t, 15*time.Minute, AccessTokenTTL())
	require.Equal(t, 30*24*time.Hour, RefreshTokenTTL())
}

// 界面把有效期填成 0 / 负数时一律回落缺省值：access 有效期 0 会让所有人
// 一登录就掉线，连后台都进不去改回来（「改错可自愈」的底线）。
func TestTokenTTLFallsBackOnInvalidSetting(t *testing.T) {
	setKey(t)
	db := newSettingDB(t)
	// 系统设置是包级全局缓存，还原缺省值，避免污染同包其它用例
	t.Cleanup(func() {
		_ = setting.Upsert("access_token_ttl_minutes", "15")
		_ = setting.Upsert("refresh_token_ttl_days", "30")
	})

	require.NoError(t, setting.Upsert("access_token_ttl_minutes", "0"))
	require.Equal(t, 15*time.Minute, AccessTokenTTL(), "0 应回落 15 分钟")

	require.NoError(t, setting.Upsert("access_token_ttl_minutes", "-5"))
	require.Equal(t, 15*time.Minute, AccessTokenTTL(), "负数应回落 15 分钟")

	require.NoError(t, setting.Upsert("refresh_token_ttl_days", "0"))
	require.Equal(t, 30*24*time.Hour, RefreshTokenTTL(), "0 应回落 30 天")

	// 合法值必须真的生效（否则「回落」就成了「永远用缺省」）
	require.NoError(t, setting.Upsert("access_token_ttl_minutes", "5"))
	require.Equal(t, 5*time.Minute, AccessTokenTTL())
	require.NoError(t, setting.Upsert("refresh_token_ttl_days", "7"))
	require.Equal(t, 7*24*time.Hour, RefreshTokenTTL())

	_, _, expiresIn, err := IssueTokens(1, "admin", 0, 0)
	require.NoError(t, err)
	require.Equal(t, 300, expiresIn, "expiresIn 应跟着设置走")
	require.NotNil(t, db)
}

// ---- 随机串 ----

func TestRandomJTIUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		s, err := RandomJTI()
		require.NoError(t, err)
		require.Len(t, s, 32, "16 字节十六进制")
		require.False(t, seen[s])
		seen[s] = true
	}
}

// 熵源失效必须报错而不是返回空串：空 jti / 空 sid 会让「一次登录一个会话」
// 退化成「所有会话共用一个标识」，登出就会误伤其它设备。
func TestRandomJTIPropagatesEntropyFailure(t *testing.T) {
	orig := randRead
	randRead = func(b []byte) (int, error) { return 0, errors.New("熵源不可用") }
	t.Cleanup(func() { randRead = orig })

	s, err := RandomJTI()
	require.Error(t, err)
	require.Empty(t, s)
}

// mustJTI / mustSessionID 是「尽力而为」的退化路径：拿不到随机数时退回时间戳，
// 宁可标识弱一些，也不能让签发整条链路失败（管理员登不进去比标识弱更糟）。
func TestMustHelpersDegradeWithoutPanic(t *testing.T) {
	orig := randRead
	randRead = func(b []byte) (int, error) { return 0, errors.New("熵源不可用") }
	t.Cleanup(func() { randRead = orig })

	require.NotEmpty(t, mustJTI())
	require.NotEmpty(t, mustSessionID())
}
