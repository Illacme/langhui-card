//go:build windows

package setup

import "errors"

// restartAvailable Windows 无 exec 家族调用，无法「替换式自重启」。
var restartAvailable = false

// reexecSelf 返回错误即代表不可用：安装流程会把 Restarted 置为 false，
// 页面据此提示运维手动重启（Windows 不是本项目的目标部署形态，
// 不为它引入「起子进程再退出」那套会改变容器进程树语义的方案）。
func reexecSelf() error {
	return errors.New("Windows 暂不支持安装后自重启，请手动重启服务")
}
