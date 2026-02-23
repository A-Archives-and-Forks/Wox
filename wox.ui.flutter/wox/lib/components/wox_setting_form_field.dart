import 'package:flutter/material.dart';
import 'package:wox/utils/colors.dart';

class WoxSettingFormField extends StatelessWidget {
  final String label;
  final Widget child;
  final Widget? tips;
  final double labelWidth;
  final double labelGap;
  final double bottomSpacing;
  final double tipsTopSpacing;
  final CrossAxisAlignment rowCrossAxisAlignment;

  const WoxSettingFormField({
    super.key,
    required this.label,
    required this.child,
    this.tips,
    this.labelWidth = 160,
    this.labelGap = 20,
    this.bottomSpacing = 20,
    this.tipsTopSpacing = 2,
    this.rowCrossAxisAlignment = CrossAxisAlignment.center,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.only(bottom: bottomSpacing),
      child: Column(
        children: [
          Row(
            crossAxisAlignment: rowCrossAxisAlignment,
            children: [
              Padding(
                padding: EdgeInsets.only(right: labelGap),
                child: SizedBox(
                  width: labelWidth,
                  child: Text(label, textAlign: TextAlign.right, style: TextStyle(color: getThemeTextColor(), fontSize: 13), overflow: TextOverflow.ellipsis),
                ),
              ),
              Flexible(child: Align(alignment: Alignment.centerLeft, child: child)),
            ],
          ),
          if (tips != null)
            Padding(
              padding: EdgeInsets.only(top: tipsTopSpacing),
              child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [SizedBox(width: labelWidth + labelGap), Flexible(child: tips!)]),
            ),
        ],
      ),
    );
  }
}
