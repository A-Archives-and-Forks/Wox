package system

import (
	"context"
	"fmt"
	"os"
	"strings"
	"wox/plugin"
	"wox/util"
	"wox/util/shell"

	"github.com/mitchellh/go-homedir"
)

var browserBookmarkIcon = plugin.PluginBookmarkIcon

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &BrowserBookmarkPlugin{})
}

type Bookmark struct {
	Name string
	Url  string
}

type BrowserBookmarkPlugin struct {
	api       plugin.API
	bookmarks []Bookmark
}

func (c *BrowserBookmarkPlugin) GetMetadata() plugin.Metadata {
	return plugin.Metadata{
		Id:            "95d041d3-be7e-4b20-8517-88dda2db280b",
		Name:          "BrowserBookmark",
		Author:        "Wox Launcher",
		Website:       "https://github.com/Wox-launcher/Wox",
		Version:       "1.0.0",
		MinWoxVersion: "2.0.0",
		Runtime:       "Go",
		Description:   "Search browser bookmarks",
		Icon:          browserBookmarkIcon.String(),
		Entry:         "",
		TriggerKeywords: []string{
			"*",
		},
		Commands: []plugin.MetadataCommand{},
		SupportedOS: []string{
			"Windows",
			"Macos",
			"Linux",
		},
	}
}

func (c *BrowserBookmarkPlugin) Init(ctx context.Context, initParams plugin.InitParams) {
	c.api = initParams.API

	profiles := []string{"Default", "Profile 1", "Profile 2", "Profile 3"}

	if util.IsMacOS() {
		// Load Chrome bookmarks
		for _, profile := range profiles {
			chromeBookmarks := c.loadChromeBookmarkInMacos(ctx, profile)
			c.bookmarks = append(c.bookmarks, chromeBookmarks...)
		}

		// Load Edge bookmarks
		for _, profile := range profiles {
			edgeBookmarks := c.loadEdgeBookmarkInMacos(ctx, profile)
			c.bookmarks = append(c.bookmarks, edgeBookmarks...)
		}
	} else if util.IsWindows() {
		// Load Chrome bookmarks
		for _, profile := range profiles {
			chromeBookmarks := c.loadChromeBookmarkInWindows(ctx, profile)
			c.bookmarks = append(c.bookmarks, chromeBookmarks...)
		}

		// Load Edge bookmarks
		for _, profile := range profiles {
			edgeBookmarks := c.loadEdgeBookmarkInWindows(ctx, profile)
			c.bookmarks = append(c.bookmarks, edgeBookmarks...)
		}
	} else if util.IsLinux() {
		// Load Chrome bookmarks
		for _, profile := range profiles {
			chromeBookmarks := c.loadChromeBookmarkInLinux(ctx, profile)
			c.bookmarks = append(c.bookmarks, chromeBookmarks...)
		}

		// Load Edge bookmarks
		for _, profile := range profiles {
			edgeBookmarks := c.loadEdgeBookmarkInLinux(ctx, profile)
			c.bookmarks = append(c.bookmarks, edgeBookmarks...)
		}
	}

	// Remove duplicate bookmarks (same name and url)
	c.bookmarks = c.removeDuplicateBookmarks(c.bookmarks)
}

func (c *BrowserBookmarkPlugin) Query(ctx context.Context, query plugin.Query) (results []plugin.QueryResult) {
	for _, b := range c.bookmarks {
		var bookmark = b
		var isMatch bool
		var matchScore int64

		var minMatchScore int64 = 10 // bookmark plugin has strict match score to avoid too many unrelated results
		isNameMatch, nameScore := IsStringMatchScore(ctx, bookmark.Name, query.Search)
		if isNameMatch && nameScore >= minMatchScore {
			isMatch = true
			matchScore = nameScore
		} else {
			//url match must be exact part match
			if strings.Contains(bookmark.Url, query.Search) {
				isUrlMatch, urlScore := IsStringMatchScoreNoPinYin(ctx, bookmark.Url, query.Search)
				if isUrlMatch && urlScore >= minMatchScore {
					isMatch = true
					matchScore = urlScore
				}
			}
		}

		if isMatch {
			results = append(results, plugin.QueryResult{
				Title:    bookmark.Name,
				SubTitle: bookmark.Url,
				Score:    matchScore,
				Icon:     browserBookmarkIcon,
				Actions: []plugin.QueryResultAction{
					{
						Name: "i18n:plugin_browser_bookmark_open_in_browser",
						Action: func(ctx context.Context, actionContext plugin.ActionContext) {
							shell.Open(bookmark.Url)
						},
					},
				},
			})
		}
	}

	return
}

func (c *BrowserBookmarkPlugin) loadChromeBookmarkInMacos(ctx context.Context, profile string) []Bookmark {
	return c.loadBookmarkFromFile(ctx, fmt.Sprintf("~/Library/Application Support/Google/Chrome/%s/Bookmarks", profile), "Chrome")
}

func (c *BrowserBookmarkPlugin) loadChromeBookmarkInWindows(ctx context.Context, profile string) []Bookmark {
	// Use a different approach to avoid fmt.Sprintf converting %% to %
	path := "%%LOCALAPPDATA%%\\Google\\Chrome\\User Data\\" + profile + "\\Bookmarks"
	return c.loadBookmarkFromFile(ctx, path, "Chrome")
}

func (c *BrowserBookmarkPlugin) loadChromeBookmarkInLinux(ctx context.Context, profile string) []Bookmark {
	return c.loadBookmarkFromFile(ctx, fmt.Sprintf("~/.config/google-chrome/%s/Bookmarks", profile), "Chrome")
}

func (c *BrowserBookmarkPlugin) loadEdgeBookmarkInMacos(ctx context.Context, profile string) []Bookmark {
	return c.loadBookmarkFromFile(ctx, fmt.Sprintf("~/Library/Application Support/Microsoft Edge/%s/Bookmarks", profile), "Edge")
}

func (c *BrowserBookmarkPlugin) loadEdgeBookmarkInWindows(ctx context.Context, profile string) []Bookmark {
	// Use a different approach to avoid fmt.Sprintf converting %% to %
	path := "%%LOCALAPPDATA%%\\Microsoft\\Edge\\User Data\\" + profile + "\\Bookmarks"
	return c.loadBookmarkFromFile(ctx, path, "Edge")
}

func (c *BrowserBookmarkPlugin) loadEdgeBookmarkInLinux(ctx context.Context, profile string) []Bookmark {
	return c.loadBookmarkFromFile(ctx, fmt.Sprintf("~/.config/microsoft-edge/%s/Bookmarks", profile), "Edge")
}

func (c *BrowserBookmarkPlugin) loadBookmarkFromFile(ctx context.Context, bookmarkPath string, browserName string) []Bookmark {
	var bookmarkLocation string
	var err error

	if strings.Contains(bookmarkPath, "%%LOCALAPPDATA%%") {
		// Windows path with environment variable
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			return []Bookmark{}
		}
		bookmarkLocation = strings.Replace(bookmarkPath, "%%LOCALAPPDATA%%", localAppData, 1)
	} else {
		// Unix-style path
		bookmarkLocation, err = homedir.Expand(bookmarkPath)
		if err != nil {
			c.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("error expanding %s bookmark path: %s", browserName, err.Error()))
			return []Bookmark{}
		}
	}

	if _, err := os.Stat(bookmarkLocation); os.IsNotExist(err) {
		return []Bookmark{}
	}

	file, readErr := os.ReadFile(bookmarkLocation)
	if readErr != nil {
		c.api.Log(ctx, plugin.LogLevelError, fmt.Sprintf("error reading %s bookmark file: %s", browserName, readErr.Error()))
		return []Bookmark{}
	}

	// Use a more robust regex pattern that works for both Chrome and Edge bookmark formats
	var results []Bookmark
	groups := util.FindRegexGroups(`(?ms)"name": "(?P<name>[^"]*)",.*?"type": "url",.*?"url": "(?P<url>[^"]*)"`, string(file))

	for _, group := range groups {
		if name, nameOk := group["name"]; nameOk {
			if url, urlOk := group["url"]; urlOk {
				results = append(results, Bookmark{
					Name: name,
					Url:  url,
				})
			}
		}
	}

	return results
}

// removeDuplicateBookmarks removes duplicate bookmarks based on name and url
func (c *BrowserBookmarkPlugin) removeDuplicateBookmarks(bookmarks []Bookmark) []Bookmark {
	seen := make(map[string]bool)
	var result []Bookmark

	for _, bookmark := range bookmarks {
		// Create a unique key based on name and url
		key := bookmark.Name + "|" + bookmark.Url

		if !seen[key] {
			seen[key] = true
			result = append(result, bookmark)
		}
	}

	return result
}
