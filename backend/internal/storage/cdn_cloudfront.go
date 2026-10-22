package storage

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// cloudFrontAPIVersion CloudFront REST API 版本（路径里带版本号）
const cloudFrontAPIVersion = "2020-05-31"

// CloudFrontCDNRefresher AWS CloudFront 缓存失效（CreateInvalidation + SigV4，纯标准库）。
//
// 与另外三家不同：CloudFront 提交的是**失效路径**而不是完整 URL，故这里把 URL 的
// path 部分摘出来（query 不参与失效匹配）。默认每批上限按官方默认配额取 1000 条
// （实际单批最多 3000 条，且前 1000 条/月免费），宁可少打也不触发 BatchTooLarge。
type CloudFrontCDNRefresher struct {
	AccessKeyID     string
	SecretAccessKey string
	DistributionID  string // 分发 ID（对应「资源标识」）
	Endpoint        string // 默认 cloudfront.amazonaws.com
	Region          string // 默认 us-east-1：CloudFront 是全局服务，签名地域固定
}

// NewCloudFrontCDNRefresher 构造刷新器；endpoint / region 为空时取默认值
func NewCloudFrontCDNRefresher(ak, sk, distributionID, endpoint, region string) *CloudFrontCDNRefresher {
	if region == "" {
		region = "us-east-1"
	}
	return &CloudFrontCDNRefresher{
		AccessKeyID:     ak,
		SecretAccessKey: sk,
		DistributionID:  distributionID,
		Endpoint:        cdnBaseURL(endpoint, "cloudfront.amazonaws.com"),
		Region:          region,
	}
}

// Refresh 刷新一组 URL（提交的是 URL 的 path；单批最多 1000 条）
func (c *CloudFrontCDNRefresher) Refresh(urls []string) error {
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return fmt.Errorf("CloudFront 未配置 AccessKeyId / SecretAccessKey")
	}
	if c.DistributionID == "" {
		return fmt.Errorf("CloudFront 未配置 Distribution ID（资源标识）")
	}
	// 完整 URL → 失效路径（CloudFront 只认 path；同一批去重，省配额也避免 Quantity 虚高）
	paths := make([]string, 0, len(urls))
	seen := map[string]bool{}
	for _, u := range urls {
		p := cloudFrontPath(u)
		if seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return cdnBatch(paths, 1000, c.refreshOnce)
}

func (c *CloudFrontCDNRefresher) refreshOnce(paths []string) error {
	payload := []byte(cloudFrontInvalidationXML(paths))
	host := hostOf(c.Endpoint)
	canonURI := "/" + cloudFrontAPIVersion + "/distribution/" + awsURIEncode(c.DistributionID) + "/invalidation"
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")

	// 参与签名的头与官方 SDK（botocore）对 CloudFront 的处理保持一致：
	// content-type;host;x-amz-date。x-amz-content-sha256 只有 S3 要求带，
	// 非 S3 服务带上反而要自己保证与服务端一致，不如跟着官方走。
	sig := awsSignV4(awsSignV4Input{
		AccessKeyID:     c.AccessKeyID,
		SecretAccessKey: c.SecretAccessKey,
		Region:          c.Region,
		Service:         "cloudfront",
		Method:          http.MethodPost,
		CanonicalURI:    canonURI,
		Headers: map[string]string{
			"content-type": "application/xml",
			"host":         host,
			"x-amz-date":   amzDate,
		},
		Payload: payload,
		Time:    now,
	})

	req, err := http.NewRequest(http.MethodPost, c.Endpoint+canonURI, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cdn refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/xml")
	req.Host = host
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("Authorization", sig.Authorization)

	// 成功返回 201 Created（也接受其它 2xx），失败时 cdnDo 已带上状态码与响应体
	_, err = cdnDo(req)
	return err
}

// cloudFrontPath 从完整 URL 取失效路径；解析失败或空路径回退 "/"。
// CloudFront 的失效只按 path 匹配（不含协议/host/query），与另外三家提交整条 URL 不同。
func cloudFrontPath(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		return u.Path
	}
	return "/"
}

// cloudFrontInvalidationXML 组装 InvalidationBatch 请求体。
// CallerReference 每次必须不同：CloudFront 用它做幂等去重，相同的
// CallerReference + 不同 Paths 会直接返回 InvalidationBatchAlreadyExists。
func cloudFrontInvalidationXML(paths []string) string {
	var b strings.Builder
	b.WriteString(`<InvalidationBatch xmlns="http://cloudfront.amazonaws.com/doc/2020-05-31/">`)
	b.WriteString(`<CallerReference>weichuan-`)
	b.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
	b.WriteString(`</CallerReference><Paths><Quantity>`)
	b.WriteString(strconv.Itoa(len(paths)))
	b.WriteString(`</Quantity><Items>`)
	for _, p := range paths {
		b.WriteString(`<Path>`)
		_ = xml.EscapeText(&b, []byte(p)) // 路径里理论上只有 URL 安全字符，仍按 XML 规则转义
		b.WriteString(`</Path>`)
	}
	b.WriteString(`</Items></Paths></InvalidationBatch>`)
	return b.String()
}
