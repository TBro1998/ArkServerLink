// Package clicker 是连点器：按全局热键开关，按设定的间隔模拟鼠标点击或按键，也可以一直按住。
// 默认只在 ARK 窗口位于前台时点击，切到别的窗口自动暂停（按住的键会松开）。
package clicker

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// 点击的对象。
const (
	ActionLeft  = "left"  // 鼠标左键
	ActionRight = "right" // 鼠标右键
	ActionKey   = "key"   // 键盘按键，见 Config.Key
)

// 点击方式。
const (
	ModeClick = "click" // 按间隔连续点击
	ModeHold  = "hold"  // 一直按住，再按热键松开
)

// 间隔范围（毫秒）。
const (
	MinInterval = 20
	MaxInterval = 60_000
)

// pressGap 是一次点击里按下和松开之间的时间，太短游戏可能收不到。
const pressGap = 12 * time.Millisecond

// Config 是连点器设置。
type Config struct {
	Action     string `json:"action"`
	Key        string `json:"key"` // ActionKey 时的按键：A-Z、0-9、Space
	Mode       string `json:"mode"`
	IntervalMs int    `json:"interval_ms"`
	Hotkey     string `json:"hotkey"`    // 开关热键：F5-F12
	GameOnly   bool   `json:"game_only"` // 只在 ARK 窗口位于前台时点击
}

// DefaultConfig 是第一次使用时的设置：F8 开关，每 100 毫秒点一下左键，只在游戏里生效。
func DefaultConfig() Config {
	return Config{Action: ActionLeft, Key: "E", Mode: ModeClick, IntervalMs: 100, Hotkey: "F8", GameOnly: true}
}

// Hotkeys 是可以选的开关热键。
var Hotkeys = []string{"F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12"}

// Validate 检查设置，返回规范化后的值（按键转成大写）。
func (c Config) Validate() (Config, error) {
	c.Key = strings.ToUpper(strings.TrimSpace(c.Key))
	c.Hotkey = strings.ToUpper(strings.TrimSpace(c.Hotkey))
	switch c.Action {
	case ActionLeft, ActionRight:
	case ActionKey:
		if _, ok := KeyCode(c.Key); !ok {
			return c, fmt.Errorf("不支持的按键 %q，可以用 A-Z、0-9 或 Space", c.Key)
		}
	default:
		return c, fmt.Errorf("不支持的点击对象 %q", c.Action)
	}
	if c.Mode != ModeClick && c.Mode != ModeHold {
		return c, fmt.Errorf("不支持的点击方式 %q", c.Mode)
	}
	if c.Mode == ModeClick && (c.IntervalMs < MinInterval || c.IntervalMs > MaxInterval) {
		return c, fmt.Errorf("间隔要在 %d 到 %d 毫秒之间", MinInterval, MaxInterval)
	}
	if _, ok := hotkeyCode(c.Hotkey); !ok {
		return c, fmt.Errorf("不支持的热键 %q", c.Hotkey)
	}
	return c, nil
}

// KeyCode 返回按键的 Windows 虚拟键码。
func KeyCode(k string) (uint16, bool) {
	switch {
	case k == "SPACE":
		return 0x20, true
	case len(k) == 1 && k[0] >= 'A' && k[0] <= 'Z', len(k) == 1 && k[0] >= '0' && k[0] <= '9':
		return uint16(k[0]), true
	}
	return 0, false
}

func hotkeyCode(k string) (uint16, bool) {
	for i, h := range Hotkeys {
		if h == k {
			return uint16(0x74 + i), true // VK_F5 = 0x74
		}
	}
	return 0, false
}

// State 是连点器状态，变化时通过 onChange 通知。
type State struct {
	Running bool   `json:"running"` // 已用热键或按钮打开
	Paused  bool   `json:"paused"`  // 打开了，但 ARK 不在前台，暂时不点
	Holding bool   `json:"holding"` // 按住模式下正按着
	Clicks  int64  `json:"clicks"`  // 这次打开以来点了多少下
	Hotkey  string `json:"hotkey"`  // 已注册的热键，注册失败时为空
	Error   string `json:"error"`
}

// driver 是系统相关的部分，测试时替换。
type driver interface {
	// press 按下（down 为 true）或松开一次。
	press(action string, key uint16, down bool) error
	// gameFocused 报告前台窗口是不是 ARK（ASE 或 ASA）。
	gameFocused() bool
	// registerHotkey 注册全局热键，按下时调用 fn。返回注销函数。
	registerHotkey(vk uint16, fn func()) (func(), error)
}

// Clicker 是连点器。
type Clicker struct {
	drv      driver
	onChange func(State)
	tick     time.Duration // 按住模式和暂停时检查前台窗口的间隔

	mu         sync.Mutex
	cfg        Config
	state      State
	stop       chan struct{} // 正在运行时有值
	done       chan struct{}
	unregister func()
}

// New 创建连点器并注册 cfg 里的热键。热键注册失败不报错，记在 State.Error 里，按钮仍然可用。
func New(cfg Config, onChange func(State)) *Clicker {
	return newWith(newDriver(), cfg, onChange)
}

func newWith(d driver, cfg Config, onChange func(State)) *Clicker {
	if onChange == nil {
		onChange = func(State) {}
	}
	c := &Clicker{drv: d, onChange: onChange, tick: 50 * time.Millisecond}
	if v, err := cfg.Validate(); err == nil {
		cfg = v
	} else {
		cfg = DefaultConfig()
	}
	c.mu.Lock()
	c.cfg = cfg
	c.registerLocked()
	c.mu.Unlock()
	return c
}

// Config 返回当前设置。
func (c *Clicker) Config() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// SetConfig 换设置：正在运行时先停下，热键变了重新注册。
func (c *Clicker) SetConfig(cfg Config) (Config, error) {
	cfg, err := cfg.Validate()
	if err != nil {
		return c.Config(), err
	}
	c.Stop()
	c.mu.Lock()
	old := c.cfg.Hotkey
	c.cfg = cfg
	if cfg.Hotkey != old || c.state.Hotkey == "" {
		c.registerLocked()
	}
	st := c.state
	c.mu.Unlock()
	c.onChange(st)
	return cfg, nil
}

// registerLocked 按 c.cfg 重新注册热键。
func (c *Clicker) registerLocked() {
	if c.unregister != nil {
		c.unregister()
		c.unregister = nil
	}
	c.state.Hotkey, c.state.Error = "", ""
	vk, _ := hotkeyCode(c.cfg.Hotkey)
	un, err := c.drv.registerHotkey(vk, func() { go c.Toggle() })
	if err != nil {
		c.state.Error = fmt.Sprintf("热键 %s 注册失败（可能被别的程序占用了），换一个热键或用按钮开关：%v", c.cfg.Hotkey, err)
		return
	}
	c.unregister = un
	c.state.Hotkey = c.cfg.Hotkey
}

// State 返回当前状态。
func (c *Clicker) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Toggle 打开或关闭。
func (c *Clicker) Toggle() {
	c.mu.Lock()
	running := c.stop != nil
	c.mu.Unlock()
	if running {
		c.Stop()
	} else {
		c.Start()
	}
}

// Start 开始点击，已经在运行时什么都不做。
func (c *Clicker) Start() {
	c.mu.Lock()
	if c.stop != nil {
		c.mu.Unlock()
		return
	}
	stop, done := make(chan struct{}), make(chan struct{})
	c.stop, c.done = stop, done
	cfg := c.cfg
	c.state.Running, c.state.Paused, c.state.Holding, c.state.Clicks = true, false, false, 0
	st := c.state
	c.mu.Unlock()
	c.onChange(st)
	go c.run(cfg, stop, done)
}

// Stop 停止点击并松开按住的键，等点击协程退出后返回。
func (c *Clicker) Stop() {
	c.mu.Lock()
	stop, done := c.stop, c.done
	c.stop, c.done = nil, nil
	c.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	<-done
	c.mu.Lock()
	c.state.Running, c.state.Paused, c.state.Holding = false, false, false
	st := c.state
	c.mu.Unlock()
	c.onChange(st)
}

// Close 停止并注销热键。
func (c *Clicker) Close() {
	c.Stop()
	c.mu.Lock()
	if c.unregister != nil {
		c.unregister()
		c.unregister = nil
	}
	c.mu.Unlock()
}

func (c *Clicker) run(cfg Config, stop, done chan struct{}) {
	defer close(done)
	vk, _ := KeyCode(cfg.Key)
	holding := false
	release := func() {
		if holding {
			_ = c.drv.press(cfg.Action, vk, false)
			holding = false
			c.update(func(s *State) { s.Holding = false })
		}
	}
	defer release()

	wait := c.tick
	if cfg.Mode == ModeClick {
		wait = time.Duration(cfg.IntervalMs) * time.Millisecond
	}
	t := time.NewTicker(wait)
	defer t.Stop()
	for {
		paused := cfg.GameOnly && !c.drv.gameFocused()
		c.update(func(s *State) { s.Paused = paused })
		switch {
		case paused:
			release()
		case cfg.Mode == ModeHold:
			if !holding {
				if err := c.drv.press(cfg.Action, vk, true); err != nil {
					c.fail(err)
					return
				}
				holding = true
				c.update(func(s *State) { s.Holding = true })
			}
		default:
			if err := c.click(cfg.Action, vk, stop); err != nil {
				if errors.Is(err, errStopped) {
					return
				}
				c.fail(err)
				return
			}
			c.mu.Lock()
			c.state.Clicks++
			c.mu.Unlock()
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

var errStopped = errors.New("stopped")

func (c *Clicker) click(action string, vk uint16, stop chan struct{}) error {
	if err := c.drv.press(action, vk, true); err != nil {
		return err
	}
	select {
	case <-stop:
		_ = c.drv.press(action, vk, false)
		return errStopped
	case <-time.After(pressGap):
	}
	return c.drv.press(action, vk, false)
}

// update 修改状态，有变化时通知（点击计数不触发通知）。
func (c *Clicker) update(fn func(*State)) {
	c.mu.Lock()
	before := c.state
	fn(&c.state)
	after := c.state
	c.mu.Unlock()
	before.Clicks = after.Clicks
	if before != after {
		c.onChange(after)
	}
}

// fail 在点击出错时自己停下来（不能在协程里调用 Stop，会等自己退出）。
func (c *Clicker) fail(err error) {
	c.mu.Lock()
	c.stop, c.done = nil, nil
	c.state.Running, c.state.Paused, c.state.Holding = false, false, false
	c.state.Error = "模拟点击失败：" + err.Error()
	st := c.state
	c.mu.Unlock()
	c.onChange(st)
}
