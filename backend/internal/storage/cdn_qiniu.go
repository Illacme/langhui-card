package storage

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

// qiniuRefreshPath 七牛融合 CDN 缓存刷新接口路径（fusion.qiniuapi.com 下）。
const qiniuRefreshPath = "/v2/tune/refresh"

// QiniuCDNRefresher 七牛云融合 CDN 缓存刷新（QBox 鉴权，纯标准库）。
//
// 配额（七牛官方文档，2026-10-23 核实）：文件刷新默认 500 次/天、**单次最多 20 个 URL**，
// 超单次上限报 400034；url+dir 总量 QPS ≤ 240。故这里按 20 条分批——宁可多打几批
// 也别整批被拒（用户可在控制台申请提额后再放宽）。
type QiniuCDNRefresher struct {
	AccessKey string // 七牛 AccessKey
	SecretKey string // 七牛 SecretKey
	Endpoint  string // 默认 fusion.qiniuapi.com
}

// NewQiniuCDNRefresher 构造刷新器；endpoint 为空时默认 fusion.qiniuapi.com
func NewQiniuCDNRefresher(ak, sk, endpoint string) *QiniuCDNRefresher {
	return &QiniuCDNRefresher{
		AccessKey: ak,
		SecretKey: sk,
		Endpoint:  cdnBaseURL(endpoint, "fusion.qiniuapi.com"),
	}
}

// Refresh 刷新一组 URL（单次最多 20 条，超出自动分批）
func (q *QiniuCDNRefresher) Refresh(urls []string) error {
	if q.AccessKey == "" || q.SecretKey == "" {
		return fmt.Errorf("七牛云 CDN 未配置 AccessKey / SecretKey")
	}
	return cdnBatch(urls, 20, q.refreshOnce)
}

func (q *QiniuCDNRefresher) refreshOnce(urls []string) error {
	payload, err := json.Marshal(map[string]interface{}{"urls": urls})
	if err != nil {
		return fmt.Errorf("cdn refresh encode: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, q.Endpoint+qiniuRefreshPath, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cdn refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "QBox "+q.accessToken())

	body, err := cdnDo(req)
	if err != nil {
		return err
	}
	// 成功：{"code":200,"taskIds":{...}}；失败：{"code":400034,"error":"..."} 或 {"error":"..."}
	var r struct {
		Code  int    `json:"code"`
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &r) == nil {
		if r.Code != 0 && r.Code != 200 {
			return fmt.Errorf("cdn refresh failed: code=%d %s", r.Code, r.Error)
		}
		if r.Code == 0 && r.Error != "" {
			return fmt.Errorf("cdn refresh failed: %s", r.Error)
		}
	}
	return nil
}

// accessToken 生成 QBox 管理凭证：<AK>:<urlsafe_base64(HMAC-SHA1(SK, 签名串))>。
//
// 签名串只拼「路径 + 换行」，**请求体不参与**——七牛官方鉴权指南明确：仅当
// Content-Type 为 application/x-www-form-urlencoded 且存在请求体时，请求体才参与
// QBox 签名；CDN 所有接口都用 JSON 体，因此这里不把 body 拼进去（拼了反而 401）。
func (q *QiniuCDNRefresher) accessToken() string {
	mac := hmac.New(sha1.New, []byte(q.SecretKey))
	mac.Write([]byte(qiniuRefreshPath + "\n"))
	sign := base64.URLEncoding.EncodeToString(mac.Sum(nil)) // URL 安全 Base64：+→- /→_
	return q.AccessKey + ":" + sign
}
