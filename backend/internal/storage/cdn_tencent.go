package storage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// tencentCDNVersion 腾讯云 CDN（PurgeUrlsCache）的 API 版本，接口文档明确为 2018-06-06。
// 与「云 API 版本按产品各不相同」一致，这里不能沿用别家的版本号。
const tencentCDNVersion = "2018-06-06"

// tencentCDNService TC3 签名 credential scope 里的产品名：取接口域名前缀 cdn。
// CDN 是全局服务，Region 不传（官方 SDK 也传空串），因此 scope 里没有地域段。
const tencentCDNService = "cdn"

// TencentCDNRefresher 腾讯云 CDN 缓存刷新（PurgeUrlsCache + TC3-HMAC-SHA256，纯标准库）。
//
// 每次最多提交 1000 条（接口文档 LimitExceeded.CdnPurgeExceedBatchLimit 即超此限），
// 每日境内/境外各 10000 条额度——超出会返回 LimitExceeded.CdnPurgeExceedDayLimit。
type TencentCDNRefresher struct {
	SecretID  string // 腾讯云 SecretId（对应通用配置里的 AccessKeyId）
	SecretKey string // 腾讯云 SecretKey
	Endpoint  string // 默认 cdn.tencentcloudapi.com
}

// NewTencentCDNRefresher 构造刷新器；endpoint 为空时默认 cdn.tencentcloudapi.com
func NewTencentCDNRefresher(id, secret, endpoint string) *TencentCDNRefresher {
	return &TencentCDNRefresher{
		SecretID:  id,
		SecretKey: secret,
		Endpoint:  cdnBaseURL(endpoint, "cdn.tencentcloudapi.com"),
	}
}

// Refresh 刷新一组 URL（单次最多 1000 条，超出自动分批）
func (t *TencentCDNRefresher) Refresh(urls []string) error {
	if t.SecretID == "" || t.SecretKey == "" {
		return fmt.Errorf("腾讯云 CDN 未配置 SecretId / SecretKey")
	}
	return cdnBatch(urls, 1000, t.refreshOnce)
}

func (t *TencentCDNRefresher) refreshOnce(urls []string) error {
	payload, err := json.Marshal(map[string]interface{}{"Urls": urls})
	if err != nil {
		return fmt.Errorf("cdn refresh encode: %w", err)
	}
	host := hostOf(t.Endpoint)
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	ts := strconv.FormatInt(now.Unix(), 10)

	// 规范请求：参与签名的头只有 content-type 与 host（TC3 要求这两个必选）
	const signedHeaders = "content-type;host"
	canonical := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		"content-type:application/json",
		"host:" + host,
		"",
		signedHeaders,
		sha256Hex(payload),
	}, "\n")
	scope := date + "/" + tencentCDNService + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		ts,
		scope,
		sha256Hex([]byte(canonical)),
	}, "\n")

	// 派生密钥：TC3+SecretKey → date → service → tc3_request
	k := hmacSHA256([]byte("TC3"+t.SecretKey), date)
	k = hmacSHA256(k, tencentCDNService)
	k = hmacSHA256(k, "tc3_request")
	signature := hmacSHA256Hex(k, stringToSign)

	req, err := http.NewRequest(http.MethodPost, t.Endpoint+"/", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("cdn refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Host = host
	req.Header.Set("X-TC-Action", "PurgeUrlsCache")
	req.Header.Set("X-TC-Version", tencentCDNVersion)
	req.Header.Set("X-TC-Timestamp", ts)
	req.Header.Set("Authorization", fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		t.SecretID, scope, signedHeaders, signature))

	body, err := cdnDo(req)
	if err != nil {
		return err
	}
	// 成功：{"Response":{"RequestId":"...","TaskId":"..."}}
	// 失败：{"Response":{"Error":{"Code":"...","Message":"..."},"RequestId":"..."}}
	var r struct {
		Response struct {
			TaskID string `json:"TaskId"`
			Error  struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if json.Unmarshal(body, &r) == nil && r.Response.Error.Code != "" {
		return fmt.Errorf("cdn refresh failed: %s %s", r.Response.Error.Code, r.Response.Error.Message)
	}
	return nil
}
