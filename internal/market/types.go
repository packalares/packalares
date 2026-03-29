package market

import "encoding/json"

// MarketApp represents an app in the marketplace catalog.
// This structure is compatible with the Olares marketplace format
// so existing marketplace apps work without modification.
type MarketApp struct {
	Name             string        `json:"name"`
	CfgType          string        `json:"cfgType,omitempty"`
	ChartName        string        `json:"chartName"`
	Icon             string        `json:"icon"`
	Description      string        `json:"description"`
	FullDescription  string        `json:"fullDescription,omitempty"`
	UpgradeDescription string     `json:"upgradeDescription,omitempty"`
	PromoteImage     []string      `json:"promoteImage,omitempty"`
	PromoteVideo     string        `json:"promoteVideo,omitempty"`
	SubCategory      string        `json:"subCategory,omitempty"`
	Developer        string        `json:"developer"`
	Owner            string        `json:"owner,omitempty"`
	UID              string        `json:"uid,omitempty"`
	Title            string        `json:"title"`
	Target           string        `json:"target,omitempty"`
	Entrances        []Entrance    `json:"entrances,omitempty"`
	Version          string        `json:"version"`
	VersionName      string        `json:"versionName,omitempty"`
	Categories       []string      `json:"categories"`
	Rating           float64       `json:"rating"`
	Namespace        string        `json:"namespace,omitempty"`
	OnlyAdmin        bool          `json:"onlyAdmin,omitempty"`
	RequiredMemory   string        `json:"requiredMemory,omitempty"`
	RequiredDisk     string        `json:"requiredDisk,omitempty"`
	RequiredGPU      string        `json:"requiredGpu,omitempty"`
	RequiredCPU      string        `json:"requiredCpu,omitempty"`
	LimitedMemory    string        `json:"limitedMemory,omitempty"`
	LimitedCPU       string        `json:"limitedCPU,omitempty"`
	SupportArch      []string      `json:"supportArch,omitempty"`
	Status           string        `json:"status,omitempty"`
	Type             string        `json:"type,omitempty"`
	Backend              string   `json:"backend,omitempty"`
	ModelID              string   `json:"modelId,omitempty"`
	HFRepo               string   `json:"hfRepo,omitempty"`
	HFRef                string   `json:"hfRef,omitempty"`
	GPUMemoryUtilization string   `json:"gpuMemoryUtilization,omitempty"`
	MaxModelLen          string   `json:"maxModelLen,omitempty"`
	TiktokenFiles        string   `json:"tiktokenFiles,omitempty"`
	Locale               []string `json:"locale,omitempty"`
	Permission       *AppPermission `json:"permission,omitempty"`
	Dependencies     []Dependency  `json:"dependencies,omitempty"`
	Source           string        `json:"source,omitempty"`
	MobileSupported  bool          `json:"mobileSupported,omitempty"`
	Options          *AppOptions   `json:"options,omitempty"`
	Doc              string        `json:"doc,omitempty"`
	Website          string        `json:"website,omitempty"`
	SourceCode       string        `json:"sourceCode,omitempty"`
	License          []License     `json:"license,omitempty"`
	InstallCount     int64         `json:"installCount,omitempty"`
	LastUpdated      string        `json:"lastUpdated,omitempty"`
	HasChart         bool          `json:"hasChart,omitempty"`
	FeaturedImage    string        `json:"featuredImage,omitempty"`
}

// Entrance for marketplace app.
type Entrance struct {
	Name       string `json:"name" yaml:"name"`
	Host       string `json:"host" yaml:"host"`
	Port       int32  `json:"port" yaml:"port"`
	Icon       string `json:"icon,omitempty" yaml:"icon,omitempty"`
	Title      string `json:"title,omitempty" yaml:"title,omitempty"`
	AuthLevel  string `json:"authLevel,omitempty" yaml:"authLevel,omitempty"`
	Invisible  bool   `json:"invisible,omitempty" yaml:"invisible,omitempty"`
	OpenMethod string `json:"openMethod,omitempty" yaml:"openMethod,omitempty"`
}

// AppPermission describes permissions an app requires.
type AppPermission struct {
	AppData  bool     `json:"appData"`
	AppCache bool     `json:"appCache"`
	UserData []string `json:"userData"`
}

// Dependency on another app or system component.
type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

// AppOptions holds optional app configuration.
type AppOptions struct {
	MobileSupported bool `json:"mobileSupported,omitempty"`
}

// License info.
type License struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Category represents an app category.
// When populated from appstore tags, includes icon and sort order.
type Category struct {
	Name  string            `json:"name"`
	Title map[string]string `json:"title,omitempty"` // locale -> display title
	Icon  string            `json:"icon,omitempty"`
	Sort  int               `json:"sort,omitempty"`
	Count int               `json:"count,omitempty"`
}

// RecommendGroup is a named recommendation group from the appstore API.
type RecommendGroup struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	AppIDs      []string          `json:"appIds"`
	Title       map[string]string `json:"title,omitempty"` // locale -> display title
}

// TopicListEntry is a curated topic list section from the appstore API.
type TopicListEntry struct {
	Name        string            `json:"name"`
	Type        string            `json:"type,omitempty"`
	Description string            `json:"description,omitempty"`
	TopicIDs    []string          `json:"topicIds"`
	Title       map[string]string `json:"title,omitempty"` // locale -> display title
}

// PageLayout holds page layout data from the appstore API.
type PageLayout struct {
	Category string          `json:"category"`
	Content  json.RawMessage `json:"content,omitempty"`
}

// EnrichedCatalog is the full catalog structure saved to catalog.json.
// It includes all appstore metadata beyond just the app list.
type EnrichedCatalog struct {
	Apps            []MarketApp               `json:"apps"`
	Categories      []Category                `json:"categories"`
	Recommendations map[string]RecommendGroup `json:"recommendations,omitempty"`
	TopicLists      map[string]TopicListEntry `json:"topicLists,omitempty"`
	Tops            []TopApp                  `json:"tops,omitempty"`
	Latest          []string                  `json:"latest,omitempty"`
	Pages           map[string]PageLayout     `json:"pages,omitempty"`
}

// --- API response types ---

// Response is the base response envelope.
type Response struct {
	Code int32 `json:"code"`
}

// CatalogResponse returns the full app catalog.
type CatalogResponse struct {
	Response
	Data []MarketApp `json:"data"`
}

// AppDetailResponse returns a single app's details.
type AppDetailResponse struct {
	Response
	Data *MarketApp `json:"data"`
}

// CategoriesResponse returns all categories.
type CategoriesResponse struct {
	Response
	Data []Category `json:"data"`
}

// SearchResponse returns search results.
type SearchResponse struct {
	Response
	Data []MarketApp `json:"data"`
}

// RecommendationsResponse returns recommendation groups.
type RecommendationsResponse struct {
	Response
	Data map[string][]MarketApp `json:"data"`
}

// Recommendation holds a named group of recommended app IDs.
type Recommendation struct {
	Name   string   `json:"name"`
	AppIDs []string `json:"appIds"`
}

// TopApp holds a ranked app reference from the appstore API.
type TopApp struct {
	AppID string `json:"appId"`
	Rank  int    `json:"rank"`
}
