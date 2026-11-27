package graphics

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const sampleIni = "[DeviceProfiles]\r\n" +
	"+DeviceProfileNameAndTypes=Windows,Windows\r\n" +
	"\r\n" +
	"[Windows DeviceProfile]\r\n" +
	"DeviceType=Windows\r\n" +
	"BaseProfileName=\r\n" +
	"\r\n" +
	"[WindowsNoEditor DeviceProfile]\r\n" +
	"DeviceType=WindowsNoEditor\r\n" +
	"BaseProfileName=Windows\r\n"

func TestPatchInsertsIntoWindowsSection(t *testing.T) {
	out := Patch(sampleIni, For(ASE), []string{"underwater", "fog"})

	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Fatalf("CRLF 文件里混进了 LF:\n%q", out)
	}
	lines := strings.Split(out, "\r\n")
	idx := func(s string) int {
		for i, l := range lines {
			if l == s {
				return i
			}
		}
		return -1
	}
	sec, base, next := idx("[Windows DeviceProfile]"), idx("BaseProfileName="), idx("[WindowsNoEditor DeviceProfile]")
	begin, cd := idx(beginMark), idx("+CVars=r.CustomDepth=0")
	if !(sec < base && base < begin && begin < cd && cd < next) {
		t.Fatalf("插入位置不对:\n%s", out)
	}
	// 顺序按目录：fog 在 underwater 前面
	if got := Applied(out); !reflect.DeepEqual(got, []string{"fog", "underwater"}) {
		t.Fatalf("Applied = %v", got)
	}
	// 节之间的空行保留
	if lines[idx(endMark)+1] != "" {
		t.Fatalf("下一节前的空行丢了:\n%s", out)
	}
}

func TestPatchReplacesAndUnpatchRestores(t *testing.T) {
	cat := For(ASE)
	once := Patch(sampleIni, cat, []string{"shadows"})
	twice := Patch(once, cat, []string{"sky"})
	if strings.Count(twice, beginMark) != 1 || strings.Contains(twice, "r.ShadowQuality") {
		t.Fatalf("重新应用没有替换旧的一段:\n%s", twice)
	}
	if got := Unpatch(twice); got != sampleIni {
		t.Fatalf("还原后和原文件不同:\n%q\n%q", got, sampleIni)
	}
	if got := Patch(once, cat, nil); got != sampleIni {
		t.Fatalf("不选任何选项应当等于还原:\n%q", got)
	}
	if Applied(sampleIni) != nil {
		t.Fatal("原文件不应有已应用的选项")
	}
}

func TestPatchWithoutSection(t *testing.T) {
	out := Patch("[DeviceProfiles]\n+DeviceProfileNameAndTypes=Windows,Windows\n", For(ASE), []string{"fog"})
	if !strings.Contains(out, "\n[Windows DeviceProfile]\nDeviceType=Windows\nBaseProfileName=\n"+beginMark) {
		t.Fatalf("没有补上 [Windows DeviceProfile]:\n%s", out)
	}
	if got := Applied(out); !reflect.DeepEqual(got, []string{"fog"}) {
		t.Fatalf("Applied = %v", got)
	}
}

func TestDedupeCVarsKeepsLastValue(t *testing.T) {
	got := dedupeCVars([]Option{
		{Lines: []string{"r.A=1", "r.B=1"}},
		{Lines: []string{"R.a=0"}},
	})
	if !reflect.DeepEqual(got, []string{"R.a=0", "r.B=1"}) {
		t.Fatalf("dedupe = %v", got)
	}
}

func TestCatalogsAreConsistent(t *testing.T) {
	for _, g := range []Game{ASE, ASA} {
		cat := For(g)
		seen := map[string]bool{}
		for _, o := range cat.Options {
			if seen[o.ID] || strings.Contains(o.ID, ",") {
				t.Errorf("%s: 选项 ID %q 重复或含逗号", g, o.ID)
			}
			seen[o.ID] = true
			if o.Group != GroupPerf && o.Group != GroupCombat {
				t.Errorf("%s/%s: 分组 %q", g, o.ID, o.Group)
			}
			if len(o.Lines) == 0 {
				t.Errorf("%s/%s: 没有参数", g, o.ID)
			}
			for _, l := range o.Lines {
				if g == ASE && !strings.Contains(l, "=") {
					t.Errorf("%s/%s: ini 行 %q 没有 =", g, o.ID, l)
				}
				if g == ASA && (strings.Contains(l, "|") || strings.Contains(l, "=")) {
					t.Errorf("%s/%s: 控制台命令 %q 格式不对", g, o.ID, l)
				}
			}
			for _, l := range o.Console {
				if g == ASA || strings.Contains(l, "|") || strings.Contains(l, "=") || strings.HasPrefix(l, "ShowFlag.") {
					t.Errorf("%s/%s: 控制台写法 %q 不对（ASA 用 Lines，ShowFlag 不能在控制台改）", g, o.ID, l)
				}
			}
			if o.ConsoleNote != "" && len(o.Console) == 0 {
				t.Errorf("%s/%s: 没有控制台写法却有说明", g, o.ID)
			}
		}
		for _, p := range cat.Presets {
			for _, id := range p.Options {
				if !seen[id] {
					t.Errorf("%s 档位 %s 引用了不存在的选项 %q", g, p.ID, id)
				}
			}
		}
	}
}

func TestCommand(t *testing.T) {
	cat := For(ASA)
	got := Command(cat, []string{"fog", "clouds", "nope"}, 3)
	if got != "r.VolumetricCloud 0 | r.Fog 0 | r.VolumetricFog 0 | gamma 3" {
		t.Fatalf("Command = %q", got)
	}
	if got := Command(cat, nil, 0); got != "" {
		t.Fatalf("空选择 = %q", got)
	}
	// ASE 用 Console，只能写 ini 的选项跳过
	ase := Command(For(ASE), []string{"lightshafts", "fog", "translucency"}, 0)
	if ase != "r.LightShafts 0 | r.LightShaftQuality 0" {
		t.Fatalf("ASE Command = %q", ase)
	}
	if GammaCommand(2.5) != "gamma 2.5" || GammaCommand(9) != "" {
		t.Fatal("GammaCommand 范围不对")
	}
}

func TestApplyAndRestoreFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "BaseDeviceProfiles.ini")
	if err := os.WriteFile(p, []byte(sampleIni), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := For(ASE)
	if err := ApplyFile(p, cat, []string{"post"}); err != nil {
		t.Fatal(err)
	}
	// 第二次应用不覆盖备份
	if err := ApplyFile(p, cat, []string{"sky"}); err != nil {
		t.Fatal(err)
	}
	bak, _ := os.ReadFile(p + BackupSuffix)
	if string(bak) != sampleIni {
		t.Fatalf("备份不是原文件:\n%s", bak)
	}
	if got, _ := ReadApplied(p); !reflect.DeepEqual(got, []string{"sky"}) {
		t.Fatalf("ReadApplied = %v", got)
	}
	if err := RestoreFile(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != sampleIni {
		t.Fatalf("还原后:\n%s", data)
	}
}

func TestBackupRefreshedWhenFileReplaced(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "BaseDeviceProfiles.ini")
	cat := For(ASE)
	if b := BackupOf(p); b.Exists {
		t.Fatal("还没有备份")
	}
	if err := RestoreBackupFile(p); err == nil {
		t.Fatal("没有备份时应当报错")
	}
	os.WriteFile(p, []byte(sampleIni), 0o644)
	if err := ApplyFile(p, cat, []string{"post"}); err != nil {
		t.Fatal(err)
	}
	// 游戏更新覆盖了文件（没有助手的一段），再次生效时备份新的原文件
	updated := sampleIni + "; game update\r\n"
	os.WriteFile(p, []byte(updated), 0o644)
	if err := ApplyFile(p, cat, []string{"fog"}); err != nil {
		t.Fatal(err)
	}
	if b := BackupOf(p); !b.Exists || b.Path != p+BackupSuffix {
		t.Fatalf("BackupOf = %+v", b)
	}
	if err := RestoreBackupFile(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != updated {
		t.Fatalf("恢复的不是最新的原文件:\n%s", data)
	}
}

type fakeInfo struct{ dir bool }

func (f fakeInfo) Name() string       { return "" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return 0 }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.dir }
func (f fakeInfo) Sys() any           { return nil }

func TestFindASEConfig(t *testing.T) {
	want := filepath.Join("D:", "Games", "steamapps", "common", "ARK", "Engine", "Config", "BaseDeviceProfiles.ini")
	stat := func(p string) (fs.FileInfo, error) {
		if p == want {
			return fakeInfo{}, nil
		}
		return nil, fs.ErrNotExist
	}
	got, err := FindASEConfig([]string{filepath.Join("C:", "Steam"), filepath.Join("D:", "Games")}, stat)
	if err != nil || got != want {
		t.Fatalf("FindASEConfig = %q, %v", got, err)
	}
	if _, err := FindASEConfig([]string{"x"}, stat); err == nil {
		t.Fatal("找不到时应当报错")
	}
}
