import 'dart:async';

import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:uuid/v4.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/entity/wox_backup.dart';
import 'package:wox/entity/wox_plugin.dart';
import 'package:wox/entity/wox_runtime_status.dart';
import 'package:wox/entity/wox_theme.dart';
import 'package:wox/entity/wox_usage_stats.dart';
import 'package:wox/enums/wox_position_type_enum.dart';
import 'package:wox/enums/wox_plugin_runtime_enum.dart';
import 'package:wox/utils/log.dart';
import 'package:wox/utils/wox_fuzzy_match_util.dart';
import 'package:wox/utils/wox_setting_util.dart';

class WoxSettingController extends GetxController {
  final activeNavPath = 'general'.obs;
  final woxSetting = WoxSettingUtil.instance.currentSetting.obs;
  final userDataLocation = "".obs;
  final backups = <WoxBackup>[].obs;
  final woxVersion = "".obs;
  final runtimeStatuses = <WoxRuntimeStatus>[].obs;
  final isRuntimeStatusLoading = false.obs;
  final runtimeStatusError = ''.obs;
  final isClearingLogs = false.obs;
  final isUpdatingLogLevel = false.obs;

  final usageStats = WoxUsageStats.empty().obs;
  final isUsageStatsLoading = false.obs;
  final usageStatsError = ''.obs;
  final systemFontFamilies = <String>[].obs;

  //plugins
  final pluginList = <PluginDetail>[];
  final storePlugins = <PluginDetail>[];
  final installedPlugins = <PluginDetail>[];
  final filterPluginKeywordController = TextEditingController();
  final filteredPluginList = <PluginDetail>[].obs;
  final filterEnabledPluginsOnly = false.obs;
  final filterDisabledPluginsOnly = false.obs;
  final filterUpgradablePluginsOnly = false.obs;
  final filterUninstalledPluginsOnly = false.obs;
  final filterRuntimeNodejsOnly = false.obs;
  final filterRuntimePythonOnly = false.obs;
  final filterRuntimeScriptOnly = false.obs;
  final filterRuntimeScriptNodejsOnly = false.obs;
  final filterRuntimeScriptPythonOnly = false.obs;
  final activePlugin = PluginDetail.empty().obs;
  final isStorePluginList = true.obs;
  String pendingInstalledPluginFocusRef = '';
  final pluginListScrollController = ScrollController();
  final Map<String, GlobalKey> pluginListItemKeys = <String, GlobalKey>{};
  late TabController activePluginTabController;

  // UI state: show loading spinner when refreshing visible plugin list
  final isRefreshingPluginList = false.obs;

  //themes
  final themeList = <WoxTheme>[];
  final installedThemesList = <WoxTheme>[]; // All installed themes for auto theme lookup
  final filteredThemeList = <WoxTheme>[].obs;
  final activeTheme = WoxTheme.empty().obs;
  final isStoreThemeList = true.obs;

  //lang
  var langMap = <String, String>{}.obs;

  final isInstallingPlugin = false.obs;
  final isUpgradingPlugin = false.obs;
  final pluginInstallError = ''.obs;
  final FocusNode settingFocusNode = FocusNode();
  final Map<String, GlobalKey> generalSectionKeys = <String, GlobalKey>{};
  final RxnInt pendingTrayQueryEditRowIndex = RxnInt();
  bool _hasPreloadedSettingViewData = false;
  String _pendingGeneralSectionAnchor = '';

  @override
  void onInit() {
    super.onInit();
  }

  void preloadSettingViewData(String traceId, {bool forceRefresh = false}) {
    if (_hasPreloadedSettingViewData && !forceRefresh) {
      return;
    }

    _hasPreloadedSettingViewData = true;
    unawaited(loadSystemFontFamilies());
    unawaited(loadUserDataLocation());
    unawaited(refreshBackups());
    unawaited(loadWoxVersion());
    unawaited(refreshRuntimeStatuses());
    unawaited(refreshUsageStats());
    unawaited(reloadPlugins(traceId));
  }

  GlobalKey getGeneralSectionKey(String sectionId) {
    return generalSectionKeys.putIfAbsent(sectionId, () => GlobalKey(debugLabel: 'settings-general-$sectionId'));
  }

  void focusGeneralSection(String sectionId) {
    final request = _parseGeneralSectionFocusRequest(sectionId);
    if (request.sectionId.isEmpty) {
      return;
    }

    _pendingGeneralSectionAnchor = request.sectionId;
    pendingTrayQueryEditRowIndex.value = request.trayQueryEditRowIndex;
    _schedulePendingGeneralSectionFocus();
  }

  int? consumePendingTrayQueryEditRowIndex() {
    final rowIndex = pendingTrayQueryEditRowIndex.value;
    pendingTrayQueryEditRowIndex.value = null;
    return rowIndex;
  }

  void notifyGeneralViewReady() {
    if (_pendingGeneralSectionAnchor.isEmpty) {
      return;
    }

    _schedulePendingGeneralSectionFocus();
  }

  void _schedulePendingGeneralSectionFocus({int attempt = 0}) {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_pendingGeneralSectionAnchor.isEmpty || activeNavPath.value != 'general') {
        return;
      }

      final targetKey = generalSectionKeys[_pendingGeneralSectionAnchor];
      final targetContext = targetKey?.currentContext;
      if (targetContext == null) {
        if (attempt >= 10) {
          return;
        }

        Future.delayed(const Duration(milliseconds: 80), () {
          _schedulePendingGeneralSectionFocus(attempt: attempt + 1);
        });
        return;
      }

      Scrollable.ensureVisible(targetContext, duration: const Duration(milliseconds: 220), curve: Curves.easeOutCubic, alignment: 0.1);
      _pendingGeneralSectionAnchor = '';
    });
  }

  _GeneralSectionFocusRequest _parseGeneralSectionFocusRequest(String rawRequest) {
    final normalizedRequest = rawRequest.trim();
    if (normalizedRequest.isEmpty) {
      return const _GeneralSectionFocusRequest(sectionId: '');
    }

    final separatorIndex = normalizedRequest.indexOf(':');
    if (separatorIndex < 0) {
      return _GeneralSectionFocusRequest(sectionId: normalizedRequest);
    }

    final sectionId = normalizedRequest.substring(0, separatorIndex).trim();
    final rawRowIndex = normalizedRequest.substring(separatorIndex + 1).trim();
    final rowIndex = int.tryParse(rawRowIndex);
    return _GeneralSectionFocusRequest(sectionId: sectionId, trayQueryEditRowIndex: rowIndex);
  }

  Future<void> loadLang(String langCode) async {
    final traceId = const UuidV4().generate();
    langMap.value = await WoxApi.instance.getLangJson(traceId, langCode);
  }

  Future<void> loadWoxVersion() async {
    final traceId = const UuidV4().generate();
    try {
      final version = await WoxApi.instance.getWoxVersion(traceId);
      woxVersion.value = version;
    } catch (e) {
      woxVersion.value = '';
      Logger.instance.error(traceId, 'Failed to load Wox version: $e');
    }
  }

  Future<void> refreshRuntimeStatuses() async {
    isRuntimeStatusLoading.value = true;
    runtimeStatusError.value = '';
    final traceId = const UuidV4().generate();
    try {
      final statuses = await WoxApi.instance.getRuntimeStatuses(traceId);
      runtimeStatuses.assignAll(statuses);
      Logger.instance.info(traceId, 'Runtime statuses loaded, count: ${statuses.length}');
    } catch (e) {
      runtimeStatuses.clear();
      runtimeStatusError.value = e.toString();
      Logger.instance.error(traceId, 'Failed to load runtime statuses: $e');
    } finally {
      isRuntimeStatusLoading.value = false;
    }
  }

  Future<void> refreshUsageStats() async {
    isUsageStatsLoading.value = true;
    usageStatsError.value = '';
    final traceId = const UuidV4().generate();
    try {
      final stats = await WoxApi.instance.getUsageStats(traceId);
      usageStats.value = stats;
      Logger.instance.info(traceId, 'Usage stats loaded');
    } catch (e) {
      usageStats.value = WoxUsageStats.empty();
      usageStatsError.value = e.toString();
      Logger.instance.error(traceId, 'Failed to load usage stats: $e');
    } finally {
      isUsageStatsLoading.value = false;
    }
  }

  Future<void> loadSystemFontFamilies() async {
    final traceId = const UuidV4().generate();
    try {
      final families = await WoxApi.instance.getSystemFontFamilies(traceId);
      final normalized = families.map((family) => family.trim()).where((family) => family.isNotEmpty).toSet().toList()..sort((a, b) => a.toLowerCase().compareTo(b.toLowerCase()));
      systemFontFamilies.assignAll(normalized);
      Logger.instance.info(traceId, 'System font families loaded, count: ${normalized.length}');
    } catch (e) {
      systemFontFamilies.clear();
      Logger.instance.error(traceId, 'Failed to load system font families: $e');
    }
  }

  void hideWindow(String traceId) {
    Get.find<WoxLauncherController>().exitSetting(traceId);
  }

  Future<void> updateConfig(String key, String value) async {
    final traceId = const UuidV4().generate();
    await WoxApi.instance.updateSetting(traceId, key, value);
    await reloadSetting(traceId);
    Logger.instance.info(traceId, 'Setting updated: $key=$value');

    // If user switches to last_location, save current window position immediately
    if (key == "ShowPosition" && value == WoxPositionTypeEnum.POSITION_TYPE_LAST_LOCATION.code) {
      try {
        final launcherController = Get.find<WoxLauncherController>();
        launcherController.saveWindowPositionIfNeeded();
        Logger.instance.info(traceId, 'Saved current window position when switching to last_location');
      } catch (e) {
        Logger.instance.error(traceId, 'Failed to save window position when switching to last_location: $e');
      }
    }

    // Sync lastLaunchMode immediately so hideApp uses the correct mode
    // without waiting for the next show cycle from the backend.
    if (key == "LaunchMode") {
      try {
        final launcherController = Get.find<WoxLauncherController>();
        launcherController.lastLaunchMode = value;
        Logger.instance.info(traceId, 'Synced lastLaunchMode to $value');
      } catch (e) {
        Logger.instance.error(traceId, 'Failed to sync lastLaunchMode: $e');
      }
    }
  }

  Future<void> updateLang(String langCode) async {
    await updateConfig("LangCode", langCode);
    final traceId = const UuidV4().generate();
    langMap.value = await WoxApi.instance.getLangJson(traceId, langCode);

    // Refresh all loaded plugins to update translations
    // Reload installed plugins list
    if (installedPlugins.isNotEmpty) {
      await loadInstalledPlugins(traceId);
    }

    // Reload store plugins list if loaded
    if (storePlugins.isNotEmpty) {
      await loadStorePlugins(traceId);
    }

    // Refresh current view
    if (activeNavPath.value == 'plugins.installed' || activeNavPath.value == 'plugins.store') {
      await switchToPluginList(traceId, isStorePluginList.value);
    }

    // Refresh active plugin detail if one is selected
    if (activePlugin.value.id.isNotEmpty) {
      await refreshPlugin(activePlugin.value.id, "update");
    }
  }

  // get translation
  String tr(String key) {
    if (key.startsWith("i18n:")) {
      key = key.substring(5);
    }

    return langMap[key] ?? key;
  }

  // ---------- Plugins ----------

  Future<void> loadStorePlugins(String traceId) async {
    try {
      var start = DateTime.now();
      final storePluginsFromAPI = await WoxApi.instance.findStorePlugins(traceId);
      storePluginsFromAPI.sort((a, b) => a.name.compareTo(b.name));
      storePlugins.clear();
      storePlugins.addAll(storePluginsFromAPI);
      Logger.instance.info(traceId, 'Store plugins loaded, cost ${DateTime.now().difference(start).inMilliseconds} ms');
    } catch (e) {
      storePlugins.clear();
      Logger.instance.error(traceId, 'Failed to load store plugins: $e');
    }
  }

  Future<void> loadInstalledPlugins(String traceId) async {
    try {
      var start = DateTime.now();
      final installedPluginsFromAPI = await WoxApi.instance.findInstalledPlugins(traceId);
      installedPluginsFromAPI.sort((a, b) => a.name.compareTo(b.name));
      installedPlugins.clear();
      installedPlugins.addAll(installedPluginsFromAPI);
      Logger.instance.info(traceId, 'Installed plugins loaded, cost ${DateTime.now().difference(start).inMilliseconds} ms');
    } catch (e) {
      installedPlugins.clear();
      Logger.instance.error(traceId, 'Failed to load installed plugins: $e');
    }
  }

  /// Preload both plugin lists at startup without awaiting to avoid blocking UI.
  void preloadPlugins(String traceId) {
    unawaited(loadInstalledPlugins(traceId));
    unawaited(loadStorePlugins(traceId));
  }

  Future<void> reloadPlugins(String traceId) async {
    final currentActivePluginId = activePlugin.value.id;

    await Future.wait([loadInstalledPlugins(traceId), loadStorePlugins(traceId)]);

    if (activeNavPath.value != 'plugins.installed' && activeNavPath.value != 'plugins.store') {
      return;
    }

    filterPlugins();

    if (pendingInstalledPluginFocusRef.isNotEmpty) {
      final focused = focusInstalledPlugin(pendingInstalledPluginFocusRef, keepPendingFocus: true);
      if (focused) {
        pendingInstalledPluginFocusRef = '';
        return;
      }
    }

    if (currentActivePluginId.isEmpty) {
      setFirstFilteredPluginDetailActive();
      return;
    }

    syncActivePluginWithFilteredList(currentActivePluginId: currentActivePluginId);
  }

  Future<void> refreshPlugin(String pluginId, String refreshType /* update / add / remove */) async {
    final traceId = const UuidV4().generate();
    Logger.instance.info(traceId, 'Refreshing plugin: $pluginId, refreshType: $refreshType');
    if (refreshType == "add") {
      PluginDetail updatedPlugin = await WoxApi.instance.getPluginDetail(traceId, pluginId);
      if (updatedPlugin.id.isEmpty) {
        Logger.instance.info(traceId, 'Plugin not found: $pluginId');
        return;
      }

      int storeIndex = storePlugins.indexWhere((p) => p.id == pluginId);
      if (storeIndex >= 0) {
        storePlugins[storeIndex] = updatedPlugin;
      }
      int installedIndex = installedPlugins.indexWhere((p) => p.id == pluginId);
      if (installedIndex >= 0) {
        installedPlugins[installedIndex] = updatedPlugin;
      } else {
        installedPlugins.add(updatedPlugin);
      }
      int pluginListIndex = pluginList.indexWhere((p) => p.id == pluginId);
      if (pluginListIndex >= 0) {
        pluginList[pluginListIndex] = updatedPlugin;
      } else if (activeNavPath.value == 'plugins.installed') {
        pluginList.add(updatedPlugin);
      }
      int filteredPluginListIndex = filteredPluginList.indexWhere((p) => p.id == pluginId);
      if (filteredPluginListIndex >= 0) {
        filteredPluginList[filteredPluginListIndex] = updatedPlugin;
      } else {
        filteredPluginList.add(updatedPlugin);
      }
      if (activePlugin.value.id == pluginId) {
        activePlugin.value = updatedPlugin;
      }
    } else if (refreshType == "remove") {
      installedPlugins.removeWhere((p) => p.id == pluginId);
      int storeIndex = storePlugins.indexWhere((p) => p.id == pluginId);
      if (storeIndex >= 0) {
        storePlugins[storeIndex].isInstalled = false;
      }
      // if is in installed plugin view, remove from plugin list
      if (activeNavPath.value == 'plugins.installed') {
        pluginList.removeWhere((p) => p.id == pluginId);
        filteredPluginList.removeWhere((p) => p.id == pluginId);
      }
      // if is in store plugin view, update the installed property
      if (activeNavPath.value == 'plugins.store') {
        pluginList.firstWhere((p) => p.id == pluginId).isInstalled = false;
        filteredPluginList.firstWhere((p) => p.id == pluginId).isInstalled = false;
      }
      if (activePlugin.value.id == pluginId) {
        activePlugin.value = installedPlugins.isNotEmpty ? installedPlugins[0] : PluginDetail.empty();
      }
    } else if (refreshType == "update") {
      PluginDetail updatedPlugin = await WoxApi.instance.getPluginDetail(traceId, pluginId);
      if (updatedPlugin.id.isEmpty) {
        Logger.instance.info(traceId, 'Plugin not found: $pluginId');
        return;
      }

      int installedIndex = installedPlugins.indexWhere((p) => p.id == pluginId);
      if (installedIndex >= 0) {
        installedPlugins[installedIndex] = updatedPlugin;
      }
      int storeIndex = storePlugins.indexWhere((p) => p.id == pluginId);
      if (storeIndex >= 0) {
        storePlugins[storeIndex] = updatedPlugin;
      }
      int pluginListIndex = pluginList.indexWhere((p) => p.id == pluginId);
      if (pluginListIndex >= 0) {
        pluginList[pluginListIndex] = updatedPlugin;
      }
      int filteredPluginListIndex = filteredPluginList.indexWhere((p) => p.id == pluginId);
      if (filteredPluginListIndex >= 0) {
        filteredPluginList[filteredPluginListIndex] = updatedPlugin;
      }
      if (activePlugin.value.id == pluginId) {
        activePlugin.value = updatedPlugin;
      }
    }

    filterPlugins();
    syncActivePluginWithFilteredList();
  }

  Future<void> switchToPluginList(String traceId, bool isStorePlugin) async {
    Logger.instance.info(traceId, 'Switching to plugin list: $isStorePlugin');
    if (isStorePlugin) {
      pluginList.clear();
      pluginList.addAll(storePlugins);
    } else {
      pluginList.clear();
      pluginList.addAll(installedPlugins);
    }

    activeNavPath.value = isStorePlugin ? 'plugins.store' : 'plugins.installed';
    isStorePluginList.value = isStorePlugin;
    activePlugin.value = PluginDetail.empty();
    filterPluginKeywordController.text = "";

    filterPlugins();

    //active plugin
    if (activePlugin.value.id.isNotEmpty) {
      activePlugin.value = filteredPluginList.firstWhere(
        (element) => element.id == activePlugin.value.id,
        orElse: () => filteredPluginList.isNotEmpty ? filteredPluginList[0] : PluginDetail.empty(),
      );
    } else {
      setFirstFilteredPluginDetailActive();
    }
  }

  Future<void> switchToDataView(String traceId) async {
    activeNavPath.value = 'data';
  }

  GlobalKey getPluginListItemKey(String pluginId) {
    return pluginListItemKeys.putIfAbsent(pluginId, () => GlobalKey());
  }

  Future<void> ensurePluginVisible(String pluginId) async {
    final targetIndex = filteredPluginList.indexWhere((plugin) => plugin.id == pluginId);
    if (targetIndex < 0) {
      return;
    }
    final itemKey = pluginListItemKeys[pluginId];

    WidgetsBinding.instance.addPostFrameCallback((_) async {
      if (pluginListScrollController.hasClients) {
        const estimatedItemExtent = 88.0;
        final targetOffset = targetIndex * estimatedItemExtent;
        final maxExtent = pluginListScrollController.position.maxScrollExtent;
        final clampedOffset = targetOffset.clamp(0.0, maxExtent);

        if ((pluginListScrollController.offset - clampedOffset).abs() > 4) {
          await pluginListScrollController.animateTo(clampedOffset, duration: const Duration(milliseconds: 180), curve: Curves.easeOutCubic);
        }
      }

      WidgetsBinding.instance.addPostFrameCallback((_) {
        final itemContext = itemKey?.currentContext;
        if (itemContext != null) {
          Scrollable.ensureVisible(itemContext, duration: const Duration(milliseconds: 120), curve: Curves.easeOutCubic, alignment: 0.5);
        }
      });
    });
  }

  void resetPluginFilters() {
    filterPluginKeywordController.text = "";
    filterEnabledPluginsOnly.value = false;
    filterDisabledPluginsOnly.value = false;
    filterUpgradablePluginsOnly.value = false;
    filterUninstalledPluginsOnly.value = false;
    filterRuntimeNodejsOnly.value = false;
    filterRuntimePythonOnly.value = false;
    filterRuntimeScriptOnly.value = false;
    filterRuntimeScriptNodejsOnly.value = false;
    filterRuntimeScriptPythonOnly.value = false;
  }

  bool focusInstalledPlugin(String pluginRef, {bool keepPendingFocus = false}) {
    if (!keepPendingFocus) {
      pendingInstalledPluginFocusRef = pluginRef;
    }

    resetPluginFilters();
    filterPlugins();

    if (pluginRef.isEmpty) {
      setFirstFilteredPluginDetailActive();
      ensurePluginVisible(activePlugin.value.id);
      return activePlugin.value.id.isNotEmpty;
    }

    final exactPluginIndex = filteredPluginList.indexWhere((plugin) => plugin.id == pluginRef);
    if (exactPluginIndex >= 0) {
      activePlugin.value = filteredPluginList[exactPluginIndex];
      ensurePluginVisible(activePlugin.value.id);
      return true;
    }

    filterPluginKeywordController.text = pluginRef;
    filterPlugins();
    setFirstFilteredPluginDetailActive();
    ensurePluginVisible(activePlugin.value.id);
    return activePlugin.value.id.isNotEmpty;
  }

  void setFirstFilteredPluginDetailActive() {
    if (filteredPluginList.isNotEmpty) {
      activePlugin.value = filteredPluginList[0];
      return;
    }

    activePlugin.value = PluginDetail.empty();
  }

  void syncActivePluginWithFilteredList({String? currentActivePluginId}) {
    if (filteredPluginList.isEmpty) {
      activePlugin.value = PluginDetail.empty();
      return;
    }

    final targetPluginId = currentActivePluginId ?? activePlugin.value.id;
    if (targetPluginId.isEmpty) {
      setFirstFilteredPluginDetailActive();
      return;
    }

    final idx = filteredPluginList.indexWhere((plugin) => plugin.id == targetPluginId);
    if (idx >= 0) {
      activePlugin.value = filteredPluginList[idx];
      return;
    }

    setFirstFilteredPluginDetailActive();
  }

  bool get hasInstalledRuntimePluginFilterApplied =>
      filterRuntimeNodejsOnly.value || filterRuntimePythonOnly.value || filterRuntimeScriptNodejsOnly.value || filterRuntimeScriptPythonOnly.value;

  bool get hasStoreRuntimePluginFilterApplied => filterRuntimeNodejsOnly.value || filterRuntimePythonOnly.value || filterRuntimeScriptOnly.value;

  bool get hasInstalledPluginFilterApplied =>
      filterEnabledPluginsOnly.value || filterDisabledPluginsOnly.value || filterUpgradablePluginsOnly.value || hasInstalledRuntimePluginFilterApplied;

  bool get hasStorePluginFilterApplied => filterUninstalledPluginsOnly.value || hasStoreRuntimePluginFilterApplied;

  bool get hasPluginFilterApplied => isStorePluginList.value ? hasStorePluginFilterApplied : hasInstalledPluginFilterApplied;

  void updatePluginFilters({
    bool? enabledOnly,
    bool? disabledOnly,
    bool? upgradableOnly,
    bool? uninstalledOnly,
    bool? runtimeNodejsOnly,
    bool? runtimePythonOnly,
    bool? runtimeScriptOnly,
    bool? runtimeScriptNodejsOnly,
    bool? runtimeScriptPythonOnly,
  }) {
    if (enabledOnly != null) {
      filterEnabledPluginsOnly.value = enabledOnly;
    }
    if (disabledOnly != null) {
      filterDisabledPluginsOnly.value = disabledOnly;
    }
    if (upgradableOnly != null) {
      filterUpgradablePluginsOnly.value = upgradableOnly;
    }
    if (uninstalledOnly != null) {
      filterUninstalledPluginsOnly.value = uninstalledOnly;
    }
    if (runtimeNodejsOnly != null) {
      filterRuntimeNodejsOnly.value = runtimeNodejsOnly;
    }
    if (runtimePythonOnly != null) {
      filterRuntimePythonOnly.value = runtimePythonOnly;
    }
    if (runtimeScriptOnly != null) {
      filterRuntimeScriptOnly.value = runtimeScriptOnly;
    }
    if (runtimeScriptNodejsOnly != null) {
      filterRuntimeScriptNodejsOnly.value = runtimeScriptNodejsOnly;
    }
    if (runtimeScriptPythonOnly != null) {
      filterRuntimeScriptPythonOnly.value = runtimeScriptPythonOnly;
    }

    filterPlugins();
    syncActivePluginWithFilteredList();
  }

  bool isScriptNodejsPlugin(PluginDetail plugin) {
    if (!WoxPluginRuntimeEnum.equals(plugin.runtime, WoxPluginRuntimeEnum.SCRIPT)) {
      return false;
    }

    return plugin.entry.toLowerCase().endsWith('.js');
  }

  bool isScriptPythonPlugin(PluginDetail plugin) {
    if (!WoxPluginRuntimeEnum.equals(plugin.runtime, WoxPluginRuntimeEnum.SCRIPT)) {
      return false;
    }

    return plugin.entry.toLowerCase().endsWith('.py');
  }

  Future<void> installPlugin(PluginDetail plugin) async {
    try {
      pluginInstallError.value = '';
      isInstallingPlugin.value = true;
      final traceId = const UuidV4().generate();
      Logger.instance.info(traceId, 'installing plugin: ${plugin.name}');
      await WoxApi.instance.installPlugin(traceId, plugin.id);
      await refreshPlugin(plugin.id, "add");
    } catch (e) {
      final traceId = const UuidV4().generate();
      Logger.instance.error(traceId, 'Failed to install plugin ${plugin.name}: $e');
      pluginInstallError.value = e.toString();
    } finally {
      isInstallingPlugin.value = false;
    }
  }

  Future<void> upgradePlugin(PluginDetail plugin) async {
    if (isUpgradingPlugin.value) {
      return;
    }

    try {
      pluginInstallError.value = '';
      isUpgradingPlugin.value = true;
      final traceId = const UuidV4().generate();
      Logger.instance.info(traceId, 'upgrading plugin: ${plugin.name}');
      // Keep the same upgrade path as WPM plugin: install from store by plugin id.
      await WoxApi.instance.installPlugin(traceId, plugin.id);
      await refreshPlugin(plugin.id, "update");
    } catch (e) {
      final traceId = const UuidV4().generate();
      Logger.instance.error(traceId, 'Failed to upgrade plugin ${plugin.name}: $e');
      pluginInstallError.value = e.toString();
    } finally {
      isUpgradingPlugin.value = false;
    }
  }

  Future<void> disablePlugin(PluginDetail plugin) async {
    final traceId = const UuidV4().generate();
    Logger.instance.info(traceId, 'disabling plugin: ${plugin.name}');
    await WoxApi.instance.disablePlugin(traceId, plugin.id);
    await refreshPlugin(plugin.id, "update");
  }

  Future<void> enablePlugin(PluginDetail plugin) async {
    final traceId = const UuidV4().generate();
    Logger.instance.info(traceId, 'enabling plugin: ${plugin.name}');
    await WoxApi.instance.enablePlugin(traceId, plugin.id);
    await refreshPlugin(plugin.id, "update");
  }

  Future<void> uninstallPlugin(PluginDetail plugin) async {
    final traceId = const UuidV4().generate();
    Logger.instance.info(traceId, 'uninstalling plugin: ${plugin.name}');
    await WoxApi.instance.uninstallPlugin(traceId, plugin.id);
    await refreshPlugin(plugin.id, "remove");
  }

  void filterPlugins() {
    final keyword = filterPluginKeywordController.text;
    final isStoreView = isStorePluginList.value;
    final enabledOnly = !isStoreView && filterEnabledPluginsOnly.value;
    final disabledOnly = !isStoreView && filterDisabledPluginsOnly.value;
    final upgradableOnly = !isStoreView && filterUpgradablePluginsOnly.value;
    final uninstalledOnly = isStoreView && filterUninstalledPluginsOnly.value;
    final runtimeNodejsOnly = filterRuntimeNodejsOnly.value;
    final runtimePythonOnly = filterRuntimePythonOnly.value;
    final runtimeScriptOnly = filterRuntimeScriptOnly.value;
    final runtimeScriptNodejsOnly = filterRuntimeScriptNodejsOnly.value;
    final runtimeScriptPythonOnly = filterRuntimeScriptPythonOnly.value;

    bool matchesKeyword(PluginDetail plugin) {
      if (keyword.isEmpty) {
        return true;
      }

      bool match = WoxFuzzyMatchUtil.isFuzzyMatch(text: plugin.name, pattern: keyword, usePinYin: WoxSettingUtil.instance.currentSetting.usePinYin);
      if (match) {
        return true;
      }

      if (plugin.nameEn.isNotEmpty) {
        match = WoxFuzzyMatchUtil.isFuzzyMatch(text: plugin.nameEn, pattern: keyword, usePinYin: false);
        if (match) {
          return true;
        }
      }

      if (plugin.description.toLowerCase().contains(keyword.toLowerCase())) {
        return true;
      }

      if (plugin.descriptionEn.toLowerCase().contains(keyword.toLowerCase())) {
        return true;
      }

      return false;
    }

    bool matchesAdvancedFilter(PluginDetail plugin) {
      if (enabledOnly && plugin.isDisable) {
        return false;
      }
      if (disabledOnly && !plugin.isDisable) {
        return false;
      }
      if (upgradableOnly && !plugin.isUpgradable) {
        return false;
      }
      if (uninstalledOnly && plugin.isInstalled) {
        return false;
      }

      final runtimeFilterApplied = runtimeNodejsOnly || runtimePythonOnly || (isStoreView ? runtimeScriptOnly : (runtimeScriptNodejsOnly || runtimeScriptPythonOnly));
      if (!runtimeFilterApplied) {
        return true;
      }

      final matchRuntimeNodejs = runtimeNodejsOnly && WoxPluginRuntimeEnum.equals(plugin.runtime, WoxPluginRuntimeEnum.NODEJS);
      final matchRuntimePython = runtimePythonOnly && WoxPluginRuntimeEnum.equals(plugin.runtime, WoxPluginRuntimeEnum.PYTHON);
      final matchRuntimeScript = isStoreView && runtimeScriptOnly && WoxPluginRuntimeEnum.equals(plugin.runtime, WoxPluginRuntimeEnum.SCRIPT);
      final matchRuntimeScriptNodejs = runtimeScriptNodejsOnly && isScriptNodejsPlugin(plugin);
      final matchRuntimeScriptPython = runtimeScriptPythonOnly && isScriptPythonPlugin(plugin);

      if (!(matchRuntimeNodejs || matchRuntimePython || matchRuntimeScript || matchRuntimeScriptNodejs || matchRuntimeScriptPython)) {
        return false;
      }

      return true;
    }

    final filtered = pluginList.where((plugin) => matchesKeyword(plugin) && matchesAdvancedFilter(plugin)).toList();
    filteredPluginList.assignAll(filtered);
  }

  Future<void> openPluginWebsite(String website) async {
    await launchUrl(Uri.parse(website));
  }

  Future<void> openPluginDirectory(PluginDetail plugin) async {
    final directory = plugin.pluginDirectory;
    if (directory.isEmpty) {
      return;
    }
    await openFolder(directory);
  }

  Future<String?> updatePluginSetting(String pluginId, String key, String value) async {
    final traceId = const UuidV4().generate();
    final activeTabIndex = activePluginTabController.index;
    final previousValue = getPluginSettingValue(pluginId, key);
    applyPluginSettingOptimistically(pluginId, key, value);

    final saveStart = DateTime.now();
    try {
      await WoxApi.instance.updatePluginSetting(traceId, pluginId, key, value);
      Logger.instance.info(traceId, 'plugin setting saved: $key=$value, cost ${DateTime.now().difference(saveStart).inMilliseconds} ms');
    } catch (e) {
      Logger.instance.error(traceId, 'failed to save plugin setting: $key=$value, error: $e');
      restorePluginSetting(pluginId, key, previousValue);
      return e.toString().replaceFirst('Exception: ', '');
    }

    unawaited(refreshPluginAfterSettingUpdate(pluginId, activeTabIndex, traceId));
    return null;
  }

  String? getPluginSettingValue(String pluginId, String key) {
    PluginDetail? target;

    if (activePlugin.value.id == pluginId) {
      target = activePlugin.value;
    } else {
      for (final plugin in [...installedPlugins, ...storePlugins, ...pluginList, ...filteredPluginList]) {
        if (plugin.id == pluginId) {
          target = plugin;
          break;
        }
      }
    }

    if (target == null) {
      return null;
    }

    if (!target.setting.settings.containsKey(key)) {
      return null;
    }

    return target.setting.settings[key];
  }

  // Optimistically update the plugin setting in all relevant lists to provide instant feedback in the UI,
  // instead of waiting for the API response.
  void applyPluginSettingOptimistically(String pluginId, String key, String value) {
    bool updatePlugin(List<PluginDetail> plugins) {
      var updated = false;
      for (final plugin in plugins) {
        if (plugin.id != pluginId) {
          continue;
        }
        plugin.setting.settings[key] = value;
        updated = true;
      }
      return updated;
    }

    final active = activePlugin.value;
    if (active.id == pluginId) {
      active.setting.settings[key] = value;
      activePlugin.refresh();
    }

    updatePlugin(installedPlugins);
    updatePlugin(storePlugins);
    updatePlugin(pluginList);
    if (updatePlugin(filteredPluginList)) {
      filteredPluginList.refresh();
    }
  }

  void restorePluginSetting(String pluginId, String key, String? previousValue) {
    bool updatePlugin(List<PluginDetail> plugins) {
      var updated = false;
      for (final plugin in plugins) {
        if (plugin.id != pluginId) {
          continue;
        }

        if (previousValue == null) {
          plugin.setting.settings.remove(key);
        } else {
          plugin.setting.settings[key] = previousValue;
        }
        updated = true;
      }
      return updated;
    }

    final active = activePlugin.value;
    if (active.id == pluginId) {
      if (previousValue == null) {
        active.setting.settings.remove(key);
      } else {
        active.setting.settings[key] = previousValue;
      }
      activePlugin.refresh();
    }

    updatePlugin(installedPlugins);
    updatePlugin(storePlugins);
    updatePlugin(pluginList);
    if (updatePlugin(filteredPluginList)) {
      filteredPluginList.refresh();
    }
  }

  Future<void> refreshPluginAfterSettingUpdate(String pluginId, int activeTabIndex, String traceId) async {
    final refreshStart = DateTime.now();
    await refreshPlugin(pluginId, "update");
    Logger.instance.info(traceId, 'plugin detail refreshed after setting update, cost ${DateTime.now().difference(refreshStart).inMilliseconds} ms');

    // switch to the tab that was active before the update
    WidgetsBinding.instance.addPostFrameCallback((timeStamp) {
      if (activePluginTabController.index != activeTabIndex) {
        activePluginTabController.index = activeTabIndex;
      }
    });
  }

  Future<void> updatePluginTriggerKeywords(String pluginId, List<String> triggerKeywords) async {}

  bool shouldShowSettingTab() {
    return activePlugin.value.isInstalled && activePlugin.value.settingDefinitions.isNotEmpty;
  }

  void switchToPluginSettingTab() {
    if (shouldShowSettingTab()) {
      // buggy, ref https://github.com/alihaider78222/dynamic_tabbar/issues/6
      // activePluginTabController.animateTo(1, duration: Duration.zero);
    }
  }

  // ---------- Themes ----------

  Future<void> loadStoreThemes() async {
    final traceId = const UuidV4().generate();
    final storeThemes = await WoxApi.instance.findStoreThemes(traceId);
    storeThemes.sort((a, b) => a.themeName.compareTo(b.themeName));
    themeList.clear();
    for (var theme in storeThemes) {
      themeList.add(theme);
    }
    filteredThemeList.clear();
    filteredThemeList.addAll(themeList);
    // Also load installed themes for auto theme lookup
    await _loadInstalledThemesForLookup();
  }

  Future<void> loadInstalledThemes() async {
    final traceId = const UuidV4().generate();
    final installThemes = await WoxApi.instance.findInstalledThemes(traceId);
    installThemes.sort((a, b) => a.themeName.compareTo(b.themeName));
    themeList.clear();
    installedThemesList.clear();
    for (var theme in installThemes) {
      themeList.add(theme);
      installedThemesList.add(theme);
    }
    filteredThemeList.clear();
    filteredThemeList.addAll(themeList);
  }

  Future<void> _loadInstalledThemesForLookup() async {
    final traceId = const UuidV4().generate();
    final installThemes = await WoxApi.instance.findInstalledThemes(traceId);
    installedThemesList.clear();
    installedThemesList.addAll(installThemes);
  }

  Future<void> installTheme(WoxTheme theme) async {
    final traceId = const UuidV4().generate();
    Logger.instance.info(traceId, 'Installing theme: ${theme.themeId}');
    await WoxApi.instance.installTheme(traceId, theme.themeId);
    await refreshThemeList();
  }

  Future<void> uninstallTheme(WoxTheme theme) async {
    final traceId = const UuidV4().generate();
    Logger.instance.info(traceId, 'Uninstalling theme: ${theme.themeId}');
    await WoxApi.instance.uninstallTheme(traceId, theme.themeId);
    await refreshThemeList();
  }

  Future<void> applyTheme(WoxTheme theme) async {
    final traceId = const UuidV4().generate();
    Logger.instance.info(traceId, 'Applying theme: ${theme.themeId}');
    await WoxApi.instance.applyTheme(traceId, theme.themeId);
    await refreshThemeList();
    await reloadSetting(traceId);
  }

  void onFilterThemes(String filter) {
    filteredThemeList.clear();
    filteredThemeList.addAll(themeList.where((element) => element.themeName.toLowerCase().contains(filter.toLowerCase())));
  }

  void setFirstFilteredThemeActive() {
    if (filteredThemeList.isNotEmpty) {
      activeTheme.value = filteredThemeList[0];
    }
  }

  Future<void> refreshThemeList() async {
    if (isStoreThemeList.value) {
      await loadStoreThemes();
    } else {
      await loadInstalledThemes();
    }

    //active theme
    if (activeTheme.value.themeId.isNotEmpty) {
      activeTheme.value = filteredThemeList.firstWhere((element) => element.themeId == activeTheme.value.themeId, orElse: () => filteredThemeList[0]);
    } else {
      setFirstFilteredThemeActive();
    }
  }

  Future<void> switchToThemeList(bool isStoreTheme) async {
    activeNavPath.value = isStoreTheme ? 'themes.store' : 'themes.installed';
    isStoreThemeList.value = isStoreTheme;
    activeTheme.value = WoxTheme.empty();
    await refreshThemeList();
    setFirstFilteredThemeActive();
  }

  Future<void> loadUserDataLocation() async {
    final traceId = const UuidV4().generate();
    try {
      userDataLocation.value = await WoxApi.instance.getUserDataLocation(traceId);
    } catch (e) {
      userDataLocation.value = '';
      Logger.instance.error(traceId, 'Failed to load user data location: $e');
    }
  }

  Future<void> updateUserDataLocation(String newLocation) async {
    final traceId = const UuidV4().generate();
    await WoxApi.instance.updateUserDataLocation(traceId, newLocation);
    userDataLocation.value = newLocation;
  }

  Future<void> backupNow() async {
    await WoxApi.instance.backupNow(const UuidV4().generate());
    refreshBackups();
  }

  Future<void> refreshBackups() async {
    final traceId = const UuidV4().generate();
    try {
      final result = await WoxApi.instance.getAllBackups(traceId);
      backups.assignAll(result);
    } catch (e) {
      backups.clear();
      Logger.instance.error(traceId, 'Failed to load backups: $e');
    }
  }

  Future<void> clearLogs() async {
    if (isClearingLogs.value) {
      return;
    }

    final traceId = const UuidV4().generate();
    isClearingLogs.value = true;
    try {
      await WoxApi.instance.clearLogs(traceId);
      Logger.instance.info(traceId, 'Logs cleared');
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to clear logs: $e');
    } finally {
      isClearingLogs.value = false;
    }
  }

  Future<void> updateLogLevel(String level) async {
    if (isUpdatingLogLevel.value) {
      return;
    }

    final previous = woxSetting.value.logLevel;
    woxSetting.value.logLevel = level;
    woxSetting.refresh();
    Logger.instance.setLogLevel(level);

    final traceId = const UuidV4().generate();
    isUpdatingLogLevel.value = true;
    try {
      await WoxApi.instance.updateSetting(traceId, "LogLevel", level);
      await reloadSetting(traceId);
      Logger.instance.info(traceId, 'LogLevel updated: $level');
    } catch (e) {
      woxSetting.value.logLevel = previous;
      woxSetting.refresh();
      Logger.instance.setLogLevel(previous);
      Logger.instance.error(traceId, 'Failed to update LogLevel: $e');
    } finally {
      isUpdatingLogLevel.value = false;
    }
  }

  Future<void> openLogFile() async {
    final traceId = const UuidV4().generate();
    await WoxApi.instance.openLogFile(traceId);
  }

  Future<void> openFolder(String path) async {
    await WoxApi.instance.open(const UuidV4().generate(), path);
  }

  Future<void> restoreBackup(String id) async {
    final traceId = const UuidV4().generate();
    await WoxApi.instance.restoreBackup(traceId, id);
    await reloadSetting(traceId);
  }

  Future<void> reloadSetting(String traceId) async {
    await WoxSettingUtil.instance.loadSetting(traceId);
    woxSetting.value = WoxSettingUtil.instance.currentSetting;
    Logger.instance.setLogLevel(woxSetting.value.logLevel);
    Logger.instance.info(traceId, 'Setting reloaded');
  }

  @override
  void onClose() {
    pluginListScrollController.dispose();
    settingFocusNode.dispose();
    super.onClose();
  }
}

class _GeneralSectionFocusRequest {
  final String sectionId;
  final int? trayQueryEditRowIndex;

  const _GeneralSectionFocusRequest({required this.sectionId, this.trayQueryEditRowIndex});
}
