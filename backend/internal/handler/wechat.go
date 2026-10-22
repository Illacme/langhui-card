package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"weichuan/internal/config"
	"weichuan/internal/middleware"
	"weichuan/internal/setting"
	"weichuan/internal/storage"
	"weichuan/internal/wechat"

	"github.com/gin-gonic/gin"
)

// 小程序码内存缓存：避免每次请求都重新调微信 + 重新写对象存储。
// expAt 表示「二维码内容」的过期时间（到达后重新调微信生成）；私有桶下每次返回前会单独签发短期 GET URL。
var (
	wxacodeMu    sync.Mutex
	wxacodeCache = map[int64]wxacodeEntry{}
)

// wxacodeCacheMax 小程序码缓存条数上限。
// staffId 可被枚举，而每条缓存 miss 都会真调微信接口 + 写一次对象存储：
// 没有上限时，扫一段 staffId 就能把缓存撑成一张越来越大的表（同时也放大微信配额消耗）。
// 限流是第一道闸，容量上限是纵深防御——两者都不是「防止正常使用」，正常员工量远达不到这个数。
const wxacodeCacheMax = 4096

type wxacodeEntry struct {
	key       string    // 对象键，如 wxacode/12.jpg
	publicURL string    // 公开形态 URL（BaseURL/key），公开桶或本地私有兜底用
	expAt     time.Time // 二维码内容过期时间（按 cfg.WxacodeTTL）
}

// wxacodePut 写入缓存并在接近上限时先淘汰：先清已过期的，仍满则按 expAt 淘汰最早的一半。
// 调用方不持锁（本函数内部加锁）。
func wxacodePut(staffID int64, e wxacodeEntry) {
	wxacodeMu.Lock()
	defer wxacodeMu.Unlock()
	if _, exists := wxacodeCache[staffID]; !exists && len(wxacodeCache) >= wxacodeCacheMax {
		wxacodeEvictLocked()
	}
	wxacodeCache[staffID] = e
}

// wxacodeEvictLocked 淘汰：先删已过期的；删完仍满则删掉 expAt 最早的一半。
// 调用方必须已持有 wxacodeMu。
func wxacodeEvictLocked() {
	now := time.Now()
	for id, e := range wxacodeCache {
		if now.After(e.expAt) {
			delete(wxacodeCache, id)
		}
	}
	if len(wxacodeCache) < wxacodeCacheMax {
		return
	}
	// 按过期时间排序取最早的一半：保留离过期最远的（最可能还被用到的）那一半
	ids := make([]int64, 0, len(wxacodeCache))
	for id := range wxacodeCache {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return wxacodeCache[ids[i]].expAt.Before(wxacodeCache[ids[j]].expAt)
	})
	for _, id := range ids[:len(ids)/2] {
		delete(wxacodeCache, id)
	}
}

// getWXACode 小程序码生成器的间接层：生产指向 wechat.GetWXACodeUnlimit。
// 抽这一层的唯一目的是可测——微信接口要外网 + 真凭据，注入后才能在单测里覆盖
// 「固定对象名覆盖写 → CDN 刷新」这条支线（见 TestCDNRefreshOnOverwritePaths）。
var getWXACode = wechat.GetWXACodeUnlimit

// RegisterWechat 小程序码接口：按 staffId 生成（写入当前存储驱动）小程序码，返回可访问 URL
// 海报合成放在小程序端 canvas；后端只负责产出小程序码图片，存到 local/minio/oss 后返回访问 URL。
// 私有桶（STORAGE_PUBLIC=false）下，返回的是限时签名 GET URL，避免把对象设为公开读。
//
// refresher 可选：小程序码对象名固定（wxacode/<staffID>.jpg），TTL 到期重新生成 = 覆盖写，
// 覆盖后要让 CDN 边缘失效，否则已分享出去的旧链接会一直拿到旧图
// （2026-10-23 之前刷新只挂在随机名的上传接口，这条覆盖路径根本没触发）。
func RegisterWechat(r *gin.Engine, cfg config.Config, up storage.Uploader, refresher storage.CDNRefresher) {
	// AppID / AppSecret 都每次请求解析：可在后台「系统设置 → 微信」配置，
	// 未配置时回退环境变量 WX_APPID / WX_APPSECRET——换凭据不必改环境变量重启。
	wcOf := func() wechat.Config {
		return wechat.Config{AppID: resolveWXAppID(cfg), AppSecret: resolveWXAppSecret(cfg)}
	}

	// 微信身份换取（公开）：小程序 wx.login 的 code → openid。
	// 无论是否配置小程序码凭据都注册；未配置时返回明确错误，前端据此降级（不影响账号密码登录/申请）。
	r.POST("/api/wechat/openid", func(c *gin.Context) {
		var req struct {
			Code string `json:"code"`
		}
		if c.ShouldBindJSON(&req) != nil || req.Code == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "缺少 code"})
			return
		}
		openid, _, err := wechat.Code2Session(wcOf(), req.Code)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "openid": openid})
	})

	// 小程序码是**公开**接口（访客扫码看名片就要取，不能挂鉴权），
	// 但一次 cache miss = 一次真实微信接口调用（有配额）+ 一次对象存储写入，
	// staffId 又是可枚举的整数——没有闸门时，一个脚本就能打光微信配额并撑爆内存缓存。
	// 所以按客户端 IP 限流：正常分享点开远达不到这个频率，撞线的只有扫号行为
	// （不同 staffId 也走同一个桶——扫号正是靠换 staffId 放大调用量的）。
	wxacodeLimiter := middleware.NewRateLimiter(func() int {
		return setting.GetInt("wxacode_rate_limit_per_min", 30)
	}, 30, time.Minute).
		WithMessage("小程序码生成过于频繁，请稍后再试")

	r.GET("/api/wechat/wxacode", wxacodeLimiter.Handler(), func(c *gin.Context) {
		// 未配置凭据时降级为明确错误：放在请求期判定（而非注册期一次性判定），
		// 这样后台补上 AppID 后立刻可用，无需重启进程。
		wc := wcOf()
		if wc.AppID == "" || wc.AppSecret == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未配置小程序 AppID / WX_APPSECRET"})
			return
		}
		staffID, _ := strconv.ParseInt(c.Query("staffId"), 10, 64)
		if staffID <= 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "缺少 staffId"})
			return
		}
		page := c.Query("page")
		if page == "" {
			page = "pages/index/index"
		}
		scene := "staffId=" + strconv.FormatInt(staffID, 10)

		// 1) 命中缓存且二维码内容未过期：直接取（私有桶下当场签发短期 URL）
		wxacodeMu.Lock()
		e, ok := wxacodeCache[staffID]
		wxacodeMu.Unlock()
		if ok && time.Now().Before(e.expAt) {
			url, err := resolveWxacodeURL(e, cfg, up)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": url}})
			return
		}

		// 2) 重新生成：调微信 -> 写入当前存储驱动 -> 缓存
		b, err := getWXACode(wc, scene, page)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		key := fmt.Sprintf("wxacode/%d.jpg", staffID)
		publicURL, err := up.SaveBytes(key, b, "image/jpeg")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		e = wxacodeEntry{key: key, publicURL: publicURL, expAt: time.Now().Add(wxacodeTTL(cfg))}
		wxacodePut(staffID, e)
		// 固定对象名的覆盖写：刷新的是公开形态 URL（私有桶下返回的短期签名 URL 每次都变，
		// 交给 CDN 刷也没有意义；私有桶本身也不该被 CDN 缓存）
		refreshCDN(refresher, e.publicURL)

		url, err := resolveWxacodeURL(e, cfg, up)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": url}})
	})
}

// resolveWxacodeURL 公开桶返回公开 URL；私有桶（且存储支持签名）返回限时 GET URL（每次重签，1 小时有效）。
// 本地驱动无 Signer：静态路由已托管，回退公开 URL。
func resolveWxacodeURL(e wxacodeEntry, cfg config.Config, up storage.Uploader) (string, error) {
	if cfg.StoragePublic {
		return e.publicURL, nil
	}
	if s, ok := up.(storage.Signer); ok {
		// 签名时长取系统设置（默认 1 小时），下限 1 分钟
		return s.SignGetURL(e.key, signTTL("wxacode_sign_ttl_minutes", 60))
	}
	return e.publicURL, nil
}

// resolveWXAppID 小程序 AppID：优先取系统设置 wx_appid，留空时回退环境变量 WX_APPID。
// 留空回退而非「设置项存在就覆盖」，是因为 seed 会把 wx_appid 写成空串——
// 若直接以设置为准，会让既有只配了环境变量的部署凭空失效。
func resolveWXAppID(cfg config.Config) string {
	if v := strings.TrimSpace(setting.GetString("wx_appid", "")); v != "" {
		return v
	}
	return cfg.WXAppID
}

// resolveWXAppSecret 与 resolveWXAppID 同构：优先取系统设置 wx_appsecret（界面里以掩码展示、
// 提交后仅存库不回传），留空回退环境变量 WX_APPSECRET。
func resolveWXAppSecret(cfg config.Config) string {
	if v := strings.TrimSpace(setting.GetString("wx_appsecret", "")); v != "" {
		return v
	}
	return cfg.WXAppSecret
}

// wxacodeTTL 小程序码缓存有效期：优先取系统设置 upload/wxacode_ttl_hours，
// 未配置 / 非法时回退 config.WxacodeTTL（环境变量或默认 720h）。
// 让「小程序码缓存时长」从系统设置界面可改，而不再写死在环境变量。
func wxacodeTTL(cfg config.Config) time.Duration {
	h := setting.GetInt("wxacode_ttl_hours", int(cfg.WxacodeTTL.Hours()))
	if h <= 0 {
		h = int(cfg.WxacodeTTL.Hours())
	}
	return time.Duration(h) * time.Hour
}
