//go:build windows

package steam

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeFS 用一组"存在的路径"冒充文件系统，避免测试依赖本机装没装 ARK。
func fakeFS(files ...string) func(string) (os.FileInfo, error) {
	set := make(map[string]bool, len(files))
	for _, f := range files {
		set[strings.ToLower(f)] = true
	}
	return func(p string) (os.FileInfo, error) {
		if set[strings.ToLower(p)] {
			return fakeInfo{name: filepath.Base(p)}, nil
		}
		return nil, errors.New("no such file")
	}
}

type fakeInfo struct{ name string }

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return 0 }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any           { return nil }

func TestFindIn(t *testing.T) {
	steamDir := `C:\Program Files (x86)\Steam`
	libDir := `F:\Steam`

	t.Run("Steam 本体目录里有", func(t *testing.T) {
		got, err := findIn([]string{steamDir, libDir}, fakeFS(filepath.Join(steamDir, arkRelPath)))
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(steamDir, arkRelPath) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("只在其他库目录里", func(t *testing.T) {
		// 游戏装在第二个库时也要找到，这是 libraryfolders.vdf 存在的意义
		got, err := findIn([]string{steamDir, libDir}, fakeFS(filepath.Join(libDir, arkRelPath)))
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(libDir, arkRelPath) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("按顺序优先 Steam 本体目录", func(t *testing.T) {
		got, err := findIn([]string{steamDir, libDir}, fakeFS(
			filepath.Join(steamDir, arkRelPath),
			filepath.Join(libDir, arkRelPath),
		))
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(steamDir, arkRelPath) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("都没有时报错", func(t *testing.T) {
		if _, err := findIn([]string{steamDir, libDir}, fakeFS()); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("路径存在但是目录不算", func(t *testing.T) {
		stat := func(string) (os.FileInfo, error) { return dirInfo{}, nil }
		if _, err := findIn([]string{steamDir}, stat); err == nil {
			t.Fatal("want error")
		}
	})
}

type dirInfo struct{}

func (dirInfo) Name() string       { return "ShooterGame.exe" }
func (dirInfo) Size() int64        { return 0 }
func (dirInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (dirInfo) ModTime() time.Time { return time.Time{} }
func (dirInfo) IsDir() bool        { return true }
func (dirInfo) Sys() any           { return nil }

func TestParseLibraryPaths(t *testing.T) {
	// 真实 libraryfolders.vdf 里的反斜杠是转义的两个字符，解析后要变回一个
	vdf := `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"label"		""
	}
	"1"
	{
		"path"		"F:\\Steam"
		"label"		""
	}
}`
	got := parseLibraryPaths(vdf, `C:\Program Files (x86)\Steam`)
	if len(got) != 1 || got[0] != `F:\Steam` {
		t.Fatalf("got %#v", got)
	}
}

func TestParseLibraryPathsSkipsDuplicateAndEmpty(t *testing.T) {
	vdf := `"path" "F:\\Steam"`
	vdf += "\n" + `"path" "f:\\steam"` // 只差大小写，视为同一个
	vdf += "\n" + `"path" ""`
	vdf += "\n" + `"path" "F:\\SteamLibrary"`
	got := parseLibraryPaths(vdf, `C:\Steam`)
	if len(got) != 2 || got[0] != `F:\Steam` || got[1] != `F:\SteamLibrary` {
		t.Fatalf("got %#v", got)
	}
}

func TestCandidateDirsIncludesSteamFirst(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	vdf := `"libraryfolders" { "0" { "path" "` + dir + `" } "1" { "path" "F:\\SteamLibrary" } }`
	if err := os.WriteFile(filepath.Join(dir, "steamapps", "libraryfolders.vdf"), []byte(vdf), 0o644); err != nil {
		t.Fatal(err)
	}
	got := candidateDirs(dir)
	// Steam 本体目录在前，且不因为它出现在 vdf 里就重复一次
	if len(got) != 2 || got[0] != dir || got[1] != `F:\SteamLibrary` {
		t.Fatalf("got %#v", got)
	}
}

func TestCandidateDirsWithoutVdf(t *testing.T) {
	dir := t.TempDir()
	got := candidateDirs(dir)
	if len(got) != 1 || got[0] != dir {
		t.Fatalf("got %#v", got)
	}
}

// ARK 只认 -ExecCmds="..." 这种原样的引号，不能让 Go 转义成 \"
func TestArkCmdLine(t *testing.T) {
	exe := `F:\Steam\steamapps\common\ARK\ShooterGame\Binaries\Win64\ShooterGame.exe`
	want := `"` + exe + `" -ExecCmds="open 127.0.0.1:50145"`
	if got := arkCmdLine(exe, 50145); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if got := ARKCommand(exe, 50145).SysProcAttr.CmdLine; got != want {
		t.Fatalf("SysProcAttr.CmdLine = %s", got)
	}
}
