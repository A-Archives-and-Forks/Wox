import 'package:wox/entity/wox_image.dart';

class ToolbarActionInfo {
  final String name;
  final String hotkey;
  final Function()? action; // Optional action callback for cases without result (e.g., doctor check)

  ToolbarActionInfo({required this.name, required this.hotkey, this.action});
}

class ToolbarStatusActionInfo {
  final String id;
  final String name;
  final WoxImage? icon;
  final String hotkey;
  final bool isDefault;
  final bool preventHideAfterAction;
  final Map<String, String> contextData;

  ToolbarStatusActionInfo({
    required this.id,
    required this.name,
    required this.icon,
    required this.hotkey,
    required this.isDefault,
    required this.preventHideAfterAction,
    required this.contextData,
  });

  factory ToolbarStatusActionInfo.fromJson(Map<String, dynamic> json) {
    final rawContextData = json['ContextData'];
    final contextData = rawContextData is Map ? rawContextData.map((key, value) => MapEntry(key.toString(), value.toString())) : <String, String>{};

    return ToolbarStatusActionInfo(
      id: json['Id'] ?? "",
      name: json['Name'] ?? "",
      icon: json['Icon'] != null ? WoxImage.fromJson(json['Icon']) : null,
      hotkey: json['Hotkey'] ?? "",
      isDefault: json['IsDefault'] == true,
      preventHideAfterAction: json['PreventHideAfterAction'] == true,
      contextData: contextData,
    );
  }
}

class ToolbarStatusInfo {
  final String id;
  final String title;
  final WoxImage? icon;
  final int? progress;
  final bool indeterminate;
  final List<ToolbarStatusActionInfo> actions;

  ToolbarStatusInfo({required this.id, required this.title, required this.icon, required this.progress, required this.indeterminate, required this.actions});

  factory ToolbarStatusInfo.empty() {
    return ToolbarStatusInfo(id: "", title: "", icon: null, progress: null, indeterminate: false, actions: const []);
  }

  factory ToolbarStatusInfo.fromJson(Map<String, dynamic> json) {
    return ToolbarStatusInfo(
      id: json['Id'] ?? "",
      title: json['Title'] ?? "",
      icon: json['Icon'] != null ? WoxImage.fromJson(json['Icon']) : null,
      progress: json['Progress'] is int ? json['Progress'] : null,
      indeterminate: json['Indeterminate'] == true,
      actions: (json['Actions'] as List<dynamic>? ?? []).map((item) => ToolbarStatusActionInfo.fromJson(item)).toList(),
    );
  }

  String get text => title;

  bool get isEmpty => id.isEmpty || title.isEmpty;
}

class ToolbarInfo {
  // left side of the toolbar
  final WoxImage? icon;
  final String? text;

  // right side of the toolbar
  final List<ToolbarActionInfo>? actions; // All actions with hotkeys

  ToolbarInfo({this.icon, this.text, this.actions});

  static ToolbarInfo empty() {
    return ToolbarInfo(text: '');
  }

  ToolbarInfo copyWith({WoxImage? icon, String? text, List<ToolbarActionInfo>? actions}) {
    return ToolbarInfo(icon: icon ?? this.icon, text: text ?? this.text, actions: actions ?? this.actions);
  }

  ToolbarInfo emptyRightSide() {
    return ToolbarInfo(icon: icon, text: text, actions: null);
  }

  ToolbarInfo emptyLeftSide() {
    return ToolbarInfo(icon: null, text: null, actions: actions);
  }

  // text and actions are both empty
  bool isEmpty() {
    return (text == null || text!.isEmpty) && (actions == null || actions!.isEmpty);
  }

  bool isNotEmpty() {
    return !isEmpty();
  }
}

class ToolbarMsg {
  final WoxImage? icon;
  final String? text;
  final int displaySeconds; // how long to display the message, 0 for forever

  ToolbarMsg({this.icon, this.text, this.displaySeconds = 10});

  static ToolbarMsg fromJson(Map<String, dynamic> json) {
    return ToolbarMsg(icon: WoxImage.parse(json['Icon']), text: json['Text'] ?? '', displaySeconds: json['DisplaySeconds'] ?? 10);
  }
}
