package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoginRateLimiter(t *testing.T) {
	lim := NewLoginRateLimiter(func() int { return 3 }, 3, time.Minute)

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
	lim := NewLoginRateLimiter(nil, 0, 0)
	require.True(t, lim.allow("ip"))
	require.NotNil(t, lim)
	require.Equal(t, 10, lim.limit(), "fallback 非法时应回退到内建缺省 10")
}

// TestLoginRateLimiterLimitFollowsProvider 上限由 limitFn 每次判定前取，
// 而不是构造时定死——否则后台改了限流阈值必须重启才生效。
func TestLoginRateLimiterLimitFollowsProvider(t *testing.T) {
	cur := 2
	lim := NewLoginRateLimiter(func() int { return cur }, 10, time.Minute)

	// 上限 2：第三次被拒
	require.True(t, lim.allow("ip"))
	require.True(t, lim.allow("ip"))
	require.False(t, lim.allow("ip"))

	// 后台把阈值调到 5：无需重建限流器，立刻放行
	cur = 5
	require.True(t, lim.allow("ip"))
}

// TestLoginRateLimiterInvalidLimitFallsBack 界面把阈值填成 0 / 负数时，
// 必须回落兜底值而不是「限流失效」（把防爆破关掉）或「全部拒绝」（把自己锁在外面）。
func TestLoginRateLimiterInvalidLimitFallsBack(t *testing.T) {
	lim := NewLoginRateLimiter(func() int { return 0 }, 7, time.Minute)
	require.Equal(t, 7, lim.limit())
	require.True(t, lim.allow("ip"))

	lim2 := NewLoginRateLimiter(func() int { return -3 }, 7, time.Minute)
	require.Equal(t, 7, lim2.limit())
}
