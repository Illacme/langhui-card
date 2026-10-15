package storage

import "time"

// Presigner 直传预签名能力：仅对象存储（MinIO / OSS）实现；本地磁盘无此能力。
// 客户端先向后端要一个预签名 PUT URL，再直接把文件 PUT 到对象存储，绕开应用服务器，降低带宽与存储压力。
type Presigner interface {
	// PresignPut 为对象 key 生成预签名 PUT URL（putURL）与最终可访问 URL（finalURL）。
	// contentType 用于 OSS 签名绑定（MinIO 忽略），调用方 PUT 时需带上相同 Content-Type。
	PresignPut(key, contentType string, expiry time.Duration) (putURL, finalURL string, err error)
}

// Signer 限时签名访问能力（私有桶用）：根据对象 key 生成临时可访问的 GET URL。
type Signer interface {
	// SignGetURL 为对象 key 生成有效期 expiry 的临时访问 URL。
	SignGetURL(key string, expiry time.Duration) (string, error)
}

// CDNRefresher CDN 缓存刷新能力：文件覆盖后使边缘节点旧缓存失效（避免回源拿到陈旧内容）。
type CDNRefresher interface {
	Refresh(urls []string) error
}
