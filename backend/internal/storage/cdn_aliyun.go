package storage

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// cdnAPIVersion 阿里云 CDN OpenAPI 版本（如与实际不一致请按需调整）
const cdnAPIVersion = "2014-11-11"

// AliyunCDNRefresher 阿里云 CDN 缓存刷新（OpenAPI，纯标准库实现，无额外依赖）。
// 文件覆盖写后调用 Refresh，使边缘节点旧缓存失效，避免回源拿到陈旧内容。
type AliyunCDNRefresher struct {
	AccessKeyID     string
	AccessKeySecret string
	Endpoint        string // 如 cdn.aliyuncs.com
}

// NewAliyunCDNRefresher 构造刷新器；endpoint 为空时默认 cdn.aliyuncs.com
// （内部归一化为 https://<endpoint>，见 cdnBaseURL）
func NewAliyunCDNRefresher(id, secret, endpoint string) *AliyunCDNRefresher {
	return &AliyunCDNRefresher{
		AccessKeyID:     id,
		AccessKeySecret: secret,
		Endpoint:        cdnBaseURL(endpoint, "cdn.aliyuncs.com"),
	}
}

// Refresh 刷新一组 URL（单次最多 1000 条，超出自动分批）
func (a *AliyunCDNRefresher) Refresh(urls []string) error {
	return cdnBatch(urls, 1000, a.refreshOnce)
}

func (a *AliyunCDNRefresher) refreshOnce(urls []string) error {
	params := url.Values{}
	params.Set("Action", "RefreshObjectCaches")
	params.Set("ObjectType", "File")
	params.Set("Version", cdnAPIVersion)
	params.Set("Format", "JSON")
	params.Set("AccessKeyId", a.AccessKeyID)
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	params.Set("SignatureVersion", "1.0")
	params.Set("SignatureNonce", fmt.Sprintf("%d", time.Now().UnixNano()))
	// ObjectPath 多个 URL 用换行分隔
	params.Set("ObjectPath", strings.Join(urls, "\n"))

	// 规范化查询串（按 key 排序）+ V1 签名
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var canon strings.Builder
	for i, k := range keys {
		if i > 0 {
			canon.WriteString("&")
		}
		canon.WriteString(percentEncode(k))
		canon.WriteString("=")
		canon.WriteString(percentEncode(params.Get(k)))
	}
	stringToSign := "GET&" + percentEncode("/") + "&" + percentEncode(canon.String())
	mac := hmac.New(sha1.New, []byte(a.AccessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	final := url.Values{}
	for k, v := range params {
		final[k] = v
	}
	final.Set("Signature", percentEncode(signature))
	reqURL := a.Endpoint + "/?" + final.Encode()
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("cdn refresh request: %w", err)
	}
	// 走共享客户端（带 15s 超时）：刷新同步串在用户请求里，厂商不响应不能挂死请求
	body, err := cdnDo(req)
	if err != nil {
		return err
	}
	// 成功响应无 Code 字段；失败响应含 {"Code":"...","Message":"..."}
	var r struct {
		Code string `json:"Code"`
	}
	if json.Unmarshal(body, &r) == nil && r.Code != "" {
		return fmt.Errorf("cdn refresh failed: %s (%s)", r.Code, string(body))
	}
	return nil
}

// percentEncode 按阿里云规则编码：空格→%20，星号/波浪号保持，其余按 RFC3986
func percentEncode(s string) string {
	s = url.QueryEscape(s)
	s = strings.ReplaceAll(s, "+", "%20")
	s = strings.ReplaceAll(s, "*", "%2A")
	s = strings.ReplaceAll(s, "%7E", "~")
	return s
}
