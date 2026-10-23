package config

import (
	"path/filepath"
	"testing"
)

// noConfigFile 把 CONFIG_FILE 指到一个不存在的临时路径：
// 否则本机若跑过安装向导（./data/runtime.env 存在），文件里的值会渗进测试结果。
// 每个用例调用一次，t.TempDir() 自动保证路径互不相同。
func noConfigFile(t *testing.T) {
	t.Helper()
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "absent.env"))
}

// 安全门槛的语义必须钉死：这两项默认值都随演示账号写进了公开文档，
// 属已知凭据——带它们上线等于把后台开放给所有人，所以必须拦启动而非仅告警。
func TestInsecureGates(t *testing.T) {
	cases := []struct {
		name     string
		jwtKey   string
		adminPwd string
		want     int // 期望的致命项条数
	}{
		{"JWT 缺省值+口令缺省值", DefaultJWTKey, DefaultAdminPass, 2},
		{"JWT 过短+口令缺省值", "short-key", DefaultAdminPass, 2},
		{"JWT 过短但口令已改", "short-key", "a-strong-pass", 1},
		{"JWT 达标但口令仍缺省", "0123456789abcdef", DefaultAdminPass, 1},
		{"两项都已改", "0123456789abcdef", "a-strong-pass", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := Config{JWTKey: c.jwtKey, AdminPass: c.adminPwd}
			if got := len(cfg.InsecureGates()); got != c.want {
				t.Fatalf("InsecureGates() = %d 项，期望 %d 项", got, c.want)
			}
		})
	}
}

// 弱口令只是「建议改」，不拦启动——口令是给人记的，卡太死会让运维处处碰壁，
// 与 JWT 密钥（机器用、可任意长）区别对待。
func TestInsecureWarningsOnlyWarns(t *testing.T) {
	cfg := Config{JWTKey: "0123456789abcdef", AdminPass: "abc"}
	if len(cfg.InsecureGates()) != 0 {
		t.Fatalf("口令偏弱不应触发致命门槛：%v", cfg.InsecureGates())
	}
	if len(cfg.InsecureWarnings()) == 0 {
		t.Fatal("口令偏弱应产生告警")
	}
}

// 环境变量缺失时 Load 回退到内置缺省值，因此门槛必然命中——
// 这是「不配置 = 拒绝启动」这一设计的保证，改动缺省值时需要同步更新本用例。
func TestLoadDefaultsTriggerGates(t *testing.T) {
	noConfigFile(t)
	t.Setenv("JWT_KEY", "")
	t.Setenv("ADMIN_PASS", "")
	cfg := Load()
	if cfg.JWTKey != DefaultJWTKey || cfg.AdminPass != DefaultAdminPass {
		t.Fatalf("缺省值读取异常：jwt=%q pass=%q", cfg.JWTKey, cfg.AdminPass)
	}
	if len(cfg.InsecureGates()) == 0 {
		t.Fatal("未配置时 InsecureGates 应命中，否则等于允许带默认凭据上线")
	}
}

// 可信代理的缺省值决定了「限流按谁分桶」，是安全相关的配置，语义必须钉死。
// 空值必须落到私有网段而不是 gin 的 0.0.0.0/0：后者会让 X-Forwarded-For 被无条件采信，
// 而那个头是客户端随便写的——限流于是变成"每次换个头就换个桶"。
func TestParseTrustedProxies(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"空值用缺省网段", "", DefaultTrustedProxies},
		{"空白也算未设置", "   ", DefaultTrustedProxies},
		{"none 表示没有反代", "none", nil},
		{"none 不区分大小写", "NONE", nil},
		{"单项原样返回", "10.1.2.3", []string{"10.1.2.3"}},
		{"逗号分隔并去空白", " 10.0.0.0/8 , 172.16.0.0/12 ", []string{"10.0.0.0/8", "172.16.0.0/12"}},
		{"只有逗号等于未设置", " , , ", DefaultTrustedProxies},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTrustedProxies(c.raw)
			if len(got) != len(c.want) {
				t.Fatalf("parseTrustedProxies(%q) = %v，期望 %v", c.raw, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("parseTrustedProxies(%q) = %v，期望 %v", c.raw, got, c.want)
				}
			}
		})
	}
}

// Load 必须把 TRUSTED_PROXIES 接上，否则上面那条链路等于没接线。
func TestLoadReadsTrustedProxies(t *testing.T) {
	noConfigFile(t)
	t.Setenv("TRUSTED_PROXIES", "192.0.2.1,198.51.100.0/24")
	cfg := Load()
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "192.0.2.1" {
		t.Fatalf("TRUSTED_PROXIES 未生效：%v", cfg.TrustedProxies)
	}
}

// 缺省值必须真的覆盖到「反代常见落点」，否则所有用户会被算成同一个 IP。
// 这条防的是有人图省事把 DefaultTrustedProxies 改成只有 127.0.0.1——
// 那样 docker / k8s 部署下全站共用一个限流桶，一个人就能把所有人挡住。
func TestDefaultTrustedProxiesCoverPrivateRanges(t *testing.T) {
	need := []string{"127.0.0.1/32", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	for _, n := range need {
		found := false
		for _, d := range DefaultTrustedProxies {
			if d == n {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("缺省可信代理缺少 %s：容器/内网反代场景下会全站共用一个限流桶", n)
		}
	}
}
