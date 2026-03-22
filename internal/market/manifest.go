package market

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// olaresManifest is the YAML structure of an OlaresManifest.yaml file
// from the beclab/apps repository. We only parse the fields we need.
type olaresManifest struct {
	OlaresManifestVersion string `yaml:"olaresManifest.version"`
	OlaresManifestType    string `yaml:"olaresManifest.type"`
	Metadata              struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description"`
		Icon        string   `yaml:"icon"`
		AppID       string   `yaml:"appid"`
		Version     string   `yaml:"version"`
		Title       string   `yaml:"title"`
		Categories  []string `yaml:"categories"`
		Target      string   `yaml:"target"`
	} `yaml:"metadata"`
	Entrances []struct {
		Name       string `yaml:"name"`
		Host       string `yaml:"host"`
		Port       int32  `yaml:"port"`
		Title      string `yaml:"title"`
		Icon       string `yaml:"icon"`
		AuthLevel  string `yaml:"authLevel"`
		OpenMethod string `yaml:"openMethod"`
		Invisible  bool   `yaml:"invisible"`
	} `yaml:"entrances"`
	Permission *struct {
		AppData  bool     `yaml:"appData"`
		AppCache bool     `yaml:"appCache"`
		UserData []string `yaml:"userData"`
	} `yaml:"permission"`
	Spec struct {
		VersionName        string   `yaml:"versionName"`
		FullDescription    string   `yaml:"fullDescription"`
		UpgradeDescription string   `yaml:"upgradeDescription"`
		FeaturedImage      string   `yaml:"featuredImage"`
		PromoteImage       []string `yaml:"promoteImage"`
		PromoteVideo       string   `yaml:"promoteVideo"`
		Developer          string   `yaml:"developer"`
		Website            string   `yaml:"website"`
		SourceCode         string   `yaml:"sourceCode"`
		Doc                string   `yaml:"doc"`
		Submitter          string   `yaml:"submitter"`
		Locale             []string `yaml:"locale"`
		RequiredMemory     string   `yaml:"requiredMemory"`
		LimitedMemory      string   `yaml:"limitedMemory"`
		RequiredDisk       string   `yaml:"requiredDisk"`
		LimitedDisk        string   `yaml:"limitedDisk"`
		RequiredCPU        string   `yaml:"requiredCpu"`
		LimitedCPU         string   `yaml:"limitedCpu"`
		RequiredGPU        string   `yaml:"requiredGpu"`
		SupportArch        []string `yaml:"supportArch"`
		License            []struct {
			Text string `yaml:"text"`
			URL  string `yaml:"url"`
		} `yaml:"license"`
	} `yaml:"spec"`
	Options struct {
		Dependencies []struct {
			Name    string `yaml:"name"`
			Type    string `yaml:"type"`
			Version string `yaml:"version"`
		} `yaml:"dependencies"`
	} `yaml:"options"`
	Middleware interface{} `yaml:"middleware"`
}

// parseOlaresManifest parses an OlaresManifest.yaml into a MarketApp.
func parseOlaresManifest(data []byte, fallbackName string) (MarketApp, error) {
	var m olaresManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return MarketApp{}, fmt.Errorf("parse yaml: %w", err)
	}

	name := m.Metadata.Name
	if name == "" {
		name = fallbackName
	}
	if name == "" {
		return MarketApp{}, fmt.Errorf("manifest has no name")
	}

	// Skip non-app types (middleware, model, etc.) for the main catalog
	appType := m.OlaresManifestType
	if appType == "" {
		appType = "app"
	}

	app := MarketApp{
		Name:               name,
		ChartName:          name,
		Icon:               m.Metadata.Icon,
		Description:        m.Metadata.Description,
		FullDescription:    m.Spec.FullDescription,
		UpgradeDescription: m.Spec.UpgradeDescription,
		PromoteImage:       m.Spec.PromoteImage,
		PromoteVideo:       m.Spec.PromoteVideo,
		Developer:          m.Spec.Developer,
		Title:              m.Metadata.Title,
		Target:             m.Metadata.Target,
		Version:            m.Metadata.Version,
		VersionName:        m.Spec.VersionName,
		Categories:         cleanCategories(m.Metadata.Categories),
		Namespace:          "user-space",
		RequiredMemory:     m.Spec.RequiredMemory,
		RequiredDisk:       m.Spec.RequiredDisk,
		RequiredGPU:        m.Spec.RequiredGPU,
		RequiredCPU:        m.Spec.RequiredCPU,
		LimitedMemory:      m.Spec.LimitedMemory,
		LimitedCPU:         m.Spec.LimitedCPU,
		SupportArch:        m.Spec.SupportArch,
		Type:               appType,
		Locale:             m.Spec.Locale,
		Status:             "active",
		Source:             "olares",
		Doc:                m.Spec.Doc,
		Website:            m.Spec.Website,
		SourceCode:         m.Spec.SourceCode,
		CfgType:            m.OlaresManifestType,
	}

	if m.Metadata.Title == "" {
		app.Title = name
	}

	// Convert entrances
	for _, e := range m.Entrances {
		app.Entrances = append(app.Entrances, Entrance{
			Name:       e.Name,
			Host:       e.Host,
			Port:       e.Port,
			Icon:       e.Icon,
			Title:      e.Title,
			AuthLevel:  e.AuthLevel,
			Invisible:  e.Invisible,
			OpenMethod: e.OpenMethod,
		})
	}

	// Convert permission
	if m.Permission != nil {
		app.Permission = &AppPermission{
			AppData:  m.Permission.AppData,
			AppCache: m.Permission.AppCache,
			UserData: m.Permission.UserData,
		}
	}

	// Convert license
	for _, l := range m.Spec.License {
		app.License = append(app.License, License{
			Name: l.Text,
			URL:  l.URL,
		})
	}

	// Convert dependencies
	for _, d := range m.Options.Dependencies {
		app.Dependencies = append(app.Dependencies, Dependency{
			Name:    d.Name,
			Type:    d.Type,
			Version: d.Version,
		})
	}

	return app, nil
}

// categoryVersionSuffix matches version suffixes like "_v112", "_v2", "_v10".
var categoryVersionSuffix = regexp.MustCompile(`_v\d+$`)

// cleanCategories removes version suffixes like "_v112" and deduplicates.
func cleanCategories(cats []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, c := range cats {
		// Remove version suffixes like "_v112"
		clean := categoryVersionSuffix.ReplaceAllString(c, "")
		clean = strings.TrimSpace(clean)
		if clean != "" && !seen[clean] {
			seen[clean] = true
			result = append(result, clean)
		}
	}
	return result
}

// decodeBase64Content decodes base64 content from the GitHub API.
// GitHub returns base64-encoded file content with newlines.
func decodeBase64Content(content string) ([]byte, error) {
	// Remove whitespace/newlines that GitHub adds
	cleaned := strings.ReplaceAll(content, "\n", "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")

	data, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		// Try URL-safe encoding
		data, err = base64.URLEncoding.DecodeString(cleaned)
		if err != nil {
			return nil, fmt.Errorf("base64 decode: %w", err)
		}
	}
	return data, nil
}
