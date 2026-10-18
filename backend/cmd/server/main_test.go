package main

import (
	"testing"

	"langhui/internal/config"
	"langhui/internal/storage"

	"github.com/stretchr/testify/require"
)

// TestNewUploaderFallsBackToLocalOnBadConfig 云存储凭据在后台填错时，服务必须还能起来。
//
// 这是「改错可自愈」的底线：若构造失败就 Fatal，管理员连后台都进不去，
// 也就永远改不回来——只能手工改库或改环境变量，等于把人锁在系统外。
func TestNewUploaderFallsBackToLocalOnBadConfig(t *testing.T) {
	// oss 驱动但端点为空 → NewOssUploader 必然失败
	cfg := config.Config{StorageDriver: "oss", UploadDir: t.TempDir()}
	up := newUploaderWithFallback(cfg)

	_, ok := up.(*storage.LocalUploader)
	require.True(t, ok, "构造失败应降级为本地存储，而不是让进程退出")
}

// TestNewUploaderUsesConfiguredDriver 正常配置下不走降级路径（保证降级只在出错时发生）。
func TestNewUploaderUsesConfiguredDriver(t *testing.T) {
	cfg := config.Config{StorageDriver: "local", UploadDir: t.TempDir()}
	up := newUploaderWithFallback(cfg)

	local, ok := up.(*storage.LocalUploader)
	require.True(t, ok)
	require.Equal(t, cfg.UploadDir, local.Dir)
}
