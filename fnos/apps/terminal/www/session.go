package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type Subscriber struct {
	id     string
	ch     chan []byte
	closed bool
	mu     sync.Mutex
}

func (s *Subscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}

type Session struct {
	ID         string
	Title      string
	Shell      string
	User       string
	Cols       int
	Rows       int
	CreatedAt  time.Time
	DetachedAt time.Time
	LastInput  time.Time

	mu         sync.Mutex
	cmd        *exec.Cmd
	ptmx       *os.File
	buffer     *RingBuffer
	subs       map[string]*Subscriber
	exited     bool
	exitCode   int
	detached   bool
}

type userInfo struct {
	uid  uint32
	gid  uint32
	home string
	sh   string
}

func lookupUser(name string) (*userInfo, error) {
	if name == "" || name == "root" {
		return &userInfo{uid: 0, gid: 0, home: "/root", sh: "/bin/bash"}, nil
	}
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.SplitN(line, ":", 7)
		if len(fields) < 7 || fields[0] != name {
			continue
		}
		uid, _ := strconv.Atoi(fields[2])
		gid, _ := strconv.Atoi(fields[3])
		home := fields[5]
		sh := fields[6]
		if sh == "" {
			sh = "/bin/sh"
		}
		return &userInfo{uid: uint32(uid), gid: uint32(gid), home: home, sh: sh}, nil
	}
	return nil, errors.New("user not found: " + name)
}

// resolveUser 决定 PTY 用哪个用户启动
// 优先级：1. 前端指定的 user > 2. settings.DefaultUser > 3. root
func resolveUser(requested string) string {
	if requested != "" {
		if _, err := lookupUser(requested); err == nil {
			return requested
		}
	}
	settingsMu.RLock()
	defaultU := settings.DefaultUser
	settingsMu.RUnlock()
	if defaultU != "" {
		if _, err := lookupUser(defaultU); err == nil {
			return defaultU
		}
	}
	return "root"
}

func newSession(id, title, shell, user string, cols, rows int) *Session {
	if user == "" {
		user = resolveUser("")
	}
	// shell 优先级：前端指定 > 用户登录 shell (/etc/passwd 第 7 字段) > detectShell fallback
	if shell == "" {
		if u, err := lookupUser(user); err == nil && u.sh != "" {
			shell = u.sh
		} else {
			shell = detectShell()
		}
	}
	if cols <= 0 {
		cols = defaultCols
	}
	if rows <= 0 {
		rows = defaultRows
	}
	if title == "" {
		title = id[:8]
	}
	return &Session{
		ID:        id,
		Title:     title,
		Shell:     shell,
		User:      user,
		Cols:      cols,
		Rows:      rows,
		CreatedAt: time.Now(),
		buffer:    NewRingBuffer(maxBufferBytes),
		subs:      make(map[string]*Subscriber),
	}
}

func detectShell() string {
	for _, s := range []string{"/bin/bash", "/bin/sh", "/bin/zsh"} {
		if _, err := os.Stat(s); err == nil {
			return s
		}
	}
	return "/bin/sh"
}

func (s *Session) start() error {
	return s.startInternal(s.Shell, []string{"-l"}, false)
}

// startWithCmd 启动一个 session 跑指定的 script (不走 login shell, 用 -c 直接跑)
// 脚本跑完后 exec $SHELL -i 进入 interactive shell, 这样 terminal 仍可继续输入
// 注意: bash 不支持同时 -l + -c, 用纯 -c
func (s *Session) startWithCmd(scriptCmd string) error {
	u, err := lookupUser(s.User)
	if err != nil {
		u = &userInfo{uid: 0, gid: 0, home: "/root", sh: "/bin/bash"}
	}
	shell := u.sh
	if shell == "" {
		shell = "/bin/bash"
	}
	// 包一层: script 跑完后 exec 进入 user login shell (-i 强制 interactive, 避免 ohmyzsh 退出)
	// 例如 bash -c "echo hi; exec bash -i"
	//     zsh -c "echo hi; exec zsh -i"
	// exec 替换进程, 不创建新进程组, PTY 仍持有
	wrappedCmd := scriptCmd + "; exec " + shell + " -i"
	return s.startInternal(shell, []string{"-c", wrappedCmd}, true)
}

// startInternal 内部启动逻辑, isScript=true 时为脚本模式 (-c)
func (s *Session) startInternal(shell string, args []string, isScript bool) error {
	u, err := lookupUser(s.User)
	if err != nil {
		u = &userInfo{uid: 0, gid: 0, home: "/root", sh: "/bin/bash"}
	}
	if u.home == "" {
		u.home = "/root"
	}

	cmd := exec.Command(shell, args...)
	cmd.Env = []string{
		"TERM=xterm-256color",
		// 不用 LC_ALL=zh_CN.UTF-8 (很多系统没装该 locale, bash 启动会警告)
		// LANG 用 C 保证一定可用；用户可在 .bashrc 里覆盖
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"COLORTERM=truecolor",
		"HOME=" + u.home,
		"USER=" + s.User,
		"LOGNAME=" + s.User,
		"SHELL=" + s.Shell,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		// 显式关掉 zsh 的 PROMPT_EOL_MARK，避免 attach 时每条命令后多一个 % (SIGWINCH 触发)
		"PROMPT_EOL_MARK=",
		"PROMPT_SP=",
	}

	// zsh 5.4+ 即使 PROMPT_EOL_MARK='' 也会画"空字符"占一行。
	// RPROMPT 在屏幕最右列时 zsh 自动画 EOL mark (ohmyzsh 的 git 分支 RPROMPT 触发)。
	// 用临时 ZDOTDIR 覆盖，写 .zshrc 末尾强制覆盖 (source 用户 .zshrc 后再清 EOL mark + 关 promptsp)。
	if !isScript && strings.HasSuffix(shell, "/zsh") {
		zdotdir := "/tmp/fnos-zdot-" + s.ID
		_ = os.MkdirAll(zdotdir, 0755)
		// 用户的 .zshrc 在 $HOME（ohmyzsh 在那里）
		// .zshrc 末尾追加强制覆盖 EOL mark 和 promptsp
		zshrc := "source $HOME/.zshrc\n" +
			"PROMPT_EOL_MARK=''\n" +
			"setopt no_prompt_sp 2>/dev/null\n" +
			"setopt no_prompt_cr 2>/dev/null\n"
		_ = os.WriteFile(zdotdir+"/.zshrc", []byte(zshrc), 0644)
		// .zshenv 早期加载，也强制一次 (ohmyzsh 在 .zshrc 加载, .zshenv 之前)
		zshenv := "PROMPT_EOL_MARK=''\n"
		_ = os.WriteFile(zdotdir+"/.zshenv", []byte(zshenv), 0644)
		cmd.Env = append(cmd.Env, "ZDOTDIR="+zdotdir)
	}
	cmd.Dir = u.home
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
		Credential: &syscall.Credential{
			Uid:    u.uid,
			Gid:    u.gid,
			Groups: nil,
		},
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(s.Rows), Cols: uint16(s.Cols), X: 0, Y: 0})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cmd = cmd
	s.ptmx = ptmx
	s.mu.Unlock()
	go s.readLoop()
	go s.waitLoop()
	return nil
}

func (s *Session) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.buffer.Write(data)
			s.broadcast(data)
		}
		if err != nil {
			if err != io.EOF {
				// PTY 关闭是正常的，shell 退出后会触发
			}
			s.mu.Lock()
			exited := s.exited
			s.mu.Unlock()
			if exited {
				s.broadcast([]byte("\r\n\x1b[2m[会话已结束]\x1b[0m\r\n"))
			}
			return
		}
	}
}

func (s *Session) waitLoop() {
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	s.mu.Lock()
	s.exited = true
	s.exitCode = code
	s.ptmx.Close()
	subs := make([]*Subscriber, 0, len(s.subs))
	for _, sub := range s.subs {
		subs = append(subs, sub)
	}
	s.mu.Unlock()
	for _, sub := range subs {
		select {
		case sub.ch <- []byte("__EXIT__:" + itoa(code)):
		default:
		}
	}
}

func (s *Session) broadcast(data []byte) {
	s.mu.Lock()
	subs := make([]*Subscriber, 0, len(s.subs))
	for _, sub := range s.subs {
		subs = append(subs, sub)
	}
	s.mu.Unlock()
	for _, sub := range subs {
		sub.mu.Lock()
		closed := sub.closed
		sub.mu.Unlock()
		if closed {
			continue
		}
		select {
		case sub.ch <- data:
		default:
			// 慢消费者，丢弃数据避免阻塞
		}
	}
}

func (s *Session) write(data []byte) error {
	s.mu.Lock()
	ptmx := s.ptmx
	exited := s.exited
	s.mu.Unlock()
	if exited || ptmx == nil {
		return errors.New("session exited")
	}
	_, err := ptmx.Write(data)
	return err
}

func (s *Session) resize(cols, rows int) error {
	s.mu.Lock()
	ptmx := s.ptmx
	exited := s.exited
	s.mu.Unlock()
	if exited || ptmx == nil {
		return nil
	}
	if cols > 0 {
		s.Cols = cols
	}
	if rows > 0 {
		s.Rows = rows
	}
	return pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols), X: 0, Y: 0})
}

func (s *Session) attach() *Subscriber {
	sub := &Subscriber{
		id: randID(),
		ch: make(chan []byte, 128),
	}
	s.mu.Lock()
	s.subs[sub.id] = sub
	count := len(s.subs)
	s.mu.Unlock()
	_ = count
	return sub
}

func (s *Session) detachSub(sub *Subscriber) {
	s.mu.Lock()
	delete(s.subs, sub.id)
	s.mu.Unlock()
	sub.close()
}

func (s *Session) info() SessionInfo {
	s.mu.Lock()
	cols, rows := s.Cols, s.Rows
	shell := s.Shell
	user := s.User
	exited := s.exited
	detached := s.detached
	detachedAt := s.DetachedAt
	count := len(s.subs)
	var pid int
	if s.cmd != nil && s.cmd.Process != nil {
		pid = s.cmd.Process.Pid
	}
	s.mu.Unlock()
	// 从 /etc/passwd 拿用户登录 shell（zsh / ohmyzsh 时用 zsh）
	userShell := shell
	if u, err := lookupUser(user); err == nil && u.sh != "" {
		userShell = u.sh
	}
	info := SessionInfo{
		ID:        s.ID,
		Title:     s.Title,
		CreatedAt: s.CreatedAt.Format("2006-01-02 15:04:05"),
		Active:    !exited && !detached,
		Detached:  detached,
		Exited:    exited,
		Cols:      cols,
		Rows:      rows,
		Subs:      count,
		Shell:     shell,
		User:      user,
		UserShell: userShell,
		Pid:       pid,
	}
	if detached {
		info.DetachedAt = detachedAt.Format("2006-01-02 15:04:05")
	}
	return info
}

// detach 标记为后台运行（不杀进程，buffer 继续累积，ws 可继续 subscribe）
func (s *Session) detach() {
	s.mu.Lock()
	changed := false
	if !s.exited && !s.detached {
		s.detached = true
		s.DetachedAt = time.Now()
		changed = true
	}
	s.mu.Unlock()
	if changed {
		// 持久化到 sessions.json (uninstall 保留数据时, 重装可恢复)
		addPersistedSession(s)
		_ = savePersistedSessions()
	}
}

// reattach 重新 attach（从 sidebar 恢复时调用，no-op，因为前端 tab.persistent 决定后续行为）
// 我们保留 detached=true，这样 server 端知道"有 client attach 但仍标 persistent"
func (s *Session) reattach() {
	s.mu.Lock()
	// 不重置 detached — 持久 tab 关掉时还要再 detach
	s.mu.Unlock()
	_ = s
}

func (s *Session) close() {
	s.mu.Lock()
	if s.ptmx != nil {
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Signal(syscall.SIGTERM)
		}
		s.ptmx.Close()
	}
	for _, sub := range s.subs {
		sub.close()
	}
	s.subs = make(map[string]*Subscriber)
	s.exited = true
	s.detached = false
	s.mu.Unlock()
	// 真删: 从持久化列表移除 + 保存
	removePersistedSession(s.ID)
	_ = savePersistedSessions()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
