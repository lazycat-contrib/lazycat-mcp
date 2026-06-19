package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type MCPResource struct {
	AppID      string `json:"app_id"`
	ResourceID string `json:"resource_id"`
	Endpoint   string `json:"endpoint"`
	FilePath   string `json:"file_path,omitempty"`
}

type SkillResource struct {
	AppID      string `json:"app_id"`
	ResourceID string `json:"resource_id"`
	FilePath   string `json:"file_path,omitempty"`
	PublicPath string `json:"public_path"`
}

type ResourceIndex struct {
	MCPByApp    map[string][]MCPResource
	SkillsByApp map[string][]SkillResource
}

type ResourceScanner struct {
	root string
}

func NewResourceScanner(root string) *ResourceScanner {
	return &ResourceScanner{root: root}
}

func (s *ResourceScanner) Root() string {
	return s.root
}

func (s *ResourceScanner) Scan(ctx context.Context) ResourceIndex {
	index := ResourceIndex{
		MCPByApp:    make(map[string][]MCPResource),
		SkillsByApp: make(map[string][]SkillResource),
	}
	for _, item := range scanMCPResources(ctx, filepath.Join(s.root, "mcp-providers")) {
		index.MCPByApp[item.AppID] = append(index.MCPByApp[item.AppID], item)
	}
	for _, item := range scanSkillResources(ctx, filepath.Join(s.root, "skills")) {
		index.SkillsByApp[item.AppID] = append(index.SkillsByApp[item.AppID], item)
	}
	for appID := range index.MCPByApp {
		sort.Slice(index.MCPByApp[appID], func(i, j int) bool {
			if index.MCPByApp[appID][i].ResourceID == "default" {
				return true
			}
			if index.MCPByApp[appID][j].ResourceID == "default" {
				return false
			}
			return index.MCPByApp[appID][i].ResourceID < index.MCPByApp[appID][j].ResourceID
		})
	}
	for appID := range index.SkillsByApp {
		sort.Slice(index.SkillsByApp[appID], func(i, j int) bool {
			return index.SkillsByApp[appID][i].ResourceID < index.SkillsByApp[appID][j].ResourceID
		})
	}
	return index
}

func (idx ResourceIndex) AppIDs() []string {
	seen := make(map[string]struct{})
	for appID := range idx.MCPByApp {
		seen[appID] = struct{}{}
	}
	for appID := range idx.SkillsByApp {
		seen[appID] = struct{}{}
	}
	appIDs := make([]string, 0, len(seen))
	for appID := range seen {
		appIDs = append(appIDs, appID)
	}
	sort.Strings(appIDs)
	return appIDs
}

func (idx ResourceIndex) DefaultMCPEndpoint(appID string) string {
	items := idx.MCPByApp[appID]
	if len(items) == 0 {
		return ""
	}
	return items[0].Endpoint
}

func (idx ResourceIndex) DefaultMCPResourceID(appID string) string {
	items := idx.MCPByApp[appID]
	if len(items) == 0 {
		return ""
	}
	return items[0].ResourceID
}

func scanMCPResources(ctx context.Context, root string) []MCPResource {
	appDirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var resources []MCPResource
	for _, appDir := range appDirs {
		if ctx.Err() != nil {
			return resources
		}
		if !appDir.IsDir() || skipResourceDir(appDir.Name()) {
			continue
		}
		providerRoot := filepath.Join(root, appDir.Name())
		providerDirs, err := os.ReadDir(providerRoot)
		if err != nil {
			continue
		}
		for _, providerDir := range providerDirs {
			if !providerDir.IsDir() || skipResourceDir(providerDir.Name()) {
				continue
			}
			filePath := filepath.Join(providerRoot, providerDir.Name(), "mcp.yml")
			endpoint, err := readMCPEndpoint(filePath)
			if err != nil {
				continue
			}
			resources = append(resources, MCPResource{
				AppID:      appDir.Name(),
				ResourceID: providerDir.Name(),
				Endpoint:   endpoint,
				FilePath:   filePath,
			})
		}
	}
	return resources
}

func readMCPEndpoint(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	var doc struct {
		Endpoint string `yaml:"endpoint"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	endpoint := strings.TrimSpace(doc.Endpoint)
	if endpoint == "" {
		return "", errors.New("empty endpoint")
	}
	return endpoint, nil
}

func scanSkillResources(ctx context.Context, root string) []SkillResource {
	appDirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var resources []SkillResource
	for _, appDir := range appDirs {
		if ctx.Err() != nil {
			return resources
		}
		if !appDir.IsDir() || skipResourceDir(appDir.Name()) {
			continue
		}
		appRoot := filepath.Join(root, appDir.Name())
		if fileExists(filepath.Join(appRoot, "SKILL.md")) {
			resources = append(resources, SkillResource{
				AppID:      appDir.Name(),
				ResourceID: appDir.Name(),
				FilePath:   filepath.Join(appRoot, "SKILL.md"),
				PublicPath: fmt.Sprintf("/skills/%s/SKILL.md", appDir.Name()),
			})
			continue
		}
		skillDirs, err := os.ReadDir(appRoot)
		if err != nil {
			continue
		}
		for _, skillDir := range skillDirs {
			if !skillDir.IsDir() || skipResourceDir(skillDir.Name()) {
				continue
			}
			filePath := filepath.Join(appRoot, skillDir.Name(), "SKILL.md")
			if !fileExists(filePath) {
				continue
			}
			resources = append(resources, SkillResource{
				AppID:      appDir.Name(),
				ResourceID: skillDir.Name(),
				FilePath:   filePath,
				PublicPath: fmt.Sprintf("/skills/%s/%s/SKILL.md", appDir.Name(), skillDir.Name()),
			})
		}
	}
	return resources
}

func skipResourceDir(name string) bool {
	return name == "" || name == ".digest" || strings.HasPrefix(name, ".")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func skillRoots(resourceRoot string) []string {
	return []string{
		filepath.Join(resourceRoot, "skills"),
		"/lzcapp/pkg/content/resources/skills",
		"resources/skills",
	}
}
