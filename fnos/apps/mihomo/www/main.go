package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"strconv"
	"sync"
	"syscall"
	"time"
)

//go:embed ui/index.html
var frontend embed.FS

var (
	mihomoBin  = findBin("mihomo", "")
	appDest    = getEnv("TRIM_APPDEST", "/var/apps/mihomo")
	appVar     = getEnv("TRIM_PKGVAR", "/var/apps/mihomo/data")
	mihomoAPI  = "http://127.0.0.1:19090"
	servicePort = "9097"

	mihomoCmd      *exec.Cmd
	mihomoMu       sync.Mutex
	opMu           sync.Mutex
	logBuf         LogBuffer
	mihomoLogBuf   LogBuffer
	mihomoLogPath  string

	configDir   string
	profilesDir string
	activeFile  string
	settingsPath string
)

const defaultProfile = "default"

type Settings struct {
	ServiceEnabled     bool `json:"service_enabled"`
	TUNEnabled         bool `json:"tun_enabled"`
	LocalProxyOnly     bool `json:"local_proxy_only"`
	SystemProxyEnabled bool `json:"system_proxy_enabled"`
}

var currentSettings Settings
var settingsMu sync.Mutex

func getSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	return currentSettings
}

func loadSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return Settings{ServiceEnabled: false}
	}
	var s Settings
	json.Unmarshal(data, &s)
	return s
}

func saveSettings(s Settings) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	currentSettings = s
	data, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(settingsPath, data, 0644)
}

func isMihomoRunning() bool {
	mihomoMu.Lock()
	defer mihomoMu.Unlock()
	if mihomoCmd == nil || mihomoCmd.Process == nil {
		return false
	}
	return mihomoCmd.Process.Signal(syscall.Signal(0)) == nil
}

func applySystemProxy(enable bool) {
	proxyFile := "/etc/profile.d/mihomo-proxy.sh"
	envFile := "/etc/environment"
	if enable {
		content := `export http_proxy=http://127.0.0.1:7890
export https_proxy=http://127.0.0.1:7890
export all_proxy=http://127.0.0.1:7890
export no_proxy=localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
export ALL_PROXY=http://127.0.0.1:7890
export NO_PROXY=localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10
`
		os.WriteFile(proxyFile, []byte(content), 0644)
		setEnvFile(envFile, true)
	} else {
		os.Remove(proxyFile)
		setEnvFile(envFile, false)
	}
}

func setEnvFile(path string, add bool) {
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "http_proxy=") || strings.HasPrefix(trimmed, "HTTP_PROXY=") ||
			strings.HasPrefix(trimmed, "https_proxy=") || strings.HasPrefix(trimmed, "HTTPS_PROXY=") ||
			strings.HasPrefix(trimmed, "all_proxy=") || strings.HasPrefix(trimmed, "ALL_PROXY=") ||
			strings.HasPrefix(trimmed, "no_proxy=") || strings.HasPrefix(trimmed, "NO_PROXY=") {
			continue
		}
		result = append(result, line)
	}
	if add {
		result = append(result,
			"http_proxy=http://127.0.0.1:7890",
			"https_proxy=http://127.0.0.1:7890",
			"all_proxy=http://127.0.0.1:7890",
			"no_proxy=localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10",
			"HTTP_PROXY=http://127.0.0.1:7890",
			"HTTPS_PROXY=http://127.0.0.1:7890",
			"ALL_PROXY=http://127.0.0.1:7890",
			"NO_PROXY=localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10",
		)
	}
	os.WriteFile(path, []byte(strings.Join(result, "\n")+"\n"), 0644)
}

func hasTUN(config string) bool {
	lines := strings.Split(config, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "tun:") && !strings.HasPrefix(trimmed, "tun://") {
			return true
		}
	}
	return false
}

func injectTUN(config string) string {
	if hasTUN(config) {
		return config
	}
	tun := `
tun:
  enable: true
  stack: system
  dns-hijack:
    - any:53
  auto-route: true
  auto-redirect: false
  mtu: 1500
`
	return config + tun
}

func setLocalBind(config string) string {
	if strings.Contains(config, "\nbind-address:") || strings.HasPrefix(config, "bind-address:") {
		lines := strings.Split(config, "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "bind-address:") {
				lines[i] = "bind-address: \"127.0.0.1\""
				break
			}
		}
		return strings.Join(lines, "\n")
	}
	lines := strings.Split(config, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "mixed-port:") || strings.HasPrefix(trimmed, "port:") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			insert := indent + "bind-address: \"127.0.0.1\""
			lines = append(lines[:i+1], append([]string{insert}, lines[i+1:]...)...)
			return strings.Join(lines, "\n")
		}
	}
	return config
}

func removeTUN(config string) string {
	lines := strings.Split(config, "\n")
	var result []string
	skip := false
	skipIndent := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !skip && strings.HasPrefix(trimmed, "tun:") && !strings.HasPrefix(trimmed, "tun://") {
			skip = true
			skipIndent = len(line) - len(strings.TrimLeft(line, " "))
			continue
		}
		if skip {
			if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			indent := len(line) - len(strings.TrimLeft(line, " "))
			if indent > skipIndent {
				continue
			}
			skip = false
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func removeLocalBind(config string) string {
	lines := strings.Split(config, "\n")
	var result []string
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "bind-address:") {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func applyConfigOverrides(config string) string {
	cfg := config
	cfg = ensureExternalController(cfg)
	s := getSettings()
	if s.TUNEnabled && !hasTUN(cfg) {
		cfg = injectTUN(cfg)
	}
	if !s.TUNEnabled {
		cfg = removeTUN(cfg)
	}
	if s.LocalProxyOnly {
		cfg = setLocalBind(cfg)
	} else {
		cfg = removeLocalBind(cfg)
	}
	cfg = ensureDNS(cfg)
	return cfg
}

func ensureExternalController(config string) string {
	lines := strings.Split(config, "\n")
	ecIdx := -1
	secretIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "external-controller:") {
			ecIdx = i
		}
		if strings.HasPrefix(trimmed, "secret:") && !strings.Contains(trimmed, "://") {
			secretIdx = i
		}
	}
	if ecIdx >= 0 {
		lines[ecIdx] = ensureIndent(lines[ecIdx]) + "external-controller: 127.0.0.1:19090"
	} else {
		insertAt := findTopLevelInsertPos(lines, "mixed-port:")
		lines = insertLine(lines, insertAt, "external-controller: 127.0.0.1:19090")
		if secretIdx >= 0 {
			secretIdx++
		}
	}
	if secretIdx >= 0 {
		lines[secretIdx] = ensureIndent(lines[secretIdx]) + `secret: ""`
	} else {
		ecLine := -1
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "external-controller:") {
				ecLine = i
				break
			}
		}
		insertAt := ecLine + 1
		if insertAt == 0 {
			insertAt = findTopLevelInsertPos(lines, "mixed-port:")
		}
		lines = insertLine(lines, insertAt, `secret: ""`)
	}
	return strings.Join(lines, "\n")
}

func ensureIndent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " "))]
}

func findTopLevelInsertPos(lines []string, afterKey string) int {
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), afterKey) {
			return i + 1
		}
	}
	return 1
}

func insertLine(lines []string, at int, line string) []string {
	result := make([]string, 0, len(lines)+1)
	result = append(result, lines[:at]...)
	result = append(result, line)
	result = append(result, lines[at:]...)
	return result
}

func ensureDNS(config string) string {
	essentialDNS := []string{"223.5.5.5", "114.114.114.114", "8.8.8.8", "100.100.100.100"}
	ipv6DNS := []string{"2606:4700:4700::1111", "2001:4860:4860::8888", "2400:3200::1", "2400:3200:baba::1"}
	lines := strings.Split(config, "\n")

	dnsStart, dnsEnd, dnsIndent := findYAMLBlock(lines, "dns")
	if dnsStart == -1 {
		block := "dns:\n  enable: true\n  ipv6: true\n  default-nameserver:\n"
		for _, e := range essentialDNS {
			block += "    - " + e + "\n"
		}
		for _, e := range ipv6DNS {
			block += "    - " + e + "\n"
		}
		block += "  direct-nameserver:\n"
		for _, e := range essentialDNS {
			block += "    - " + e + "\n"
		}
		for _, e := range ipv6DNS {
			block += "    - " + e + "\n"
		}
		block += "  nameserver:\n    - https://dns.alidns.com/dns-query\n"
		return config + "\n" + block
	}

	enableIdx := -1
	ipv6LineIdx := -1
	for i := dnsStart + 1; i < dnsEnd; i++ {
		t := strings.TrimSpace(lines[i])
		if t == "enable:" || strings.HasPrefix(t, "enable:") {
			enableIdx = i
		}
		if t == "ipv6:" || strings.HasPrefix(t, "ipv6:") {
			ipv6LineIdx = i
		}
	}
	if enableIdx == -1 {
		lines = insertBlock(lines, dnsStart+1, strings.Repeat(" ", dnsIndent+2)+"enable: true\n")
		dnsEnd++
	}
	if ipv6LineIdx == -1 {
		lines = insertBlock(lines, dnsStart+1, strings.Repeat(" ", dnsIndent+2)+"ipv6: true\n")
		dnsEnd++
	} else {
		lines[ipv6LineIdx] = strings.Repeat(" ", dnsIndent+2) + "ipv6: true"
	}

	for _, section := range []string{"default-nameserver", "direct-nameserver"} {
		secStart, secEnd, secIndent := findSubBlock(lines, section, dnsStart, dnsEnd)
		if secStart == -1 {
			insert := strings.Repeat(" ", dnsIndent+2) + section + ":\n"
			for _, e := range essentialDNS {
				insert += strings.Repeat(" ", dnsIndent+4) + "- " + e + "\n"
			}
			for _, e := range ipv6DNS {
				insert += strings.Repeat(" ", dnsIndent+4) + "- " + e + "\n"
			}
			lines = insertBlock(lines, dnsStart+1, insert)
			dnsEnd += strings.Count(insert, "\n")
			continue
		}
		existing := collectListItems(lines, secStart, secEnd)
		for _, e := range essentialDNS {
			if !existing[e] {
				line := strings.Repeat(" ", secIndent+2) + "- " + e
				lines = insertBlock(lines, secEnd, line+"\n")
				dnsEnd++
				secEnd++
			}
		}
		for _, e := range ipv6DNS {
			if !existing[e] {
				line := strings.Repeat(" ", secIndent+2) + "- " + e
				lines = insertBlock(lines, secEnd, line+"\n")
				dnsEnd++
				secEnd++
			}
		}
	}

	return strings.Join(lines, "\n")
}

func findYAMLBlock(lines []string, key string) (start, end, indent int) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == key+":" || (strings.HasPrefix(trimmed, key+":") && !strings.Contains(trimmed, "://") && !strings.Contains(trimmed[strings.Index(trimmed, ":")+1:], ":")) {
			indent = len(line) - len(strings.TrimLeft(line, " "))
			start = i
			end = len(lines)
			for j := i + 1; j < len(lines); j++ {
				if lines[j] == "" {
					continue
				}
				lv := len(lines[j]) - len(strings.TrimLeft(lines[j], " "))
				if lv <= indent && !strings.HasPrefix(strings.TrimSpace(lines[j]), "#") {
					end = j
					break
				}
			}
			return
		}
	}
	return -1, -1, 0
}

func findSubBlock(lines []string, key string, parentStart, parentEnd int) (start, end, indent int) {
	for i := parentStart; i < parentEnd && i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == key+":" {
			indent = len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
			start = i
			end = i + 1
			for j := i + 1; j < len(lines); j++ {
				trimmed2 := strings.TrimSpace(lines[j])
				if trimmed2 == "" || strings.HasPrefix(trimmed2, "#") {
					end = j + 1
					continue
				}
				lv := len(lines[j]) - len(strings.TrimLeft(lines[j], " "))
				if lv <= indent {
					end = j
					break
				}
				end = j + 1
			}
			return
		}
	}
	return -1, -1, 0
}

func collectListItems(lines []string, start, end int) map[string]bool {
	m := map[string]bool{}
	for i := start + 1; i < end && i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "- ") {
			ip := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			m[ip] = true
		}
	}
	return m
}

func insertBlock(lines []string, at int, block string) []string {
	newLines := strings.Split(block, "\n")
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}
	result := make([]string, 0, len(lines)+len(newLines))
	result = append(result, lines[:at]...)
	result = append(result, newLines...)
	result = append(result, lines[at:]...)
	return result
}

func findBin(name, prefer string) string {
	if prefer != "" {
		return prefer
	}
	dest := os.Getenv("TRIM_APPDEST")
	if dest == "" {
		dest = "/var/apps/mihomo"
	}
	for _, d := range []string{dest + "/app", dest} {
		p := d + "/" + name
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return name
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func seedGeoFiles() {
	os.MkdirAll(configDir, 0755)
	srcDir := appDest + "/app"
	for _, name := range []string{"geoip.metadb", "geosite.dat"} {
		src := srcDir + "/" + name
		dst := configDir + "/" + name
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			data, err := os.ReadFile(src)
			if err == nil {
				os.WriteFile(dst, data, 0644)
				log.Printf("seeded %s from package", name)
			}
		}
	}
}

func generateDefaultConfig() string {
	return `mixed-port: 7890
log-level: info
mode: rule
bind-address: "127.0.0.1"
external-controller: 127.0.0.1:19090
secret: ""
geo-auto-update: false

dns:
  enable: true
  listen: 127.0.0.1:53
  enhanced-mode: fake-ip
  default-nameserver:
    - 223.5.5.5
    - 114.114.114.114
    - 8.8.8.8
    - 100.100.100.100
  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
    - https://dns.google/dns-query
  direct-nameserver:
    - 223.5.5.5
    - 114.114.114.114
    - 8.8.8.8
    - 100.100.100.100
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter:
    - "*.lan"
    - "*.local"

proxies: []

proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - DIRECT

rules:
  - MATCH,DIRECT
`
}

func ensureDefaultProfile() {
	os.MkdirAll(profilesDir, 0755)
	profilePath := profilesDir + "/" + defaultProfile + ".yaml"
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		os.WriteFile(profilePath, []byte(generateDefaultConfig()), 0644)
		log.Printf("created default profile")
	}
	configPath := configDir + "/config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		data, _ := os.ReadFile(profilePath)
		os.WriteFile(configPath, data, 0644)
		os.WriteFile(activeFile, []byte(defaultProfile), 0644)
		log.Printf("initialized config from default profile")
	}
}

func listProfiles() ([]map[string]any, error) {
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return nil, err
	}
	activeBytes, _ := os.ReadFile(activeFile)
	active := strings.TrimSpace(string(activeBytes))

	var profiles []map[string]any
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		info, _ := e.Info()
		p := map[string]any{
			"name":   name,
			"active": name == active,
			"size":   info.Size(),
			"mod":    info.ModTime().Unix(),
		}
		if name == defaultProfile {
			p["builtin"] = true
		}
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i]["name"].(string) < profiles[j]["name"].(string)
	})
	return profiles, nil
}

func readProfile(name string) (string, error) {
	path := profilesDir + "/" + name + ".yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func extractConfiguredGroupNames(config string) []string {
	var names []string
	lines := strings.Split(config, "\n")
	inGroups := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "proxy-groups:") {
			inGroups = true
			continue
		}
		if inGroups {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.HasPrefix(trimmed, "- name:") {
				name := strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))
				name = strings.Trim(name, "\"'")
				names = append(names, name)
				continue
			}
			indent := len(line) - len(strings.TrimLeft(line, " "))
			if indent == 0 {
				break
			}
		}
	}
	return names
}

func handleGroups(w http.ResponseWriter, r *http.Request) {
	if !isMihomoRunning() {
		writeJSON(w, map[string]string{"error": "mihomo not running"})
		return
	}
	groupTypes := map[string]bool{
		"Selector": true, "URLTest": true, "Fallback": true, "LoadBalance": true,
	}
	directRejectSet := map[string]bool{
		"DIRECT": true, "REJECT": true, "GLOBAL": true,
	}
	configPath := configDir + "/config.yaml"
	data, _ := os.ReadFile(configPath)
	configured := extractConfiguredGroupNames(string(data))
	configuredSet := map[string]bool{}
	for _, n := range configured {
		configuredSet[n] = true
	}
	resp, err := http.Get(mihomoAPI + "/proxies")
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var proxiesResp map[string]any
	if err := json.Unmarshal(body, &proxiesResp); err != nil {
		writeJSON(w, map[string]string{"error": "invalid mihomo response"})
		return
	}
	allProxies, _ := proxiesResp["proxies"].(map[string]any)
	filtered := map[string]any{}
	types := map[string]string{}
	for name, p := range allProxies {
		pmap, _ := p.(map[string]any)
		ptype, _ := pmap["type"].(string)
		if !groupTypes[ptype] {
			continue
		}
		if directRejectSet[name] {
			continue
		}
		if len(configured) > 0 && !configuredSet[name] {
			continue
		}
		types[name] = ptype
		filteredMap := map[string]any{}
		for k, v := range pmap {
			filteredMap[k] = v
		}
		if rawAll, ok := pmap["all"].([]any); ok {
			var cleanAll []any
			for _, item := range rawAll {
				if s, ok := item.(string); ok {
					if directRejectSet[s] {
						continue
					}
					if _, isGroup := allProxies[s].(map[string]any); isGroup {
						if gt, _ := allProxies[s].(map[string]any)["type"].(string); groupTypes[gt] {
							cleanAll = append(cleanAll, item)
							continue
						}
					}
				}
				cleanAll = append(cleanAll, item)
			}
			filteredMap["all"] = cleanAll
		}
		filtered[name] = filteredMap
	}
	writeJSON(w, map[string]any{"proxies": filtered, "types": types})
}

func handleSelectProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	if !isMihomoRunning() {
		writeJSON(w, map[string]string{"error": "mihomo not running"})
		return
	}
	var req struct {
		Group string `json:"group"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]string{"error": "invalid request"})
		return
	}
	if req.Group == "" || req.Name == "" {
		writeJSON(w, map[string]string{"error": "group and name required"})
		return
	}
	body := fmt.Sprintf(`{"name":%q}`, req.Name)
	req2, _ := http.NewRequest(http.MethodPut, mihomoAPI+"/proxies/"+url.PathEscape(req.Group), strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req2)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		errMsg := strings.TrimSpace(string(respBody))
		if errMsg == "" {
			errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		writeJSON(w, map[string]string{"error": errMsg})
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "", 405)
		return
	}
	if !isMihomoRunning() {
		writeJSON(w, map[string]string{"error": "mihomo not running"})
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/delay/"), "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "", 404)
		return
	}
	kind := parts[0]
	name := parts[1]
	testURL := r.URL.Query().Get("url")
	if testURL == "" {
		testURL = "http://www.gstatic.com/generate_204"
	}
	timeout := r.URL.Query().Get("timeout")
	if timeout == "" {
		timeout = "5000"
	}
	var apiPath string
	if kind == "group" {
		apiPath = fmt.Sprintf("/group/%s/delay?url=%s&timeout=%s", url.PathEscape(name), url.QueryEscape(testURL), timeout)
	} else {
		apiPath = fmt.Sprintf("/proxies/%s/delay?url=%s&timeout=%s", url.PathEscape(name), url.QueryEscape(testURL), timeout)
	}
	resp, err := http.Get(mihomoAPI + apiPath)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	if resp.StatusCode >= 400 {
		var m map[string]any
		if json.Unmarshal(respBody, &m) == nil {
			if msg, ok := m["message"].(string); ok {
				writeJSON(w, map[string]string{"error": "测速失败: " + msg})
				return
			}
		}
		writeJSON(w, map[string]string{"error": strings.TrimSpace(string(respBody))})
		return
	}
	var m map[string]any
	if json.Unmarshal(respBody, &m) != nil {
		writeJSON(w, map[string]string{"error": "invalid mihomo response"})
		return
	}
	if _, hasDelay := m["delay"]; hasDelay {
		w.Write(respBody)
		return
	}
	if msg, ok := m["message"]; ok {
		switch v := msg.(type) {
		case map[string]any:
			w.Write(respBody)
			return
		case string:
			writeJSON(w, map[string]string{"error": "测速失败: " + v})
			return
		case float64:
			w.Write(respBody)
			return
		}
	}
	if len(m) > 0 {
		firstKey := ""
		for k := range m {
			firstKey = k
			break
		}
		if firstKey != "" {
			if v, ok := m[firstKey]; ok {
				switch v.(type) {
				case float64, map[string]any:
					normalized := map[string]any{"message": m}
					nb, _ := json.Marshal(normalized)
					w.Write(nb)
					return
				}
			}
		}
	}
	w.Write(respBody)
}

func writeProfile(name, content string) error {
	path := profilesDir + "/" + name + ".yaml"
	return os.WriteFile(path, []byte(content), 0644)
}

func deleteProfile(name string) error {
	if name == defaultProfile {
		return fmt.Errorf("cannot delete default profile")
	}
	path := profilesDir + "/" + name + ".yaml"
	return os.Remove(path)
}

func activateProfile(name string) error {
	content, err := readProfile(name)
	if err != nil {
		return fmt.Errorf("profile not found: %s", name)
	}
	configPath := configDir + "/config.yaml"
	cfg := applyConfigOverrides(content)
	if err := os.WriteFile(configPath, []byte(cfg), 0644); err != nil {
		return err
	}
	os.WriteFile(activeFile, []byte(name), 0644)
	if isMihomoRunning() {
		reloadBody := fmt.Sprintf(`{"path":"%s","payload":""}`, configPath)
		rr, _ := http.NewRequest(http.MethodPut, mihomoAPI+"/configs", strings.NewReader(reloadBody))
		rr.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(rr)
		if err != nil {
			return fmt.Errorf("配置重载失败: %v", err)
		}
		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			errMsg := strings.TrimSpace(string(respBytes))
			return fmt.Errorf("mihomo 配置错误: %s", errMsg)
		}
	} else if currentSettings.ServiceEnabled {
		if err := startMihomo(); err != nil {
			return fmt.Errorf("启动失败: %v", err)
		}
	}
	return nil
}

// ─── Process lifecycle ──────────────────────────────────────────

// stopMihomoLocked assumes opMu is held by caller.
func stopMihomoLocked() {
	mihomoMu.Lock()
	cmd := mihomoCmd
	mihomoCmd = nil
	mihomoMu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	cmd.Process.Signal(syscall.SIGTERM)
	go func() {
		time.Sleep(5 * time.Second)
		cmd.Process.Kill()
	}()
	cmd.Wait()
}

func stopMihomo() {
	opMu.Lock()
	defer opMu.Unlock()
	stopMihomoLocked()
}

// startMihomoLocked assumes opMu is held by caller.
func startMihomoLocked() error {
	mihomoMu.Lock()
	if mihomoCmd != nil {
		mihomoMu.Unlock()
		return fmt.Errorf("already running")
	}
	mihomoMu.Unlock()

	configPath := configDir + "/config.yaml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	cfg := applyConfigOverrides(string(data))
	if err := os.WriteFile(configPath, []byte(cfg), 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	var mihomoWriters []io.Writer
	mihomoWriters = append(mihomoWriters, &mihomoLogBuf)
	if mihomoLogPath != "" {
		f, err := os.OpenFile(mihomoLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			mihomoWriters = append(mihomoWriters, f)
		}
	}
	mw := io.MultiWriter(mihomoWriters...)

	cmd := exec.Command(mihomoBin, "-d", configDir)
	cmd.Stdout = mw
	cmd.Stderr = mw
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mihomo: %w", err)
	}
	log.Printf("mihomo started (pid %d)", cmd.Process.Pid)

	mihomoMu.Lock()
	mihomoCmd = cmd
	mihomoMu.Unlock()

	go func() {
		err := cmd.Wait()
		exitCode := 0
		if err != nil {
			exitCode = 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
		}
		log.Printf("mihomo exited (pid %d, code %d): %v", cmd.Process.Pid, exitCode, err)
		mihomoMu.Lock()
		if mihomoCmd == cmd {
			mihomoCmd = nil
		}
		mihomoMu.Unlock()
	}()

	go func() {
		for i := 0; i < 30; i++ {
			resp, err := http.Get(mihomoAPI + "/version")
			if err == nil {
				resp.Body.Close()
				log.Printf("mihomo API ready")
				return
			}
			time.Sleep(time.Second)
		}
		log.Printf("warning: mihomo API not ready after 30s, running in degraded mode")
	}()

	return nil
}

func startMihomo() error {
	opMu.Lock()
	defer opMu.Unlock()
	return startMihomoLocked()
}

func restartMihomo() {
	opMu.Lock()
	defer opMu.Unlock()
	stopMihomoLocked()
	if err := startMihomoLocked(); err != nil {
		log.Printf("restart failed: %v", err)
	}
}

// ─── Main ────────────────────────────────────────────────────────

func main() {
	if p := os.Getenv("MIHOMO_SERVICE_PORT"); p != "" {
		servicePort = p
	}
	if p := os.Getenv("MIHOMO_API_PORT"); p != "" {
		mihomoAPI = "http://127.0.0.1:" + p
	}

	configDir = appVar + "/config"
	profilesDir = configDir + "/profiles"
	activeFile = configDir + "/.active"
	settingsPath = configDir + "/settings.json"
	mihomoLogPath = appVar + "/mihomo.log"

	log.SetOutput(&logBuf)

	seedGeoFiles()
	ensureDefaultProfile()

	currentSettings = loadSettings()

	if currentSettings.SystemProxyEnabled {
		applySystemProxy(true)
	}

	if currentSettings.ServiceEnabled {
		if err := startMihomo(); err != nil {
			log.Printf("warning: failed to start mihomo: %v", err)
		}
	} else {
		log.Printf("mihomo service disabled by settings")
	}
	defer stopMihomo()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		stopMihomo()
		os.Exit(0)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveUI)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/log/app", handleAppLog)
	mux.HandleFunc("/api/log/mihomo", handleMihomoLog)
	mux.HandleFunc("/api/log", handleAppLog)
	mux.HandleFunc("/api/events", handleEvents)
	mux.HandleFunc("/api/restart", handleRestart)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/profiles", handleProfiles)
	mux.HandleFunc("/api/profiles/", handleProfiles)
	mux.HandleFunc("/api/proxy/", handleProxy)
	mux.HandleFunc("/api/groups", handleGroups)
	mux.HandleFunc("/api/select", handleSelectProxy)
	mux.HandleFunc("/api/delay/", handleDelay)
	mux.HandleFunc("/api/settings", handleSettings)
	mux.HandleFunc("/api/service/start", handleServiceStart)
	mux.HandleFunc("/api/service/stop", handleServiceStop)
	mux.HandleFunc("/api/validate", handleValidate)

	port := servicePort
	var l net.Listener
	for i := 0; i < 100; i++ {
		var err error
		l, err = net.Listen("tcp", ":"+port)
		if err == nil {
			os.WriteFile("/tmp/mihomo-port", []byte(port), 0644)
			break
		}
		p, _ := strconv.Atoi(port)
		port = strconv.Itoa(p + 1)
	}
	if l == nil {
		log.Fatal("no available port")
	}
	log.Printf("server listening on :%s", port)
	http.Serve(l, mux)
}

// ─── Handlers ────────────────────────────────────────────────────

func serveUI(w http.ResponseWriter, r *http.Request) {
	data, err := frontend.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func proxyToMihomo(w http.ResponseWriter, r *http.Request, path string) {
	target, _ := url.Parse(mihomoAPI)
	proxy := httputil.NewSingleHostReverseProxy(target)
	r.URL.Path = path
	r.Host = target.Host
	proxy.ServeHTTP(w, r)
}

func handleProxy(w http.ResponseWriter, r *http.Request) {
	if !isMihomoRunning() {
		writeJSON(w, map[string]string{"error": "mihomo not running"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/proxy")
	proxyToMihomo(w, r, path)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	activeBytes, _ := os.ReadFile(activeFile)
	activeProfile := strings.TrimSpace(string(activeBytes))
	if activeProfile == "" {
		activeProfile = defaultProfile
	}

	resp := map[string]any{
		"profile": activeProfile,
	}
	if !isMihomoRunning() {
		resp["running"] = false
		writeJSON(w, resp)
		return
	}

	rpcResp, err := http.Get(mihomoAPI + "/version")
	if err != nil {
		resp["running"] = true
		resp["api"] = false
		writeJSON(w, resp)
		return
	}
	defer rpcResp.Body.Close()
	verBody, _ := io.ReadAll(rpcResp.Body)
	verStr := strings.TrimSpace(string(verBody))
	var verData map[string]any
	if err := json.Unmarshal([]byte(verStr), &verData); err == nil {
		if v, ok := verData["version"].(string); ok {
			verStr = strings.TrimPrefix(v, "v")
		}
	}

	trafficResp, trafficErr := http.Get(mihomoAPI + "/traffic")
	var rx, tx float64
	if trafficErr == nil {
		defer trafficResp.Body.Close()
		var t map[string]float64
		json.NewDecoder(trafficResp.Body).Decode(&t)
		rx, _ = t["up"]
		tx, _ = t["down"]
	}

	var mode string
	configResp, configErr := http.Get(mihomoAPI + "/configs")
	if configErr == nil {
		defer configResp.Body.Close()
		var c map[string]any
		json.NewDecoder(configResp.Body).Decode(&c)
		if m, ok := c["mode"].(string); ok {
			mode = m
		}
	}

	resp["running"] = true
	resp["api"] = true
	resp["version"] = verStr
	resp["mode"] = mode
	resp["rx"] = rx
	resp["tx"] = tx
	writeJSON(w, resp)
}

func readLog(buf *LogBuffer, maxLines int) string {
	out := buf.String()
	if len(out) == 0 {
		return "no logs"
	}
	lines := strings.Split(out, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func handleAppLog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"log": readLog(&logBuf, 500)})
}

func handleMihomoLog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"log": readLog(&mihomoLogBuf, 500)})
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f, _ := w.(http.Flusher)

	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			if !isMihomoRunning() {
				continue
			}
			resp, err := http.Get(mihomoAPI + "/traffic")
			if err == nil {
				var t map[string]float64
				json.NewDecoder(resp.Body).Decode(&t)
				resp.Body.Close()
				rx, _ := t["up"]
				tx, _ := t["down"]
				fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"rx": rx, "tx": tx}))
				f.Flush()
			}

			resp2, err2 := http.Get(mihomoAPI + "/proxies")
			if err2 == nil {
				var p map[string]any
				json.NewDecoder(resp2.Body).Decode(&p)
				resp2.Body.Close()
				if proxies, ok := p["proxies"].(map[string]any); ok {
					var proxyList []map[string]any
					for name, proxy := range proxies {
						if proxyMap, ok := proxy.(map[string]any); ok {
							proxyMap["name"] = name
							proxyList = append(proxyList, proxyMap)
						}
					}
					fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"type": "proxies", "proxies": proxyList}))
					f.Flush()
				}
			}
		}
	}
}

func handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	go restartMihomo()
	writeJSON(w, map[string]string{"status": "restarting"})
}

func validateConfigYAML(content string) string {
	if strings.TrimSpace(content) == "" {
		return "配置内容为空"
	}
	lines := strings.Split(content, "\n")
	topKeys := map[string]bool{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "  ") && strings.Contains(trimmed, ":") {
			key := strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[0])
			topKeys[key] = true
		}
	}
	if !topKeys["mixed-port"] && !topKeys["port"] && !topKeys["socks-port"] && !topKeys["redir-port"] {
		return "配置缺少端口设置: 需要至少一个 mixed-port、port、socks-port 或 redir-port"
	}
	portLine := ""
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "mixed-port:") {
			portLine = strings.TrimSpace(line)
			break
		}
	}
	if portLine != "" {
		parts := strings.SplitN(portLine, ":", 2)
		if len(parts) == 2 {
			v := strings.TrimSpace(parts[1])
			if _, err := strconv.Atoi(v); err != nil {
				return "mixed-port 值必须是数字，当前值: " + v
			}
		}
	}
	var prevIndent int
	for i, line := range lines {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if i > 0 && indent > prevIndent+2 {
			return fmt.Sprintf("第 %d 行缩进错误: 层级跳变过大 (期望 %d 格缩进，实际 %d 格)", i+1, prevIndent+2, indent)
		}
		prevIndent = indent
	}
	return ""
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		configPath := configDir + "/config.yaml"
		data, err := os.ReadFile(configPath)
		if err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]string{"config": string(data)})
	case "POST":
		var req struct {
			Config string `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		if errMsg := validateConfigYAML(req.Config); errMsg != "" {
			writeJSON(w, map[string]string{"error": errMsg})
			return
		}
		configPath := configDir + "/config.yaml"
		if err := os.WriteFile(configPath, []byte(req.Config), 0644); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		if isMihomoRunning() {
			cfg := applyConfigOverrides(req.Config)
			os.WriteFile(configPath, []byte(cfg), 0644)
			body := fmt.Sprintf(`{"path":"%s","payload":""}`, configPath)
			rr, _ := http.NewRequest(http.MethodPut, mihomoAPI+"/configs", strings.NewReader(body))
			rr.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(rr)
			if err != nil {
				writeJSON(w, map[string]string{"status": "saved", "reload": "failed", "error": err.Error()})
				return
			}
			respBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				errMsg := strings.TrimSpace(string(respBytes))
				if errMsg == "" {
					errMsg = fmt.Sprintf("mihomo 返回错误 (HTTP %d)", resp.StatusCode)
				}
				data, _ := os.ReadFile(configPath)
				writeJSON(w, map[string]string{
					"status": "saved",
					"reload": "failed",
					"error":  errMsg,
					"config": string(data),
				})
				return
			}
		}
		writeJSON(w, map[string]string{"status": "saved", "reload": "ok"})
	default:
		http.Error(w, "", 405)
	}
}

func handleProfiles(w http.ResponseWriter, r *http.Request) {
	subpath := strings.TrimPrefix(r.URL.Path, "/api/profiles")
	subpath = strings.TrimPrefix(subpath, "/")

	switch r.Method {
	case "GET":
		if subpath != "" {
			name := strings.TrimSuffix(subpath, "/")
			content, err := readProfile(name)
			if err != nil {
				writeJSON(w, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, map[string]string{"name": name, "content": content})
			return
		}
		profiles, err := listProfiles()
		if err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"profiles": profiles})
	case "POST":
		switch subpath {
		case "activate":
			var req struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]string{"error": "invalid request"})
				return
			}
			if req.Name == "" {
				writeJSON(w, map[string]string{"error": "name required"})
				return
			}
			if err := activateProfile(req.Name); err != nil {
				writeJSON(w, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, map[string]string{"status": "activated", "profile": req.Name})
		default:
			var req struct {
				Name    string `json:"name"`
				Content string `json:"content"`
				URL     string `json:"url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, map[string]string{"error": "invalid request"})
				return
			}
			if req.Name == "" {
				writeJSON(w, map[string]string{"error": "name required"})
				return
			}
			if req.URL != "" {
				go importFromURL(req.Name, req.URL)
				writeJSON(w, map[string]string{"status": "importing", "profile": req.Name})
				return
			}
			if req.Content == "" {
				writeJSON(w, map[string]string{"error": "content or url required"})
				return
			}
			if errMsg := validateConfigYAML(req.Content); errMsg != "" {
				writeJSON(w, map[string]string{"error": errMsg})
				return
			}
			if err := writeProfile(req.Name, req.Content); err != nil {
				writeJSON(w, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, map[string]string{"status": "created", "profile": req.Name})
		}
	case "PUT":
		parts := strings.SplitN(subpath, "/", 2)
		name := parts[0]
		if name == "" {
			writeJSON(w, map[string]string{"error": "name required"})
			return
		}
		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]string{"error": "invalid request"})
			return
		}
		if errMsg := validateConfigYAML(req.Content); errMsg != "" {
			writeJSON(w, map[string]string{"error": errMsg})
			return
		}
		if err := writeProfile(name, req.Content); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		activeBytes, _ := os.ReadFile(activeFile)
		active := strings.TrimSpace(string(activeBytes))
		if active == name {
			configPath := configDir + "/config.yaml"
			cfg := applyConfigOverrides(req.Content)
			os.WriteFile(configPath, []byte(cfg), 0644)
			if isMihomoRunning() {
				reloadBody := fmt.Sprintf(`{"path":"%s","payload":""}`, configPath)
				rr, _ := http.NewRequest(http.MethodPut, mihomoAPI+"/configs", strings.NewReader(reloadBody))
				rr.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(rr)
				if err != nil {
					writeJSON(w, map[string]string{"status": "saved", "reload": "failed", "error": "重载配置失败: " + err.Error()})
					return
				}
				respBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					errMsg := strings.TrimSpace(string(respBytes))
					if errMsg == "" {
						errMsg = fmt.Sprintf("mihomo 配置错误 (HTTP %d)", resp.StatusCode)
					}
					data, _ := os.ReadFile(configPath)
					writeJSON(w, map[string]string{
						"status": "saved",
						"reload": "failed",
						"error":  errMsg,
						"config": string(data),
					})
					return
				}
				writeJSON(w, map[string]string{"status": "saved", "reload": "ok"})
				return
			}
		}
		writeJSON(w, map[string]string{"status": "saved", "profile": name})
	case "DELETE":
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]string{"error": "invalid request"})
			return
		}
		if req.Name == "" {
			writeJSON(w, map[string]string{"error": "name required"})
			return
		}
		if err := deleteProfile(req.Name); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]string{"status": "deleted", "profile": req.Name})
	default:
		http.Error(w, "", 405)
	}
}

func importFromURL(name, rawURL string) {
	log.Printf("importing profile %s from %s", name, rawURL)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		log.Printf("import %s failed: %v", name, err)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("import %s read failed: %v", name, err)
		return
	}
	if err := writeProfile(name, string(data)); err != nil {
		log.Printf("import %s write failed: %v", name, err)
		return
	}
	log.Printf("imported profile %s (%d bytes)", name, len(data))
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s := loadSettings()
		writeJSON(w, map[string]any{
			"service_enabled":     s.ServiceEnabled,
			"tun_enabled":         s.TUNEnabled,
			"local_proxy_only":    s.LocalProxyOnly,
			"system_proxy_enabled": s.SystemProxyEnabled,
			"service_running":     isMihomoRunning(),
		})
	case "POST":
		var s Settings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeJSON(w, map[string]string{"error": "invalid request"})
			return
		}
		prev := currentSettings
		saveSettings(s)
		applySystemProxy(s.SystemProxyEnabled)
		writeJSON(w, map[string]string{"status": "saved"})
		if s.TUNEnabled != prev.TUNEnabled || s.LocalProxyOnly != prev.LocalProxyOnly {
			if isMihomoRunning() {
				go restartMihomo()
			}
		}
	default:
		http.Error(w, "", 405)
	}
}

func handleServiceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	if isMihomoRunning() {
		writeJSON(w, map[string]string{"status": "already_running"})
		return
	}
	if err := startMihomo(); err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"status": "started"})
}

func handleServiceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	if !isMihomoRunning() {
		writeJSON(w, map[string]string{"status": "already_stopped"})
		return
	}
	stopMihomo()
	writeJSON(w, map[string]string{"status": "stopped"})
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	var req struct {
		Config string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"valid": false, "error": "无效请求"})
		return
	}
	if errMsg := validateConfigYAML(req.Config); errMsg != "" {
		writeJSON(w, map[string]any{"valid": false, "error": errMsg})
		return
	}
	writeJSON(w, map[string]any{"valid": true})
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ─── Thread-safe log buffer ──────────────────────────────────────

type LogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.Write(p)
}

func (lb *LogBuffer) String() string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.String()
}
