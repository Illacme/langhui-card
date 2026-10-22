package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Config 小程序凭据
type Config struct {
	AppID     string
	AppSecret string
}

// apiBase 微信接口基址。抽成变量是为了单测能把流量导向 httptest——
// 本包每个函数都是一次外网调用，不注入就只能「不测」或「打真接口」，
// 两者都比不上把成功 / 失败 / errcode 重试这些支线在本地跑一遍。
var apiBase = "https://api.weixin.qq.com"

var (
	tokenMu  sync.Mutex
	token    string
	tokenExp time.Time
)

// Code2Session 用小程序 wx.login 拿到的临时 code 换取 openid + session_key。
// jscode2session 无需 access_token，直接用 appid/secret/js_code 调微信接口。
// openid 是用户在本小程序内的唯一标识，用于在后台记录/核实申请人或登录者的微信身份。
func Code2Session(cfg Config, code string) (openid, sessionKey string, err error) {
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return "", "", fmt.Errorf("未配置 WX_APPID / WX_APPSECRET")
	}
	if code == "" {
		return "", "", fmt.Errorf("缺少 js_code")
	}
	u := fmt.Sprintf(
		"%s/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		apiBase, url.QueryEscape(cfg.AppID), url.QueryEscape(cfg.AppSecret), url.QueryEscape(code),
	)
	resp, err := http.Get(u)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var r struct {
		OpenID     string `json:"openid"`
		SessionKey string `json:"session_key"`
		UnionID    string `json:"unionid"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return "", "", err
	}
	if r.ErrCode != 0 || r.OpenID == "" {
		return "", "", fmt.Errorf("jscode2session err %d: %s", r.ErrCode, r.ErrMsg)
	}
	return r.OpenID, r.SessionKey, nil
}

// GetAccessToken 获取并缓存 access_token（默认 7200s，提前 5 分钟失效）
func GetAccessToken(cfg Config) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	return accessToken(cfg)
}

// accessToken 取 token 的实做，**不带锁**（调用方必须已持有 tokenMu）。
// 拆出来是因为 refreshToken 需要在同一个临界区里强制失效再重取：
// 一旦它带着锁去调 GetAccessToken，后者的 Lock 会撞上自己持有的同一把锁
// ——Go 的 mutex 不可重入，结果是永久死锁（小程序码接口在 40001 重试这条
// 支线上会整个挂住，且之后所有请求都会跟着卡在锁上）。
func accessToken(cfg Config) (string, error) {
	if token != "" && time.Now().Before(tokenExp) {
		return token, nil
	}
	// 与 Code2Session 同一条规矩：凭据没配就在本地报错，别带着空 appid 打外网
	// （微信只会回一句「无效」，排查时看不出其实是自己没配）
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return "", fmt.Errorf("未配置 WX_APPID / WX_APPSECRET")
	}
	u := fmt.Sprintf("%s/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s", apiBase, cfg.AppID, cfg.AppSecret)
	resp, err := http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var r struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return "", err
	}
	if r.ErrCode != 0 || r.AccessToken == "" {
		return "", fmt.Errorf("wx token err %d: %s", r.ErrCode, r.ErrMsg)
	}
	token = r.AccessToken
	tokenExp = time.Now().Add(time.Duration(r.ExpiresIn-300) * time.Second)
	return token, nil
}

// refreshToken 强制失效并重新获取（token 过期时重试用）。
// 与 GetAccessToken 共用同一把锁，故只调无锁的 accessToken（见其注释）。
func refreshToken(cfg Config) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	token = ""
	tokenExp = time.Time{}
	return accessToken(cfg)
}

// GetWXACodeUnlimit 生成无数量限制的小程序码（scene 携带参数），返回图片二进制
func GetWXACodeUnlimit(cfg Config, scene, page string) ([]byte, error) {
	return getWXACodeUnlimit(cfg, scene, page, true)
}

func getWXACodeUnlimit(cfg Config, scene, page string, retry bool) ([]byte, error) {
	tk, err := GetAccessToken(cfg)
	if err != nil {
		return nil, err
	}
	url := apiBase + "/wxa/getwxacodeunlimit?access_token=" + tk
	body, _ := json.Marshal(map[string]interface{}{
		"scene":       scene,
		"page":        page,
		"width":       430,
		"env_version": "release",
	})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if isImage(b) {
		return b, nil
	}
	// 失败返回 JSON {errcode, errmsg}
	var r struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	_ = json.Unmarshal(b, &r)
	// access_token 失效：刷新后重试一次
	if retry && (r.ErrCode == 40001 || r.ErrCode == 42001) {
		if _, e := refreshToken(cfg); e == nil {
			return getWXACodeUnlimit(cfg, scene, page, false)
		}
	}
	return nil, fmt.Errorf("wxacode err %d: %s", r.ErrCode, r.ErrMsg)
}

// isImage 判断字节是否为 JPEG/PNG（成功时微信返回图片二进制，失败时返回 JSON）
func isImage(b []byte) bool {
	if len(b) >= 3 && bytes.Equal(b[:3], []byte("\xff\xd8\xff") /* JPEG */) {
		return true
	}
	if len(b) >= 8 && bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n") /* PNG */) {
		return true
	}
	return false
}
