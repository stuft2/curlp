package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/stuft2/httpp/internal/config"
)

type Cache struct {
	Dir  string
	Warn func(string)
}
type entry struct {
	Version int       `json:"version"`
	Token   string    `json:"access_token"`
	Expires time.Time `json:"expires_at"`
}

func DefaultCache() (Cache, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return Cache{}, err
	}
	return Cache{Dir: filepath.Join(base, "httpp", "tokens")}, nil
}
func (c Cache) warning() {
	if c.Warn != nil {
		c.Warn("OAuth token cache unavailable or invalid; continuing without cached data")
	}
}
func cacheKey(host string, a config.Auth) string {
	b, _ := json.Marshal(struct {
		Host string
		Auth config.Auth
	}{config.Hostname(host), a})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func private(path string, dir bool) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 || st.IsDir() != dir || (!dir && !st.Mode().IsRegular()) {
		return fmt.Errorf("unsafe cache file type")
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("unsafe cache permissions")
	}
	return nil
}
func (c Cache) prepare(create bool) error {
	// Unix modes do not establish private ACLs on Windows. Do not persist
	// bearer credentials there until an ACL-aware cache is implemented.
	if runtime.GOOS == "windows" {
		return fmt.Errorf("persistent token caching requires Unix file permissions")
	}
	if c.Dir == "" || !filepath.IsAbs(c.Dir) {
		return fmt.Errorf("cache path must be absolute")
	}
	if create {
		if err := os.MkdirAll(filepath.Dir(filepath.Dir(c.Dir)), 0700); err != nil {
			return err
		}
	}
	// Verify each application-owned directory before creating or traversing children.
	for _, d := range []string{filepath.Dir(c.Dir), c.Dir} {
		if err := private(d, true); err != nil {
			if !errors.Is(err, os.ErrNotExist) || !create {
				return err
			}
			if err := os.Mkdir(d, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			if err := private(d, true); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c Cache) load(key string, now time.Time) (string, bool) {
	if err := c.prepare(false); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			c.warning()
		}
		return "", false
	}
	path := filepath.Join(c.Dir, key+".json")
	if err := private(path, false); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			c.warning()
		}
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		c.warning()
		return "", false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		c.warning()
		return "", false
	}
	var e entry
	if json.Unmarshal(b, &e) != nil || e.Version != 1 || e.Token == "" || e.Expires.IsZero() {
		c.warning()
		return "", false
	}
	if !now.Add(30 * time.Second).Before(e.Expires) {
		return "", false
	}
	return e.Token, true
}
func (c Cache) save(key, token string, expires time.Time) {
	if err := c.prepare(true); err != nil {
		c.warning()
		return
	}
	target := filepath.Join(c.Dir, key+".json")
	if err := private(target, false); err != nil && !errors.Is(err, os.ErrNotExist) {
		c.warning()
		return
	}
	b, err := json.Marshal(entry{Version: 1, Token: token, Expires: expires})
	if err != nil {
		c.warning()
		return
	}
	f, err := os.CreateTemp(c.Dir, ".token-*")
	if err != nil {
		c.warning()
		return
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), target)
	}
	if err != nil {
		c.warning()
	}
}
func (c Cache) Clear() error {
	if err := c.prepare(false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("cannot safely open token cache")
	}
	files, err := os.ReadDir(c.Dir)
	if err != nil {
		return fmt.Errorf("read token cache: %w", err)
	}
	for _, f := range files {
		name := f.Name()
		if len(name) != 69 || filepath.Ext(name) != ".json" {
			continue
		}
		if _, err := hex.DecodeString(name[:64]); err != nil {
			continue
		}
		// Remove the entry itself, never follow symlinks or descend into directories.
		if f.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(c.Dir, name)); err != nil {
			return fmt.Errorf("remove token cache entry: %w", err)
		}
	}
	return nil
}
