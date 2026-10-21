//go:build !windows

package setup

import (
	"fmt"
	"os"
	"syscall"
)

// restartAvailable 本平台支持「替换式自重启」（Unix 全系）。
var restartAvailable = true

// reexecSelf 用当前可执行文件替换当前进程映像。
//
// 为什么是自重启而不是让运维手动重启：安装向导的卖点是「填完就能用」，
// 最后还要求人去 docker restart / systemctl restart 是断的；容器里同样成立——
// PID 1 被替换而不是退出，容器不会停，restart 策略也看不出区别。
//
// 环境变量原样继承（os.Environ()）：CONFIG_FILE 也在里面，新进程才找得到
// 安装向导刚写下的配置文件；这也是「配置写文件而不是改 env」能闭环的关键。
func reexecSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位可执行文件失败：%w", err)
	}
	argv := append([]string{exe}, os.Args[1:]...)
	return syscall.Exec(exe, argv, os.Environ())
}
