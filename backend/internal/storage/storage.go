package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// Uploader 文件存储抽象：本地磁盘 / MinIO / OSS 实现同一接口即可互换（云托管可逆）
type Uploader interface {
	// Save 接收上传表单文件，返回可公开访问的 URL
	Save(fh *multipart.FileHeader) (string, error)
	// SaveBytes 保存程序生成的内容（如小程序码），relPath 为对象相对路径（如 "wxacode/12.jpg"），返回可公开访问的 URL
	SaveBytes(relPath string, data []byte, contentType string) (string, error)
}

// LocalUploader 本地磁盘实现（骨架默认），生产可换 MinIOUploader / OssUploader 不改业务代码。
// 本地驱动 URL 一律返回相对 /uploads/<name>，由各端按自身源拼绝对（见 Save/SaveBytes 注释）。
type LocalUploader struct {
	Dir      string          // 本地目录，如 ./uploads
	BaseURL  string          // 仅作兼容保留（云存储用）；本地驱动已不再参与 URL 生成
	MaxBytes int64           // 单文件上限（字节）
	AllowExt map[string]bool // 允许扩展名白名单（小写，不含点）
}

// DefaultAllowExt 图片 + mp4 默认白名单
var DefaultAllowExt = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true, "mp4": true,
}

// 运行时上限覆盖（优先取系统设置；未注入或返回 <=0 时用 LocalUploader.MaxBytes 兜底）。
// 由 cmd/server 与 devserver 在 setting.Init 后注入，实现「后台改上限即时生效、无需重启」。
// 这样上传限制不再写死在 main 的 10MB 常量里——它由 sys_setting.upload_max_mb 驱动。
var UploadLimitBytes func() int64

// AllowExtProvider 运行时扩展名白名单覆盖（优先取系统设置 upload_allow_ext）。
// 返回 nil 时 checkFile 退用传入的 LocalUploader.AllowExt（默认白名单）。
var AllowExtProvider func() map[string]bool

// Save 落盘并返回相对 URL（/uploads/<name>）。
// 故意返回相对路径而非绝对 localhost URL：各端按自身源拼绝对地址，
// 浏览器/管理后台经反向代理或 vite 代理解析，小程序经 assetUrl 拼 baseUrl，
// 实现「零配置多端可达」——避免把 127.0.0.1 写死进库导致手机端裂图。
func (l *LocalUploader) Save(fh *multipart.FileHeader) (string, error) {
	if err := checkFile(fh, l.MaxBytes, l.AllowExt); err != nil {
		return "", err
	}
	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fh.Filename), "."))
	name := RandName(ext)
	dst := filepath.Join(l.Dir, name)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		return "", err
	}
	return "/uploads/" + name, nil
}

// SaveBytes 写入本地并返回相对 URL（relPath 直接作为子路径，如 "wxacode/12.jpg"）。
// clean 已由下方 TrimPrefix 去首斜杠，拼 /uploads/ 前缀即与 r.Static("/uploads", Dir) 路由匹配。
func (l *LocalUploader) SaveBytes(relPath string, data []byte, _ string) (string, error) {
	clean := strings.TrimPrefix(relPath, "/")
	dst := filepath.Join(l.Dir, clean)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return "", err
	}
	return "/uploads/" + clean, nil
}

// checkFile 统一的大小 / 类型 / 内容嗅探校验（所有驱动共用）。
// 先校验扩展名白名单，再对常见图片/视频做文件头（魔法字节）嗅探，防止把
// 可执行文件重命名为图片扩展名后上传，规避存储/展示被滥用的风险。
func checkFile(fh *multipart.FileHeader, max int64, allow map[string]bool) error {
	// 运行时覆盖：优先取系统设置里的上传上限 / 扩展名白名单（后台改即生效）
	if UploadLimitBytes != nil {
		if m := UploadLimitBytes(); m > 0 {
			max = m
		}
	}
	if AllowExtProvider != nil {
		if a := AllowExtProvider(); a != nil {
			allow = a
		}
	}
	if max > 0 && fh.Size > max {
		return fmt.Errorf("文件过大，上限 %d MB", max/1024/1024)
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fh.Filename), "."))
	if allow != nil && ext != "" && !allow[ext] {
		return fmt.Errorf("不支持的文件类型: %s", ext)
	}
	// 内容嗅探：打开文件读取文件头（魔法字节）校验真实类型与扩展名一致
	src, err := fh.Open()
	if err != nil {
		return fmt.Errorf("读取上传文件失败: %w", err)
	}
	defer src.Close()
	buf := make([]byte, 512)
	n, _ := src.Read(buf)
	if !sniffExt(ext, buf[:n]) {
		return fmt.Errorf("文件内容与扩展名 %s 不匹配，疑似伪造", ext)
	}
	return nil
}

// sniffExt 校验真实文件头（head）与扩展名一致。返回 true 表示通过（或无嗅探规则的扩展名，仅依赖白名单）。
func sniffExt(ext string, head []byte) bool {
	switch ext {
	case "png":
		return bytes.HasPrefix(head, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	case "jpg", "jpeg":
		return bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF})
	case "gif":
		return bytes.HasPrefix(head, []byte("GIF8"))
	case "webp":
		return len(head) >= 12 && bytes.Equal(head[0:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP"))
	case "mp4":
		// 标准 MP4：前 4 字节为 box size，随后 4 字节为 "ftyp"
		return len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp"))
	default:
		// 无魔法字节规则的扩展名，仅依赖扩展名白名单
		return true
	}
}

// RandName 生成 16 字节随机名 + 扩展名（避免依赖 uuid），供各驱动与预签名场景复用
func RandName(ext string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	if ext == "" {
		return hex.EncodeToString(b)
	}
	return hex.EncodeToString(b) + "." + ext
}
