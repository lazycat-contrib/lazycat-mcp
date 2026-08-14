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
	AppID      string   `json:"app_id"`
	ResourceID string   `json:"resource_id"`
	Endpoint   string   `json:"endpoint"`
	FilePath   string   `json:"file_path,omitempty"`
	ToolNames  []string `json:"tool_names,omitempty"`
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
	index.sort()
	return index
}

func (s *ResourceScanner) ScanForReconcile(ctx context.Context) (ResourceIndex, error) {
	index := ResourceIndex{
		MCPByApp:    make(map[string][]MCPResource),
		SkillsByApp: make(map[string][]SkillResource),
	}
	mcpResources, err := scanMCPResourcesStrict(ctx, filepath.Join(s.root, "mcp-providers"))
	if err != nil {
		return index, fmt.Errorf("scan mcp provider resources: %w", err)
	}
	for _, item := range mcpResources {
		index.MCPByApp[item.AppID] = append(index.MCPByApp[item.AppID], item)
	}
	skillResources, err := scanSkillResourcesStrict(ctx, filepath.Join(s.root, "skills"))
	if err != nil {
		return ResourceIndex{
			MCPByApp:    make(map[string][]MCPResource),
			SkillsByApp: make(map[string][]SkillResource),
		}, fmt.Errorf("scan skill resources: %w", err)
	}
	for _, item := range skillResources {
		index.SkillsByApp[item.AppID] = append(index.SkillsByApp[item.AppID], item)
	}
	index.sort()
	return index, nil
}

func (idx ResourceIndex) sort() {
	for appID := range idx.MCPByApp {
		sort.Slice(idx.MCPByApp[appID], func(i, j int) bool {
			if idx.MCPByApp[appID][i].ResourceID == "default" {
				return true
			}
			if idx.MCPByApp[appID][j].ResourceID == "default" {
				return false
			}
			return idx.MCPByApp[appID][i].ResourceID < idx.MCPByApp[appID][j].ResourceID
		})
	}
	for appID := range idx.SkillsByApp {
		sort.Slice(idx.SkillsByApp[appID], func(i, j int) bool {
			return idx.SkillsByApp[appID][i].ResourceID < idx.SkillsByApp[appID][j].ResourceID
		})
	}
}

func (idx ResourceIndex) HasProviderResource(appID, resourceID string) bool {
	resourceID = strings.TrimSpace(resourceID)
	for _, resource := range idx.MCPByApp[appID] {
		if resourceID == "" || resource.ResourceID == resourceID {
			return true
		}
	}
	skills := idx.SkillsByApp[appID]
	for _, resource := range skills {
		if resourceID == "" || resource.ResourceID == resourceID {
			return true
		}
	}
	return resourceID != "" && len(idx.MCPByApp[appID]) == 0 && len(skills) == 1
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
			if err == nil {
				resources = append(resources, MCPResource{
					AppID:      appDir.Name(),
					ResourceID: providerDir.Name(),
					Endpoint:   endpoint,
					FilePath:   filePath,
				})
				continue
			}

			nestedRoot := filepath.Join(providerRoot, providerDir.Name())
			nestedDirs, err := os.ReadDir(nestedRoot)
			if err != nil {
				continue
			}
			for _, nestedDir := range nestedDirs {
				if !nestedDir.IsDir() || skipResourceDir(nestedDir.Name()) {
					continue
				}
				filePath := filepath.Join(nestedRoot, nestedDir.Name(), "mcp.yml")
				endpoint, err := readMCPEndpoint(filePath)
				if err != nil {
					continue
				}
				resources = append(resources, MCPResource{
					AppID:      providerDir.Name(),
					ResourceID: nestedDir.Name(),
					Endpoint:   endpoint,
					FilePath:   filePath,
				})
			}
		}
	}
	return resources
}

func scanMCPResourcesStrict(ctx context.Context, root string) ([]MCPResource, error) {
	appDirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var resources []MCPResource
	for _, appDir := range appDirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !appDir.IsDir() || skipResourceDir(appDir.Name()) {
			continue
		}
		providerRoot := filepath.Join(root, appDir.Name())
		providerDirs, err := os.ReadDir(providerRoot)
		if err != nil {
			return nil, fmt.Errorf("read app %s: %w", appDir.Name(), err)
		}
		for _, providerDir := range providerDirs {
			if !providerDir.IsDir() || skipResourceDir(providerDir.Name()) {
				continue
			}
			filePath := filepath.Join(providerRoot, providerDir.Name(), "mcp.yml")
			exists, err := regularFileExistsStrict(filePath)
			if err != nil {
				return nil, fmt.Errorf("inspect %s/%s: %w", appDir.Name(), providerDir.Name(), err)
			}
			if exists {
				endpoint, err := readMCPEndpoint(filePath)
				if err != nil {
					return nil, fmt.Errorf("read %s/%s: %w", appDir.Name(), providerDir.Name(), err)
				}
				resources = append(resources, MCPResource{
					AppID:      appDir.Name(),
					ResourceID: providerDir.Name(),
					Endpoint:   endpoint,
					FilePath:   filePath,
				})
				continue
			}

			nestedRoot := filepath.Join(providerRoot, providerDir.Name())
			nestedDirs, err := os.ReadDir(nestedRoot)
			if err != nil {
				return nil, fmt.Errorf("read app %s/%s: %w", appDir.Name(), providerDir.Name(), err)
			}
			found := false
			for _, nestedDir := range nestedDirs {
				if !nestedDir.IsDir() || skipResourceDir(nestedDir.Name()) {
					continue
				}
				found = true
				filePath := filepath.Join(nestedRoot, nestedDir.Name(), "mcp.yml")
				exists, err := regularFileExistsStrict(filePath)
				if err != nil {
					return nil, fmt.Errorf("inspect %s/%s/%s: %w", appDir.Name(), providerDir.Name(), nestedDir.Name(), err)
				}
				if !exists {
					return nil, fmt.Errorf("missing mcp.yml for %s/%s/%s", appDir.Name(), providerDir.Name(), nestedDir.Name())
				}
				endpoint, err := readMCPEndpoint(filePath)
				if err != nil {
					return nil, fmt.Errorf("read %s/%s/%s: %w", appDir.Name(), providerDir.Name(), nestedDir.Name(), err)
				}
				resources = append(resources, MCPResource{
					AppID:      providerDir.Name(),
					ResourceID: nestedDir.Name(),
					Endpoint:   endpoint,
					FilePath:   filePath,
				})
			}
			if !found {
				return nil, fmt.Errorf("missing mcp.yml for %s/%s", appDir.Name(), providerDir.Name())
			}
		}
	}
	return resources, nil
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
			if fileExists(filePath) {
				resources = append(resources, SkillResource{
					AppID:      appDir.Name(),
					ResourceID: skillDir.Name(),
					FilePath:   filePath,
					PublicPath: fmt.Sprintf("/skills/%s/%s/SKILL.md", appDir.Name(), skillDir.Name()),
				})
				continue
			}

			nestedRoot := filepath.Join(appRoot, skillDir.Name())
			nestedDirs, err := os.ReadDir(nestedRoot)
			if err != nil {
				continue
			}
			for _, nestedDir := range nestedDirs {
				if !nestedDir.IsDir() || skipResourceDir(nestedDir.Name()) {
					continue
				}
				filePath := filepath.Join(nestedRoot, nestedDir.Name(), "SKILL.md")
				if !fileExists(filePath) {
					continue
				}
				resources = append(resources, SkillResource{
					AppID:      skillDir.Name(),
					ResourceID: nestedDir.Name(),
					FilePath:   filePath,
					PublicPath: fmt.Sprintf("/skills/%s/%s/%s/SKILL.md", appDir.Name(), skillDir.Name(), nestedDir.Name()),
				})
			}
		}
	}
	return resources
}

func scanSkillResourcesStrict(ctx context.Context, root string) ([]SkillResource, error) {
	appDirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var resources []SkillResource
	for _, appDir := range appDirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !appDir.IsDir() || skipResourceDir(appDir.Name()) {
			continue
		}
		appRoot := filepath.Join(root, appDir.Name())
		directPath := filepath.Join(appRoot, "SKILL.md")
		exists, err := regularFileExistsStrict(directPath)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", directPath, err)
		}
		if exists {
			resources = append(resources, SkillResource{
				AppID:      appDir.Name(),
				ResourceID: appDir.Name(),
				FilePath:   directPath,
				PublicPath: fmt.Sprintf("/skills/%s/SKILL.md", appDir.Name()),
			})
			continue
		}
		skillDirs, err := os.ReadDir(appRoot)
		if err != nil {
			return nil, fmt.Errorf("read app %s: %w", appDir.Name(), err)
		}
		for _, skillDir := range skillDirs {
			if !skillDir.IsDir() || skipResourceDir(skillDir.Name()) {
				continue
			}
			filePath := filepath.Join(appRoot, skillDir.Name(), "SKILL.md")
			exists, err := regularFileExistsStrict(filePath)
			if err != nil {
				return nil, fmt.Errorf("inspect %s: %w", filePath, err)
			}
			if exists {
				resources = append(resources, SkillResource{
					AppID:      appDir.Name(),
					ResourceID: skillDir.Name(),
					FilePath:   filePath,
					PublicPath: fmt.Sprintf("/skills/%s/%s/SKILL.md", appDir.Name(), skillDir.Name()),
				})
				continue
			}

			nestedRoot := filepath.Join(appRoot, skillDir.Name())
			nestedDirs, err := os.ReadDir(nestedRoot)
			if err != nil {
				return nil, fmt.Errorf("read app %s/%s: %w", appDir.Name(), skillDir.Name(), err)
			}
			found := false
			for _, nestedDir := range nestedDirs {
				if !nestedDir.IsDir() || skipResourceDir(nestedDir.Name()) {
					continue
				}
				found = true
				filePath := filepath.Join(nestedRoot, nestedDir.Name(), "SKILL.md")
				exists, err := regularFileExistsStrict(filePath)
				if err != nil {
					return nil, fmt.Errorf("inspect %s: %w", filePath, err)
				}
				if !exists {
					return nil, fmt.Errorf("missing SKILL.md for %s/%s/%s", appDir.Name(), skillDir.Name(), nestedDir.Name())
				}
				resources = append(resources, SkillResource{
					AppID:      skillDir.Name(),
					ResourceID: nestedDir.Name(),
					FilePath:   filePath,
					PublicPath: fmt.Sprintf("/skills/%s/%s/%s/SKILL.md", appDir.Name(), skillDir.Name(), nestedDir.Name()),
				})
			}
			if !found {
				return nil, fmt.Errorf("missing SKILL.md for %s/%s", appDir.Name(), skillDir.Name())
			}
		}
	}
	return resources, nil
}

func regularFileExistsStrict(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("expected regular file")
	}
	return true, nil
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
