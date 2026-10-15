package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Config 小程序凭据
type Config struct {
	AppID     string
	AppSecret string
}

var (
	tokenMu  sync.Mutex
	token    string
	tokenExp time.Time
)

// GetAccessToken 获取并缓存 access_token（默认 7200s，提前 5 分钟失效）
func GetAccessToken(cfg Config) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	if token != "" && time.Now().Before(tokenExp) {
		return token, nil
	}
	u := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s", cfg.AppID, cfg.AppSecret)
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

// refreshToken 强制失效并重新获取（token 过期时重试用）
func refreshToken(cfg Config) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	token = ""
	tokenExp = time.Time{}
	return GetAccessToken(cfg)
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
	url := "https://api.weixin.qq.com/wxa/getwxacodeunlimit?access_token=" + tk
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
