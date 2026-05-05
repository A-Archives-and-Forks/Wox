import 'package:wox/entity/wox_image.dart';
import 'package:wox/entity/wox_glance.dart';

class WoxSetting {
  late bool enableAutostart;
  late String mainHotkey;
  late String selectionHotkey;
  late List<IgnoredHotkeyApp> ignoredHotkeyApps;
  late String logLevel;
  late bool usePinYin;
  late bool switchInputMethodABC;
  late bool hideOnStart;
  late bool hideOnLostFocus;
  late bool showTray;
  late String langCode;
  late List<QueryHotkey> queryHotkeys;
  late List<QueryShortcut> queryShortcuts;
  late List<TrayQuery> trayQueries;
  late String launchMode;
  late String startPage;
  late String showPosition;
  late List<AIProvider> aiProviders;
  late int appWidth;
  late int maxResultCount;
  late String themeId;
  late String appFontFamily;
  late bool enableGlance;
  late GlanceRef primaryGlance;
  late bool httpProxyEnabled;
  late String httpProxyUrl;
  late bool enableAutoBackup;
  late bool enableAutoUpdate;
  late bool enableAnonymousUsageStats;
  late String customPythonPath;
  late String customNodejsPath;
  late bool showScoreTail;
  late bool showPerformanceTail;

  WoxSetting({
    required this.enableAutostart,
    required this.mainHotkey,
    required this.selectionHotkey,
    required this.ignoredHotkeyApps,
    required this.logLevel,
    required this.usePinYin,
    required this.switchInputMethodABC,
    required this.hideOnStart,
    required this.hideOnLostFocus,
    required this.showTray,
    required this.langCode,
    required this.queryHotkeys,
    required this.queryShortcuts,
    required this.trayQueries,
    required this.launchMode,
    required this.startPage,
    required this.showPosition,
    required this.aiProviders,
    required this.appWidth,
    required this.maxResultCount,
    required this.themeId,
    required this.appFontFamily,
    required this.enableGlance,
    required this.primaryGlance,
    required this.httpProxyEnabled,
    required this.httpProxyUrl,
    required this.enableAutoBackup,
    required this.enableAutoUpdate,
    required this.enableAnonymousUsageStats,
    required this.customPythonPath,
    required this.customNodejsPath,
    required this.showScoreTail,
    required this.showPerformanceTail,
  });

  WoxSetting.fromJson(Map<String, dynamic> json) {
    enableAutostart = json['EnableAutostart'] ?? false;
    mainHotkey = json['MainHotkey'];
    selectionHotkey = json['SelectionHotkey'];
    if (json['IgnoredHotkeyApps'] != null) {
      ignoredHotkeyApps = <IgnoredHotkeyApp>[];
      json['IgnoredHotkeyApps'].forEach((v) {
        ignoredHotkeyApps.add(IgnoredHotkeyApp.fromJson(v));
      });
    } else {
      ignoredHotkeyApps = <IgnoredHotkeyApp>[];
    }
    logLevel = json['LogLevel'] ?? 'INFO';
    usePinYin = json['UsePinYin'] ?? false;
    switchInputMethodABC = json['SwitchInputMethodABC'] ?? false;
    hideOnStart = json['HideOnStart'] ?? false;
    hideOnLostFocus = json['HideOnLostFocus'];
    showTray = json['ShowTray'] ?? false;
    langCode = json['LangCode'];
    showPosition = json['ShowPosition'] ?? 'mouse_screen';

    if (json['QueryHotkeys'] != null) {
      queryHotkeys = <QueryHotkey>[];
      json['QueryHotkeys'].forEach((v) {
        queryHotkeys.add(QueryHotkey.fromJson(v));
      });
    } else {
      queryHotkeys = <QueryHotkey>[];
    }

    if (json['QueryShortcuts'] != null) {
      queryShortcuts = <QueryShortcut>[];
      json['QueryShortcuts'].forEach((v) {
        queryShortcuts.add(QueryShortcut.fromJson(v));
      });
    } else {
      queryShortcuts = <QueryShortcut>[];
    }
    if (json['TrayQueries'] != null) {
      trayQueries = <TrayQuery>[];
      json['TrayQueries'].forEach((v) {
        trayQueries.add(TrayQuery.fromJson(v));
      });
    } else {
      trayQueries = <TrayQuery>[];
    }

    launchMode = json['LaunchMode'] ?? 'continue';
    startPage = json['StartPage'] ?? 'mru';

    if (json['AIProviders'] != null) {
      aiProviders = <AIProvider>[];
      json['AIProviders'].forEach((v) {
        aiProviders.add(AIProvider.fromJson(v));
      });
    } else {
      aiProviders = <AIProvider>[];
    }

    appWidth = json['AppWidth'];
    maxResultCount = json['MaxResultCount'];
    themeId = json['ThemeId'];
    appFontFamily = json['AppFontFamily'] ?? '';
    enableGlance = json['EnableGlance'] ?? true;
    primaryGlance = GlanceRef.fromJson(json['PrimaryGlance']);
    httpProxyEnabled = json['HttpProxyEnabled'] ?? false;
    httpProxyUrl = json['HttpProxyUrl'] ?? '';
    enableAutoBackup = json['EnableAutoBackup'] ?? false;
    enableAutoUpdate = json['EnableAutoUpdate'] ?? true;
    enableAnonymousUsageStats = json['EnableAnonymousUsageStats'] ?? true;
    customPythonPath = json['CustomPythonPath'] ?? '';
    customNodejsPath = json['CustomNodejsPath'] ?? '';
    showScoreTail = json['ShowScoreTail'] ?? false;
    showPerformanceTail = json['ShowPerformanceTail'] ?? false;
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['EnableAutostart'] = enableAutostart;
    data['MainHotkey'] = mainHotkey;
    data['SelectionHotkey'] = selectionHotkey;
    data['IgnoredHotkeyApps'] = ignoredHotkeyApps;
    data['LogLevel'] = logLevel;
    data['UsePinYin'] = usePinYin;
    data['SwitchInputMethodABC'] = switchInputMethodABC;
    data['HideOnStart'] = hideOnStart;
    data['HideOnLostFocus'] = hideOnLostFocus;
    data['ShowTray'] = showTray;
    data['LangCode'] = langCode;
    data['QueryHotkeys'] = queryHotkeys;
    data['QueryShortcuts'] = queryShortcuts;
    data['TrayQueries'] = trayQueries;
    data['LaunchMode'] = launchMode;
    data['StartPage'] = startPage;
    data['ShowPosition'] = showPosition;
    data['AIProviders'] = aiProviders;
    data['AppWidth'] = appWidth;
    data['MaxResultCount'] = maxResultCount;
    data['ThemeId'] = themeId;
    data['AppFontFamily'] = appFontFamily;
    data['EnableGlance'] = enableGlance;
    data['PrimaryGlance'] = primaryGlance.toJson();
    data['HttpProxyEnabled'] = httpProxyEnabled;
    data['HttpProxyUrl'] = httpProxyUrl;
    data['EnableAutoBackup'] = enableAutoBackup;
    data['EnableAutoUpdate'] = enableAutoUpdate;
    data['EnableAnonymousUsageStats'] = enableAnonymousUsageStats;
    data['CustomPythonPath'] = customPythonPath;
    data['CustomNodejsPath'] = customNodejsPath;
    data['ShowScoreTail'] = showScoreTail;
    data['ShowPerformanceTail'] = showPerformanceTail;
    return data;
  }
}

class QueryHotkey {
  late String hotkey;

  late String query; // Support plugin.QueryVariable

  late bool isSilentExecution;
  late bool hideQueryBox;
  late bool hideToolbar;
  late String width;
  late String maxResultCount;
  late String position;

  late bool disabled;

  QueryHotkey({
    required this.hotkey,
    required this.query,
    required this.isSilentExecution,
    required this.hideQueryBox,
    required this.hideToolbar,
    required this.width,
    required this.maxResultCount,
    required this.position,
    required this.disabled,
  });

  QueryHotkey.fromJson(Map<String, dynamic> json) {
    hotkey = json['Hotkey'];
    query = json['Query'];
    isSilentExecution = json['IsSilentExecution'] ?? false;
    hideQueryBox = json['HideQueryBox'] ?? false;
    hideToolbar = json['HideToolbar'] ?? false;
    width = json['Width'] == null ? "" : json['Width'].toString();
    maxResultCount = json['MaxResultCount'] == null ? "" : json['MaxResultCount'].toString();
    position = json['Position'] ?? 'system_default';
    disabled = json['Disabled'] ?? false;
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['Hotkey'] = hotkey;
    data['Query'] = query;
    data['IsSilentExecution'] = isSilentExecution;
    data['HideQueryBox'] = hideQueryBox;
    data['HideToolbar'] = hideToolbar;
    data['Width'] = width;
    data['MaxResultCount'] = maxResultCount;
    data['Position'] = position;
    data['Disabled'] = disabled;
    return data;
  }
}

class IgnoredHotkeyApp {
  late String name;
  late String identity;
  late String path;
  late WoxImage icon;

  IgnoredHotkeyApp({required this.name, required this.identity, required this.path, required this.icon});

  IgnoredHotkeyApp.fromJson(Map<String, dynamic> json) {
    name = json['Name'] ?? '';
    identity = json['Identity'] ?? '';
    path = json['Path'] ?? '';
    icon = json['Icon'] != null ? WoxImage.fromJson(json['Icon']) : WoxImage.empty();
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['Name'] = name;
    data['Identity'] = identity;
    data['Path'] = path;
    data['Icon'] = icon.toJson();
    return data;
  }

  static IgnoredHotkeyApp empty() {
    return IgnoredHotkeyApp(name: '', identity: '', path: '', icon: WoxImage.empty());
  }
}

class QueryShortcut {
  late String shortcut;

  late String query;

  late bool disabled;

  QueryShortcut({required this.shortcut, required this.query, required this.disabled});

  QueryShortcut.fromJson(Map<String, dynamic> json) {
    shortcut = json['Shortcut'];
    query = json['Query'];
    disabled = json['Disabled'] ?? false;
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['Shortcut'] = shortcut;
    data['Query'] = query;
    data['Disabled'] = disabled;
    return data;
  }
}

class TrayQuery {
  late WoxImage icon;

  late String query;
  late String width;
  late String maxResultCount;
  late bool hideQueryBox;
  late bool hideToolbar;

  late bool disabled;

  TrayQuery({
    required this.icon,
    required this.query,
    required this.width,
    required this.maxResultCount,
    required this.hideQueryBox,
    required this.hideToolbar,
    required this.disabled,
  });

  TrayQuery.fromJson(Map<String, dynamic> json) {
    if (json['Icon'] is Map<String, dynamic>) {
      icon = WoxImage.fromJson(json['Icon']);
    } else if (json['Icon'] is String) {
      icon = WoxImage.parse(json['Icon']) ?? WoxImage.empty();
    } else {
      icon = WoxImage.empty();
    }
    query = json['Query'];
    if (json['Width'] == null) {
      width = "";
    } else {
      width = json['Width'].toString();
    }
    if (json['MaxResultCount'] == null) {
      maxResultCount = "";
    } else {
      maxResultCount = json['MaxResultCount'].toString();
    }
    hideQueryBox = json['HideQueryBox'] ?? false;
    hideToolbar = json['HideToolbar'] ?? false;
    disabled = json['Disabled'] ?? false;
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['Icon'] = icon;
    data['Query'] = query;
    data['Width'] = width;
    data['MaxResultCount'] = maxResultCount;
    data['HideQueryBox'] = hideQueryBox;
    data['HideToolbar'] = hideToolbar;
    data['Disabled'] = disabled;
    return data;
  }
}

class SettingWindowContext {
  late String path;
  late String param;

  SettingWindowContext({required this.path, required this.param});

  SettingWindowContext.fromJson(Map<String, dynamic> json) {
    path = json['Path'];
    param = json['Param'];
  }
}

class AIProvider {
  late String name;
  late String alias;
  late String apiKey;

  late String host;

  AIProvider({required this.name, required this.alias, required this.apiKey, required this.host});

  AIProvider.fromJson(Map<String, dynamic> json) {
    name = json['Name'];
    alias = json['Alias'] ?? '';
    apiKey = json['ApiKey'];
    host = json['Host'];
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['Name'] = name;
    data['Alias'] = alias;
    data['ApiKey'] = apiKey;
    data['Host'] = host;
    return data;
  }
}

class AIProviderInfo {
  late String name;
  late WoxImage icon;
  late String defaultHost;

  AIProviderInfo({required this.name, required this.icon, required this.defaultHost});

  AIProviderInfo.fromJson(Map<String, dynamic> json) {
    name = json['Name'];
    icon = WoxImage.fromJson(json['Icon']);
    defaultHost = json['DefaultHost'] ?? "";
  }
}
