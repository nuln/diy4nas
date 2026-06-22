package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// resolveRequestUser 获取当前请求的 fnOS 登录用户
//
// 优先级（每次请求都查，不用 settings 缓存）：
//   1. USER env（fnOS 框架 setuid 切到当前登录用户跑 cmd/main，$USER 自动设）
//   2. FNOS_USER env（cmd/main 显式透传）
//   3. fnOS 反代 HTTP header（X-Forwarded-User / X-Real-User / Remote-User）
//   4. settings.DefaultUser
//   5. detectDefaultUser 现场算
//   6. 全部拿不到 → 报错
//
// header 传了但不存在的用户 / 系统服务用户（如 libvirt-qemu）→ 忽略该 header
func resolveRequestUser(r *http.Request) (string, error) {
	// 1. USER env（fnOS 框架 setuid 切用户后自动设）
	if u := strings.TrimSpace(os.Getenv("USER")); u != "" && u != "root" {
		if isLoginUser(u) {
			appLogf("resolveRequestUser: USER env=%q", u)
			return u, nil
		}
		appLogf("resolveRequestUser: USER env=%q 不是有效登录用户，跳过", u)
	}

	// 2. FNOS_USER env（cmd/main 显式透传）
	if u := strings.TrimSpace(os.Getenv("FNOS_USER")); u != "" {
		if isLoginUser(u) {
			appLogf("resolveRequestUser: FNOS_USER env=%q", u)
			return u, nil
		}
		appLogf("resolveRequestUser: FNOS_USER env=%q 不是有效登录用户，跳过", u)
	}

	// 3. fnOS 反代 header
	for _, h := range []string{"X-Forwarded-User", "X-Real-User", "Remote-User"} {
		var u string
		if r != nil {
			u = strings.TrimSpace(r.Header.Get(h))
		}
		if u != "" {
			appLogf("resolveRequestUser: header %s=%q", h, u)
			if isLoginUser(u) {
				return u, nil
			}
			appLogf("resolveRequestUser: header %s 传的用户 %q 不是有效登录用户，跳过", h, u)
		}
	}

	// 4. settings.DefaultUser
	settingsMu.RLock()
	u := settings.DefaultUser
	settingsMu.RUnlock()
	if u != "" && isLoginUser(u) {
		appLogf("resolveRequestUser: settings.DefaultUser=%q", u)
		return u, nil
	}

	// 5. 现场算 detectDefaultUser
	u = detectDefaultUser()
	if u != "" && isLoginUser(u) {
		appLogf("resolveRequestUser: detectDefaultUser=%q", u)
		settingsMu.Lock()
		settings.DefaultUser = u
		settingsMu.Unlock()
		_ = saveSettingsFile()
		return u, nil
	}

	return "", errors.New("无法识别当前 fnOS 登录用户（USER/FNOS_USER env 未设，fnOS header 没传，settings 没配，detectDefaultUser 也找不到）")
}

// isLoginUser 验证用户是合法登录用户
// 拒绝：root (PTY 启动会 setuid root 不需要走这条路)、libvirt-qemu 等系统服务用户、nobody
func isLoginUser(name string) bool {
	if name == "root" || name == "nobody" {
		return false
	}
	// 拒绝明显的系统服务用户名
	denyList := map[string]bool{
		"libvirt-qemu": true, "libvirt-dnsmasq": true, "sshd": true,
		"messagebus": true, "avahi": true, "daemon": true,
		"_apt": true, "systemd-network": true,
		"systemd-resolve": true, "systemd-timesync": true, "syslog": true,
		"pollinate": true, "uuidd": true, "tcpdump": true,
	}
	if denyList[name] {
		return false
	}
	// 必须在 listSystemUsers 里（确保 shell 合法 + UID 正常）
	for _, u := range listSystemUsers() {
		if u == name {
			return true
		}
	}
	return false
}

// listSystemUsers 列出系统所有可登录用户（UID >= 1000，shell 存在）
func listSystemUsers() []string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}
	out := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.SplitN(line, ":", 7)
		if len(fields) < 7 {
			continue
		}
		uid, _ := strconv.Atoi(fields[2])
		name := fields[0]
		sh := fields[6]
		if uid < 1000 || uid >= 65534 || name == "nobody" {
			continue
		}
		if sh == "" || sh == "/usr/sbin/nologin" || sh == "/bin/false" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
