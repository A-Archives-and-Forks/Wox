import 'package:flutter/material.dart';
import 'package:wox/entity/wox_preview_ai_stream.dart';
import 'package:wox/entity/wox_theme.dart';
import 'package:wox/utils/color_util.dart';

class WoxAIStreamPreviewView extends StatefulWidget {
  final WoxPreviewAIStream data;
  final WoxTheme woxTheme;

  const WoxAIStreamPreviewView({super.key, required this.data, required this.woxTheme});

  @override
  State<WoxAIStreamPreviewView> createState() => _WoxAIStreamPreviewViewState();
}

class _WoxAIStreamPreviewViewState extends State<WoxAIStreamPreviewView> {
  late bool isReasoningExpanded;
  late bool hasAutoCollapsedReasoning;

  @override
  void initState() {
    super.initState();
    isReasoningExpanded = _shouldAutoExpandReasoning(widget.data);
    hasAutoCollapsedReasoning = _shouldAutoCollapseReasoning(widget.data);
  }

  @override
  void didUpdateWidget(covariant WoxAIStreamPreviewView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.data.reasoning.isEmpty && widget.data.reasoning.isNotEmpty && _shouldAutoExpandReasoning(widget.data)) {
      setState(() {
        isReasoningExpanded = true;
      });
    }
    if (!hasAutoCollapsedReasoning && isReasoningExpanded && _shouldAutoCollapseReasoning(widget.data)) {
      // Reasoning is useful while the model is thinking, but once answer tokens
      // start or the stream finishes it should stop competing with the final
      // answer. Collapse only once so a user's manual re-open is respected.
      setState(() {
        isReasoningExpanded = false;
        hasAutoCollapsedReasoning = true;
      });
    }
  }

  bool _shouldAutoExpandReasoning(WoxPreviewAIStream data) {
    return data.reasoning.isNotEmpty && !_shouldAutoCollapseReasoning(data);
  }

  bool _shouldAutoCollapseReasoning(WoxPreviewAIStream data) {
    return data.reasoning.isNotEmpty && (data.answer.trim().isNotEmpty || data.status == "finished");
  }

  @override
  Widget build(BuildContext context) {
    final textColor = safeFromCssColor(widget.woxTheme.previewFontColor);
    final splitLineColor = safeFromCssColor(widget.woxTheme.previewSplitLineColor);
    final propertyColor = safeFromCssColor(widget.woxTheme.previewPropertyContentColor);
    final surfaceColor = textColor.withValues(alpha: 0.035);
    final borderColor = textColor.withValues(alpha: 0.14);
    final bodyColor = textColor.withValues(alpha: 0.86);
    final answerText = widget.data.answer.trim();
    final reasoningText = widget.data.reasoning.trim();

    // AI stream previews reuse the text-preview reading surface so AI output,
    // clipboard text, and selection text share one visual language. Reasoning is
    // kept inside the same frame as lower-priority context instead of becoming a
    // separate card that competes with the final answer.
    return Container(
      width: double.infinity,
      constraints: const BoxConstraints(minHeight: 180),
      decoration: BoxDecoration(color: surfaceColor, borderRadius: BorderRadius.circular(8), border: Border.all(color: borderColor)),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (reasoningText.isNotEmpty) ...[
              _ReasoningSection(
                title: widget.data.reasoningTitle,
                statusLabel: widget.data.statusLabel,
                reasoning: reasoningText,
                isExpanded: isReasoningExpanded,
                woxTheme: widget.woxTheme,
                onToggle: () {
                  setState(() {
                    isReasoningExpanded = !isReasoningExpanded;
                  });
                },
              ),
              Padding(padding: const EdgeInsets.symmetric(vertical: 18), child: Divider(height: 1, color: splitLineColor.withValues(alpha: 0.28))),
            ],
            if (widget.data.answerTitle.isNotEmpty && reasoningText.isNotEmpty) ...[
              Text(widget.data.answerTitle, style: TextStyle(color: propertyColor.withValues(alpha: 0.72), fontSize: 12, height: 1.2, fontWeight: FontWeight.w700)),
              const SizedBox(height: 10),
            ],
            answerText.isEmpty
                ? _WaitingForAnswer(statusLabel: widget.data.statusLabel, woxTheme: widget.woxTheme)
                : SelectableText(answerText, style: TextStyle(color: bodyColor, fontSize: 16, height: 1.52, fontWeight: FontWeight.w400, letterSpacing: 0)),
          ],
        ),
      ),
    );
  }
}

class _ReasoningSection extends StatelessWidget {
  final String title;
  final String statusLabel;
  final String reasoning;
  final bool isExpanded;
  final WoxTheme woxTheme;
  final VoidCallback onToggle;

  const _ReasoningSection({required this.title, required this.statusLabel, required this.reasoning, required this.isExpanded, required this.woxTheme, required this.onToggle});

  @override
  Widget build(BuildContext context) {
    final fontColor = safeFromCssColor(woxTheme.previewFontColor);
    final splitLineColor = safeFromCssColor(woxTheme.previewSplitLineColor);
    final propertyColor = safeFromCssColor(woxTheme.previewPropertyContentColor);

    return Container(
      decoration: BoxDecoration(
        color: fontColor.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: splitLineColor.withValues(alpha: 0.34)),
      ),
      child: IntrinsicHeight(
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Container(width: 3, decoration: BoxDecoration(color: propertyColor.withValues(alpha: 0.52), borderRadius: const BorderRadius.horizontal(left: Radius.circular(8)))),
            Expanded(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(12, 10, 12, 12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    InkWell(
                      onTap: onToggle,
                      borderRadius: BorderRadius.circular(6),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(vertical: 2),
                        child: Row(
                          children: [
                            Icon(isExpanded ? Icons.keyboard_arrow_down : Icons.keyboard_arrow_right, color: fontColor.withValues(alpha: 0.52), size: 18),
                            const SizedBox(width: 4),
                            Expanded(
                              child: Text(
                                title,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: TextStyle(color: fontColor.withValues(alpha: 0.68), fontSize: 12.5, height: 1.2, fontWeight: FontWeight.w700),
                              ),
                            ),
                            if (statusLabel.isNotEmpty) _StatusPill(label: statusLabel, woxTheme: woxTheme),
                          ],
                        ),
                      ),
                    ),
                    const SizedBox(height: 8),
                    SelectableText(
                      reasoning,
                      maxLines: isExpanded ? null : 2,
                      style: TextStyle(color: fontColor.withValues(alpha: 0.58), fontSize: 13, height: 1.42, fontWeight: FontWeight.w400, letterSpacing: 0),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _StatusPill extends StatelessWidget {
  final String label;
  final WoxTheme woxTheme;

  const _StatusPill({required this.label, required this.woxTheme});

  @override
  Widget build(BuildContext context) {
    final fontColor = safeFromCssColor(woxTheme.previewFontColor);
    final splitLineColor = safeFromCssColor(woxTheme.previewSplitLineColor);

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: fontColor.withValues(alpha: 0.035),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: splitLineColor.withValues(alpha: 0.42)),
      ),
      child: Text(
        label,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(color: fontColor.withValues(alpha: 0.58), fontSize: 11, height: 1.1, fontWeight: FontWeight.w700),
      ),
    );
  }
}

class _WaitingForAnswer extends StatelessWidget {
  final String statusLabel;
  final WoxTheme woxTheme;

  const _WaitingForAnswer({required this.statusLabel, required this.woxTheme});

  @override
  Widget build(BuildContext context) {
    final fontColor = safeFromCssColor(woxTheme.previewFontColor);

    return Row(
      children: [
        SizedBox(
          width: 20,
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [_Dot(color: fontColor.withValues(alpha: 0.32)), _Dot(color: fontColor.withValues(alpha: 0.24)), _Dot(color: fontColor.withValues(alpha: 0.16))],
          ),
        ),
        const SizedBox(width: 9),
        Flexible(child: Text(statusLabel, style: TextStyle(color: fontColor.withValues(alpha: 0.54), fontSize: 14, height: 1.3, fontWeight: FontWeight.w500))),
      ],
    );
  }
}

class _Dot extends StatelessWidget {
  final Color color;

  const _Dot({required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(width: 4, height: 4, decoration: BoxDecoration(color: color, borderRadius: BorderRadius.circular(999)));
  }
}
