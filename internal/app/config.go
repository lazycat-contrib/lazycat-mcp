package app

import (
	"os"
	"path/filepath"
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
	Addr         string
	DBPath       string
	ResourceRoot string
}

func LoadConfig() Config {
	cfg := Config{
		Addr:         getenv("LAZYCAT_MCP_ADDR", defaultAddr),
		DBPath:       resolveDBPath(defaultLazyCatVarDir),
		ResourceRoot: getenv("LAZYCAT_MCP_RESOURCE_ROOT", defaultResourceRoot),
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

func SelfSkillInstallPath() string {
	return "/skills/" + selfPackageID + "/" + selfSkillResourceID + "/SKILL.md"
}
