package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"github.com/samber/lo"
	"os"
	"path"
	"sync"
	"time"
	"wox/util"
)

type storeManifest struct {
	Name string
	Url  string
}

type storePluginManifest struct {
	Id             string
	Name           string
	Author         string
	Version        string
	MinWoxVersion  string
	Runtime        Runtime
	Description    string
	IconUrl        string
	Website        string
	DownloadUrl    string
	ScreenshotUrls []string
	DateCreated    string
	DateUpdated    string
}

var storeInstance *Store
var storeOnce sync.Once

type Store struct {
	pluginManifests []storePluginManifest
}

func GetStoreManager() *Store {
	storeOnce.Do(func() {
		storeInstance = &Store{}
	})
	return storeInstance
}

func (s *Store) getStoreManifests(ctx context.Context) []storeManifest {
	return []storeManifest{
		{
			Name: "Wox Official Plugin Store",
			Url:  "https://raw.githubusercontent.com/Wox-launcher/Wox/v2/plugin-store.json",
		},
	}
}

// get plugin manifests from plugin stores, and update in the background every 10 minutes
func (s *Store) Start(ctx context.Context) {
	s.pluginManifests = s.GetStorePluginManifests(ctx)

	util.Go(ctx, "load store plugins", func() {
		for range time.NewTicker(time.Minute * 10).C {
			pluginManifests := s.GetStorePluginManifests(util.NewTraceContext())
			if len(pluginManifests) > 0 {
				s.pluginManifests = pluginManifests
			}
		}
	})
}

func (s *Store) GetStorePluginManifests(ctx context.Context) []storePluginManifest {
	var storePluginManifests []storePluginManifest

	for _, store := range s.getStoreManifests(ctx) {
		pluginManifest, manifestErr := s.GetStorePluginManifest(ctx, store)
		if manifestErr != nil {
			logger.Error(ctx, fmt.Sprintf("failed to get plugin manifest from %s store: %s", store.Name, manifestErr.Error()))
			continue
		}

		for _, manifest := range pluginManifest {
			existingManifest, found := lo.Find(storePluginManifests, func(manifest storePluginManifest) bool {
				return manifest.Id == manifest.Id
			})
			if found {
				existingVersion, existingErr := semver.NewVersion(existingManifest.Version)
				currentVersion, currentErr := semver.NewVersion(manifest.Version)
				if existingErr != nil && currentErr != nil {
					if existingVersion.GreaterThan(currentVersion) {
						logger.Info(ctx, fmt.Sprintf("skip %s(%s) from %s store, because it's already installed(%s)", manifest.Name, manifest.Version, store.Name, existingManifest.Version))
						continue
					}
				}
			}

			storePluginManifests = append(storePluginManifests, manifest)
		}
	}

	logger.Info(ctx, fmt.Sprintf("found %d plugins from stores", len(storePluginManifests)))
	return storePluginManifests
}

func (s *Store) GetStorePluginManifest(ctx context.Context, store storeManifest) ([]storePluginManifest, error) {
	logger.Info(ctx, fmt.Sprintf("start to get plugin manifest from %s(%s)", store.Name, store.Url))

	response, getErr := util.HttpGet(ctx, store.Url)
	if getErr != nil {
		return nil, getErr
	}

	var storePluginManifests []storePluginManifest
	unmarshalErr := json.Unmarshal(response, &storePluginManifests)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}

	return storePluginManifests, nil
}

func (s *Store) Search(ctx context.Context, keyword string) []storePluginManifest {
	return lo.Filter(s.pluginManifests, func(manifest storePluginManifest, _ int) bool {
		return util.IsStringMatch(manifest.Name, keyword, false)
	})
}

func (s *Store) Install(ctx context.Context, manifest storePluginManifest) {
	logger.Info(ctx, fmt.Sprintf("start to install plugin %s(%s)", manifest.Name, manifest.Version))

	// check if installed newer version
	installedPlugin, exist := lo.Find(GetPluginManager().GetPluginInstances(), func(item *Instance) bool {
		return item.Metadata.Id == manifest.Id
	})
	if exist {
		logger.Info(ctx, fmt.Sprintf("found this plugin has installed %s(%s)", installedPlugin.Metadata.Name, installedPlugin.Metadata.Version))
		installedVersion, installedErr := semver.NewVersion(installedPlugin.Metadata.Version)
		currentVersion, currentErr := semver.NewVersion(manifest.Version)
		if installedErr == nil && currentErr == nil {
			if installedVersion.GreaterThan(currentVersion) {
				logger.Info(ctx, fmt.Sprintf("skip %s(%s) from %s store, because it's already installed(%s)", manifest.Name, manifest.Version, manifest.Name, installedPlugin.Metadata.Version))
				return
			}
		}

		uninstallErr := s.Uninstall(ctx, installedPlugin)
		if uninstallErr != nil {
			logger.Error(ctx, fmt.Sprintf("failed to uninstall plugin %s(%s): %s", installedPlugin.Metadata.Name, installedPlugin.Metadata.Version, uninstallErr.Error()))
			return
		}
	}

	// download plugin
	logger.Info(ctx, fmt.Sprintf("start to download plugin: %s", manifest.DownloadUrl))
	pluginDirectory := path.Join(util.GetLocation().GetPluginDirectory(), fmt.Sprintf("%s_%s@%s", manifest.Id, manifest.Name, manifest.Version))
	directoryErr := util.GetLocation().EnsureDirectoryExist(pluginDirectory)
	if directoryErr != nil {
		logger.Error(ctx, fmt.Sprintf("failed to create plugin directory %s: %s", pluginDirectory, directoryErr.Error()))
		return
	}
	pluginZipPath := path.Join(pluginDirectory, "plugin.zip")
	downloadErr := util.HttpDownload(ctx, manifest.DownloadUrl, pluginZipPath)
	if downloadErr != nil {
		logger.Error(ctx, fmt.Sprintf("failed to download plugin %s(%s): %s", manifest.Name, manifest.Version, downloadErr.Error()))
		return
	}

	//unzip plugin
	logger.Info(ctx, fmt.Sprintf("start to unzip plugin %s(%s)", manifest.Name, manifest.Version))
	util.Unzip(pluginZipPath, pluginDirectory)

	//load plugin
	logger.Info(ctx, fmt.Sprintf("start to load plugin %s(%s)", manifest.Name, manifest.Version))
	loadErr := GetPluginManager().LoadPlugin(ctx, pluginDirectory)
	if loadErr != nil {
		logger.Error(ctx, fmt.Sprintf("failed to load plugin %s(%s): %s", manifest.Name, manifest.Version, loadErr.Error()))
		return
	}

	//remove plugin zip
	removeErr := os.Remove(pluginZipPath)
	if removeErr != nil {
		logger.Error(ctx, fmt.Sprintf("failed to remove plugin zip %s: %s", pluginZipPath, removeErr.Error()))
		return
	}
}

func (s *Store) Uninstall(ctx context.Context, plugin *Instance) error {
	logger.Info(ctx, fmt.Sprintf("start to uninstall plugin %s(%s)", plugin.Metadata.Name, plugin.Metadata.Version))
	GetPluginManager().UnloadPlugin(ctx, plugin)
	removeErr := os.Remove(plugin.PluginDirectory)
	if removeErr != nil {
		logger.Error(ctx, fmt.Sprintf("failed to remove plugin directory %s: %s", plugin.PluginDirectory, removeErr.Error()))
		return removeErr
	}

	return nil
}
