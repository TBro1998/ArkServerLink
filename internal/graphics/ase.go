package graphics

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ASE 的画质设置写进客户端的 Engine\Config\BaseDeviceProfiles.ini：在 [Windows DeviceProfile]
// 末尾插入一段带标记的 +CVars= 行，还原时整段删除，文件里其他内容不动。
// 标记只用 ASCII，游戏按 ANSI 读这个文件。

// ASEConfigRel 是 BaseDeviceProfiles.ini 相对 Steam 库目录的路径。
const ASEConfigRel = `steamapps\common\ARK\Engine\Config\BaseDeviceProfiles.ini`

// BackupSuffix 是第一次修改前备份原文件的后缀。
const BackupSuffix = ".arkfgf-backup"

const (
	beginMark   = "; >>> ARKFGF Link graphics (generated, removed on restore)"
	endMark     = "; <<< ARKFGF Link graphics"
	optionsMark = "; arkfgf-options="
	sectionName = "[Windows DeviceProfile]"
)

// Patch 在 ini 里写入所选选项（先去掉以前写入的一段）。ids 里没有认得的选项时等同于 Unpatch。
func Patch(ini string, cat Catalog, ids []string) string {
	nl := newline(ini)
	lines := splitLines(Unpatch(ini))
	opts := cat.Ordered(ids)
	if len(opts) == 0 {
		return strings.Join(lines, nl)
	}

	block := []string{beginMark, optionsMark + joinIDs(opts)}
	for _, l := range dedupeCVars(opts) {
		block = append(block, "+CVars="+l)
	}
	block = append(block, endMark)

	start := -1
	for i, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l), sectionName) {
			start = i
			break
		}
	}
	if start < 0 {
		// 没有这一节（文件被改过或是空的）：在末尾补上
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sectionName, "DeviceType=Windows", "BaseProfileName=")
		lines = append(lines, block...)
		return strings.Join(append(lines, ""), nl)
	}

	// 插在这一节最后一个非空行后面
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
			end = i
			break
		}
	}
	at := end
	for at > start+1 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:at]...)
	out = append(out, block...)
	out = append(out, lines[at:]...)
	return strings.Join(out, nl)
}

// Unpatch 去掉助手写入的那一段。
func Unpatch(ini string) string {
	nl := newline(ini)
	lines := splitLines(ini)
	out := make([]string, 0, len(lines))
	in := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == beginMark:
			in = true
		case in && t == endMark:
			in = false
		case !in:
			out = append(out, l)
		}
	}
	return strings.Join(out, nl)
}

// Applied 返回 ini 里助手写入的选项 ID，没写过时为空。
func Applied(ini string) []string {
	in := false
	for _, l := range splitLines(ini) {
		t := strings.TrimSpace(l)
		switch {
		case t == beginMark:
			in = true
		case in && strings.HasPrefix(t, optionsMark):
			v := strings.TrimSpace(strings.TrimPrefix(t, optionsMark))
			if v == "" {
				return nil
			}
			return strings.Split(v, ",")
		case t == endMark:
			in = false
		}
	}
	return nil
}

// Outdated 报告 ini 里助手写入的那一段和用当前目录重新生成的不一样：
// 选项改过参数或改了名（旧版本写入的），需要重新应用。没有写过时为 false。
func Outdated(ini string, cat Catalog) bool {
	ids := Applied(ini)
	if ids == nil {
		return false
	}
	return Patch(ini, cat, ids) != ini
}

// dedupeCVars 按顺序展开选项里的 CVar，同名的只留最后一个值。
func dedupeCVars(opts []Option) []string {
	var order []string
	val := map[string]string{}
	for _, o := range opts {
		for _, l := range o.Lines {
			name := strings.ToLower(strings.TrimSpace(strings.SplitN(l, "=", 2)[0]))
			if _, ok := val[name]; !ok {
				order = append(order, name)
			}
			val[name] = l
		}
	}
	out := make([]string, len(order))
	for i, n := range order {
		out[i] = val[n]
	}
	return out
}

func joinIDs(opts []Option) string {
	ids := make([]string, len(opts))
	for i, o := range opts {
		ids[i] = o.ID
	}
	return strings.Join(ids, ",")
}

func newline(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func splitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// FindASEConfig 在各 Steam 库目录里找 ASE 客户端的 BaseDeviceProfiles.ini。stat 可注入，便于测试。
func FindASEConfig(dirs []string, stat func(string) (fs.FileInfo, error)) (string, error) {
	for _, dir := range dirs {
		p := filepath.Join(dir, "steamapps", "common", "ARK", "Engine", "Config", "BaseDeviceProfiles.ini")
		if fi, err := stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("没有找到 ARK: Survival Evolved 的 BaseDeviceProfiles.ini，请确认这台电脑通过 Steam 装了 ASE")
}

// ApplyFile 把所选选项写进 path，生效前先备份原文件到 path+BackupSuffix：
// 当前文件里没有助手写入的一段时（原版文件，或者被游戏更新、验证文件覆盖过）就备份，
// 覆盖旧的备份；已经有一段时说明备份过，不动备份。备份失败不修改文件。
func ApplyFile(path string, cat Catalog, ids []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !hasBlock(string(data)) {
		if err := writeFile(path+BackupSuffix, string(data)); err != nil {
			return err
		}
	}
	return writeFile(path, Patch(string(data), cat, ids))
}

// Backup 描述原文件的备份。
type Backup struct {
	Path    string    `json:"path"`
	Exists  bool      `json:"exists"`
	ModTime time.Time `json:"mod_time"`
}

// BackupOf 返回 path 的备份信息。
func BackupOf(path string) Backup {
	b := Backup{Path: path + BackupSuffix}
	if fi, err := os.Stat(b.Path); err == nil && !fi.IsDir() {
		b.Exists, b.ModTime = true, fi.ModTime()
	}
	return b
}

// RestoreBackupFile 用备份整份覆盖 path，恢复成第一次生效前的原文件（包括玩家自己改过的内容）。
func RestoreBackupFile(path string) error {
	data, err := os.ReadFile(path + BackupSuffix)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.New("没有找到备份：助手还没有修改过这个文件")
	}
	if err != nil {
		return err
	}
	return writeFile(path, string(data))
}

func hasBlock(ini string) bool {
	for _, l := range splitLines(ini) {
		if strings.TrimSpace(l) == beginMark {
			return true
		}
	}
	return false
}

// RestoreFile 去掉 path 里助手写入的一段。
func RestoreFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(data)
	out := Unpatch(s)
	if out == s {
		return nil
	}
	return writeFile(path, out)
}

// ReadApplied 读出 path 里助手写入的、当前目录里还有的选项，以及这一段是否需要重新应用（见 Outdated）。
func ReadApplied(path string, cat Catalog) (ids []string, outdated bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	s := string(data)
	for _, o := range cat.Ordered(Applied(s)) {
		ids = append(ids, o.ID)
	}
	return ids, Outdated(s, cat), nil
}

func writeFile(path, content string) error {
	tmp := path + ".arkfgf-tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return friendlyWriteErr(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return friendlyWriteErr(err)
	}
	return nil
}

func friendlyWriteErr(err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return errors.New("没有权限修改 BaseDeviceProfiles.ini：取消这个文件的「只读」属性，或以管理员身份运行助手")
	}
	return err
}
