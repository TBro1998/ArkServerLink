package auth

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Store 把登录令牌保存在用户配置目录里。Windows 上用 DPAPI 加密（只有当前 Windows 用户能解开），
// 其他系统退化为权限 0600 的明文文件。
type Store struct {
	path string
}

// DefaultStore 返回 %AppData%\ArkServerLink\token（其他系统为 ~/.config/ArkServerLink/token）。
func DefaultStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewStore(filepath.Join(dir, "ArkServerLink", "token")), nil
}

func NewStore(path string) *Store { return &Store{path: path} }

// Load 读取令牌，没有保存过时返回空串。文件损坏（例如换了 Windows 用户）也当作未登录。
func (s *Store) Load() (string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plain, err := unprotect(data)
	if err != nil {
		_ = os.Remove(s.path)
		return "", nil
	}
	return strings.TrimSpace(string(plain)), nil
}

func (s *Store) Save(token string) error {
	data, err := protect([]byte(token))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Clear() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
