package app

import (
	"os"
	"path/filepath"
)

const (
	defaultAddr          = ":3000"
	defaultResourceRoot  = "/lzcapp/run/resources"
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
		DBPath:       getenv("LAZYCAT_MCP_DB", "/lzcapp/var/data/lazycat-mcp.db"),
		ResourceRoot: getenv("LAZYCAT_MCP_RESOURCE_ROOT", defaultResourceRoot),
	}
	if _, err := os.Stat(filepath.Dir(cfg.DBPath)); err != nil && os.IsNotExist(err) {
		cfg.DBPath = getenv("LAZYCAT_MCP_DEV_DB", defaultLocalDataPath)
	}
	return cfg
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
