package app

import (
	"os"
	"path/filepath"
	"strconv"
)

const (
	defaultAddr          = ":3000"
	defaultResourceRoot  = "/lzcapp/run/resources"
	defaultLazyCatVarDir = "/lzcapp/var"
	selfPackageID        = "cloud.lazycat.app.czyt.lazycat-mcp"
	selfSkillResourceID  = "lazycat-mcp.skill"
	defaultLocalDataPath = "data/lazycat-mcp.db"
)

type Config struct {
	Addr                string
	DBPath              string
	ResourceRoot        string
	MCPLogRetentionDays int
}

func LoadConfig() Config {
	cfg := Config{
		Addr:                getenv("LAZYCAT_MCP_ADDR", defaultAddr),
		DBPath:              resolveDBPath(defaultLazyCatVarDir),
		ResourceRoot:        getenv("LAZYCAT_MCP_RESOURCE_ROOT", defaultResourceRoot),
		MCPLogRetentionDays: getenvNonNegativeInt("LAZYCAT_MCP_LOG_RETENTION_DAYS", 30),
	}
	return cfg
}

func resolveDBPath(lazycatVarDir string) string {
	if value := os.Getenv("LAZYCAT_MCP_DB"); value != "" {
		return value
	}
	if info, err := os.Stat(lazycatVarDir); err == nil && info.IsDir() {
		return filepath.Join(lazycatVarDir, "data", "lazycat-mcp.db")
	}
	return getenv("LAZYCAT_MCP_DEV_DB", defaultLocalDataPath)
}

func getenv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvNonNegativeInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func SelfSkillInstallPath() string {
	return "/skills/" + selfPackageID + "/" + selfSkillResourceID + "/SKILL.md"
}
