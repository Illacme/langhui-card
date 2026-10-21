package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 配置文件解析规则：注释 / 空行 / 引号 / 坏行，逐条钉死。
func TestParseFileRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.env")
	content := "# 注释行\n" +
		"\n" +
		"DB_DSN=root:root@tcp(127.0.0.1:3306)/langhui?charset=utf8mb4\n" +
		"  SPACED  =  value-with-spaces  \n" +
		"QUOTED=\"  keep inner spaces  \"\n" +
		"SINGLE='single'\n" +
		"EMPTY=\n" +
		"坏行没有等号\n" +
		"=只有等号前为空\n" +
		"HASH=va#lue不是注释\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	vals := parseFile(path)
	want := map[string]string{
		"DB_DSN": "root:root@tcp(127.0.0.1:3306)/langhui?charset=utf8mb4",
		"SPACED": "value-with-spaces",
		"QUOTED": "  keep inner spaces  ",
		"SINGLE": "single",
		"EMPTY":  "",
		"HASH":   "va#lue不是注释",
	}
	for k, v := range want {
		if got, ok := vals[k]; !ok || got != v {
			t.Fatalf("%s = %q (ok=%v), want %q", k, got, ok, v)
		}
	}
	if _, ok := vals["坏行没有等号"]; ok {
		t.Fatal("无等号的行不应被解析出键")
	}
	if len(vals) != len(want) {
		t.Fatalf("解析出 %d 项，期望 %d 项：%+v", len(vals), len(want), vals)
	}
}

// 不存在的文件 = 无配置值，不是错误（首次部署时文件本来就不存在）。
func TestParseFileMissingIsEmpty(t *testing.T) {
	vals := parseFile(filepath.Join(t.TempDir(), "absent.env"))
	if len(vals) != 0 {
		t.Fatalf("文件不存在应返回空 map，实际 %+v", vals)
	}
}

// 优先级：环境变量 > 配置文件 > 内置缺省值，对每一个键都成立。
func TestPriorityEnvOverFileOverDefault(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "runtime.env")
	t.Setenv("CONFIG_FILE", cfgPath)

	// 只有文件：文件值生效
	if err := WriteFileValues(cfgPath, map[string]string{"JWT_KEY": "from-file-key-0123456789"}); err != nil {
		t.Fatal(err)
	}
	if got := getEnv("JWT_KEY", "def"); got != "from-file-key-0123456789" {
		t.Fatalf("文件值应生效，实际 %q", got)
	}
	if got := Resolved("JWT_KEY"); got != "from-file-key-0123456789" {
		t.Fatalf("Resolved 应读到文件值，实际 %q", got)
	}

	// 环境变量盖过文件
	t.Setenv("JWT_KEY", "from-env-key-9876543210")
	if got := getEnv("JWT_KEY", "def"); got != "from-env-key-9876543210" {
		t.Fatalf("环境变量应盖过文件，实际 %q", got)
	}

	// 两者都无：回落缺省
	t.Setenv("JWT_KEY", "")
	if err := os.Remove(cfgPath); err != nil {
		t.Fatal(err)
	}
	// 删文件后要清缓存路径：换个 CONFIG_FILE 指向（同路径缓存由写操作失效，
	// 这里直接删文件模拟「运维删了配置」，缓存需能反映——通过写空值再删太绕，
	// 改为把缓存也删掉：fileCache 按路径缓存，删除文件后重新加载是合理行为）
	fileMu.Lock()
	delete(fileCache, cfgPath)
	fileMu.Unlock()
	if got := getEnv("JWT_KEY", "def"); got != "def" {
		t.Fatalf("文件和环境变量都无值时应回落缺省，实际 %q", got)
	}
}

// 空串 / 纯空白 = 未设置：文件里写了空串也要继续往下一级回落。
func TestBlankValueMeansUnset(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "runtime.env")
	t.Setenv("CONFIG_FILE", cfgPath)
	t.Setenv("DB_DSN", "   ") // 纯空白环境变量也算没配
	if err := WriteFileValues(cfgPath, map[string]string{"DB_DSN": ""}); err != nil {
		t.Fatal(err)
	}
	if got := Resolved("DB_DSN"); got != "" {
		t.Fatalf("纯空白 + 文件空串应判定为未配置，实际 %q", got)
	}
	t.Setenv("DB_DSN", "")
	if err := WriteFileValues(cfgPath, map[string]string{"DB_DSN": "sqlite:///data/x.db"}); err != nil {
		t.Fatal(err)
	}
	if got := Resolved("DB_DSN"); got != "sqlite:///data/x.db" {
		t.Fatalf("文件值应生效，实际 %q", got)
	}
}

// 写文件：合并（不抹掉别人的键）、0600、原子（临时文件 + rename）。
func TestWriteFileValuesMergeAndPerms(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "data", "runtime.env")
	t.Setenv("CONFIG_FILE", cfgPath)

	if err := WriteFileValues(cfgPath, map[string]string{"JWT_KEY": "k1", "ADMIN_PASS": "p1"}); err != nil {
		t.Fatal(err)
	}
	// 目录不存在时应自动创建（安装向导首次就跑这条路径）
	if err := WriteFileValues(cfgPath, map[string]string{"DB_DSN": "sqlite:///data/x.db"}); err != nil {
		t.Fatal(err)
	}
	vals := parseFile(cfgPath)
	for k, want := range map[string]string{"JWT_KEY": "k1", "ADMIN_PASS": "p1", "DB_DSN": "sqlite:///data/x.db"} {
		if vals[k] != want {
			t.Fatalf("合并写丢失 %s：got %q want %q", k, vals[k], want)
		}
	}

	fi, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("配置文件权限应 0600，实际 %v", fi.Mode().Perm())
	}

	// 写操作要使同路径缓存失效，否则「写完立刻读」读到旧值
	if got := Resolved("DB_DSN"); got != "sqlite:///data/x.db" {
		t.Fatalf("写后 Resolved 应立刻读到新值，实际 %q", got)
	}
}

// Load() 必须吃到配置文件的值：安装向导的全部意义就在「重启后配置还在」。
func TestLoadReadsConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "runtime.env")
	t.Setenv("CONFIG_FILE", cfgPath)
	t.Setenv("DB_DSN", "")
	t.Setenv("JWT_KEY", "")
	t.Setenv("ADMIN_PASS", "")
	if err := WriteFileValues(cfgPath, map[string]string{
		"DB_DSN":        "sqlite:///data/langhui.db",
		"JWT_KEY":       "0123456789abcdef0123456789abcdef",
		"ADMIN_PASS":    "installer-pass",
		"COOKIE_SECURE": "true",
	}); err != nil {
		t.Fatal(err)
	}
	cfg := Load()
	if cfg.DBDSN != "sqlite:///data/langhui.db" {
		t.Fatalf("DBDSN 应来自配置文件，实际 %q", cfg.DBDSN)
	}
	if cfg.JWTKey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("JWTKey 应来自配置文件，实际 %q", cfg.JWTKey)
	}
	if cfg.AdminPass != "installer-pass" {
		t.Fatalf("AdminPass 应来自配置文件，实际 %q", cfg.AdminPass)
	}
	if !cfg.CookieSecure {
		t.Fatal("COOKIE_SECURE=true 应被解析为 true")
	}
	if len(cfg.InsecureGates()) != 0 {
		t.Fatalf("安装向导写出的配置应能通过安全门槛：%v", cfg.InsecureGates())
	}
}

func TestConfigFilePath(t *testing.T) {
	t.Setenv("CONFIG_FILE", "/custom/path.env")
	if got := ConfigFilePath(); got != "/custom/path.env" {
		t.Fatalf("CONFIG_FILE 应优先生效，实际 %q", got)
	}
	t.Setenv("CONFIG_FILE", "")
	if got := ConfigFilePath(); got != ConfigFileDefault {
		t.Fatalf("未设置时应回落 %q，实际 %q", ConfigFileDefault, got)
	}
}

// 含首尾空白的值必须加引号写盘，否则再读回来时会被 TrimSpace 吃掉（静默改配置）。
func TestQuoteValueRoundTrip(t *testing.T) {
	for _, v := range []string{"  padded  ", `has"quote`, "", "plain"} {
		quoted := quoteValue(v)
		dir := t.TempDir()
		p := filepath.Join(dir, "f.env")
		if err := os.WriteFile(p, []byte("K="+quoted+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := parseFile(p)["K"]; got != v {
			t.Fatalf("值 %q 经引号往返后变成 %q", v, got)
		}
	}
	_ = strings.TrimSpace // 保持 import 稳定
}
