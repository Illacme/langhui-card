package main

import (
	"testing"
)

func TestUploadBaseURL(t *testing.T) {
	t.Setenv("DEV_UPLOAD_BASE_URL", "")
	if got := uploadBaseURL("8080"); got != "http://127.0.0.1:8080/uploads" {
		t.Fatalf("未设置时应回退本机默认值，got %s", got)
	}

	// 覆盖为局域网地址（末尾斜杠应被裁剪，避免 URL 出现双斜杠）
	t.Setenv("DEV_UPLOAD_BASE_URL", "http://192.168.1.10:8080/uploads/")
	if got := uploadBaseURL("8080"); got != "http://192.168.1.10:8080/uploads" {
		t.Fatalf("设置后应使用覆盖值并裁剪末尾斜杠，got %s", got)
	}

	// 仅空白字符等同于未设置
	t.Setenv("DEV_UPLOAD_BASE_URL", "   ")
	if got := uploadBaseURL("9090"); got != "http://127.0.0.1:9090/uploads" {
		t.Fatalf("空白值应回退默认值，got %s", got)
	}
}
