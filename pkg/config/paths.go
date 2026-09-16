package config

import (
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
