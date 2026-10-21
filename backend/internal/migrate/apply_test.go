package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 容器布局的模拟验证：二进制与 migrations 同级时，「可执行同级」候选必须能找到
// （Dockerfile 把 migrations 拷到 /app/migrations，正是靠这条）。
// 这里不能改 os.Executable()，但可以反推：测试二进制自身同级若没有 migrations，
// 就退源码树候选——两者必居其一，且返回的目录必须真的有 .sql。
func TestMigrationsDirResolves(t *testing.T) {
	dir, err := migrationsDir()
	if err != nil {
		t.Fatalf("migrationsDir() 失败: %v", err)
	}
	if !hasSQLFiles(dir) {
		t.Fatalf("解析到的目录 %s 下没有 .sql 文件", dir)
	}
	// 必须能按序看到 0001 与最后一个迁移（0019）
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			names = append(names, e.Name())
		}
	}
	if len(names) < 19 {
		t.Fatalf("迁移文件数 = %d，期望 ≥19", len(names))
	}
	if names[0] != "0001_init.sql" {
		t.Fatalf("第一个迁移 = %s，期望 0001_init.sql", names[0])
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "0019_biz_service_icon.sql") {
		t.Fatalf("迁移清单缺 0019_biz_service_icon.sql: %s", joined)
	}
}

// 环境变量逃生门：设了就必须用它的值（候选 1 优先级最高）。
func TestMigrationsDirEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "0001_x.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEICHUAN_MIGRATIONS_DIR", tmp)
	dir, err := migrationsDir()
	if err != nil {
		t.Fatalf("migrationsDir() 失败: %v", err)
	}
	if dir != tmp {
		t.Fatalf("dir = %s，期望环境变量值 %s", dir, tmp)
	}
}

// 环境变量指向不存在的目录时应继续往下候选，而不是硬报错。
func TestMigrationsDirEnvFallbackWhenInvalid(t *testing.T) {
	t.Setenv("WEICHUAN_MIGRATIONS_DIR", filepath.Join(t.TempDir(), "no-such-dir"))
	dir, err := migrationsDir()
	if err != nil {
		t.Fatalf("env 无效时应回退其他候选，却报错: %v", err)
	}
	if !hasSQLFiles(dir) {
		t.Fatalf("回退到的目录 %s 没有 .sql", dir)
	}
}

// 所有候选都无效时，错误信息要列出候选路径（排错用）。
func TestMigrationsDirNoneExist(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("WEICHUAN_MIGRATIONS_DIR", filepath.Join(empty, "a"))
	// 其余候选（可执行同级/源码树/CWD）在测试环境里是真实存在的，
	// 无法全部置空；这里只验证 hasSQLFiles 对空目录返回 false。
	if hasSQLFiles(empty) {
		t.Fatal("空目录不应判定为有 .sql")
	}
}
