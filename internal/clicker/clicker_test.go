package clicker

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type event struct {
	action string
	vk     uint16
	down   bool
}

type fakeDriver struct {
	mu        sync.Mutex
	events    []event
	focused   bool
	pressErr  error
	hotkeyErr error
	hotkey    func()
	unreg     int
}

func (d *fakeDriver) press(action string, vk uint16, down bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pressErr != nil {
		return d.pressErr
	}
	d.events = append(d.events, event{action, vk, down})
	return nil
}

func (d *fakeDriver) gameFocused() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.focused
}

func (d *fakeDriver) registerHotkey(vk uint16, fn func()) (func(), error) {
	if d.hotkeyErr != nil {
		return nil, d.hotkeyErr
	}
	d.hotkey = fn
	return func() { d.mu.Lock(); d.unreg++; d.mu.Unlock() }, nil
}

func (d *fakeDriver) setFocused(v bool) { d.mu.Lock(); d.focused = v; d.mu.Unlock() }

func (d *fakeDriver) snapshot() []event {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]event(nil), d.events...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时：%s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()
	if _, err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []Config{
		{Action: "middle", Mode: ModeClick, IntervalMs: 100, Hotkey: "F8"},
		{Action: ActionKey, Key: "ctrl", Mode: ModeClick, IntervalMs: 100, Hotkey: "F8"},
		{Action: ActionLeft, Mode: ModeClick, IntervalMs: 5, Hotkey: "F8"},
		{Action: ActionLeft, Mode: ModeClick, IntervalMs: 100, Hotkey: "F1"},
		{Action: ActionLeft, Mode: "double", IntervalMs: 100, Hotkey: "F8"},
	}
	for _, c := range bad {
		if _, err := c.Validate(); err == nil {
			t.Errorf("%+v 应当无效", c)
		}
	}
	// 按住模式不看间隔，按键规范成大写
	got, err := Config{Action: ActionKey, Key: " e ", Mode: ModeHold, Hotkey: "f9"}.Validate()
	if err != nil || got.Key != "E" || got.Hotkey != "F9" {
		t.Fatalf("Validate = %+v, %v", got, err)
	}
	if vk, _ := KeyCode("SPACE"); vk != 0x20 {
		t.Fatal("Space")
	}
	if vk, _ := hotkeyCode("F12"); vk != 0x7B {
		t.Fatalf("F12 = %#x", vk)
	}
}

func TestClickOnlyWhenGameFocused(t *testing.T) {
	d := &fakeDriver{}
	c := newWith(d, Config{Action: ActionLeft, Mode: ModeClick, IntervalMs: MinInterval, Hotkey: "F8", GameOnly: true}, nil)
	defer c.Close()

	c.Start()
	waitFor(t, "暂停", func() bool { return c.State().Paused })
	time.Sleep(60 * time.Millisecond)
	if n := len(d.snapshot()); n != 0 {
		t.Fatalf("游戏不在前台时点了 %d 次", n)
	}

	d.setFocused(true)
	waitFor(t, "点击", func() bool { return c.State().Clicks >= 3 })
	c.Stop()
	ev := d.snapshot()
	for i, e := range ev {
		if e.action != ActionLeft || e.down != (i%2 == 0) {
			t.Fatalf("第 %d 个事件 %+v：按下和松开没有成对", i, e)
		}
	}
	if len(ev)%2 != 0 {
		t.Fatal("停下时没有松开")
	}
	if s := c.State(); s.Running || s.Paused {
		t.Fatalf("停止后状态 %+v", s)
	}
}

func TestHoldReleasesWhenFocusLost(t *testing.T) {
	d := &fakeDriver{focused: true}
	c := newWith(d, Config{Action: ActionKey, Key: "E", Mode: ModeHold, Hotkey: "F8", GameOnly: true}, nil)
	c.tick = 5 * time.Millisecond
	defer c.Close()

	c.Start()
	waitFor(t, "按住", func() bool { return c.State().Holding })
	d.setFocused(false)
	waitFor(t, "切走后松开", func() bool { return !c.State().Holding && c.State().Paused })
	d.setFocused(true)
	waitFor(t, "切回来再按住", func() bool { return c.State().Holding })
	c.Stop()

	want := []bool{true, false, true, false}
	ev := d.snapshot()
	if len(ev) != len(want) {
		t.Fatalf("事件 %+v", ev)
	}
	for i, e := range ev {
		if e.down != want[i] || e.vk != 'E' {
			t.Fatalf("事件 %+v", ev)
		}
	}
}

func TestHotkeyToggles(t *testing.T) {
	d := &fakeDriver{focused: true}
	c := newWith(d, DefaultConfig(), nil)
	defer c.Close()
	if c.State().Hotkey != "F8" {
		t.Fatalf("热键没注册：%+v", c.State())
	}
	d.hotkey()
	waitFor(t, "热键打开", func() bool { return c.State().Running })
	d.hotkey()
	waitFor(t, "热键关闭", func() bool { return !c.State().Running })
}

func TestHotkeyFailureAndReRegister(t *testing.T) {
	d := &fakeDriver{hotkeyErr: errors.New("busy")}
	c := newWith(d, DefaultConfig(), nil)
	defer c.Close()
	if s := c.State(); s.Hotkey != "" || s.Error == "" {
		t.Fatalf("注册失败应当记下错误：%+v", s)
	}
	d.hotkeyErr = nil
	cfg := DefaultConfig()
	cfg.Hotkey = "F9"
	if _, err := c.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if s := c.State(); s.Hotkey != "F9" || s.Error != "" {
		t.Fatalf("换热键后 %+v", s)
	}
	cfg.Hotkey = "F10"
	c.SetConfig(cfg)
	if d.unreg != 1 {
		t.Fatalf("换热键应当注销旧的，注销了 %d 次", d.unreg)
	}
}

func TestPressErrorStops(t *testing.T) {
	d := &fakeDriver{focused: true, pressErr: errors.New("blocked")}
	c := newWith(d, DefaultConfig(), nil)
	defer c.Close()
	c.Start()
	waitFor(t, "出错停下", func() bool { s := c.State(); return !s.Running && s.Error != "" })
	c.Start() // 出错后可以再打开
	waitFor(t, "再次出错", func() bool { return !c.State().Running })
}
