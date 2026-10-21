// Package migrate 按序应用 backend/migrations/*.sql（MySQL 方言）。
//
// 两个使用方：
//   - 安装向导（internal/setup/install.go）的 MySQL 分支：表结构归迁移脚本管，
//     与 cmd/server 对 MySQL 不跑 AutoMigrate 的策略一致；
//   - 测试（handler_test 的 MySQL parity）：与生产同一条代码路径。
//
// 幂等性：建表用 IF NOT EXISTS；对已存在的对象，ALTER/INSERT 会报
// "Duplicate column" / "already exists" / "Duplicate entry"（1060/1050/1062），
// 这些错误一律忽略——与 docs/deploy-runbook.md §4「存量数据库手动应用迁移」
// 的容错说明一致。因此对「compose initdb.d 已应用过」的库重复执行也安全。
//
// 为什么切分语句只按 ";"：migrations 里只有 DDL/DML，无存储过程 / 触发器 /
// 含分号的字符串字面量（企业名等中文数据不含分号）。若将来加入这类语句，
// 必须换成真正的 SQL 解析器，不能继续 naive split。
package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// ignoredErrs 已应用过的迁移语句报的错（见包注释）：
//   1060 Duplicate column name  —— ALTER ADD COLUMN 列已存在
//   1050 Table already exists   —— CREATE TABLE（有 IF NOT EXISTS，双保险）
//   1061 Duplicate key name     —— ALTER ADD INDEX 索引已存在
//   1062 Duplicate entry        —— 固定主键的种子 INSERT 重复执行
var ignoredErrs = []string{"Duplicate column", "already exists", "Duplicate key name", "Duplicate entry"}

// migrationsDir 定位 migrations 目录：按优先级逐个候选，第一个「存在且有 .sql」的胜出。
//  1. 环境变量 WEICHUAN_MIGRATIONS_DIR（运维显式覆盖的逃生门）；
//  2. 可执行文件同级的 migrations/（容器布局：/app/server + /app/migrations；
//     -trimpath 构建时 runtime.Caller 只剩相对路径，也只有这条能找到）；
//  3. runtime.Caller 推导的源码树路径（dev / go test：源码树在，编译期绝对路径可用）；
//  4. CWD 下的 migrations/（在 backend/ 目录里直接跑二进制）。
// 背景：曾只用 runtime.Caller，容器里它返回**编译期**路径 /src/migrations（运行期不存在），
// MySQL 形态安装直接报「读取 migrations 目录失败」——「从未跑起来」的又一形态。
func migrationsDir() (string, error) {
	var cands []string
	if v := strings.TrimSpace(os.Getenv("WEICHUAN_MIGRATIONS_DIR")); v != "" {
		cands = append(cands, v)
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), "migrations"))
	}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		cands = append(cands, filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations"))
	}
	cands = append(cands, "migrations")
	for _, d := range cands {
		if hasSQLFiles(d) {
			return d, nil
		}
	}
	return "", fmt.Errorf("找不到 migrations 目录（候选过：%s）", strings.Join(cands, " | "))
}

func hasSQLFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			return true
		}
	}
	return false
}

// Apply 按文件名顺序执行 migrations 目录下全部 *.sql。
func Apply(db *gorm.DB) error {
	dir, err := migrationsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取 migrations 目录失败：%v", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".sql" {
			continue
		}
		files = append(files, e.Name())
	}
	if len(files) == 0 {
		return fmt.Errorf("migrations 目录 %s 下没有 .sql 文件", dir)
	}
	sort.Strings(files) // 0001 → 0018 按序应用

	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("读取 %s 失败：%v", name, err)
		}
		for _, stmt := range strings.Split(string(raw), ";") {
			s := strings.TrimSpace(stmt)
			if s == "" || isCommentOnly(s) {
				continue
			}
			if err := db.Exec(s).Error; err != nil {
				if isIgnored(err) {
					continue
				}
				return fmt.Errorf("应用 %s 失败：%v", name, err)
			}
		}
	}
	return nil
}

// isCommentOnly 整段都是注释行（-- 开头）时没有可执行语句。
// 注意：naive 按 ";" 切分后，注释可能和半条语句在同一段里，
// 那种情况直接交给 MySQL 报错（不会发生——迁移文件的分号只在语句末尾）。
func isCommentOnly(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "--") {
			return false
		}
	}
	return true
}

func isIgnored(err error) bool {
	msg := err.Error()
	for _, want := range ignoredErrs {
		if strings.Contains(msg, want) {
			return true
		}
	}
	return false
}
