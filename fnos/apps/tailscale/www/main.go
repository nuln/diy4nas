package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed ui/index.html ui/vis-network.min.js
var frontend embed.FS

var sockPath = getEnv("TAILSCALE_SOCKET", "/var/apps/tailscale/data/tailscaled.sock")
var tsBin = findBin("tailscale", getEnv("TAILSCALE_BIN", ""))
var tsdBin = findBin("tailscaled", getEnv("TAILSCALED_BIN", ""))
var appVar = getEnv("TRIM_PKGVAR", "/var/apps/tailscale/data")
var stateFile = appVar + "/tailscaled.state"

var upLock sync.Mutex
var lastUpErr string
var lastUpOut string
var lastUpFlags []string

var logBuf bytes.Buffer
var logMu sync.Mutex
var appLog bytes.Buffer
var appLogMu sync.Mutex
var logFile *os.File
var tsdCmd *exec.Cmd

var (
	cacheMu           sync.Mutex
	cachedStatus      []byte
	cachedStatusErr   error
	cachedNetcheck    []byte
	cachedNetcheckErr error
)

func fetchStatus() ([]byte, error) {
	out, err := ts("status", "--json")
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if err == nil {
		cachedStatus = out
		cachedStatusErr = nil
	} else {
		cachedStatusErr = err
	}
	return out, err
}

func fetchNetcheckData() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tsBin, "--socket="+sockPath, "netcheck", "--format=json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.Bytes()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if err != nil {
		errStr := err.Error()
		if stderr.Len() > 0 {
			errStr += ": " + strings.TrimSpace(stderr.String())
		}
		appLogf("netcheck 失败: %s", errStr)

		if len(out) > 0 {
			cachedNetcheck = out
			cachedNetcheckErr = nil
			return out, nil
		}

		cachedNetcheckErr = fmt.Errorf("%s", errStr)
		return nil, cachedNetcheckErr
	}

	cachedNetcheck = out
	cachedNetcheckErr = nil
	return out, nil
}

func startBackgroundFetchers() {
	go func() {
		fetchStatus()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			fetchStatus()
		}
	}()

	go func() {
		time.Sleep(2 * time.Second)
		fetchNetcheckData()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			fetchNetcheckData()
		}
	}()
}

func appLogf(format string, args ...any) {
	appLogMu.Lock()
	defer appLogMu.Unlock()
	appLog.WriteString(time.Now().Format("01-02 15:04:05 ") + fmt.Sprintf(format, args...) + "\n")
}

func findBin(name, prefer string) string {
	if prefer != "" { return prefer }
	dest := os.Getenv("TRIM_APPDEST")
	if dest == "" { dest = "/var/apps/tailscale" }
	for _, d := range []string{dest + "/app", dest} {
		p := d + "/" + name
		if _, err := os.Stat(p); err == nil { return p }
	}
	return name
}

func setLastUpErr(err, out string) {
	upLock.Lock()
	defer upLock.Unlock()
	lastUpErr = err
	lastUpOut = out
}

func getAndClearLastUpErr() (string, string) {
	upLock.Lock()
	defer upLock.Unlock()
	err := lastUpErr
	out := lastUpOut
	lastUpErr = ""
	lastUpOut = ""
	return err, out
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" { return v }
	return def
}
func initLog() {
	os.MkdirAll(appVar, 0755)
	f, err := os.OpenFile(appVar+"/server.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Printf("initLog: open error: %v", err)
		return
	}
	logFile = f
	existing, _ := os.ReadFile(appVar + "/server.log")
	if len(existing) > 0 {
		logBuf.Write(existing)
		if !bytes.HasSuffix(existing, []byte("\n")) {
			logBuf.Write([]byte("\n"))
		}
	}
}
func writeLogf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...) + "\n"
	logBuf.Write([]byte(msg))
	if logFile != nil {
		logFile.Write([]byte(msg))
	}
	appLogf(format, args...)
}

func runCLI(args ...string) (string, error) {
	fullArgs := append([]string{"--socket=" + sockPath}, args...)
	cmd := exec.Command(tsBin, fullArgs...)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func startTailscaled() error {
	os.MkdirAll(appVar, 0755)
	exec.Command("modprobe", "tun").Run()
	os.Remove(sockPath)

	writeLogf("启动 tailscaled: bin=%s sock=%s state=%s port=41641", tsdBin, sockPath, stateFile)

	tsdCmd = exec.Command(tsdBin,
		"--state="+stateFile,
		"--socket="+sockPath,
		"--port=41641",
	)

	tsdCmd.Env = os.Environ()

	var writers []io.Writer
	writers = append(writers, &logBuf)
	if logFile != nil {
		writers = append(writers, logFile)
	}
	mw := io.MultiWriter(writers...)
	tsdCmd.Stdout = mw
	tsdCmd.Stderr = mw
	if err := tsdCmd.Start(); err != nil {
		return fmt.Errorf("start tailscaled: %w", err)
	}
	writeLogf("tailscaled 已启动 (pid %d)", tsdCmd.Process.Pid)

	// 等待 socket 就绪
	for i := 0; i < 10; i++ {
		s, stErr := os.Stat(sockPath)
		if s != nil {
			writeLogf("socket 就绪 (尝试 %d)", i+1)
			time.Sleep(500 * time.Millisecond)

			// 用 tailscale CLI 连接
			upOut, upErr := 			runCLI("up", "--accept-routes", "--reset", "--operator=www-data")
			if upErr != nil {
				writeLogf("自动连接失败: %v\n%s", upErr, upOut)
			} else {
				writeLogf("自动连接成功")
			}

			writeLogf("auto-up done")
			return nil
		}
		writeLogf("socket 未就绪 (尝试 %d): %v", i+1, stErr)
		time.Sleep(time.Second)
	}

	return fmt.Errorf("tailscaled socket not ready after 10s")
}

func stopTailscaled() {
	if tsdCmd != nil && tsdCmd.Process != nil {
		tsdCmd.Process.Signal(syscall.SIGTERM)
		go func() { time.Sleep(5 * time.Second); tsdCmd.Process.Kill() }()
		tsdCmd.Wait()
	}
}

func main() {
	initLog()
	port := os.Getenv("TAILSCALE_PORT")
	if port == "" { port = "8088" }

	// tailscaled 在后台 goroutine 启动，不阻塞 HTTP 服务
	// HTTP 服务立即启动，UI 可正常访问；tailscaled 就绪前 status 显示未连接
	go func() {
		if err := startTailscaled(); err != nil {
			writeLogf("tailscaled 启动失败: %v", err)
		}
	}()

	// Graceful shutdown on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		stopTailscaled()
		os.Exit(0)
	}()
	defer stopTailscaled() // 安全网：main 异常退出时也尝试停 tailscaled

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveUI)
	mux.HandleFunc("/vis-network.min.js", serveVisNetwork)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/traffic", handleTraffic)
	mux.HandleFunc("/api/up", handleUp)
	mux.HandleFunc("/api/down", handleDown)
	mux.HandleFunc("/api/logout", handleLogout)
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/log", handleLog)
	mux.HandleFunc("/api/netcheck", handleNetcheck)
	mux.HandleFunc("/api/whois", handleWhois)
	mux.HandleFunc("/api/dns/status", handleDNSStatus)
	mux.HandleFunc("/api/ip", handleIP)
	mux.HandleFunc("/api/profile", handleProfile)
	mux.HandleFunc("/api/profile/switch", handleProfileSwitch)
	mux.HandleFunc("/api/file/list", handleFileList)
	mux.HandleFunc("/api/file/get", handleFileGet)
	mux.HandleFunc("/api/serve/status", handleServeStatus)
	mux.HandleFunc("/api/serve/set", handleServeSet)
	mux.HandleFunc("/api/cert", handleCert)
	mux.HandleFunc("/api/metrics", handleMetrics)
	mux.HandleFunc("/api/appc-routes", handleAppcRoutes)
	mux.HandleFunc("/api/nc", handleNC)
	mux.HandleFunc("/api/bugreport", handleBugReport)
	mux.HandleFunc("/api/update/check", handleUpdateCheck)
	mux.HandleFunc("/api/log/ts", handleLogTs)
	mux.HandleFunc("/api/log/app", handleLogApp)
	mux.HandleFunc("/api/log/clear", handleLogClear)
	mux.HandleFunc("/api/events", handleEvents)
	mux.HandleFunc("/api/healthz", handleHealthz)

	startBackgroundFetchers()

	for i := 0; i < 100; i++ {
		addr := fmt.Sprintf(":%s", port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			os.WriteFile("/tmp/tailscale-port", []byte(port), 0644)
			log.Printf("listening on %s", addr)
			http.Serve(l, mux)
			return
		}
		p, _ := strconv.Atoi(port)
		port = strconv.Itoa(p + 1)
	}
	log.Fatal("no available port")
}

func serveUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && len(r.URL.Path) < 20 {
		http.NotFound(w, r); return
	}
	data, err := frontend.ReadFile("ui/index.html")
	if err != nil { http.Error(w, "not found", 404); return }
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func serveVisNetwork(w http.ResponseWriter, r *http.Request) {
	data, err := frontend.ReadFile("ui/vis-network.min.js")
	if err != nil { http.Error(w, "not found", 404); return }
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Write(data)
}

func ts(args ...string) ([]byte, error) { return exec.Command(tsBin, append([]string{"--socket="+sockPath}, args...)...).Output() }

func handleStatus(w http.ResponseWriter, r *http.Request) {
	upErr, upOut := getAndClearLastUpErr()
	force := r.URL.Query().Get("force") == "true"
	var out []byte
	var e error
	if force {
		out, e = fetchStatus()
	} else {
		cacheMu.Lock()
		out = cachedStatus
		e = cachedStatusErr
		cacheMu.Unlock()
	}

	if e != nil && len(out) == 0 {
		appLogf("status 失败: %v", e)
		resp := map[string]any{"online":false,"daemon":false,"error":e.Error()}
		if upErr != "" {
			resp["upError"] = upErr; resp["upOutput"] = upOut
		}
		writeJSON(w, resp); return
	}

	var raw map[string]any; json.Unmarshal(out, &raw)
	if raw == nil { raw = make(map[string]any) }
	bs, _ := raw["BackendState"].(string)
	self, _ := raw["Self"].(map[string]any)
	// 检查 IP：看顶层 TailscaleIPs 或 Self.TailscaleIPs（TailAddr 不是真实字段）
	tsIPs, _ := raw["TailscaleIPs"].([]any)
	hasIP := len(tsIPs) > 0
	if !hasIP && self != nil {
		if ips, ok := self["TailscaleIPs"].([]any); ok && len(ips) > 0 { hasIP = true }
	}
	if self == nil || !hasIP {
		resp := map[string]any{"online":false,"daemon":true,"backendState":bs}
		if self != nil && bs != "NeedsLogin" && bs != "NoState" {
			// 已认证过但丢了连接（如 admin 移除），或等待激活
			resp["Self"] = self
			if dns, _ := self["DNSName"].(string); dns != "" {
				if active, ok := self["Active"].(bool); ok && !active {
					resp["pending"] = true
				}
			}
		}
		// BackendState=NeedsLogin/NoState：不返回 Self → 前端 hasSelf=false → 显示登录页
		if upErr != "" { resp["upError"] = upErr; resp["upOutput"] = upOut }
		writeJSON(w, resp); return
	}
	if self != nil {
		keys := []string{}
		for k := range self {
			keys = append(keys, k)
		}
		appLogf("[DEBUG] Self keys: %s", strings.Join(keys, ", "))
		if hi, ok := self["HostInfo"].(map[string]any); ok {
			hiKeys := []string{}
			for k := range hi {
				hiKeys = append(hiKeys, k)
			}
			appLogf("[DEBUG] HostInfo keys: %s", strings.Join(hiKeys, ", "))
			appLogf("[DEBUG] RoutableIPs: %v", hi["RoutableIPs"])
		} else if hi, ok := self["Hostinfo"].(map[string]any); ok {
			hiKeys := []string{}
			for k := range hi {
				hiKeys = append(hiKeys, k)
			}
			appLogf("[DEBUG] Hostinfo keys: %s", strings.Join(hiKeys, ", "))
			appLogf("[DEBUG] RoutableIPs: %v", hi["RoutableIPs"])
		} else {
			appLogf("[DEBUG] HostInfo/Hostinfo is missing or not a map")
		}
		appLogf("[DEBUG] PrimaryRoutes: %v", self["PrimaryRoutes"])
		appLogf("[DEBUG] AllowedIPs: %v", self["AllowedIPs"])
	}
	raw["online"] = (bs == "Running"); raw["totalRx"], raw["totalTx"] = calcTraffic(raw)
	writeJSON(w, raw)
}

func handleTraffic(w http.ResponseWriter, r *http.Request) {
	cacheMu.Lock()
	out := cachedStatus
	e := cachedStatusErr
	cacheMu.Unlock()

	if e != nil && len(out) == 0 {
		writeJSON(w, map[string]any{"rx":0,"tx":0})
		return
	}
	var raw map[string]any; json.Unmarshal(out, &raw)
	if raw == nil { raw = map[string]any{} }
	rx, tx := calcTraffic(raw); writeJSON(w, map[string]any{"rx":rx,"tx":tx})
}

func handleUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, "", 405); return }
	b, _ := io.ReadAll(r.Body)
	var req struct {
		Reconnect bool `json:"reconnect"`
		AuthKey, Hostname, Routes, LoginServer, ExitNode, NetfilterMode string
		AcceptDNS, ShieldsUp, AcceptRoutes, AdvertiseExitNode, Ssh, ExitNodeAllowLan, SnatSubnetRoutes, StatefulFiltering bool
	}
	json.Unmarshal(b, &req); a := []string{"up","--accept-risk=all","--operator=www-data"}
	writeLogf("up: reconnect=%v hostname=%q routes=%q loginServer=%q exitNode=%q authKey=%v", req.Reconnect, req.Hostname, req.Routes, req.LoginServer, req.ExitNode, req.AuthKey != "")
	if !req.Reconnect {
		if req.AuthKey != "" { a = append(a, "--authkey="+req.AuthKey) }
		if req.Hostname != "" { a = append(a, "--hostname="+req.Hostname) }
		if req.Routes != "" { a = append(a, "--advertise-routes="+req.Routes) }
		if req.LoginServer != "" { a = append(a, "--login-server="+req.LoginServer) }
		if req.ExitNode != "" { a = append(a, "--exit-node="+req.ExitNode) }
		if req.ExitNodeAllowLan { a = append(a, "--exit-node-allow-lan-access") }
		if req.AdvertiseExitNode {
			a = append(a, "--advertise-exit-node")
		} else {
			a = append(a, "--advertise-exit-node=false")
		}
		if req.Ssh {
			a = append(a, "--ssh")
		} else {
			a = append(a, "--ssh=false")
		}
		if !req.AcceptDNS { a = append(a, "--accept-dns=false") }
		if req.ShieldsUp { a = append(a, "--shields-up") }
		if !req.AcceptRoutes { a = append(a, "--accept-routes=false") }
		if !req.SnatSubnetRoutes { a = append(a, "--snat-subnet-routes=false") }
		if !req.StatefulFiltering { a = append(a, "--stateful-filtering=false") }
		if req.NetfilterMode != "" { a = append(a, "--netfilter-mode="+req.NetfilterMode) }
		// 保存 flags（不含 authkey，一次性密钥重连不可用）
		flags := []string{"up"}
		for _, f := range a[1:] {
			if !strings.HasPrefix(f, "--authkey=") {
				flags = append(flags, f)
			}
		}
		lastUpFlags = flags
	}

	setLastUpErr("", "")

	if req.Reconnect {
		// 用 --reset 清除之前 up 的校验，带 --accept-routes --operator=www-data
		a = []string{"up", "--accept-risk=all", "--operator=www-data", "--reset", "--accept-routes"}
	}

	cmd := exec.Command(tsBin, append([]string{"--socket=" + sockPath}, a...)...)
	outPipe, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		setLastUpErr(err.Error(), "")
		writeJSON(w, map[string]string{"error": err.Error()}); return
	}

	// 读取第一段输出（包含认证 URL），tailscale up 会立即打印 URL 然后阻塞
	outputChan := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := outPipe.Read(buf)
		if n > 0 {
			outputChan <- string(buf[:n])
		} else {
			outputChan <- ""
		}
	}()

	output := ""
	select {
	case out := <-outputChan:
		output = out
	case <-time.After(10 * time.Second):
	}

	writeLogf("up 响应: output=%q (URL=%v)", output, strings.Contains(output, "https://"))

	// 后台等待认证完成，捕获任何错误
	go func() {
		err := cmd.Wait()
		if err != nil {
			// 尝试读取剩余输出
			rest := make([]byte, 4096)
			m, _ := outPipe.Read(rest)
			outStr := output
			if m > 0 { outStr = output + string(rest[:m]) }
			setLastUpErr(err.Error(), outStr)
		}
		writeLogf("up 完成: err=%v output=%q", err, output)
		fetchStatus()
	}()

	writeJSON(w, map[string]string{"output": output})
}

func handleDown(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, tsBin, "--socket="+sockPath, "down").Output()
	writeLogf("down: %s err=%v", string(out), err)
	fetchStatus()
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error(), "output": string(out)})
		return
	}
	writeJSON(w, map[string]string{"output": string(out)})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	out, _ := ts("logout")
	appLogf("logout: %s", string(out))
	fetchStatus()
	writeJSON(w, map[string]string{"output": string(out)})
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, "", 405); return }
	b, _ := io.ReadAll(r.Body); var req struct { Target string; Count int }
	json.Unmarshal(b, &req); if req.Target == "" { writeJSON(w, map[string]string{"error":"target required"}); return }
	if req.Count <= 0 { req.Count = 10 }
	out, _ := ts("ping","--c",fmt.Sprintf("%d",req.Count),req.Target)
	appLogf("ping %s (count=%d): %s", req.Target, req.Count, strings.TrimSpace(string(out)))
	writeJSON(w, map[string]string{"output":string(out)})
}

func handleNetcheck(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "true"
	var out []byte
	var e error
	if force {
		out, e = fetchNetcheckData()
	} else {
		cacheMu.Lock()
		out = cachedNetcheck
		e = cachedNetcheckErr
		cacheMu.Unlock()
	}

	if e != nil && len(out) == 0 {
		writeJSON(w, map[string]string{"error": e.Error()})
		return
	}
	writeJSON(w, map[string]any{"output": string(out)})
}

func handleWhois(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" { writeJSON(w, map[string]string{"error": "ip required"}); return }
	out, err := ts("whois", ip)
	if err != nil { writeJSON(w, map[string]string{"error": err.Error()}); return }
	writeJSON(w, map[string]string{"output": string(out)})
}

func handleDNSStatus(w http.ResponseWriter, r *http.Request) {
	out, err := ts("dns", "status")
	if err != nil { writeJSON(w, map[string]string{"error": err.Error()}); return }
	writeJSON(w, map[string]string{"output": string(out)})
}

func handleIP(w http.ResponseWriter, r *http.Request) {
	out, err := ts("ip")
	if err != nil { writeJSON(w, map[string]string{"error": err.Error()}); return }
	writeJSON(w, map[string]string{"output": string(out)})
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	out, err := ts("switch", "--list")
	if err != nil {
		// --list may not be supported in older versions, try current profile
		out, err = ts("status", "--json")
		if err != nil { writeJSON(w, map[string]string{"error": err.Error()}); return }
		var raw map[string]any
		json.Unmarshal(out, &raw)
		writeJSON(w, map[string]any{"output": raw})
		return
	}
	writeJSON(w, map[string]string{"output": string(out)})
}

func handleProfileSwitch(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" { writeJSON(w, map[string]string{"error": "name required"}); return }
	out, err := ts("switch", name)
	if err != nil { writeJSON(w, map[string]string{"error": err.Error()}); return }
	writeJSON(w, map[string]string{"output": string(out)})
}

func handleFileList(w http.ResponseWriter, r *http.Request) { out, e := ts("file", "list"); if e != nil { writeJSON(w, map[string]string{"error": e.Error()}); return }; writeJSON(w, map[string]string{"output": string(out)}) }
func handleFileGet(w http.ResponseWriter, r *http.Request) { name := r.URL.Query().Get("name"); if name == "" { writeJSON(w, map[string]string{"error": "name required"}); return }; w.Header().Set("Content-Disposition", "attachment; filename="+name); data, _ := ts("file", "get", name, "--stdout"); w.Write(data) }
func handleServeStatus(w http.ResponseWriter, r *http.Request) { out, e := ts("serve", "status", "--json"); if e != nil { writeJSON(w, map[string]string{"error": e.Error()}); return }; writeJSON(w, map[string]any{"output": string(out)}) }
func handleServeSet(w http.ResponseWriter, r *http.Request) { b, _ := io.ReadAll(r.Body); var req struct{ Port int; Local string; Funnel bool }; json.Unmarshal(b, &req); if req.Port == 0 { writeJSON(w, map[string]string{"error": "port required"}); return }; a := []string{"serve", fmt.Sprintf("--bg=%d", req.Port)}; if req.Local != "" { a = append(a, "http://"+req.Local) }; if req.Funnel { a = append(a, "--funnel") }; out, e := ts(a...); if e != nil { writeJSON(w, map[string]string{"error": e.Error()}); return }; writeJSON(w, map[string]string{"output": string(out)}) }
func handleCert(w http.ResponseWriter, r *http.Request) { domain := r.URL.Query().Get("domain"); if domain == "" { writeJSON(w, map[string]string{"error": "domain required"}); return }; out, e := ts("cert", domain); if e != nil { writeJSON(w, map[string]string{"error": e.Error()}); return }; writeJSON(w, map[string]string{"output": string(out)}) }
func handleMetrics(w http.ResponseWriter, r *http.Request) { out, e := ts("metrics"); if e != nil { writeJSON(w, map[string]string{"error": e.Error()}); return }; writeJSON(w, map[string]string{"output": string(out)}) }
func handleAppcRoutes(w http.ResponseWriter, r *http.Request) {
	out, e := ts("appc-routes")
	if len(out) > 0 {
		writeJSON(w, map[string]string{"output": string(out)})
		return
	}
	if e != nil {
		writeJSON(w, map[string]string{"error": e.Error()})
		return
	}
	writeJSON(w, map[string]string{"output": ""})
}
func handleNC(w http.ResponseWriter, r *http.Request) { target := r.URL.Query().Get("target"); if target == "" { writeJSON(w, map[string]string{"error": "target required"}); return }; out, e := ts("nc", target); if e != nil { writeJSON(w, map[string]string{"error": e.Error()}); return }; writeJSON(w, map[string]string{"output": string(out)}) }
func handleBugReport(w http.ResponseWriter, r *http.Request) { out, e := ts("bugreport"); if e != nil { writeJSON(w, map[string]string{"error": e.Error()}); return }; writeJSON(w, map[string]string{"output": string(out)}) }
func handleUpdateCheck(w http.ResponseWriter, r *http.Request) { out, e := ts("update", "--check"); if e != nil { writeJSON(w, map[string]string{"error": e.Error()}); return }; writeJSON(w, map[string]string{"output": string(out)}) }

func readLog(buf *bytes.Buffer, mu *sync.Mutex) string {
	mu.Lock()
	defer mu.Unlock()
	out := buf.String()
	if len(out) == 0 { return "（无）" }
	lines := strings.Split(out, "\n")
	if len(lines) > 200 { lines = lines[len(lines)-200:] }
	return strings.Join(lines, "\n")
}

func handleLog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"log": "【Tailscale 日志】\n" + readLog(&logBuf, &logMu) + "\n\n【应用操作日志】\n" + readLog(&appLog, &appLogMu)})
}
func handleLogTs(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]string{"log": readLog(&logBuf, &logMu)}) }
func handleLogApp(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]string{"log": readLog(&appLog, &appLogMu)}) }

func handleLogClear(w http.ResponseWriter, r *http.Request) {
	logMu.Lock()
	logBuf.Reset()
	logMu.Unlock()
	appLogMu.Lock()
	appLog.Reset()
	appLogMu.Unlock()
	appLogf("日志已清空")
	writeJSON(w, map[string]string{"output": "ok"})
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type","text/event-stream"); f, _ := w.(http.Flusher)
	for {
		out, e := ts("status","--json"); if e == nil {
			var raw map[string]any; json.Unmarshal(out, &raw)
			if raw != nil { rx, tx := calcTraffic(raw); fmt.Fprintf(w,"data: %s\n\n",mustJSON(map[string]any{"rx":rx,"tx":tx})); f.Flush() }
		}
		select { case <-r.Context().Done(): return; case <-time.After(1*time.Second): }
	}
}

func calcTraffic(raw map[string]any) (float64, float64) {
	var rx, tx float64
	if s, ok := raw["Self"].(map[string]any); ok { rx,_=s["RxBytes"].(float64); tx,_=s["TxBytes"].(float64) }
	if p, ok := raw["Peer"].(map[string]any); ok { for _,v := range p { if m,ok:=v.(map[string]any); ok { rx+=gf(m,"RxBytes"); tx+=gf(m,"TxBytes") } } }
	return rx, tx
}
func gf(m map[string]any, k string) float64 { v,_:=m[k].(float64); return v }
func writeJSON(w http.ResponseWriter, v any) { w.Header().Set("Content-Type","application/json"); json.NewEncoder(w).Encode(v) }
func mustJSON(v any) string { b,_:=json.Marshal(v); return string(b); }

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "time": time.Now().Format("2006-01-02 15:04:05")})
}

