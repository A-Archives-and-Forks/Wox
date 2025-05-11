import 'package:wox/entity/wox_image.dart';
import 'package:wox/entity/wox_list_item.dart';
import 'package:wox/entity/wox_preview.dart';
import 'package:wox/enums/wox_last_query_mode_enum.dart';
import 'package:wox/enums/wox_position_type_enum.dart';
import 'package:wox/enums/wox_query_type_enum.dart';
import 'package:wox/enums/wox_selection_type_enum.dart';

class PlainQuery {
  late String queryId;
  late WoxQueryType queryType;
  late String queryText;
  late Selection querySelection;

  PlainQuery({required this.queryId, required this.queryType, required this.queryText, required this.querySelection});

  PlainQuery.fromJson(Map<String, dynamic> json) {
    queryId = json['QueryId'] ?? "";
    queryType = json['QueryType'];
    queryText = json['QueryText'];
    querySelection = Selection.fromJson(json['QuerySelection']);
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['QueryId'] = queryId;
    data['QueryType'] = queryType;
    data['QueryText'] = queryText;
    data['QuerySelection'] = querySelection.toJson();
    return data;
  }

  bool get isEmpty => queryText.isEmpty && querySelection.type.isEmpty;

  static PlainQuery empty() {
    return PlainQuery(queryId: "", queryType: "", queryText: "", querySelection: Selection.empty());
  }

  static PlainQuery text(String text) {
    return PlainQuery(queryId: "", queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code, queryText: text, querySelection: Selection.empty());
  }

  static PlainQuery emptyInput() {
    return PlainQuery(queryId: "", queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code, queryText: "", querySelection: Selection.empty());
  }
}

class Selection {
  late WoxSelectionType type;

  // Only available when Type is SelectionTypeText
  late String text;

  // Only available when Type is SelectionTypeFile
  late List<String> filePaths;

  Selection.fromJson(Map<String, dynamic> json) {
    type = json['Type'];
    text = json['Text'];
    filePaths = List<String>.from(json['FilePaths'] ?? []);
  }

  Map<String, dynamic> toJson() {
    return <String, dynamic>{
      'Type': type,
      'Text': text,
      'FilePaths': filePaths,
    };
  }

  Selection({required this.type, required this.text, required this.filePaths});

  static Selection empty() {
    return Selection(type: "", text: "", filePaths: []);
  }
}

class QueryHistory {
  PlainQuery? query;
  int? timestamp;

  QueryHistory.fromJson(Map<String, dynamic> json) {
    query = json['Query'] != null ? PlainQuery.fromJson(json['Query']) : null;
    timestamp = json['Timestamp'];
  }
}

class WoxQueryResult {
  late String queryId;
  late String id;
  late String title;
  late String subTitle;
  late WoxImage icon;
  late WoxPreview preview;
  late int score;
  late String group;
  late int groupScore;
  late List<WoxListItemTail> tails;
  late String contextData;

  late List<WoxResultAction> actions;
  late int refreshInterval;

  // Used by the frontend to determine if this result is a group
  late bool isGroup;

  WoxQueryResult(
      {required this.queryId,
      required this.id,
      required this.title,
      required this.subTitle,
      required this.icon,
      required this.preview,
      required this.score,
      required this.group,
      required this.groupScore,
      required this.tails,
      required this.contextData,
      required this.actions,
      required this.refreshInterval,
      required this.isGroup});

  WoxQueryResult.empty() {
    queryId = "";
    id = "";
    title = "";
    subTitle = "";
    icon = WoxImage.empty();
    preview = WoxPreview.empty();
    score = 0;
    group = "";
    groupScore = 0;
    tails = [];
    contextData = "";
    actions = [];
    refreshInterval = 0;
    isGroup = false;
  }

  WoxQueryResult.fromJson(Map<String, dynamic> json) {
    queryId = json['QueryId'];
    id = json['Id'];
    title = json['Title'];
    subTitle = json['SubTitle'];
    icon = (json['Icon'] != null ? WoxImage.fromJson(json['Icon']) : null)!;
    preview = (json['Preview'] != null ? WoxPreview.fromJson(json['Preview']) : null)!;
    score = json['Score'];
    group = json['Group'];
    groupScore = json['GroupScore'];
    contextData = json['ContextData'];

    if (json['Tails'] != null) {
      tails = [];
      json['Tails'].forEach((v) {
        tails.add(WoxListItemTail.fromJson(v));
      });
    } else {
      tails = [];
    }

    if (json['Actions'] != null) {
      actions = [];
      json['Actions'].forEach((v) {
        var action = WoxResultAction.fromJson(v);
        action.resultId = id;
        actions.add(action);
      });
    } else {
      actions = [];
    }

    refreshInterval = json['RefreshInterval'];
    isGroup = false;
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['QueryId'] = queryId;
    data['Id'] = id;
    data['Title'] = title;
    data['SubTitle'] = subTitle;
    data['Icon'] = icon.toJson();
    data['Preview'] = preview.toJson();
    data['Score'] = score;
    data['Group'] = group;
    data['GroupScore'] = groupScore;
    data['ContextData'] = contextData;
    data['Actions'] = actions.map((v) => v.toJson()).toList();
    data['RefreshInterval'] = refreshInterval;
    data['Tails'] = tails.map((v) => v.toJson()).toList();
    return data;
  }
}

class WoxResultAction {
  late String id;
  late String name;
  late WoxImage icon;
  late bool isDefault;
  late bool preventHideAfterAction;
  late String hotkey;
  late bool isSystemAction;
  late String resultId;

  WoxResultAction({
    required this.id,
    required this.name,
    required this.icon,
    required this.isDefault,
    required this.preventHideAfterAction,
    required this.hotkey,
    required this.isSystemAction,
    required this.resultId,
  });

  WoxResultAction.fromJson(Map<String, dynamic> json) {
    id = json['Id'];
    name = json['Name'];
    icon = (json['Icon'] != null ? WoxImage.fromJson(json['Icon']) : null)!;
    isDefault = json['IsDefault'];
    preventHideAfterAction = json['PreventHideAfterAction'];
    if (json['Hotkey'] != null) {
      hotkey = json['Hotkey'];
    }
    isSystemAction = json['IsSystemAction'];
    resultId = json['ResultId'] ?? "";
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['Id'] = id;
    data['Name'] = name;
    data['Icon'] = icon.toJson();
    data['IsDefault'] = isDefault;
    data['PreventHideAfterAction'] = preventHideAfterAction;
    data['Hotkey'] = hotkey;
    data['IsSystemAction'] = isSystemAction;
    data['ResultId'] = resultId;
    return data;
  }

  static WoxResultAction empty() {
    return WoxResultAction(id: "", name: "", icon: WoxImage.empty(), isDefault: false, preventHideAfterAction: false, hotkey: "", isSystemAction: false, resultId: "");
  }
}

class Position {
  late WoxPositionType type;
  late int x;
  late int y;

  Position({required this.type, required this.x, required this.y});

  Position.fromJson(Map<String, dynamic> json) {
    type = json['Type'];
    x = json['X'];
    y = json['Y'];
  }
}

class ShowAppParams {
  late bool selectAll;
  late Position position;
  late List<QueryHistory> queryHistories;
  late WoxLastQueryMode lastQueryMode;
  late bool autoFocusToChatInput;

  ShowAppParams({required this.selectAll, required this.position, required this.queryHistories, required this.lastQueryMode, this.autoFocusToChatInput = false});

  ShowAppParams.fromJson(Map<String, dynamic> json) {
    selectAll = json['SelectAll'];
    if (json['Position'] != null) {
      position = Position.fromJson(json['Position']);
    }
    queryHistories = <QueryHistory>[];
    if (json['QueryHistories'] != null) {
      json['QueryHistories'].forEach((v) {
        queryHistories.add(QueryHistory.fromJson(v));
      });
    } else {
      queryHistories = <QueryHistory>[];
    }
    lastQueryMode = json['LastQueryMode'];
    autoFocusToChatInput = json['AutoFocusToChatInput'] ?? false;
  }
}

class WoxListViewItemParams {
  late String title;
  late String subTitle;
  late WoxImage icon;

  WoxListViewItemParams.fromJson(Map<String, dynamic> json) {
    title = json['Title'];
    subTitle = json['SubTitle'] ?? "";
    icon = json['Icon'];
  }
}

class WoxRefreshableResult {
  late String resultId;
  late String title;
  late String subTitle;
  late WoxImage icon;
  late WoxPreview preview;
  late List<WoxListItemTail> tails;
  late String contextData;
  late int refreshInterval;
  late List<WoxResultAction> actions;

  WoxRefreshableResult({
    required this.resultId,
    required this.title,
    required this.subTitle,
    required this.icon,
    required this.preview,
    required this.tails,
    required this.contextData,
    required this.refreshInterval,
    required this.actions,
  });

  WoxRefreshableResult.fromJson(Map<String, dynamic> json) {
    resultId = json['ResultId'];
    title = json['Title'];
    subTitle = json['SubTitle'] ?? "";
    icon = WoxImage.fromJson(json['Icon']);
    preview = WoxPreview.fromJson(json['Preview']);
    tails = <WoxListItemTail>[];
    if (json['Tails'] != null) {
      json['Tails'].forEach((v) {
        tails.add(WoxListItemTail.fromJson(v));
      });
    }
    contextData = json['ContextData'];
    refreshInterval = json['RefreshInterval'];
    actions = <WoxResultAction>[];
    if (json['Actions'] != null) {
      json['Actions'].forEach((v) {
        var action = WoxResultAction.fromJson(v);
        action.resultId = resultId;
        actions.add(action);
      });
    }
  }

  Map<String, dynamic> toJson() {
    final Map<String, dynamic> data = <String, dynamic>{};
    data['ResultId'] = resultId;
    data['Title'] = title;
    data['SubTitle'] = subTitle;
    data['Icon'] = icon.toJson();
    data['Preview'] = preview.toJson();
    data['Tails'] = tails.map((v) => v.toJson()).toList();
    data['ContextData'] = contextData;
    data['RefreshInterval'] = refreshInterval;
    data['Actions'] = actions.map((v) => v.toJson()).toList();
    return data;
  }
}

class QueryIconInfo {
  final WoxImage icon;
  final Function()? action;

  QueryIconInfo({
    required this.icon,
    this.action,
  });

  static QueryIconInfo empty() {
    return QueryIconInfo(icon: WoxImage.empty());
  }
}

class UpdateableResult {
  late String id;
  late String? title;

  UpdateableResult({required this.id, this.title});

  UpdateableResult.fromJson(Map<String, dynamic> json) {
    id = json['Id'];
    title = json['Title'];
  }
}
