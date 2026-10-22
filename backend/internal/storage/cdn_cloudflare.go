package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CloudflareCDNRefresher Cloudflare CDN 缓存刷新（purge_cache by URL，纯标准库）。
//
// 凭据形态与 AK/SK 三家不同：Cloudflare 用一个 API Token（Bearer），另需 Zone ID
// 定位站点——所以通用配置里的「资源标识」对它是 Zone ID，对 CloudFront 是 Distribution ID。
//
// 单次最多 30 条 URL（Free/Pro/Business 档；Enterprise 为 500），超出自动分批。
type CloudflareCDNRefresher struct {
	APIToken string // Cloudflare API Token（对应通用配置里的 Secret）
	ZoneID   string // 站点 Zone ID（对应「资源标识」）
	Endpoint string // 默认 api.cloudflare.com
}

// NewCloudflareCDNRefresher 构造刷新器；endpoint 为空时默认 api.cloudflare.com
func NewCloudflareCDNRefresher(token, zoneID, endpoint string) *CloudflareCDNRefresher {
	return &CloudflareCDNRefresher{
		APIToken: token,
		ZoneID:   zoneID,
		Endpoint: cdnBaseURL(endpoint, "api.cloudflare.com"),
	}
}

// Refresh 刷新一组 URL（单次最多 30 条，超出自动分批）
func (c *CloudflareCDNRefresher) Refresh(urls []string) error {
	if c.APIToken == "" {
		return fmt.Errorf("Cloudflare CDN 未配置 API Token")
	}
	if c.ZoneID == "" {
		return fmt.Errorf("Cloudflare CDN 未配置 Zone ID（资源标识）")
	}
	return cdnBatch(urls, 30, c.refreshOnce)
}

func (c *CloudflareCDNRefresher) refreshOnce(urls []string) error {
	payload, err := json.Marshal(map[string]interface{}{"files": urls})
	if err != nil {
		return fmt.Errorf("cdn refresh encode: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost,
		c.Endpoint+"/client/v4/zones/"+url.PathEscape(c.ZoneID)+"/purge_cache",
		bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cdn refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIToken)

	body, err := cdnDo(req)
	if err != nil {
		return err
	}
	// 成功：{"success":true,...,"result":{"id":"..."}}
	// 失败：{"success":false,"errors":[{"code":..,"message":".."}]}
	var r struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &r) == nil && !r.Success {
		msg := "unknown error"
		if len(r.Errors) > 0 && r.Errors[0].Message != "" {
			msg = r.Errors[0].Message
		}
		return fmt.Errorf("cdn refresh failed: %s", msg)
	}
	return nil
}
