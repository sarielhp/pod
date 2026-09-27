package config

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	ConfigDirName       = ".config/pod"
	LegacyConfigDirName = ".config/abs"
	ConfigFileName      = "config.json"
	OpencodeConfigFile  = ".config/opencode/opencode.json"
)

var testConfigPath string

func SetTestConfigPath(p string) {
	testConfigPath = p
}

func userTmpDir() string {
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("LOGNAME")
	}
	if username == "" {
		username = "user"
	}
	dir := filepath.Join(os.TempDir(), username, "pod")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func ConfigDir() string {
	if testConfigPath != "" {
		return filepath.Dir(testConfigPath)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return userTmpDir()
	}
	podDir := filepath.Join(home, ConfigDirName)
	if _, err := os.Stat(podDir); err == nil {
		return podDir
	}
	legacyDir := filepath.Join(home, LegacyConfigDirName)
	if _, err := os.Stat(legacyDir); err == nil {
		return legacyDir
	}
	return podDir
}

func ConfigPath() string {
	if testConfigPath != "" {
		return testConfigPath
	}
	return filepath.Join(ConfigDir(), ConfigFileName)
}

func OpencodeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, OpencodeConfigFile)
}

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ip := ipnet.IP.String()
			if !ipnet.IP.IsMulticast() && !ipnet.IP.IsLinkLocalUnicast() {
				return ip
			}
		}
	}
	return "127.0.0.1"
}

func replaceIP(url, ip string) string {
	return strings.Replace(url, "192.168.1.230", ip, 1)
}

func CacheBaseDir() string {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "cache")
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	podCacheDir := filepath.Join(cacheHome, "pod", "podcasts")
	legacyDir := filepath.Join(cacheHome, "abs", "podcasts")
	if _, err := os.Stat(podCacheDir); err == nil {
		return podCacheDir
	}
	if _, err := os.Stat(legacyDir); err == nil {
		return legacyDir
	}
	_ = os.MkdirAll(podCacheDir, 0755)
	return podCacheDir
}

func SanitizeDirName(dirPath string) string {
	clean := filepath.Clean(dirPath)
	base := filepath.Base(clean)
	h := sha256.Sum256([]byte(clean))
	hashPrefix := hex.EncodeToString(h[:4])
	safeBase := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, base)
	return safeBase + "_" + hashPrefix
}

func CacheDirForPodcast(podcastDir string) string {
	absDir, err := filepath.Abs(podcastDir)
	if err != nil {
		absDir = podcastDir
	}
	name := SanitizeDirName(absDir)
	dir := filepath.Join(CacheBaseDir(), name)
	_ = os.MkdirAll(dir, 0755)
	_ = os.MkdirAll(filepath.Join(dir, "details"), 0755)
	return dir
}
