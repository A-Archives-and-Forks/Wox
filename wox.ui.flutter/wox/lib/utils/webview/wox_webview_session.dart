import 'package:flutter/widgets.dart';

enum WoxWebViewSessionAction { toggleActionPanel, focusQueryBox }

abstract class WoxWebViewSession {
  bool get isCached;

  String? get cacheKey;

  Stream<WoxWebViewSessionAction> get actions;

  Widget buildWidget();

  Future<void> dispose();
}
