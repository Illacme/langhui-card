package storage

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- 假 CDN 服务端：记录收到的请求，便于断言「签名头 / 路径 / 请求体」 ----

type capturedReq struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

type fakeCDNServer struct {
	mu     sync.Mutex
	reqs   []capturedReq
	status int    // 0 → 200
	body   string // 响应体
}

func (f *fakeCDNServer) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, capturedReq{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(),
			Header: r.Header.Clone(), Body: b,
		})
		f.mu.Unlock()
		code := f.status
		if code == 0 {
			code = http.StatusOK
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeCDNServer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func (f *fakeCDNServer) last(t *testing.T) capturedReq {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(t, f.reqs, "应至少收到一次请求")
	return f.reqs[len(f.reqs)-1]
}

// ---- SigV4：拿 AWS 官方公布的向量验证，签名写错在服务端只会得到一句 SignatureDoesNotMatch ----

// TestAWSSignV4KnownVector 用两个可独立复核的基准验证 SigV4：
//
//  1. 派生密钥：AWS 文档「Deriving the signing key」给出的中间向量
//     （20120215 / us-east-1 / iam，密钥 wJalrXUt...EXAMPLEKEY）；
//  2. 完整 Authorization：**与 AWS 官方 SDK botocore 1.43.117 对同一个
//     CloudFront 失效请求产出的签名逐字一致**（黄金值见下）。
//
// 为什么用官方向量而不是自己再算一遍：自算只能证明「实现与实现一致」。
// 而且踩过一次——最初凭记忆写的期望签名是错的（把别家文档的示例串记混了），
// 靠 botocore 交叉验证才发现。签名错了服务端只回一句 SignatureDoesNotMatch，
// 本地无从定位，所以这个用例必须锚在外部事实上。
func TestAWSSignV4KnownVector(t *testing.T) {
	const secret = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"

	// 1) 派生密钥向量
	k := hmacSHA256([]byte("AWS4"+secret), "20120215")
	k = hmacSHA256(k, "us-east-1")
	k = hmacSHA256(k, "iam")
	k = hmacSHA256(k, "aws4_request")
	require.Equal(t,
		"f4780e2d9f65fa895f9c67b32ce1baf0b0d8a43505a000a1a9e090d414db404d",
		hexOf(k))

	// 2) 与 botocore 同输入同输出（固定时间戳，保证可复现）
	payload := []byte(`<InvalidationBatch xmlns="http://cloudfront.amazonaws.com/doc/2020-05-31/">` +
		`<CallerReference>weichuan-1700000000000000000</CallerReference>` +
		`<Paths><Quantity>1</Quantity><Items><Path>/uploads/a.png</Path></Items></Paths>` +
		`</InvalidationBatch>`)
	ts, err := time.Parse("20060102T150405Z", "20261022T195538Z")
	require.NoError(t, err)
	got := awsSignV4(awsSignV4Input{
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: secret,
		Region:          "us-east-1",
		Service:         "cloudfront",
		Method:          http.MethodPost,
		CanonicalURI:    "/2020-05-31/distribution/DIST123/invalidation",
		Headers: map[string]string{
			"content-type": "application/xml",
			"host":         "cloudfront.amazonaws.com",
			"x-amz-date":   "20261022T195538Z",
		},
		Payload: payload,
		Time:    ts,
	})
	require.Equal(t, "content-type;host;x-amz-date", got.SignedHeaders,
		"与官方 SDK 一致：非 S3 服务不签 x-amz-content-sha256")
	require.Equal(t, "20261022T195538Z", got.AmzDate)
	require.Equal(t, "ed1908a25baf525f488104fb73a9c142f291ad3a2353133adc1a465674b087dc", got.PayloadHash)
	require.Equal(t,
		"AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261022/us-east-1/cloudfront/aws4_request, "+
			"SignedHeaders=content-type;host;x-amz-date, "+
			"Signature=90b84ec82a2add8cc90d5092cc1012a1654fbc65b26c19f06a503773442b9714",
		got.Authorization)
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0x0f])
	}
	return string(out)
}

// ---- 阿里云（既有实现，重构后补上用例）----

func TestAliyunCDNRefresh(t *testing.T) {
	fake := &fakeCDNServer{body: `{"RequestId":"r1"}`}
	srv := fake.server(t)
	up := NewAliyunCDNRefresher("ak", "sk", srv.URL)
	require.NoError(t, up.Refresh([]string{"https://c.example.com/a.jpg", "https://c.example.com/b.jpg"}))
	require.Equal(t, 1, fake.count())
	r := fake.last(t)
	require.Equal(t, http.MethodGet, r.Method)
	require.Equal(t, "RefreshObjectCaches", r.Query.Get("Action"))
	require.Equal(t, "2014-11-11", r.Query.Get("Version"))
	require.Equal(t, "https://c.example.com/a.jpg\nhttps://c.example.com/b.jpg", r.Query.Get("ObjectPath"))
	require.NotEmpty(t, r.Query.Get("Signature"))

	// 空列表不发请求
	fake.reqs = nil
	require.NoError(t, up.Refresh(nil))
	require.Zero(t, fake.count())

	// 厂商返回业务错误 → 报错（调用方只记日志）
	fake.body = `{"Code":"InvalidAccessKeyId","Message":"bad key"}`
	require.ErrorContains(t, up.Refresh([]string{"https://c.example.com/a.jpg"}), "InvalidAccessKeyId")
}

// ---- 腾讯云 ----

func TestTencentCDNRefresh(t *testing.T) {
	fake := &fakeCDNServer{body: `{"Response":{"TaskId":"t1","RequestId":"r1"}}`}
	srv := fake.server(t)
	up := NewTencentCDNRefresher("secret-id", "secret-key", srv.URL)
	require.NoError(t, up.Refresh([]string{"https://c.example.com/a.jpg"}))
	require.Equal(t, 1, fake.count())

	r := fake.last(t)
	require.Equal(t, http.MethodPost, r.Method)
	require.Equal(t, "/", r.Path)
	require.Equal(t, "PurgeUrlsCache", r.Header.Get("X-TC-Action"))
	require.Equal(t, "2018-06-06", r.Header.Get("X-TC-Version"), "CDN 的 API 版本是 2018-06-06")
	require.NotEmpty(t, r.Header.Get("X-TC-Timestamp"))
	auth := r.Header.Get("Authorization")
	require.Contains(t, auth, "TC3-HMAC-SHA256 Credential=secret-id/")
	require.Contains(t, auth, "/cdn/tc3_request", "CDN 是全局服务：scope 里没有地域段")
	require.Contains(t, auth, "SignedHeaders=content-type;host")
	var body struct {
		Urls []string `json:"Urls"`
	}
	require.NoError(t, json.Unmarshal(r.Body, &body))
	require.Equal(t, []string{"https://c.example.com/a.jpg"}, body.Urls)

	// 缺凭据 → 直接报错，不发请求
	fake.reqs = nil
	require.Error(t, (&TencentCDNRefresher{}).Refresh([]string{"https://c/x.jpg"}))
	require.Zero(t, fake.count())

	// 厂商返回业务错误
	fake.body = `{"Response":{"Error":{"Code":"AuthFailure.InvalidAuthorization","Message":"bad"},"RequestId":"r"}}`
	require.ErrorContains(t, up.Refresh([]string{"https://c/x.jpg"}), "AuthFailure.InvalidAuthorization")

	// HTTP 5xx
	fake.body, fake.status = "boom", http.StatusInternalServerError
	require.ErrorContains(t, up.Refresh([]string{"https://c/x.jpg"}), "cdn refresh http 500")
}

// ---- 七牛云 ----

func TestQiniuCDNRefresh(t *testing.T) {
	fake := &fakeCDNServer{body: `{"code":200,"taskIds":{"a":"b"}}`}
	srv := fake.server(t)
	up := NewQiniuCDNRefresher("ak", "sk", srv.URL)
	require.NoError(t, up.Refresh([]string{"https://c.example.com/a.jpg"}))
	require.Equal(t, 1, fake.count())

	r := fake.last(t)
	require.Equal(t, http.MethodPost, r.Method)
	require.Equal(t, "/v2/tune/refresh", r.Path)
	// QBox 签名只拼「路径 + 换行」，JSON 体不参与签名（七牛官方鉴权指南明确）
	mac := hmac.New(sha1.New, []byte("sk"))
	mac.Write([]byte("/v2/tune/refresh\n"))
	require.Equal(t, "QBox ak:"+base64.URLEncoding.EncodeToString(mac.Sum(nil)), r.Header.Get("Authorization"))
	var body struct {
		Urls []string `json:"urls"`
	}
	require.NoError(t, json.Unmarshal(r.Body, &body))
	require.Equal(t, []string{"https://c.example.com/a.jpg"}, body.Urls)

	// 单次上限 20 条：25 条应分两批（20 + 5）
	fake.reqs = nil
	urls := make([]string, 25)
	for i := range urls {
		urls[i] = "https://c.example.com/" + string(rune('a'+i)) + ".jpg"
	}
	require.NoError(t, up.Refresh(urls))
	require.Equal(t, 2, fake.count(), "七牛单次上限 20 条")
	require.Len(t, mustURLs(t, fake.reqs[0].Body), 20)
	require.Len(t, mustURLs(t, fake.reqs[1].Body), 5)

	// 配额/参数错误码
	fake.body = `{"code":400034,"error":"url count exceeds limit"}`
	require.ErrorContains(t, up.Refresh([]string{"https://c/x.jpg"}), "400034")
}

func mustURLs(t *testing.T, body []byte) []string {
	t.Helper()
	var m struct {
		Urls []string `json:"urls"`
	}
	require.NoError(t, json.Unmarshal(body, &m))
	return m.Urls
}

// ---- Cloudflare ----

func TestCloudflareCDNRefresh(t *testing.T) {
	fake := &fakeCDNServer{body: `{"success":true,"errors":[],"result":{"id":"p1"}}`}
	srv := fake.server(t)
	up := NewCloudflareCDNRefresher("cf-token", "zone-123", srv.URL)
	require.NoError(t, up.Refresh([]string{"https://c.example.com/a.jpg"}))
	require.Equal(t, 1, fake.count())

	r := fake.last(t)
	require.Equal(t, http.MethodPost, r.Method)
	require.Equal(t, "/client/v4/zones/zone-123/purge_cache", r.Path)
	require.Equal(t, "Bearer cf-token", r.Header.Get("Authorization"))
	var body struct {
		Files []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal(r.Body, &body))
	require.Equal(t, []string{"https://c.example.com/a.jpg"}, body.Files)

	// 缺 Zone ID / Token：Cloudflare 靠这两个定位站点与鉴权，缺一不可
	require.ErrorContains(t, (&CloudflareCDNRefresher{APIToken: "t"}).Refresh([]string{"https://c/x"}), "Zone ID")
	require.ErrorContains(t, (&CloudflareCDNRefresher{ZoneID: "z"}).Refresh([]string{"https://c/x"}), "API Token")

	// 单次上限 30 条：31 条分两批
	fake.reqs = nil
	urls := make([]string, 31)
	for i := range urls {
		urls[i] = "https://c.example.com/" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + ".jpg"
	}
	require.NoError(t, up.Refresh(urls))
	require.Equal(t, 2, fake.count(), "Cloudflare 单次上限 30 条")

	// success=false → 报错
	fake.body = `{"success":false,"errors":[{"code":1000,"message":"no such zone"}]}`
	require.ErrorContains(t, up.Refresh([]string{"https://c/x.jpg"}), "no such zone")
}

// ---- AWS CloudFront ----

func TestCloudFrontCDNRefresh(t *testing.T) {
	fake := &fakeCDNServer{body: `<?xml version="1.0"?><Invalidation><Id>I1</Id><Status>InProgress</Status></Invalidation>`, status: http.StatusCreated}
	srv := fake.server(t)
	up := NewCloudFrontCDNRefresher("akid", "secret", "DIST123", srv.URL, "")
	// CloudFront 只认 path：host 与 query 都要剥掉
	require.NoError(t, up.Refresh([]string{"https://cdn.example.com/uploads/a.png?v=1"}))
	require.Equal(t, 1, fake.count())

	r := fake.last(t)
	require.Equal(t, http.MethodPost, r.Method)
	require.Equal(t, "/2020-05-31/distribution/DIST123/invalidation", r.Path)
	auth := r.Header.Get("Authorization")
	require.Contains(t, auth, "AWS4-HMAC-SHA256 Credential=akid/")
	require.Contains(t, auth, "/us-east-1/cloudfront/aws4_request", "CloudFront 签名地域固定 us-east-1")
	require.Contains(t, auth, "SignedHeaders=content-type;host;x-amz-date",
		"与官方 SDK 一致：非 S3 服务不签 x-amz-content-sha256")
	require.NotEmpty(t, r.Header.Get("X-Amz-Date"))

	bodyStr := string(r.Body)
	require.Contains(t, bodyStr, "<Path>/uploads/a.png</Path>", "只提交 path，不含 host 与 query")
	require.Contains(t, bodyStr, "<Quantity>1</Quantity>")
	require.Contains(t, bodyStr, "<CallerReference>weichuan-")

	// CallerReference 每次必须不同：CloudFront 用它做幂等（相同值 + 不同 path 会报 AlreadyExists）
	require.NoError(t, up.Refresh([]string{"https://cdn.example.com/uploads/b.png"}))
	require.NotEqual(t, callerRefOf(t, string(fake.reqs[0].Body)),
		callerRefOf(t, string(fake.reqs[1].Body)), "CallerReference 应每次不同")

	// 缺 Distribution ID 或凭据
	require.ErrorContains(t, (&CloudFrontCDNRefresher{AccessKeyID: "a", SecretAccessKey: "b"}).Refresh([]string{"https://c/x"}), "Distribution ID")
	require.ErrorContains(t, (&CloudFrontCDNRefresher{DistributionID: "D"}).Refresh([]string{"https://c/x"}), "AccessKeyId")
}

func callerRefOf(t *testing.T, xmlBody string) string {
	t.Helper()
	const open, close = "<CallerReference>", "</CallerReference>"
	s := strings.Index(xmlBody, open)
	e := strings.Index(xmlBody, close)
	require.True(t, s >= 0 && e > s, "请求体应含 CallerReference：%s", xmlBody)
	return xmlBody[s+len(open) : e]
}

// ---- 厂商选择与装配 ----

func TestNewCDNRefresherProviders(t *testing.T) {
	base := CDNConfig{AccessKeyID: "id", AccessKeySecret: "secret", ResourceID: "res"}

	for provider, want := range map[string]string{
		CDNProviderAliyun:     "*storage.AliyunCDNRefresher",
		CDNProviderTencent:    "*storage.TencentCDNRefresher",
		CDNProviderQiniu:      "*storage.QiniuCDNRefresher",
		CDNProviderCloudflare: "*storage.CloudflareCDNRefresher",
		CDNProviderCloudFront: "*storage.CloudFrontCDNRefresher",
		"":                    "*storage.AliyunCDNRefresher", // 留空=阿里云（兼容既有部署）
		"ALIYUN":              "*storage.AliyunCDNRefresher", // 大小写容错
	} {
		cfg := base
		cfg.Provider = provider
		got, err := NewCDNRefresher(cfg)
		require.NoError(t, err, "provider=%q", provider)
		require.Equal(t, want, fmt.Sprintf("%T", got), "provider=%q", provider)
	}

	// 未知厂商 / 缺字段 → 报错但绝不 panic（调用方只记日志、不启用刷新）
	_, err := NewCDNRefresher(CDNConfig{Provider: "akamai"})
	require.ErrorContains(t, err, "未知 CDN 厂商")
	_, err = NewCDNRefresher(CDNConfig{Provider: CDNProviderTencent})
	require.ErrorContains(t, err, "腾讯云 CDN 需要填")
	_, err = NewCDNRefresher(CDNConfig{Provider: CDNProviderQiniu})
	require.ErrorContains(t, err, "七牛云 CDN 需要填")
	_, err = NewCDNRefresher(CDNConfig{Provider: CDNProviderCloudflare, AccessKeySecret: "tok"})
	require.ErrorContains(t, err, "Zone ID")
	_, err = NewCDNRefresher(CDNConfig{Provider: CDNProviderCloudFront, AccessKeyID: "a", AccessKeySecret: "b"})
	require.ErrorContains(t, err, "Distribution ID")

	// Cloudflare 不需要 AK（只用 Token），AK/SK 三家不需要资源标识
	require.False(t, CDNProviderNeedsKeyPair(CDNProviderCloudflare))
	require.True(t, CDNProviderNeedsKeyPair(CDNProviderAliyun))
	require.True(t, CDNProviderNeedsResource(CDNProviderCloudflare))
	require.True(t, CDNProviderNeedsResource(CDNProviderCloudFront))
	require.False(t, CDNProviderNeedsResource(CDNProviderTencent))
}

func TestCDNBatch(t *testing.T) {
	var got [][]string
	require.NoError(t, cdnBatch([]string{"a", "b", "c"}, 2, func(batch []string) error {
		got = append(got, batch)
		return nil
	}))
	require.Equal(t, [][]string{{"a", "b"}, {"c"}}, got)

	// 任一批失败立即返回，不再打后续批次（省厂商配额）
	calls := 0
	require.Error(t, cdnBatch([]string{"a", "b", "c"}, 1, func([]string) error {
		calls++
		return errStop
	}))
	require.Equal(t, 1, calls)

	// 空列表不调用
	require.NoError(t, cdnBatch(nil, 10, func([]string) error {
		t.Fatal("空列表不应调用")
		return nil
	}))
}

func TestCDNBaseURL(t *testing.T) {
	// 裸域名补 https（生产形态）
	require.Equal(t, "https://cdn.aliyuncs.com", cdnBaseURL("cdn.aliyuncs.com", "x"))
	require.Equal(t, "https://fallback.example.com", cdnBaseURL("", "fallback.example.com"))
	// 带 scheme 原样保留（单测据此指向 httptest）
	require.Equal(t, "http://127.0.0.1:8080", cdnBaseURL("http://127.0.0.1:8080/", "x"))
}

var errStop = &cdnStopErr{}

type cdnStopErr struct{}

func (*cdnStopErr) Error() string { return "stop" }
