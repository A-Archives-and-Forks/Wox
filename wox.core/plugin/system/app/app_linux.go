package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"wox/common"
	"wox/plugin"
	"wox/util"
	"wox/util/locale"
	"wox/util/shell"
)

var appRetriever = &LinuxRetriever{}

type linuxDesktopEntry struct {
	Name            string
	GenericName     string
	Icon            string
	Type            string
	TryExec         string
	Hidden          bool
	NoDisplay       bool
	DesktopID       string
	SearchableNames []string
}

type LinuxRetriever struct {
	api plugin.API
}

func (a *LinuxRetriever) UpdateAPI(api plugin.API) {
	a.api = api
}

func (a *LinuxRetriever) GetPlatform() string {
	return util.PlatformLinux
}

func (a *LinuxRetriever) GetAppDirectories(ctx context.Context) []appDirectory {
	_ = ctx
	homeDir, _ := os.UserHomeDir()
	xdgDataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if xdgDataHome == "" && homeDir != "" {
		xdgDataHome = filepath.Join(homeDir, ".local", "share")
	}

	xdgDataDirs := strings.Split(strings.TrimSpace(os.Getenv("XDG_DATA_DIRS")), ":")
	if len(xdgDataDirs) == 1 && xdgDataDirs[0] == "" {
		xdgDataDirs = []string{"/usr/local/share", "/usr/share"}
	}

	roots := []string{
		filepath.Join(xdgDataHome, "applications"),
		filepath.Join(homeDir, ".local", "share", "flatpak", "exports", "share", "applications"),
		"/var/lib/flatpak/exports/share/applications",
		"/var/lib/snapd/desktop/applications",
		"/snap/applications",
	}
	for _, dir := range xdgDataDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		roots = append(roots, filepath.Join(dir, "applications"))
	}

	directories := make([]appDirectory, 0, len(roots))
	seen := map[string]struct{}{}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		cleanRoot := filepath.Clean(root)
		if _, ok := seen[cleanRoot]; ok {
			continue
		}
		seen[cleanRoot] = struct{}{}

		// Bug fix: the previous Linux retriever returned an empty root, so the app plugin
		// never scanned any launcher directories and always indexed zero apps. Linux apps
		// are advertised through .desktop launchers, so index the standard XDG locations.
		directories = append(directories, appDirectory{
			Path:           cleanRoot,
			Recursive:      true,
			RecursiveDepth: 4,
			trackChanges:   true,
		})
	}

	return directories
}

func (a *LinuxRetriever) GetAppExtensions(ctx context.Context) []string {
	_ = ctx
	return []string{"desktop"}
}

func (a *LinuxRetriever) ParseAppInfo(ctx context.Context, path string) (appInfo, error) {
	entry, err := parseLinuxDesktopEntry(path)
	if err != nil {
		return appInfo{}, err
	}
	if entry.Hidden || entry.NoDisplay {
		return appInfo{}, fmt.Errorf("%w: launcher is hidden", errSkipAppIndexing)
	}
	if entry.Type != "" && !strings.EqualFold(entry.Type, "Application") {
		return appInfo{}, fmt.Errorf("%w: unsupported desktop entry type %s", errSkipAppIndexing, entry.Type)
	}
	if strings.TrimSpace(entry.TryExec) != "" && !linuxTryExecExists(entry.TryExec) {
		return appInfo{}, fmt.Errorf("%w: TryExec target missing", errSkipAppIndexing)
	}

	icon := appIcon
	if iconPath := resolveLinuxDesktopIcon(entry.Icon); iconPath != "" {
		icon = common.NewWoxImageAbsolutePath(iconPath)
	}

	// Bug fix: the previous Linux parser always returned "not implemented", so even
	// valid .desktop files such as Chrome never became searchable. Parsing the desktop
	// entry keeps Linux aligned with the launcher metadata desktop environments already use.
	return appInfo{
		Name:            entry.Name,
		SearchableNames: entry.SearchableNames,
		Path:            filepath.Clean(path),
		Icon:            icon,
		Type:            AppTypeDesktop,
		IsDefaultIcon:   icon.ImageData == appIcon.ImageData,
	}, nil
}

func resolveAppIdentityForPlatform(ctx context.Context, info appInfo) string {
	_ = ctx
	lowerPath := strings.ToLower(strings.TrimSpace(info.Path))
	if !strings.HasSuffix(lowerPath, ".desktop") {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(lowerPath), filepath.Ext(lowerPath))
}

func (a *LinuxRetriever) GetExtraApps(ctx context.Context) ([]appInfo, error) {
	return []appInfo{}, nil
}

func (a *LinuxRetriever) GetPid(ctx context.Context, app appInfo) int {
	return 0
}

func (a *LinuxRetriever) GetProcessStat(ctx context.Context, app appInfo) (*ProcessStat, error) {
	return nil, errors.New("not implemented")
}

func (a *LinuxRetriever) OpenAppFolder(ctx context.Context, app appInfo) error {
	return shell.OpenFileInFolder(app.Path)
}

func parseLinuxDesktopEntry(desktopPath string) (linuxDesktopEntry, error) {
	file, err := os.Open(desktopPath)
	if err != nil {
		return linuxDesktopEntry{}, err
	}
	defer file.Close()

	values := map[string]string{}
	inDesktopEntry := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inDesktopEntry = strings.EqualFold(line, "[Desktop Entry]")
			continue
		}
		if !inDesktopEntry {
			continue
		}

		separator := strings.Index(line, "=")
		if separator <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		value := strings.TrimSpace(line[separator+1:])
		values[key] = unescapeLinuxDesktopValue(value)
	}
	if err := scanner.Err(); err != nil {
		return linuxDesktopEntry{}, err
	}

	displayName := resolveLinuxDesktopLocalizedValue("Name", values)
	if displayName == "" {
		displayName = strings.TrimSuffix(filepath.Base(desktopPath), filepath.Ext(desktopPath))
	}

	searchableNames := []string{
		strings.TrimSpace(resolveLinuxDesktopLocalizedValue("GenericName", values)),
		strings.TrimSpace(values["StartupWMClass"]),
		strings.TrimSpace(strings.TrimSuffix(filepath.Base(desktopPath), filepath.Ext(desktopPath))),
	}
	searchableNames = append(searchableNames, collectLinuxDesktopLocalizedValues("Name", values)...)
	searchableNames = append(searchableNames, collectLinuxDesktopLocalizedValues("GenericName", values)...)

	filteredSearchableNames := make([]string, 0, len(searchableNames))
	for _, name := range util.UniqueStrings(searchableNames) {
		name = strings.TrimSpace(name)
		if name == "" || strings.EqualFold(name, displayName) {
			continue
		}
		filteredSearchableNames = append(filteredSearchableNames, name)
	}

	return linuxDesktopEntry{
		Name:            displayName,
		GenericName:     resolveLinuxDesktopLocalizedValue("GenericName", values),
		Icon:            strings.TrimSpace(values["Icon"]),
		Type:            strings.TrimSpace(values["Type"]),
		TryExec:         strings.TrimSpace(values["TryExec"]),
		Hidden:          parseLinuxDesktopBool(values["Hidden"]),
		NoDisplay:       parseLinuxDesktopBool(values["NoDisplay"]),
		DesktopID:       strings.TrimSuffix(filepath.Base(desktopPath), filepath.Ext(desktopPath)),
		SearchableNames: filteredSearchableNames,
	}, nil
}

func resolveLinuxDesktopLocalizedValue(baseKey string, values map[string]string) string {
	for _, candidate := range linuxDesktopLocaleKeys(baseKey) {
		if value := strings.TrimSpace(values[candidate]); value != "" {
			return value
		}
	}
	return strings.TrimSpace(values[baseKey])
}

func collectLinuxDesktopLocalizedValues(baseKey string, values map[string]string) []string {
	prefix := baseKey + "["
	collected := []string{}
	for key, value := range values {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		collected = append(collected, value)
	}
	return util.UniqueStrings(collected)
}

func linuxDesktopLocaleKeys(baseKey string) []string {
	lang, region := locale.GetLocale()
	lang = strings.TrimSpace(lang)
	region = strings.TrimSpace(region)
	if lang == "" {
		return nil
	}

	region = strings.ToUpper(region)
	keys := []string{}
	if region != "" {
		keys = append(keys,
			fmt.Sprintf("%s[%s_%s]", baseKey, lang, region),
			fmt.Sprintf("%s[%s-%s]", baseKey, lang, region),
		)
	}
	keys = append(keys, fmt.Sprintf("%s[%s]", baseKey, lang))
	return util.UniqueStrings(keys)
}

func parseLinuxDesktopBool(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func unescapeLinuxDesktopValue(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\s`, " ",
		`\n`, "\n",
		`\t`, "\t",
		`\;`, ";",
	)
	return replacer.Replace(value)
}

func linuxTryExecExists(tryExecValue string) bool {
	tryExecValue = strings.TrimSpace(tryExecValue)
	if tryExecValue == "" {
		return true
	}

	fields := strings.Fields(tryExecValue)
	if len(fields) == 0 {
		return true
	}
	command := fields[0]
	if filepath.IsAbs(command) {
		_, err := os.Stat(command)
		return err == nil
	}

	_, err := exec.LookPath(command)
	return err == nil
}

func resolveLinuxDesktopIcon(iconValue string) string {
	iconValue = strings.TrimSpace(iconValue)
	if iconValue == "" {
		return ""
	}

	if filepath.IsAbs(iconValue) {
		return resolveLinuxAbsoluteIconPath(iconValue)
	}

	searchRoots := linuxDesktopIconSearchRoots()
	iconNames := []string{iconValue}
	if ext := filepath.Ext(iconValue); ext != "" {
		iconNames = append(iconNames, strings.TrimSuffix(iconValue, ext))
	}
	iconNames = util.UniqueStrings(iconNames)

	// Bug fix: Linux launchers expose themed icon names rather than file paths.
	// Resolving common hicolor/Adwaita/pixmaps locations is enough to surface real
	// app icons for standard installs without introducing a heavier GTK dependency.
	for _, root := range searchRoots {
		for _, iconName := range iconNames {
			if resolved := resolveLinuxNamedIconFromRoot(root, iconName); resolved != "" {
				return resolved
			}
		}
	}

	return ""
}

func linuxDesktopIconSearchRoots() []string {
	homeDir, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(homeDir, ".local", "share", "icons", "hicolor"),
		filepath.Join(homeDir, ".local", "share", "icons", "Adwaita"),
		filepath.Join(homeDir, ".icons", "hicolor"),
		filepath.Join(homeDir, ".icons", "Adwaita"),
		"/usr/local/share/icons/hicolor",
		"/usr/local/share/icons/Adwaita",
		"/usr/share/icons/hicolor",
		"/usr/share/icons/Adwaita",
		"/usr/local/share/pixmaps",
		"/usr/share/pixmaps",
	}
	return util.UniqueStrings(paths)
}

func resolveLinuxNamedIconFromRoot(root string, iconName string) string {
	root = strings.TrimSpace(root)
	iconName = strings.TrimSpace(iconName)
	if root == "" || iconName == "" {
		return ""
	}

	if resolved := resolveLinuxAbsoluteIconPath(filepath.Join(root, iconName)); resolved != "" {
		return resolved
	}

	extensions := []string{".png", ".svg", ".xpm"}
	sizeDirs := []string{"512x512", "256x256", "128x128", "96x96", "64x64", "48x48", "32x32", "24x24", "22x22", "16x16", "scalable"}
	categories := []string{"apps", "mimetypes"}

	for _, sizeDir := range sizeDirs {
		for _, category := range categories {
			for _, extension := range extensions {
				candidate := filepath.Join(root, sizeDir, category, iconName+extension)
				if fileExists(candidate) {
					return candidate
				}
			}
		}
	}

	for _, extension := range extensions {
		candidate := filepath.Join(root, iconName+extension)
		if fileExists(candidate) {
			return candidate
		}
	}

	return ""
}

func resolveLinuxAbsoluteIconPath(iconPath string) string {
	iconPath = strings.TrimSpace(iconPath)
	if iconPath == "" {
		return ""
	}
	if fileExists(iconPath) {
		return iconPath
	}

	if filepath.Ext(iconPath) == "" {
		for _, extension := range []string{".png", ".svg", ".xpm"} {
			candidate := iconPath + extension
			if fileExists(candidate) {
				return candidate
			}
		}
	}

	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
