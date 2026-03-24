package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"wox/common"
	"wox/plugin"
	"wox/setting/definition"
	"wox/setting/validator"
	"wox/util"
	"wox/util/browser"
)

const (
	webPreviewSitesSettingKey          = "sites"
	webPreviewDefaultAddedKey          = "defaultSiteAdded"
	webPreviewDefaultInstagramAddedKey = "defaultInstagramAdded"
	webPreviewCacheMigrationAddedKey   = "cacheMigrationAdded"
)

type webPreviewSite struct {
	Keyword      string
	Title        string
	Url          string
	InjectCss    string
	CacheEnabled bool
	CacheKey     string
	Icon         common.WoxImage
	Enabled      bool
}

type WebPreviewPlugin struct {
	api   plugin.API
	sites []webPreviewSite
}

type webViewPreviewData struct {
	Url          string `json:"url"`
	InjectCss    string `json:"injectCss,omitempty"`
	CacheEnabled bool   `json:"cacheEnabled,omitempty"`
	CacheKey     string `json:"cacheKey,omitempty"`
}

func defaultInstagramSite() webPreviewSite {
	return webPreviewSite{
		Keyword: "ig",
		Title:   "Instagram",
		Url:     "https://www.instagram.com",
		InjectCss: `
main section > div:first-child:has(a[href^="/stories/"]) {
	display: none !important;
}
`,
		CacheEnabled: true,
		CacheKey:     "instagram",
		Icon:         common.NewWoxImageUrl("https://static.cdninstagram.com/rsrc.php/v4/yI/r/VsNE-OHk_8a.png"),
		Enabled:      true,
	}
}

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &WebPreviewPlugin{})
}

func (p *WebPreviewPlugin) GetMetadata() plugin.Metadata {
	return plugin.Metadata{
		Id:            "2ac1b5cf-bf55-41f0-8c34-421c323be780",
		Name:          "Web Preview",
		Author:        "Wox Launcher",
		Website:       "https://github.com/Wox-launcher/Wox",
		Version:       "1.0.0",
		MinWoxVersion: "2.0.0",
		Runtime:       "Go",
		Description:   "Preview configurable websites inside Wox with a mobile-style embedded webview.",
		Icon:          common.PluginBrowserIcon.String(),
		TriggerKeywords: []string{
			"web",
		},
		Commands: []plugin.MetadataCommand{},
		SupportedOS: []string{
			"Macos",
		},
		Features: []plugin.MetadataFeature{
			{
				Name: plugin.MetadataFeatureResultPreviewWidthRatio,
				Params: map[string]any{
					"WidthRatio": 0.0,
				},
			},
		},
		SettingDefinitions: []definition.PluginSettingDefinitionItem{
			{
				Type:               definition.PluginSettingDefinitionTypeTable,
				IsPlatformSpecific: true,
				Value: &definition.PluginSettingValueTable{
					Key:           webPreviewSitesSettingKey,
					Title:         "Preview Sites",
					SortColumnKey: "Keyword",
					SortOrder:     definition.PluginSettingValueTableSortOrderAsc,
					MaxHeight:     500,
					Columns: []definition.PluginSettingValueTableColumn{
						{
							Key:   "Icon",
							Label: "Icon",
							Type:  definition.PluginSettingValueTableColumnTypeWoxImage,
							Width: 40,
						},
						{
							Key:     "Keyword",
							Label:   "Keyword",
							Type:    definition.PluginSettingValueTableColumnTypeText,
							Width:   80,
							Tooltip: "Type `web <keyword>`, for example `web tw `.",
							Validators: []validator.PluginSettingValidator{
								{
									Type:  validator.PluginSettingValidatorTypeNotEmpty,
									Value: &validator.PluginSettingValidatorNotEmpty{},
								},
							},
						},
						{
							Key:     "Title",
							Label:   "Title",
							Type:    definition.PluginSettingValueTableColumnTypeText,
							Tooltip: "Displayed result title. Supports {query}.",
							Validators: []validator.PluginSettingValidator{
								{
									Type:  validator.PluginSettingValidatorTypeNotEmpty,
									Value: &validator.PluginSettingValidatorNotEmpty{},
								},
							},
						},
						{
							Key:     "Url",
							Label:   "URL",
							Type:    definition.PluginSettingValueTableColumnTypeText,
							Tooltip: "Preview URL. Supports {query}, which will be URL-escaped.",
							Validators: []validator.PluginSettingValidator{
								{
									Type:  validator.PluginSettingValidatorTypeNotEmpty,
									Value: &validator.PluginSettingValidatorNotEmpty{},
								},
							},
						},
						{
							Key:          "InjectCss",
							Label:        "Inject CSS",
							Type:         definition.PluginSettingValueTableColumnTypeText,
							TextMaxLines: 12,
							HideInTable:  true,
							Tooltip:      "Optional CSS injected into the preview page.",
						},
						{
							Key:     "CacheEnabled",
							Label:   "Cache",
							Type:    definition.PluginSettingValueTableColumnTypeCheckbox,
							Width:   60,
							Tooltip: "Reuse the same embedded webview between openings.",
						},
						{
							Key:         "CacheKey",
							Label:       "Cache Key",
							Type:        definition.PluginSettingValueTableColumnTypeText,
							HideInTable: true,
							Tooltip:     "Stable key for keyed cache reuse, for example `instagram`.",
						},
						{
							Key:   "Enabled",
							Label: "Enabled",
							Type:  definition.PluginSettingValueTableColumnTypeCheckbox,
							Width: 70,
						},
					},
				},
			},
		},
	}
}

func (p *WebPreviewPlugin) Init(ctx context.Context, initParams plugin.InitParams) {
	p.api = initParams.API
	p.sites = p.loadSites(ctx)
	p.registerSiteCommands(ctx)
	p.api.Log(ctx, plugin.LogLevelInfo, fmt.Sprintf("loaded %d web preview sites", len(p.sites)))

	p.api.OnSettingChanged(ctx, func(callbackCtx context.Context, key string, value string) {
		if key != webPreviewSitesSettingKey {
			return
		}

		p.sites = p.loadSites(callbackCtx)
		p.registerSiteCommands(callbackCtx)
		p.indexIcons(callbackCtx)
	})

	util.Go(ctx, "parse web preview icons", func() {
		p.indexIcons(ctx)
	})
}

func (p *WebPreviewPlugin) Query(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	if query.Type != plugin.QueryTypeInput {
		return nil
	}
	if strings.TrimSpace(query.Command) == "" {
		return nil
	}

	searchText := query.Search
	escapedSearchText := url.QueryEscape(searchText)

	var results []plugin.QueryResult
	for _, site := range p.sites {
		if !site.Enabled {
			continue
		}
		if !strings.EqualFold(site.Keyword, query.Command) {
			continue
		}

		resolvedURL := p.replaceVariables(site.Url, escapedSearchText)
		if resolvedURL == "" {
			continue
		}

		resultTitle := p.replaceVariables(site.Title, searchText)
		if strings.TrimSpace(resultTitle) == "" {
			resultTitle = site.Title
		}

		currentSite := site
		currentURL := resolvedURL
		previewPayload, marshalErr := json.Marshal(webViewPreviewData{
			Url:          currentURL,
			InjectCss:    currentSite.InjectCss,
			CacheEnabled: currentSite.CacheEnabled,
			CacheKey:     currentSite.resolvedCacheKey(),
		})
		if marshalErr != nil {
			p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("failed to marshal webview preview payload for %s: %s", currentURL, marshalErr.Error()))
			continue
		}

		results = append(results, plugin.QueryResult{
			Title:    resultTitle,
			SubTitle: currentURL,
			Icon:     currentSite.Icon,
			Score:    100,
			Preview: plugin.WoxPreview{
				PreviewType: plugin.WoxPreviewTypeWebView,
				PreviewData: string(previewPayload),
			},
			Actions: []plugin.QueryResultAction{
				{
					Name:      "Open in Browser",
					Icon:      common.SearchIcon,
					IsDefault: true,
					Action: func(actionCtx context.Context, actionContext plugin.ActionContext) {
						if openErr := browser.OpenURL(currentURL, ""); openErr != nil {
							p.api.Log(actionCtx, plugin.LogLevelError, fmt.Sprintf("failed to open url %s: %s", currentURL, openErr.Error()))
						}
					},
				},
			},
		})
	}

	return results
}

func (p *WebPreviewPlugin) registerSiteCommands(ctx context.Context) {
	var commands []plugin.MetadataCommand
	for _, site := range p.sites {
		if !site.Enabled {
			continue
		}

		description := site.Title
		if strings.TrimSpace(description) == "" {
			description = site.Url
		}

		commands = append(commands, plugin.MetadataCommand{
			Command:     site.Keyword,
			Description: common.I18nString(description),
		})
	}

	p.api.RegisterQueryCommands(ctx, commands)
}

func (p *WebPreviewPlugin) loadSites(ctx context.Context) []webPreviewSite {
	sitesJSON := p.api.GetSetting(ctx, webPreviewSitesSettingKey)
	if sitesJSON == "" {
		defaultAdded := p.api.GetSetting(ctx, webPreviewDefaultAddedKey)
		if defaultAdded == "" {
			sites := []webPreviewSite{
				{
					Keyword: "tw",
					Title:   "X",
					Url:     "https://x.com",
					Icon:    common.NewWoxImageUrl("https://abs.twimg.com/favicons/twitter.2.ico"),
					Enabled: true,
				},
				{
					Keyword:      "ig",
					Title:        "Instagram",
					Url:          defaultInstagramSite().Url,
					InjectCss:    defaultInstagramSite().InjectCss,
					CacheEnabled: defaultInstagramSite().CacheEnabled,
					CacheKey:     defaultInstagramSite().CacheKey,
					Icon:         defaultInstagramSite().Icon,
					Enabled:      defaultInstagramSite().Enabled,
				},
			}
			if encoded, err := json.Marshal(sites); err == nil {
				p.api.SaveSetting(ctx, webPreviewSitesSettingKey, string(encoded), false)
				p.api.SaveSetting(ctx, webPreviewDefaultAddedKey, "true", false)
			}
			return sites
		}

		return nil
	}

	var sites []webPreviewSite
	if err := json.Unmarshal([]byte(sitesJSON), &sites); err != nil {
		p.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("failed to unmarshal web preview sites: %s", err.Error()))
		return nil
	}

	sitesChanged := false
	for i, site := range sites {
		if site.CacheEnabled && strings.TrimSpace(site.CacheKey) == "" {
			sites[i].CacheKey = site.Keyword
			sitesChanged = true
		}
	}

	if p.api.GetSetting(ctx, webPreviewCacheMigrationAddedKey) == "" {
		for i, site := range sites {
			if strings.EqualFold(site.Keyword, "ig") && !site.CacheEnabled && strings.TrimSpace(site.CacheKey) == "" {
				sites[i].CacheEnabled = true
				sites[i].CacheKey = defaultInstagramSite().CacheKey
				sitesChanged = true
			}
		}

		p.api.SaveSetting(ctx, webPreviewCacheMigrationAddedKey, "true", false)
	}

	if p.api.GetSetting(ctx, webPreviewDefaultInstagramAddedKey) == "" {
		hasInstagramSite := false
		for _, site := range sites {
			if strings.EqualFold(site.Keyword, "ig") {
				hasInstagramSite = true
				break
			}
		}

		if !hasInstagramSite {
			sites = append(sites, defaultInstagramSite())
			sitesChanged = true
		}

		p.api.SaveSetting(ctx, webPreviewDefaultInstagramAddedKey, "true", false)
	}

	if sitesChanged {
		if encoded, err := json.Marshal(sites); err == nil {
			p.api.SaveSetting(ctx, webPreviewSitesSettingKey, string(encoded), false)
		}
	}

	return sites
}

func (p *WebPreviewPlugin) indexIcons(ctx context.Context) {
	hasAnyIconIndexed := false
	for i, site := range p.sites {
		if !site.Icon.IsEmpty() {
			continue
		}

		p.sites[i].Icon = p.indexSiteIcon(ctx, site)
		hasAnyIconIndexed = true
	}

	if !hasAnyIconIndexed {
		return
	}

	encoded, err := json.Marshal(p.sites)
	if err == nil {
		p.api.SaveSetting(ctx, webPreviewSitesSettingKey, string(encoded), false)
	}
}

func (p *WebPreviewPlugin) indexSiteIcon(ctx context.Context, site webPreviewSite) common.WoxImage {
	icon, err := getWebsiteIconWithCache(ctx, site.Url)
	if err != nil {
		p.api.Log(ctx, plugin.LogLevelWarning, fmt.Sprintf("failed to load icon for %s: %s", site.Url, err.Error()))
		return common.PluginBrowserIcon
	}

	return icon
}

func (p *WebPreviewPlugin) replaceVariables(value string, query string) string {
	result := strings.ReplaceAll(value, "{query}", query)
	result = strings.ReplaceAll(result, "{lower_query}", strings.ToLower(query))
	result = strings.ReplaceAll(result, "{upper_query}", strings.ToUpper(query))
	return result
}

func (s webPreviewSite) resolvedCacheKey() string {
	if !s.CacheEnabled {
		return ""
	}

	cacheKey := strings.TrimSpace(s.CacheKey)
	if cacheKey != "" {
		return cacheKey
	}

	return strings.TrimSpace(s.Keyword)
}
