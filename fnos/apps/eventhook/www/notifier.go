package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func executeHook(h Hook, etype, detail, ts string) error {
	switch h.Type {
	case "webhook":
		return executeWebhook(h, etype, detail, ts)
	case "bark":
		return executeBark(h, etype, detail, ts)
	case "telegram":
		return executeTelegram(h, etype, detail, ts)
	case "script":
		return executeScript(h, etype, detail, ts)
	default:
		return fmt.Errorf("unknown hook type: %s", h.Type)
	}
}

type eventMeta struct {
	category string
	action   string
}

var eventMetaMap = map[string]eventMeta{
	// 应用
	"APP_CRASH":            {"应用", "崩溃"},
	"APP_STARTED":          {"应用", "启动"},
	"APP_STOPPED":          {"应用", "停止"},
	"APP_UPDATED":          {"应用", "更新"},
	"APP_INSTALLED":        {"应用", "安装"},
	"APP_UNINSTALLED":      {"应用", "卸载"},
	"APP_UPDATE_FAILED":    {"应用", "更新失败"},
	"APP_START_FAILED":     {"应用", "启动失败"},
	"APP_AUTO_START_FAILED": {"应用", "自启失败"},
	"APP_ENABLED":          {"应用", "启用"},
	"APP_DISABLED":         {"应用", "停用"},

	// 登录 (fnOS native names)
	"LoginSucc":            {"登录", "登录成功"},
	"LoginSucc2FA1":        {"登录", "两步验证登录"},
	"LoginFail":            {"登录", "登录失败"},
	"Logout":               {"登录", "登出"},
	"USER_LOGIN":           {"登录", "登录"},
	"USER_LOGOUT":          {"登录", "登出"},
	"LOGIN_FAILED":         {"登录", "登录失败"},
	"LOGIN_TOKEN":          {"登录", "Token 登录"},

	// SSH (fnOS native names)
	"SshdLoginSucc":        {"SSH", "登录成功"},
	"SshdLoginFail":        {"SSH", "登录失败"},
	"SSH_LOGIN_SUCCESS":    {"SSH", "登录成功"},
	"SSH_INVALID_USER":     {"SSH", "非法用户"},
	"SSH_AUTH_FAILED":      {"SSH", "认证失败"},
	"SSH_DISCONNECTED":     {"SSH", "断开连接"},
	"SSH_LOGIN":            {"SSH", "登录"},
	"SSH_LOGOUT":           {"SSH", "登出"},

	// 硬盘 (fnOS native names)
	"FoundDisk":            {"硬盘", "发现新硬盘"},
	"DiskWakeup":           {"硬盘", "唤醒"},
	"DiskSpindown":         {"硬盘", "休眠"},
	"DISK_IO_ERR":          {"硬盘", "IO 错误"},
	"DISK_SLEEP":           {"硬盘", "休眠"},
	"DISK_WAKEUP":          {"硬盘", "唤醒"},
	"DISK_ERROR":           {"硬盘", "错误"},
	"DISK_REMOVED":         {"硬盘", "移除"},
	"DISK_INSERTED":        {"硬盘", "插入"},
	"DISK_SMART_ERROR":     {"硬盘", "SMART 错误"},
	"DISK_FORMAT":          {"硬盘", "格式化"},
	"DISK_MOUNT":           {"硬盘", "挂载"},
	"DISK_UNMOUNT":         {"硬盘", "卸载"},
	"DISK_BAD_SECTOR":      {"硬盘", "坏道"},
	"DISK_TEMP_HIGH":       {"硬盘", "温度过高"},

	// 存储池 / RAID
	"RAID_CREATED":         {"存储池", "创建"},
	"RAID_DELETED":         {"存储池", "删除"},
	"RAID_DEGRADED":        {"存储池", "降级"},
	"RAID_RECOVER":         {"存储池", "恢复"},
	"RAID_ERROR":           {"存储池", "错误"},
	"RAID_SCRUB_START":     {"存储池", "一致性检查开始"},
	"RAID_SCRUB_END":       {"存储池", "一致性检查完成"},
	"STORAGE_DEGRADED":     {"存储", "存储降级"},
	"STORAGE_DAMAGED":      {"存储", "存储损坏"},
	"VOLUME_CREATED":       {"存储", "创建"},
	"VOLUME_DELETED":       {"存储", "删除"},
	"VOLUME_MOUNTED":       {"存储", "挂载"},
	"VOLUME_UNMOUNTED":     {"存储", "卸载"},
	"VOLUME_FULL":          {"存储", "容量已满"},
	"VOLUME_LOW_SPACE":     {"存储", "空间不足"},

	// UPS (fnOS native names)
	"UPS_ONBATT":           {"UPS", "电池供电"},
	"UPS_ONBATT_LOWBATT":   {"UPS", "电池电量低"},
	"UPS_ONLINE":           {"UPS", "恢复正常"},
	"UPS_ENABLE":           {"UPS", "已启用"},
	"UPS_DISABLE":          {"UPS", "已禁用"},
	"UPS_LOW_BATTERY":      {"UPS", "低电量"},
	"UPS_ON_BATTERY":       {"UPS", "电池供电"},
	"UPS_SHUTDOWN":         {"UPS", "关机"},
	"UPS_TEST":             {"UPS", "自检"},
	"UPS_OVERLOAD":         {"UPS", "过载"},

	// 系统
	"SYSTEM_BOOT":          {"系统", "启动"},
	"SYSTEM_SHUTDOWN":      {"系统", "关机"},
	"SYSTEM_REBOOT":        {"系统", "重启"},
	"SYSTEM_UPDATE":        {"系统", "更新"},
	"SYSTEM_UPGRADE":       {"系统", "升级"},
	"SYSTEM_CRASH":         {"系统", "崩溃"},
	"SYSTEM_RECOVER":       {"系统", "恢复"},

	// 网络
	"NETWORK_UP":           {"网络", "连接"},
	"NETWORK_DOWN":         {"网络", "断开"},
	"NETWORK_CHANGE":       {"网络", "变更"},
	"NETWORK_IP_CHANGE":    {"网络", "IP 变更"},
	"NETWORK_DHCP_RENEW":   {"网络", "DHCP 续租"},
	"NETWORK_DDNS_UPDATE":  {"网络", "DDNS 更新"},
	"PORT_FORWARD_ADD":     {"网络", "端口转发添加"},
	"PORT_FORWARD_REMOVE":  {"网络", "端口转发移除"},

	// 备份
	"BACKUP_START":         {"备份", "开始"},
	"BACKUP_COMPLETE":      {"备份", "完成"},
	"BACKUP_FAILED":        {"备份", "失败"},
	"BACKUP_CANCEL":        {"备份", "取消"},

	// 任务
	"TASK_STARTED":         {"任务", "开始"},
	"TASK_COMPLETED":       {"任务", "完成"},
	"TASK_FAILED":          {"任务", "失败"},
	"TASK_CANCELLED":       {"任务", "取消"},

	// 共享
	"SHARE_ADDED":          {"共享", "添加"},
	"SHARE_REMOVED":        {"共享", "移除"},
	"SHARE_MODIFIED":       {"共享", "修改"},
	"SMB_SERVICE":          {"共享", "SMB"},
	"NFS_SERVICE":          {"共享", "NFS"},
	"FTP_SERVICE":          {"共享", "FTP"},
	"WEBDAV_SERVICE":       {"共享", "WebDAV"},

	// 证书
	"CERT_CREATED":         {"证书", "创建"},
	"CERT_EXPIRED":         {"证书", "过期"},
	"CERT_RENEW":           {"证书", "续期"},
	"CERT_FAILED":          {"证书", "申请失败"},

	// 用户管理
	"USER_CREATED":         {"用户", "创建"},
	"USER_DELETED":         {"用户", "删除"},
	"USER_PASSWORD_CHANGE": {"用户", "密码修改"},
	"USER_PERMISSION":      {"用户", "权限变更"},

	// Docker / 容器
	"CONTAINER_START":      {"容器", "启动"},
	"CONTAINER_STOP":       {"容器", "停止"},
	"CONTAINER_RESTART":    {"容器", "重启"},
	"CONTAINER_CRASH":      {"容器", "崩溃"},
	"CONTAINER_CREATED":    {"容器", "创建"},
	"CONTAINER_DELETED":    {"容器", "删除"},
	"IMAGE_PULL":           {"容器", "拉取镜像"},
	"IMAGE_PULL_FAILED":    {"容器", "拉取镜像失败"},

	// 防火墙 / 安全
	"FIREWALL_RULE_ADD":    {"防火墙", "规则添加"},
	"FIREWALL_RULE_REMOVE": {"防火墙", "规则移除"},
	"FIREWALL_ATTACK":      {"防火墙", "攻击拦截"},

	// 硬件
	"CPU_USAGE_ALARM":      {"硬件", "CPU 使用率告警"},
	"CPU_TEMPERATURE_ALARM": {"硬件", "CPU 温度告警"},
	"FAN_ERROR":            {"硬件", "风扇故障"},
	"TEMP_HIGH":            {"硬件", "温度过高"},
	"CPU_HIGH":             {"硬件", "CPU 高负载"},
	"MEMORY_LOW":           {"硬件", "内存不足"},
	"USB_INSERTED":         {"硬件", "USB 接入"},
	"USB_REMOVED":          {"硬件", "USB 移除"},

	// 虚拟机
	"VM_START":             {"虚拟机", "启动"},
	"VM_STOP":              {"虚拟机", "停止"},
	"VM_CRASH":             {"虚拟机", "崩溃"},
	"VM_PAUSE":             {"虚拟机", "暂停"},
	"VM_RESUME":            {"虚拟机", "恢复"},
}

func getField(data map[string]any, keys ...string) string {
	if data == nil {
		return ""
	}
	// Build lookup map for case-insensitive search
	lookup := make(map[string]string)
	for k := range data {
		lookup[strings.ToLower(k)] = k
	}
	for _, key := range keys {
		if actualKey, ok := lookup[strings.ToLower(key)]; ok {
			if v, ok := data[actualKey].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

func formatPair(k, v string) string {
	if v == "" {
		return ""
	}
	return k + ": " + v
}

// Extract a readable name and optional extra info from event detail JSON
func parseEventDetail(detail string) (name, extra string) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(detail), &raw); err != nil {
		return detail, ""
	}
	// Determine field source: nested "data" or flat top-level
	data, ok := raw["data"].(map[string]any)
	if !ok {
		data = raw
	}

	var parts []string
	name = getField(data, "user", "USER", "USERNAME", "USER_NAME", "name", "NAME", "DISPLAY_NAME", "APP_NAME", "TASK_NAME", "JOB_NAME", "VOLUME_NAME", "SHARE_NAME", "CERT_NAME", "CONTAINER_NAME", "IMAGE_NAME", "DEVICE", "MODEL")

	if ip := getField(data, "IP", "ip", "ip_address", "IP_ADDRESS"); ip != "" {
		parts = append(parts, formatPair("IP", ip))
	}
	if host := getField(data, "HOST", "host"); host != "" {
		parts = append(parts, formatPair("主机", host))
	}
	if port := getField(data, "PORT", "port"); port != "" {
		parts = append(parts, formatPair("端口", port))
	}
	if via := getField(data, "via", "VIA"); via != "" {
		parts = append(parts, formatPair("方式", via))
	}
	if path := getField(data, "PATH", "path"); path != "" {
		parts = append(parts, formatPair("路径", path))
	}
	if model := getField(data, "MODEL", "model"); model != "" && name == "" {
		if size := getField(data, "SIZE", "size", "capacity", "CAPACITY"); size != "" {
			name = model + " (" + size + ")"
		} else {
			name = model
		}
	}
	if size := getField(data, "SIZE", "size", "capacity", "CAPACITY"); size != "" {
		parts = append(parts, formatPair("容量", size))
	}
	if serial := getField(data, "SERIAL", "serial"); serial != "" {
		parts = append(parts, formatPair("序列号", serial))
	}
	if disk := getField(data, "DISK", "disk", "DRIVE", "drive"); disk != "" {
		parts = append(parts, formatPair("硬盘", disk))
	}
	if from := getField(data, "FROM", "from", "SOURCE", "source"); from != "" {
		parts = append(parts, formatPair("来源", from))
	}
	if to := getField(data, "TO", "to", "TARGET", "target"); to != "" {
		parts = append(parts, formatPair("目标", to))
	}
	if status := getField(data, "STATUS", "status", "STATE", "state"); status != "" {
		parts = append(parts, formatPair("状态", status))
	}
	if uid := getField(data, "uid", "UID"); uid != "" {
		parts = append(parts, formatPair("UID", uid))
	}

	if name == "" {
		name = getField(data, "template", "msg", "message", "MESSAGE", "MSG", "TEXT", "text")
	}
	if name == "" {
		for _, v := range raw {
			if s, ok := v.(string); ok && s != "" && s != raw["eventId"] {
				name = s
				break
			}
		}
	}
	if name == "" {
		name = detail
	}
	if len(parts) > 0 {
		extra = strings.Join(parts, " ")
	}
	return
}

var categoryPatterns = []struct {
	prefix string
	cat    string
}{
	{"Sshd", "SSH"},
	{"SSH_", "SSH"},
	{"Login", "登录"},
	{"Logout", "登录"},
	{"LOGIN_", "登录"},
	{"FoundDisk", "硬盘"},
	{"Disk", "硬盘"},
	{"DISK_", "硬盘"},
	{"APP_", "应用"},
	{"UPS_", "UPS"},
	{"CPU_", "硬件"},
	{"TEMP_", "硬件"},
	{"FAN_", "硬件"},
	{"MEMORY_", "硬件"},
	{"USB_", "硬件"},
	{"STORAGE_", "存储"},
	{"VOLUME_", "存储"},
	{"RAID_", "存储池"},
	{"VM_", "虚拟机"},
	{"SYSTEM_", "系统"},
	{"NETWORK_", "网络"},
	{"BACKUP_", "备份"},
	{"TASK_", "任务"},
	{"SHARE_", "共享"},
	{"CERT_", "证书"},
	{"USER_", "用户"},
	{"CONTAINER_", "容器"},
	{"IMAGE_", "容器"},
	{"FIREWALL_", "防火墙"},
}

func getCategory(etype string) string {
	if m, ok := eventMetaMap[etype]; ok {
		return m.category
	}
	for _, cp := range categoryPatterns {
		if strings.HasPrefix(etype, cp.prefix) {
			return cp.cat
		}
	}
	return ""
}

func formatNotification(etype, detail, ts string) (title, body string) {
	category := getCategory(etype)
	meta, ok := eventMetaMap[etype]
	if !ok && category != "" {
		// Known category but unknown exact type — use category default
		meta = eventMeta{category: category, action: category}
		ok = true
	}
	if !ok {
		// Try to match by template field in the detail JSON
		var raw map[string]any
		if json.Unmarshal([]byte(detail), &raw) == nil {
			if template, _ := raw["template"].(string); template != "" {
				if m, ok := eventMetaMap[template]; ok {
					meta = m
					ok = true
				} else if cat := getCategory(template); cat != "" {
					meta = eventMeta{category: cat, action: cat}
					ok = true
				}
			}
		}
	}
	if !ok {
		name, extra := parseEventDetail(detail)
		if name != "" && name != detail {
			title = "fnos · " + etype
			body = name
			if extra != "" {
				body += " (" + extra + ")"
			}
			return
		}
		title = "fnos · " + etype
		body = detail
		return
	}

	title = "fnos - " + meta.category
	name, extra := parseEventDetail(detail)
	body = meta.action
	if name != "" {
		body += " - " + name
	}
	if extra != "" {
		body += " (" + extra + ")"
	}
	body += " [" + ts + "]"
	return
}

func buildEventPayload(etype, detail, ts string) map[string]any {
	title, body := formatNotification(etype, detail, ts)
	name, extra := parseEventDetail(detail)

	payload := map[string]any{
		"event_type": etype,
		"title":      title,
		"body":       body,
		"detail":     detail,
		"summary":    name,
		"timestamp":  ts,
		"hostname":   getHostname(),
		"app":        "fnos-eventhook",
	}
	if extra != "" {
		payload["extra"] = extra
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(detail), &raw); err != nil {
		return payload
	}

	// Determine field source: nested "data" or flat top-level
	data, ok := raw["data"].(map[string]any)
	if !ok {
		data = raw
	}

	knownFields := []string{"user", "USER", "USERNAME", "USER_NAME", "name", "NAME", "IP", "ip", "PORT", "port", "HOST", "host", "PATH", "path", "SIZE", "size", "CAPACITY", "capacity", "USED", "FROM", "from", "TO", "to", "SOURCE", "source", "TARGET", "target", "DISK", "disk", "DRIVE", "drive", "MODEL", "model", "SERIAL", "serial", "TEMPERATURE", "SPEED", "STATUS", "status", "STATE", "state", "MSG", "msg", "MESSAGE", "message", "TEXT", "text", "DETAIL", "detail", "INFO", "info", "NOTE", "note", "via", "VIA", "uid", "UID", "DEVICE", "device", "DISPLAY_NAME", "APP_NAME", "TASK_NAME", "JOB_NAME", "VOLUME_NAME", "SHARE_NAME", "CERT_NAME", "CONTAINER_NAME", "IMAGE_NAME", "DESCRIPTION", "description", "Title", "TitleZh", "Content"}
	added := make(map[string]bool)
	for _, k := range knownFields {
		if added[k] {
			continue
		}
		if v, exists := data[k]; exists {
			payload[k] = v
			added[k] = true
		}
	}
	// Also add any remaining top-level strings from raw (for nested case)
	if ok {
		for key, value := range raw {
			if key == "data" || added[key] {
				continue
			}
			if s, ok := value.(string); ok && s != "" {
				payload[key] = s
				added[key] = true
			}
		}
	}
	return payload
}

func getHostname() string {
	h, _ := os.Hostname()
	return h
}

func executeWebhook(h Hook, etype, detail, ts string) error {
	payload := buildEventPayload(etype, detail, ts)
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", h.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "fnos-eventhook/1.0")

	if h.Headers != "" {
		var customHeaders map[string]string
		if err := json.Unmarshal([]byte(h.Headers), &customHeaders); err == nil {
			for k, v := range customHeaders {
				req.Header.Set(k, v)
			}
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func executeBark(h Hook, etype, detail, ts string) error {
	title, body := formatNotification(etype, detail, ts)

	payload := map[string]any{
		"title": title,
		"body":  body,
		"group": "fnos-eventhook",
		"icon":  "https://icons.r2.dukangxu.com/fnos.png",
	}
	jsonBody, _ := json.Marshal(payload)

	resp, err := http.Post(h.URL, "application/json; charset=utf-8", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("bark request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

func executeTelegram(h Hook, etype, detail, ts string) error {
	token := h.URL
	chatID := h.Token
	if token == "" || chatID == "" {
		return fmt.Errorf("telegram: bot token and chat id required")
	}

	title, body := formatNotification(etype, detail, ts)
	text := fmt.Sprintf("<b>%s</b>\n%s\n%s", title, body, ts)

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	jsonBody, _ := json.Marshal(payload)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if !result["ok"].(bool) {
		errDesc, _ := result["description"].(string)
		return fmt.Errorf("telegram error: %s", errDesc)
	}
	return nil
}

func executeScript(h Hook, etype, detail, ts string) error {
	cmd := exec.Command("/bin/sh", "-c", h.Cmd)
	title, body := formatNotification(etype, detail, ts)
	name, _ := parseEventDetail(detail)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("EVENT_TYPE=%s", etype),
		fmt.Sprintf("EVENT_TITLE=%s", title),
		fmt.Sprintf("EVENT_BODY=%s", body),
		fmt.Sprintf("EVENT_DETAIL=%s", detail),
		fmt.Sprintf("EVENT_SUMMARY=%s", name),
		fmt.Sprintf("EVENT_TIME=%s", ts),
		fmt.Sprintf("APP_DEST=%s", appDest),
		fmt.Sprintf("APP_VAR=%s", appVar),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Errorf("script error: %s", errMsg)
	}

	out := strings.TrimSpace(stdout.String())
	if out != "" {
		appLogf("  script output: %s", out)
	}
	return nil
}
