package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoginRateLimiter(t *testing.T) {
	lim := NewLoginRateLimiter(3, time.Minute)

	// 同一 IP 前 3 次放行，第 4 次被拒
	require.True(t, lim.allow("10.0.0.1"))
	require.True(t, lim.allow("10.0.0.1"))
	require.True(t, lim.allow("10.0.0.1"))
	require.False(t, lim.allow("10.0.0.1"), "超过上限应被拒绝")

	// 不同 IP 互不影响
	require.True(t, lim.allow("10.0.0.2"))
}

func TestLoginRateLimiterDefaults(t *testing.T) {
	// 非法参数应回退到安全默认值，不 panic 且能正常工作
	lim := NewLoginRateLimiter(0, 0)
	require.True(t, lim.allow("ip"))
	require.NotNil(t, lim)
}
