package main

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// resolveJobUser 解析任务执行用户
// 优先级: jobUsername > FNOS_USER env (cmd/main 从 TRIM_RUN_USERNAME 注入) > "root"
// 返回 uid, gid, homeDir, 实际使用的 username, error (仅当 username 非空且 lookup 失败时返回 error; 这种情况 caller 决定 fallback 到 root 还是中止)
func resolveJobUser(jobUsername string) (uid, gid uint32, homeDir, username string, err error) {
	username = jobUsername
	if username == "" {
		username = os.Getenv("FNOS_USER")
	}
	if username == "" {
		username = "root"
	}

	u, lookupErr := user.Lookup(username)
	if lookupErr != nil {
		// username 是用户显式指定 (jobUsername) -> 报错
		// username 是 fallback (FNOS_USER 或 root) -> 静默回退
		if jobUsername != "" {
			return 0, 0, "/root", "root", fmt.Errorf("user %q not found: %w", jobUsername, lookupErr)
		}
		return 0, 0, "/root", "root", nil
	}

	uid64, _ := strconv.ParseUint(u.Uid, 10, 32)
	gid64, _ := strconv.ParseUint(u.Gid, 10, 32)
	uid = uint32(uid64)
	gid = uint32(gid64)
	homeDir = u.HomeDir
	if homeDir == "" {
		homeDir = "/"
	}
	return uid, gid, homeDir, u.Username, nil
}

// currentFnOSUser 返回启动时 fnOS 框架注入的当前登录用户 (用于前端默认值展示)
func currentFnOSUser() string {
	return os.Getenv("FNOS_USER")
}

// credentialFor 返回 SysProcAttr (非 root 才 setuid)
func credentialFor(uid, gid uint32) *syscall.SysProcAttr {
	if uid == 0 && gid == 0 {
		return nil
	}
	return &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:         uid,
			Gid:         gid,
			NoSetGroups: false,
		},
	}
}
