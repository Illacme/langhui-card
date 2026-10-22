package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 口令哈希是登录链路的地基：这里钉住「同值不同哈希」「错误口令必失败」
// 「非法哈希不 panic」三条——任何一条出问题都直接等于鉴权失效。

func TestHashPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("Str0ng!Pass")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(h, "$2"), "bcrypt 哈希应以 $2 开头")
	require.NotEqual(t, "Str0ng!Pass", h, "绝不能存明文")

	require.True(t, CheckPassword(h, "Str0ng!Pass"))
	require.False(t, CheckPassword(h, "str0ng!pass"), "bcrypt 大小写敏感")
	require.False(t, CheckPassword(h, "Str0ng!Pass "), "尾随空格应算不同口令")
	require.False(t, CheckPassword(h, ""))
}

// 同一个明文两次哈希必须不同（bcrypt 自带随机盐）：否则哈希值本身就成了
// 一张「相同口令对照表」，库泄露后能直接分组推断。
func TestHashPasswordUsesSalt(t *testing.T) {
	h1, err := HashPassword("same-password")
	require.NoError(t, err)
	h2, err := HashPassword("same-password")
	require.NoError(t, err)
	require.NotEqual(t, h1, h2)
	require.True(t, CheckPassword(h1, "same-password"))
	require.True(t, CheckPassword(h2, "same-password"))
}

// 非 bcrypt 值 / 空串 / 截断的哈希必须返回 false 而不是 panic：
// 库里若混进历史明文或别算法的值，登录接口会走到这里。
func TestCheckPasswordToleratesMalformedHash(t *testing.T) {
	for _, bad := range []string{"", "not-a-hash", "$2y$xx", "plain-text-password"} {
		require.False(t, CheckPassword(bad, "anything"), "非法哈希 %q 应返回 false", bad)
	}
}

// 空口令也要能哈希并校验（不拦业务规则，只保证不崩）
func TestHashPasswordEmpty(t *testing.T) {
	h, err := HashPassword("")
	require.NoError(t, err)
	require.True(t, CheckPassword(h, ""))
	require.False(t, CheckPassword(h, "x"))
}
