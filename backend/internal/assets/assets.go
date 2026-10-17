// Package assets 内嵌「常用开源素材库」：PC 后台所有需要图片的位置
// （分类图标 / 服务图标 / 项目封面 / 相册 / 公司简介封面 / 公司 LOGO）
// 都可以直接选内置素材，免去四处找图、按尺寸处理图。
//
// 素材由同目录 gen_library.py 生成（Lucide ISC 图标 + 程序绘制封面/LOGO），
// 通过 go:embed 编译进二进制——单文件部署（Dockerfile 只 COPY 二进制）无需额外挂载。
// 重新生成/扩充素材：改 gen_library.py 后运行，再提交 library/ 目录。
package assets

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// libraryFS 内嵌整个 library/ 目录（manifest.json + CREDITS.md + 各分类素材）。
// all: 前缀保证以 . / _ 开头的文件也纳入（当前没有，防御性写法）。
//
//go:embed all:library
var libraryFS embed.FS

// Category 素材分类（选择器顶部 tab）
type Category struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Item 单个素材项
type Item struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"` // icon / cover / logo
	File     string   `json:"file"`     // library 内相对路径，如 icons/icon-house.png
	Thumb    string   `json:"thumb"`    // 预览图路径（缩略版，图标/LOGO 与 File 相同）
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Tags     []string `json:"tags"` // 搜索标签（中文名/别名）
}

// Manifest 素材清单（gen_library.py 生成）
type Manifest struct {
	Version    int        `json:"version"`
	Categories []Category `json:"categories"`
	Items      []Item     `json:"items"`
}

var (
	loadOnce sync.Once
	manifest *Manifest
	loadErr  error
)

// Load 读取并缓存内嵌 manifest.json（首次调用解析，之后零成本）
func Load() (*Manifest, error) {
	loadOnce.Do(func() {
		data, err := libraryFS.ReadFile("library/manifest.json")
		if err != nil {
			loadErr = fmt.Errorf("读取素材清单失败: %w", err)
			return
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			loadErr = fmt.Errorf("解析素材清单失败: %w", err)
			return
		}
		if len(m.Items) == 0 {
			loadErr = fmt.Errorf("素材清单为空")
			return
		}
		manifest = &m
	})
	return manifest, loadErr
}

// Find 按素材 ID 查找（不存在返回 error）
func Find(id string) (*Item, error) {
	m, err := Load()
	if err != nil {
		return nil, err
	}
	for i := range m.Items {
		if m.Items[i].ID == id {
			return &m.Items[i], nil
		}
	}
	return nil, fmt.Errorf("素材不存在: %s", id)
}

// ContentType 按扩展名推断素材 MIME（采用时写入存储用）
func ContentType(rel string) string {
	switch strings.ToLower(rel[strings.LastIndex(rel, ".")+1:]) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

// ReadFile 读取 library 内素材字节（rel 为 Item.File / Item.Thumb）。
// 安全：io/fs 语义拒绝含 ".." 的路径，embed 根外的文件不可达。
func ReadFile(rel string) ([]byte, error) {
	if rel == "" || strings.Contains(rel, "..") {
		return nil, os.ErrNotExist
	}
	return libraryFS.ReadFile("library/" + strings.TrimPrefix(rel, "/"))
}
