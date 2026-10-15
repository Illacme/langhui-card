package storage

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// makeFileHeader 用给定文件名与内容构造一个 *multipart.FileHeader，供 checkFile 校验。
// 复用与集成测试一致的 ParseMultipartForm 路径，避免直接 ReadForm 的边界问题。
func makeFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, _ = part.Write(content)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest("POST", "/", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(int64(len(content))+1024))
	return req.MultipartForm.File["file"][0]
}

func TestCheckFileSizeAndExt(t *testing.T) {
	allow := DefaultAllowExt

	t.Run("大小超限", func(t *testing.T) {
		fh := makeFileHeader(t, "a.png", []byte("\x89PNG\r\n\x1a\n"))
		require.Error(t, checkFile(fh, 1, allow)) // 上限 1 字节
	})

	t.Run("扩展名不在白名单", func(t *testing.T) {
		fh := makeFileHeader(t, "a.exe", []byte("MZ"))
		require.Error(t, checkFile(fh, 10<<20, allow))
	})

	t.Run("合法 PNG 通过大小/扩展名校验", func(t *testing.T) {
		fh := makeFileHeader(t, "a.png", []byte("\x89PNG\r\n\x1a\nsomedata"))
		require.NoError(t, checkFile(fh, 10<<20, allow))
	})
}

// TestSniffExt 直接对文件头字节做内容嗅探，避免 multipart 短内容解析的边界问题。
func TestSniffExt(t *testing.T) {
	cases := []struct {
		ext   string
		head  []byte
		match bool
	}{
		{"png", []byte("\x89PNG\r\n\x1a\nsomedata"), true},
		{"png", []byte("this is definitely not a png"), false},
		{"jpg", []byte("\xff\xd8\xff\x00some"), true},
		{"jpeg", []byte("\xff\xd8\xff"), true},
		{"gif", []byte("GIF89a"), true},
		{"gif", []byte("AGIFxx"), false},
		{"webp", []byte("RIFF\x1a\x00\x00\x00WEBP"), true},
		{"webp", []byte("RIFF\x1a\x00\x00\x00JPEG"), false},
		{"mp4", []byte("\x00\x00\x00\x1cftypmp42"), true},
		{"mp4", []byte("not an mp4 file at all"), false},
		// 无嗅探规则的扩展名一律放行（仅依赖白名单）
		{"xyz", []byte("anything"), true},
	}
	for _, c := range cases {
		require.Equalf(t, c.match, sniffExt(c.ext, c.head), "sniffExt(%q)", c.ext)
	}
}

func TestRandName(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		name := RandName("png")
		require.True(t, len(name) > 4 && name[len(name)-4:] == ".png")
		require.False(t, seen[name], "RandName 出现重复: %s", name)
		seen[name] = true
	}
	// 无扩展名时不应带点
	require.NotContains(t, RandName(""), ".")
}
