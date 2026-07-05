package main

import (
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

var (
	appVar       = envOr("TRIM_PKGVAR", "/var/apps/easytier/data")
	appDest      = envOr("TRIM_APPDEST", "/var/apps/easytier")
	servicePort  = envOr("TRIM_SERVICE_PORT", "11210")
	coreBin      = findBin("easytier-core", "")
	cliBin       = findBin("easytier-cli", "")
	settingsDir  string
	settingsPath string
)

type Settings struct {
	ServiceEnabled bool     `json:"service_enabled"`
	NetworkName    string   `json:"network_name"`
	NetworkSecret  string   `json:"network_secret"`
	VirtualIPv4    string   `json:"virtual_ipv4"`
	VirtualIPv6    string   `json:"virtual_ipv6"`
	Hostname       string   `json:"hostname"`
	InstanceName   string   `json:"instance_name"`
	PeerURLs       []string `json:"peer_urls"`
	ListenerURLs   []string `json:"listener_urls"`
	ProxyCIDRs     []string `json:"proxy_cidrs"`
	ManualRoutes   []string `json:"manual_routes"`
	MappedListeners []string `json:"mapped_listeners"`
	DHCP           bool     `json:"dhcp"`
	Encryption     bool     `json:"encryption"`
	LatencyFirst   bool     `json:"latency_first"`
	NoTun          bool     `json:"no_tun"`
	DisableIPv6    bool     `json:"disable_ipv6"`
	PrivateMode    bool     `json:"private_mode"`
	P2POnly        bool     `json:"p2p_only"`
	LazyP2P        bool     `json:"lazy_p2p"`
	MultiThread    bool     `json:"multi_thread"`
	Compression    string   `json:"compression"`
	MTU            int      `json:"mtu"`
	DefaultProto   string   `json:"default_protocol"`
	EncryptionAlgo string   `json:"encryption_algorithm"`
	EnableExitNode bool     `json:"enable_exit_node"`
	ExitNodes      []string `json:"exit_nodes"`
	Socks5         int      `json:"socks5"`
	TCPPortForward   []string `json:"tcp_port_forward"`
	UDPPortForward   []string `json:"udp_port_forward"`
	TCPWhitelist   []string `json:"tcp_whitelist"`
	UDPWhitelist   []string `json:"udp_whitelist"`
	VpnPortal      string   `json:"vpn_portal"`
	TldDNSZone     string   `json:"tld_dns_zone"`
	LogLevel       string   `json:"log_level"`
}

var (
	settings   Settings
	settingsMu sync.Mutex

	coreCmd    *exec.Cmd
	coreOnline bool
	coreMu     sync.Mutex
	opMu       sync.Mutex

	appLog  = NewRingLog(500)
	coreLog = NewRingLog(500)

	lastUpErr string
	lastUpOut string
	upLock    sync.Mutex

	cachedStatus map[string]any
	statusMu     sync.Mutex
)

type RingLog struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func NewRingLog(max int) *RingLog {
	return &RingLog{max: max}
}

func (r *RingLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, line := range strings.Split(string(p), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r.lines = append(r.lines, line)
		if len(r.lines) > r.max {
			r.lines = r.lines[len(r.lines)-r.max:]
		}
	}
	return len(p), nil
}

func (r *RingLog) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

func (r *RingLog) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func findBin(name, prefer string) string {
	if prefer != "" {
		if _, err := os.Stat(prefer); err == nil {
			return prefer
		}
	}
	for _, d := range []string{appDest + "/app", appDest} {
		p := d + "/" + name
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func cli(args ...string) ([]byte, error) {
	if cliBin == "" {
		return nil, fmt.Errorf("easytier-cli not found")
	}
	out, err := exec.Command(cliBin, args...).CombinedOutput()
	return out, err
}

func loadSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		settings = Settings{}
		ensureSettingsSlice()
		return
	}
	json.Unmarshal(data, &settings)
	ensureSettingsSlice()
}

func ensureSettingsSlice() {
	if settings.PeerURLs == nil {
		settings.PeerURLs = []string{}
	}
	if settings.ListenerURLs == nil {
		settings.ListenerURLs = []string{}
	}
	if settings.ProxyCIDRs == nil {
		settings.ProxyCIDRs = []string{}
	}
	if settings.ManualRoutes == nil {
		settings.ManualRoutes = []string{}
	}
	if settings.MappedListeners == nil {
		settings.MappedListeners = []string{}
	}
	if settings.ExitNodes == nil {
		settings.ExitNodes = []string{}
	}
	if settings.TCPPortForward == nil {
		settings.TCPPortForward = []string{}
	}
	if settings.UDPPortForward == nil {
		settings.UDPPortForward = []string{}
	}
	if settings.TCPWhitelist == nil {
		settings.TCPWhitelist = []string{}
	}
	if settings.UDPWhitelist == nil {
		settings.UDPWhitelist = []string{}
	}
}

func saveSettingsLocked() error {
	ensureSettingsSlice()
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, data, 0644)
}

func saveSettings() error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	return saveSettingsLocked()
}

func buildCoreArgs() []string {
	settingsMu.Lock()
	s := settings
	settingsMu.Unlock()

	args := []string{}
	if s.NetworkName != "" {
		args = append(args, "--network-name", s.NetworkName)
	}
	if s.NetworkSecret != "" {
		args = append(args, "--network-secret", s.NetworkSecret)
	}
	if s.VirtualIPv4 != "" {
		args = append(args, "--ipv4", s.VirtualIPv4)
	}
	if s.VirtualIPv6 != "" {
		args = append(args, "--ipv6", s.VirtualIPv6)
	}
	if s.Hostname != "" {
		args = append(args, "--hostname", s.Hostname)
	}
	if s.InstanceName != "" {
		args = append(args, "--instance-name", s.InstanceName)
	}
	for _, u := range s.PeerURLs {
		u = strings.TrimSpace(u)
		if u != "" {
			args = append(args, "--peers", u)
		}
	}
	for _, u := range s.ListenerURLs {
		u = strings.TrimSpace(u)
		if u != "" {
			args = append(args, "--listeners", u)
		}
	}
	for _, c := range s.ProxyCIDRs {
		c = strings.TrimSpace(c)
		if c != "" {
			args = append(args, "--proxy-networks", c)
		}
	}
	for _, r := range s.ManualRoutes {
		r = strings.TrimSpace(r)
		if r != "" {
			args = append(args, "--manual-routes", r)
		}
	}
	for _, m := range s.MappedListeners {
		m = strings.TrimSpace(m)
		if m != "" {
			args = append(args, "--mapped-listeners", m)
		}
	}
	if s.DHCP {
		args = append(args, "--dhcp")
	}
	if s.Encryption {
		args = append(args, "--encryption")
	}
	if s.EncryptionAlgo != "" {
		args = append(args, "--encryption-algorithm", s.EncryptionAlgo)
	}
	if s.LatencyFirst {
		args = append(args, "--latency-first")
	}
	if s.NoTun {
		args = append(args, "--no-tun")
	}
	if s.DisableIPv6 {
		args = append(args, "--disable-ipv6")
	}
	if s.PrivateMode {
		args = append(args, "--private-mode")
	}
	if s.P2POnly {
		args = append(args, "--p2p-only")
	}
	if s.LazyP2P {
		args = append(args, "--lazy-p2p")
	}
	if s.MultiThread {
		args = append(args, "--multi-thread")
	}
	if s.Compression != "" {
		args = append(args, "--compression", s.Compression)
	}
	if s.MTU > 0 {
		args = append(args, "--mtu", strconv.Itoa(s.MTU))
	}
	if s.DefaultProto != "" {
		args = append(args, "--default-protocol", s.DefaultProto)
	}
	if s.EnableExitNode {
		args = append(args, "--enable-exit-node")
	}
	for _, e := range s.ExitNodes {
		e = strings.TrimSpace(e)
		if e != "" {
			args = append(args, "--exit-nodes", e)
		}
	}
	if s.Socks5 > 0 {
		args = append(args, "--socks5", strconv.Itoa(s.Socks5))
	}
	for _, p := range s.TCPPortForward {
		p = strings.TrimSpace(p)
		if p != "" {
			args = append(args, "--port-forward", "tcp://"+p)
		}
	}
	for _, p := range s.UDPPortForward {
		p = strings.TrimSpace(p)
		if p != "" {
			args = append(args, "--port-forward", "udp://"+p)
		}
	}
	for _, w := range s.TCPWhitelist {
		w = strings.TrimSpace(w)
		if w != "" {
			args = append(args, "--tcp-whitelist", w)
		}
	}
	for _, w := range s.UDPWhitelist {
		w = strings.TrimSpace(w)
		if w != "" {
			args = append(args, "--udp-whitelist", w)
		}
	}
	if s.VpnPortal != "" {
		args = append(args, "--vpn-portal", s.VpnPortal)
	}
	if s.TldDNSZone != "" {
		args = append(args, "--tld-dns-zone", s.TldDNSZone)
	}
	return args
}

func startCore() error {
	coreMu.Lock()
	defer coreMu.Unlock()

	if coreBin == "" {
		log.Printf("[ERROR] easytier-core binary not found, cannot start")
		return fmt.Errorf("easytier-core not found")
	}
	if coreOnline {
		log.Printf("[WARN] easytier-core already running, skipping start")
		return fmt.Errorf("already running")
	}

	args := buildCoreArgs()
	log.Printf("[INFO] starting easytier-core: %s %v", coreBin, args)

	cmd := exec.Command(coreBin, args...)
	cmd.Stdout = io.MultiWriter(os.Stderr, coreLog)
	cmd.Stderr = io.MultiWriter(os.Stderr, coreLog)

	if err := cmd.Start(); err != nil {
		log.Printf("[ERROR] easytier-core start failed: %v", err)
		coreLog.Write([]byte(fmt.Sprintf("[APP] easytier-core start failed: %v", err)))
		return err
	}

	coreCmd = cmd
	coreOnline = true
	log.Printf("[INFO] easytier-core started (pid %d)", cmd.Process.Pid)
	coreLog.Write([]byte(fmt.Sprintf("[APP] easytier-core started (pid %d)", cmd.Process.Pid)))

	go func() {
		err := cmd.Wait()
		coreMu.Lock()
		coreOnline = false
		coreCmd = nil
		coreMu.Unlock()
		if err != nil {
			log.Printf("[WARN] easytier-core exited: %v", err)
			coreLog.Write([]byte(fmt.Sprintf("[APP] easytier-core exited: %v", err)))
		} else {
			log.Printf("[INFO] easytier-core exited normally")
			coreLog.Write([]byte("[APP] easytier-core exited normally"))
		}
	}()

	return nil
}

func stopCore() {
	coreMu.Lock()
	if coreCmd == nil || coreCmd.Process == nil {
		coreMu.Unlock()
		coreOnline = false
		return
	}
	cmd := coreCmd
	coreOnline = false
	coreMu.Unlock()

	log.Printf("[INFO] stopping easytier-core (pid %d)", cmd.Process.Pid)
	coreLog.Write([]byte("[APP] stopping easytier-core..."))
	cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[INFO] easytier-core stopped")
		coreLog.Write([]byte("[APP] easytier-core stopped"))
	case <-time.After(5 * time.Second):
		log.Printf("[WARN] easytier-core kill after timeout")
		coreLog.Write([]byte("[APP] easytier-core killed after timeout"))
		cmd.Process.Kill()
		cmd.Wait()
	}

	coreMu.Lock()
	coreCmd = nil
	coreMu.Unlock()
}

func pollStatus() {
	for {
		result := map[string]any{
			"daemon":  coreOnline,
			"running": coreOnline,
		}

		if !coreOnline {
			result["online"] = false
		} else {
			upErr, _ := getAndClearUpErr()
			nodeOut, nodeErr := cli("node", "info", "-o", "json")
			if nodeErr != nil {
				result["online"] = false
				result["error"] = nodeErr.Error()
				if upErr != "" {
					result["upError"] = upErr
				}
			} else {
				var node map[string]any
				json.Unmarshal(nodeOut, &node)

				peersOut, _ := cli("peer", "list", "-o", "json")
				var peers []any
				if peersOut != nil {
					json.Unmarshal(peersOut, &peers)
				}
				if peers == nil {
					peers = []any{}
				}

				rx := getFloat(node, "rx_bytes")
				tx := getFloat(node, "tx_bytes")
				for _, p := range peers {
					if m, ok := p.(map[string]any); ok {
						rx += getFloat(m, "rx_bytes")
						tx += getFloat(m, "tx_bytes")
					}
				}

				result["online"] = true
				result["node"] = node
				result["peers"] = peers
				result["totalRx"] = rx
				result["totalTx"] = tx
			}
		}

		settingsMu.Lock()
		result["settings"] = settings
		result["service_enabled"] = settings.ServiceEnabled
		settingsMu.Unlock()

		statusMu.Lock()
		cachedStatus = result
		statusMu.Unlock()
		time.Sleep(3 * time.Second)
	}
}

func setUpErr(err, out string) {
	upLock.Lock()
	defer upLock.Unlock()
	lastUpErr = err
	lastUpOut = out
}

func getAndClearUpErr() (string, string) {
	upLock.Lock()
	defer upLock.Unlock()
	e, o := lastUpErr, lastUpOut
	lastUpErr, lastUpOut = "", ""
	return e, o
}

func findAvailablePort(start int) string {
	for i := 0; i < 100; i++ {
		addr := fmt.Sprintf(":%d", start)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			l.Close()
			return strconv.Itoa(start)
		}
		start++
	}
	return "11210"
}

func getFloat(m map[string]any, k string) float64 {
	v, _ := m[k].(float64)
	return v
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func serveUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := frontend.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func serveVisNetwork(w http.ResponseWriter, r *http.Request) {
	data, err := frontend.ReadFile("ui/vis-network.min.js")
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Write(data)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	statusMu.Lock()
	result := cachedStatus
	statusMu.Unlock()

	if result == nil {
		settingsMu.Lock()
		result = map[string]any{
			"online":          false,
			"daemon":          coreOnline,
			"running":         coreOnline,
			"settings":        settings,
			"service_enabled": settings.ServiceEnabled,
		}
		settingsMu.Unlock()
	}

	writeJSON(w, result)
}

func handleTraffic(w http.ResponseWriter, r *http.Request) {
	statusMu.Lock()
	st := cachedStatus
	statusMu.Unlock()

	rx, _ := st["totalRx"].(float64)
	tx, _ := st["totalTx"].(float64)
	writeJSON(w, map[string]any{"rx": rx, "tx": tx})
}

func handlePeers(w http.ResponseWriter, r *http.Request) {
	out, err := cli("peer", "list", "-o", "json")
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleRoutes(w http.ResponseWriter, r *http.Request) {
	out, err := cli("route", "list", "-o", "json")
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleConnectors(w http.ResponseWriter, r *http.Request) {
	out, err := cli("connector", "list", "-o", "json")
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleServiceStart(w http.ResponseWriter, r *http.Request) {
	log.Printf("[INFO] API: service/start")
	settingsMu.Lock()
	settings.ServiceEnabled = true
	saveSettingsLocked()
	settingsMu.Unlock()

	err := startCore()
	if err != nil {
		setUpErr(err.Error(), "")
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleServiceStop(w http.ResponseWriter, r *http.Request) {
	log.Printf("[INFO] API: service/stop")
	settingsMu.Lock()
	settings.ServiceEnabled = false
	saveSettingsLocked()
	settingsMu.Unlock()

	stopCore()
	writeJSON(w, map[string]any{"ok": true})
}

func handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	log.Printf("[INFO] API: service/restart")
	opMu.Lock()
	defer opMu.Unlock()

	settingsMu.Lock()
	svcEnabled := settings.ServiceEnabled
	settings.ServiceEnabled = true
	saveSettingsLocked()
	settingsMu.Unlock()

	stopCore()
	time.Sleep(500 * time.Millisecond)

	err := startCore()
	if err != nil {
		settingsMu.Lock()
		settings.ServiceEnabled = svcEnabled
		saveSettingsLocked()
		settingsMu.Unlock()
		setUpErr(err.Error(), "")
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		settingsMu.Lock()
		s := settings
		settingsMu.Unlock()
		writeJSON(w, s)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}

	b, _ := io.ReadAll(r.Body)
	var newSettings Settings
	json.Unmarshal(b, &newSettings)

	log.Printf("[INFO] API: save settings: name=%q secret=*** ipv4=%q hostname=%q dhcp=%v encrypt=%v peers=%d listeners=%d proxies=%d",
		newSettings.NetworkName, newSettings.VirtualIPv4, newSettings.Hostname,
		newSettings.DHCP, newSettings.Encryption,
		len(newSettings.PeerURLs), len(newSettings.ListenerURLs), len(newSettings.ProxyCIDRs))

	settingsMu.Lock()
	settings = newSettings
	ensureSettingsSlice()
	settingsMu.Unlock()

	if err := saveSettings(); err != nil {
		log.Printf("[ERROR] save settings failed: %v", err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	coreMu.Lock()
	running := coreOnline
	coreMu.Unlock()

	if running {
		log.Printf("[INFO] service running, restarting with new config")
		go restartCore()
	}

	writeJSON(w, map[string]any{"ok": true})
}

func restartCore() {
	opMu.Lock()
	defer opMu.Unlock()
	stopCore()
	time.Sleep(500 * time.Millisecond)
	startCore()
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		Target string `json:"target"`
		Count  int    `json:"count"`
	}
	json.Unmarshal(b, &req)
	if req.Target == "" {
		writeJSON(w, map[string]string{"error": "target required"})
		return
	}
	if req.Count <= 0 {
		req.Count = 4
	}
	log.Printf("[INFO] API: ping %s (count=%d)", req.Target, req.Count)
	out, _ := cli("node", "ping", req.Target, "-c", fmt.Sprintf("%d", req.Count), "-o", "json")
	writeJSON(w, map[string]string{"output": string(out)})
}

func handleLog(w http.ResponseWriter, r *http.Request) {
	combined := appLog.String() + "\n--- Core Log ---\n" + coreLog.String()
	writeJSON(w, map[string]string{"log": combined})
}

func handleLogApp(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"log": appLog.String()})
}

func handleLogCore(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"log": coreLog.String()})
}

func handleLogClear(w http.ResponseWriter, r *http.Request) {
	log.Printf("[INFO] API: clear logs")
	appLog.Clear()
	coreLog.Clear()
	writeJSON(w, map[string]string{"ok": "true"})
}

func handleLogLevelGet(w http.ResponseWriter, r *http.Request) {
	out, err := cli("logger", "get", "-o", "json")
	if err != nil {
		writeJSON(w, map[string]string{"level": "unknown", "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleLogLevelSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		Level string `json:"level"`
	}
	json.Unmarshal(b, &req)
	if req.Level == "" {
		writeJSON(w, map[string]string{"error": "level required"})
		return
	}
	log.Printf("[INFO] API: log level set: %s", req.Level)
	out, err := cli("logger", "set", req.Level)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "output": string(out)})
}

func handleConnectorAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		URL string `json:"url"`
	}
	json.Unmarshal(b, &req)
	if req.URL == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "url required"})
		return
	}
	log.Printf("[INFO] API: connector add: %s", req.URL)
	out, err := cli("connector", "add", req.URL)
	if err != nil {
		log.Printf("[ERROR] connector add failed: %v, output: %s", err, string(out))
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
		return
	}
	log.Printf("[INFO] connector add succeeded")
	writeJSON(w, map[string]any{"ok": true})
}

func handleConnectorRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		URL string `json:"url"`
	}
	json.Unmarshal(b, &req)
	if req.URL == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "url required"})
		return
	}
	log.Printf("[INFO] API: connector remove: %s", req.URL)
	out, err := cli("connector", "remove", req.URL)
	if err != nil {
		log.Printf("[ERROR] connector remove failed: %v, output: %s", err, string(out))
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
		return
	}
	log.Printf("[INFO] connector remove succeeded")
	writeJSON(w, map[string]any{"ok": true})
}

func handlePortForwardList(w http.ResponseWriter, r *http.Request) {
	out, err := cli("port-forward", "list", "-o", "json")
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handlePortForwardAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		Proto string `json:"proto"`
		Bind  string `json:"bind"`
		Dst   string `json:"dst"`
	}
	json.Unmarshal(b, &req)
	if req.Proto == "" || req.Bind == "" || req.Dst == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "proto, bind, dst required"})
		return
	}
	log.Printf("[INFO] API: port-forward add: %s %s -> %s", req.Proto, req.Bind, req.Dst)
	out, err := cli("port-forward", "add", req.Proto, req.Bind, req.Dst)
	if err != nil {
		log.Printf("[ERROR] port-forward add failed: %v", err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handlePortForwardRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		Proto string `json:"proto"`
		Bind  string `json:"bind"`
		Dst   string `json:"dst"`
	}
	json.Unmarshal(b, &req)
	if req.Proto == "" || req.Bind == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "proto, bind required"})
		return
	}
	log.Printf("[INFO] API: port-forward remove: %s %s", req.Proto, req.Bind)
	args := []string{"port-forward", "remove", req.Proto, req.Bind}
	if req.Dst != "" {
		args = append(args, req.Dst)
	}
	out, err := cli(args...)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleWhitelistShow(w http.ResponseWriter, r *http.Request) {
	out, err := cli("whitelist", "show", "-o", "json")
	if err != nil {
		writeJSON(w, map[string]string{"tcp": "", "udp": "", "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleWhitelistSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		Proto string `json:"proto"`
		Ports string `json:"ports"`
	}
	json.Unmarshal(b, &req)
	if req.Proto != "tcp" && req.Proto != "udp" {
		writeJSON(w, map[string]any{"ok": false, "error": "proto must be tcp or udp"})
		return
	}
	log.Printf("[INFO] API: whitelist set: %s = %s", req.Proto, req.Ports)
	subcmd := "set-tcp"
	if req.Proto == "udp" {
		subcmd = "set-udp"
	}
	out, err := cli("whitelist", subcmd, req.Ports)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleWhitelistClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		Proto string `json:"proto"`
	}
	json.Unmarshal(b, &req)
	if req.Proto != "tcp" && req.Proto != "udp" {
		writeJSON(w, map[string]any{"ok": false, "error": "proto must be tcp or udp"})
		return
	}
	subcmd := "clear-tcp"
	if req.Proto == "udp" {
		subcmd = "clear-udp"
	}
	log.Printf("[INFO] API: whitelist clear: %s", req.Proto)
	cli("whitelist", subcmd)
	writeJSON(w, map[string]any{"ok": true})
}

func handleCredentialList(w http.ResponseWriter, r *http.Request) {
	out, err := cli("credential", "list", "-o", "json")
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleCredentialGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		TTL             int    `json:"ttl"`
		CredentialID    string `json:"credential_id"`
		Groups          string `json:"groups"`
		AllowRelay      bool   `json:"allow_relay"`
		AllowedCIDRs    string `json:"allowed_proxy_cidrs"`
		Reusable        bool   `json:"reusable"`
	}
	json.Unmarshal(b, &req)
	if req.TTL <= 0 {
		req.TTL = 3600
	}
	log.Printf("[INFO] API: credential generate: ttl=%d id=%s", req.TTL, req.CredentialID)
	args := []string{"credential", "generate", "--ttl", strconv.Itoa(req.TTL)}
	if req.CredentialID != "" {
		args = append(args, "--credential-id", req.CredentialID)
	}
	if req.Groups != "" {
		args = append(args, "--groups", req.Groups)
	}
	if req.AllowRelay {
		args = append(args, "--allow-relay")
	}
	if req.AllowedCIDRs != "" {
		args = append(args, "--allowed-proxy-cidrs", req.AllowedCIDRs)
	}
	if req.Reusable {
		args = append(args, "--reusable")
	}
	out, err := cli(args...)
	if err != nil {
		log.Printf("[ERROR] credential generate failed: %v, output: %s", err, string(out))
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "output": string(out)})
}

func handleCredentialRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", 405)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req struct {
		ID string `json:"credential_id"`
	}
	json.Unmarshal(b, &req)
	if req.ID == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "credential_id required"})
		return
	}
	log.Printf("[INFO] API: credential revoke: %s", req.ID)
	out, err := cli("credential", "revoke", req.ID)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "output": string(out)})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleStatsShow(w http.ResponseWriter, r *http.Request) {
	out, err := cli("stats", "show", "-o", "json")
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error(), "raw": string(out)})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleStatsPrometheus(w http.ResponseWriter, r *http.Request) {
	out, err := cli("stats", "prometheus")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write(out)
}

func handleVPNPortal(w http.ResponseWriter, r *http.Request) {
	out, err := cli("vpn-portal", "-o", "json")
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error(), "raw": string(out)})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleNodeConfig(w http.ResponseWriter, r *http.Request) {
	out, err := cli("node", "config", "-o", "json")
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error(), "raw": string(out)})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f, _ := w.(http.Flusher)
	for {
		statusMu.Lock()
		st := cachedStatus
		statusMu.Unlock()

		rx, _ := st["totalRx"].(float64)
		tx, _ := st["totalTx"].(float64)
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"rx": rx, "tx": tx}))
		f.Flush()

		select {
		case <-r.Context().Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func main() {
	port := os.Getenv("EASYTIER_PORT")
	if port == "" {
		port = servicePort
	}

	settingsDir = appVar + "/config"
	settingsPath = settingsDir + "/settings.json"
	os.MkdirAll(settingsDir, 0755)
	os.MkdirAll(appVar, 0755)
	os.MkdirAll(appVar+"/logs", 0755)

	log.SetOutput(io.MultiWriter(os.Stderr, appLog))

	log.Printf("[INFO] easytier-server starting (core=%s, cli=%s)", coreBin, cliBin)
	log.Printf("[INFO] settings path: %s", settingsPath)

	loadSettings()

	settingsMu.Lock()
	shouldAutoStart := settings.ServiceEnabled
	settingsMu.Unlock()

	if shouldAutoStart {
		log.Printf("[INFO] auto-starting easytier-core (service_enabled=true)")
		appLog.Write([]byte("[APP] auto-starting easytier-core"))
		if err := startCore(); err != nil {
			log.Printf("[WARN] auto-start failed: %v", err)
			appLog.Write([]byte(fmt.Sprintf("[APP] auto-start failed: %v", err)))
		}
	} else {
		log.Printf("[INFO] service_enabled=false, not auto-starting")
		appLog.Write([]byte("[APP] service disabled, waiting for manual start"))
	}

	go pollStatus()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Printf("[INFO] shutting down...")
		appLog.Write([]byte("[APP] shutting down..."))
		stopCore()
		os.Exit(0)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveUI)
	mux.HandleFunc("/vis-network.min.js", serveVisNetwork)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/traffic", handleTraffic)
	mux.HandleFunc("/api/peers", handlePeers)
	mux.HandleFunc("/api/routes", handleRoutes)
	mux.HandleFunc("/api/connectors", handleConnectors)
	mux.HandleFunc("/api/service/start", handleServiceStart)
	mux.HandleFunc("/api/service/stop", handleServiceStop)
	mux.HandleFunc("/api/service/restart", handleServiceRestart)
	mux.HandleFunc("/api/settings", handleSettings)
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/log", handleLog)
	mux.HandleFunc("/api/log/app", handleLogApp)
	mux.HandleFunc("/api/log/core", handleLogCore)
	mux.HandleFunc("/api/log/clear", handleLogClear)
	mux.HandleFunc("/api/log/level", handleLogLevelGet)
	mux.HandleFunc("/api/log/level/set", handleLogLevelSet)
	mux.HandleFunc("/api/connector/add", handleConnectorAdd)
	mux.HandleFunc("/api/connector/remove", handleConnectorRemove)
	mux.HandleFunc("/api/port-forward/list", handlePortForwardList)
	mux.HandleFunc("/api/port-forward/add", handlePortForwardAdd)
	mux.HandleFunc("/api/port-forward/remove", handlePortForwardRemove)
	mux.HandleFunc("/api/whitelist/show", handleWhitelistShow)
	mux.HandleFunc("/api/whitelist/set", handleWhitelistSet)
	mux.HandleFunc("/api/whitelist/clear", handleWhitelistClear)
	mux.HandleFunc("/api/credential/list", handleCredentialList)
	mux.HandleFunc("/api/credential/generate", handleCredentialGenerate)
	mux.HandleFunc("/api/credential/revoke", handleCredentialRevoke)
	mux.HandleFunc("/api/stats", handleStatsShow)
	mux.HandleFunc("/api/stats/prometheus", handleStatsPrometheus)
	mux.HandleFunc("/api/vpn-portal", handleVPNPortal)
	mux.HandleFunc("/api/node/config", handleNodeConfig)
	mux.HandleFunc("/api/events", handleEvents)

	for i := 0; i < 100; i++ {
		addr := fmt.Sprintf(":%s", port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			os.WriteFile("/tmp/easytier-port", []byte(port), 0644)
			log.Printf("[INFO] listening on %s", addr)
			http.Serve(l, mux)
			return
		}
		p, _ := strconv.Atoi(port)
		port = strconv.Itoa(p + 1)
	}
	log.Fatal("no available port")
}