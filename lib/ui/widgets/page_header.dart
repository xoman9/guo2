import 'package:flutter/material.dart';

class PageHeader extends StatelessWidget {
  const PageHeader({super.key, required this.title, this.actions = const [], this.onTitleTap});
  final String title;
  final List<Widget> actions;
  final void Function(BuildContext anchor)? onTitleTap;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.fromLTRB(20, 0, 8, 0),
    child: SizedBox(
      height: 64,
      child: Row(
        children: [
          Expanded(
            child: Builder(
              builder: (anchor) => Semantics(
                header: true,
                button: onTitleTap != null,
                child: InkWell(
                  borderRadius: BorderRadius.circular(12),
                  onTap: onTitleTap == null ? null : () => onTitleTap!(anchor),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Flexible(
                        child: Text(
                          title,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                            fontSize: 28,
                            fontWeight: FontWeight.w800,
                          ),
                        ),
                      ),
                      if (onTitleTap != null) ...[
                        const SizedBox(width: 4),
                        const Icon(Icons.keyboard_arrow_down_rounded, size: 24),
                      ],
                    ],
                  ),
                ),
              ),
            ),
          ),
          ...actions,
        ],
      ),
    ),
  );
}
