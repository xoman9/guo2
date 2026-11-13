#!/usr/bin/env python3
"""Rebuild guo2 from the latest stable upstream Release and replay custom changes.

The original custom branch is only read, never changed. Replay is path-by-path
without Git merges. Custom-owned file overlays and deterministic edits are
validated by CI before a generated branch may be promoted.
"""
from __future__ import annotations

import json
import os
import pathlib
import re
import subprocess
import sys


def git(*args: str, check: bool = True, capture: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(["git", *args], check=check, capture_output=capture, text=False)


def run(*args: str) -> str:
    return git(*args).stdout.decode("utf-8", "replace").strip()


def main() -> int:
    gh_token = os.environ.get("GH_TOKEN")
    if not gh_token:
        raise SystemExit("GH_TOKEN is required to read the upstream Release")
    upstream = "https://github.com/Xanerix/guoapp.git"
    release = subprocess.run(
        ["gh", "api", "repos/Xanerix/guoapp/releases/latest"],
        check=True, capture_output=True, text=True
    )
    metadata = json.loads(release.stdout)
    tag = metadata["tag_name"]
    if metadata.get("draft") or metadata.get("prerelease"):
        raise SystemExit(f"Refusing non-stable Release: {tag}")
    if not re.fullmatch(r"[A-Za-z0-9._-]{1,90}", tag):
        raise SystemExit(f"Unexpected release tag: {tag!r}")

    dry_run = os.environ.get("SYNC_DRY_RUN") == "1"
    target_branch = "sync/release-" + tag
    git("config", "user.name", "guo2 release-sync bot")
    git("config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com")
    if subprocess.run(["git", "remote", "get-url", "upstream"], stdout=subprocess.DEVNULL,
                      stderr=subprocess.DEVNULL).returncode != 0:
        git("remote", "add", "upstream", upstream)
    git("fetch", "--no-tags", "origin", "+refs/heads/custom:refs/remotes/origin/custom")
    git("fetch", "upstream", "+refs/tags/" + tag + ":refs/tags/" + tag)

    release_sha = run("rev-parse", tag + "^{commit}")
    custom_sha = run("rev-parse", "origin/custom")
    base_sha = run("merge-base", release_sha, custom_sha)
    paths_raw = git("diff", "--name-only", "-z", base_sha, custom_sha).stdout
    paths = [x.decode("utf-8", "surrogateescape") for x in paths_raw.split(b"\x00") if x]

    git("checkout", "--detach", release_sha)
    # This namespace is generated solely by this script. Never force the custom branch.
    git("checkout", "-B", target_branch)

    def deterministic_override(path: str) -> str | None:
        """Apply reviewed conflict rules; never replace core files blindly."""
        file = pathlib.Path(path)
        if path == "lib/ui/app.dart":
            # Only remove the old terms gate; keep all new upstream UI features
            # such as pure-black themes and new navigation behavior.
            text = file.read_text(encoding="utf-8")
            old_import = "import 'terms_page.dart';\n"
            old_gate = "        LocalStore(acceptedTerms: < termsVersion) => TermsPage(store: store),\n"
            if text.count(old_import) != 1 or text.count(old_gate) != 1:
                return None
            text = text.replace(old_import, "", 1).replace(old_gate, "", 1)
            file.write_text(text, encoding="utf-8")
            return "remove terms gate without overwriting upstream app UI"
        if path == "test/local_store_access_test.dart":
            # Preserve new upstream test fixtures and only adapt build naming.
            text = file.read_text(encoding="utf-8")
            expected = "expect(appName, allSourcesEnabled ? '真果鉴' : '红果鉴');"
            replacement = (
                "expect(appName, allSourcesEnabled\n"
                "    ? const String.fromEnvironment('ZHENGUOJIAN_DISPLAY_NAME',\n"
                "        defaultValue: '小果影視-共存版')\n"
                "    : '红果鉴');"
            )
            if text.count(expected) != 1:
                return None
            file.write_text(text.replace(expected, replacement, 1), encoding="utf-8")
            return "preserve upstream test fixtures with custom coexist build name"
        if path == "README.md":
            # Do not replace current upstream documentation with a stale copy.
            text = file.read_text(encoding="utf-8")
            heading = "## guo2 共存版客製說明"
            if heading in text:
                return None
            text += (
                "\n" + heading + "\n\n"
                "本分支使用最新正式上游 Release 為基底，按介面、下載、合併、"
                "站源、設定、共存版順序套用客製修改；不以 Git 三方合併覆蓋上游。\n"
                "Android 共存版使用 com.duanju.coexist，簽章及 ARM64/v7a "
                "構建結果以 GitHub Actions 實際驗證為準。\n"
            )
            file.write_text(text, encoding="utf-8")
            return "append coexist notes without reverting upstream documentation"
        if path == "scripts/app_build.py":
            text = file.read_text(encoding="utf-8")
            before = "EXAMPLE_SOURCES = NATIVE_ROOT / 'sources_example'"
            default = "paths = [EXAMPLE_SOURCES]"
            if text.count(before) != 1 or text.count(default) != 1:
                return None
            text = text.replace(before, "CUSTOM_SOURCES = NATIVE_ROOT / 'sources_custom'", 1)
            text = text.replace(default, "paths = [CUSTOM_SOURCES]", 1)
            file.write_text(text, encoding="utf-8")
            return "connect generated real custom source module instead of upstream example"
        if path == "android/app/build.gradle.kts":
            text = file.read_text(encoding="utf-8")
            if '?: "真果鉴"' not in text or '?: "com.guoapp.all"' not in text:
                return None
            text = text.replace('?: "真果鉴"', '?: "小果影視-共存版"', 1)
            text = text.replace('?: "com.guoapp.all"', '?: "com.duanju.coexist"', 1)
            file.write_text(text, encoding="utf-8")
            return "coexist branding and application ID"
        if path == "native/core/app_categories.go":
            text = file.read_text(encoding="utf-8")
            if "func ValidNativeCategory(" not in text or ".Categories(ctx, force)" not in text:
                return None
            return "retain upstream Registry category dispatch; custom site validators live in provider adapters"
        if path == "native/core/app_runtime.go":
            text = file.read_text(encoding="utf-8")
            if "func (engine *nativeEngine) nativeDetail(" not in text or "p.Detail(ctx, sourceID)" not in text:
                return None
            if "func NativeNormalize(" not in text:
                return None
            return "retain upstream Registry runtime; custom detail and search use registered providers"
        if path == "native/core/provider_media.go":
            text = file.read_text(encoding="utf-8")
            if "func (d *Downloader) resolveProviderMedia(" not in text or "p.ResolveMedia(ctx, task)" not in text:
                return None
            return "retain upstream media pipeline; custom playback uses Provider.ResolveMedia"
        if path == "lib/models.dart":
            text = file.read_text(encoding="utf-8")
            if "static void install(Iterable<SourceSite> sites)" not in text:
                return None
            if "static List<SourceSite> get allValues" not in text:
                return None
            # All custom source entries are registered by generated Go modules.
            # Retain the new upstream dynamic source model and season metadata.
            return "retain upstream dynamic SourceSite model; custom sources come from Registry"
        if path == "lib/app_build.dart":
            file.write_text(
                "const _allSourcesDefineEnabled = bool.fromEnvironment('ALL_SOURCES');\n"
                "const configuredAppFlavor = String.fromEnvironment('APP_FLAVOR');\n"
                "const appFlavor = configuredAppFlavor != ''\n"
                "    ? configuredAppFlavor\n"
                "    : (_allSourcesDefineEnabled ? 'zhenguojian' : 'hongguojian');\n"
                "const appName = appFlavor == 'hongguojian'\n"
                "    ? '红果鉴'\n"
                "    : String.fromEnvironment('ZHENGUOJIAN_DISPLAY_NAME',\n"
                "        defaultValue: '小果影視-共存版');\n"
                "const appSlug = appFlavor;\n",
                encoding="utf-8"
            )
            return "APP_FLAVOR and ALL_SOURCES interoperability"
        if path == "lib/ui/widgets/source_bar.dart":
            # Preserve the tested title-based channel switcher and include
            # upstream's new leading widgets on phone, desktop and TV.
            source = git("show", "origin/custom:" + path).stdout.decode("utf-8")
            substitutions = [
                ("    this.televisionLeading,\n    this.actions = const [],",
                 "    this.televisionLeading,\n    this.leading = const [],\n    this.actions = const [],"),
                ("  final Widget? televisionLeading;\n  final List<Widget> actions;",
                 "  final Widget? televisionLeading;\n  final List<Widget> leading;\n  final List<Widget> actions;"),
                ("actions: [search, ...actions],", "actions: [...leading, search, ...actions],"),
                ("actions: [\n          SizedBox(", "actions: [\n          ...leading,\n          SizedBox("),
                ("children: [\n            ?televisionLeading,\n            const Spacer(),",
                 "children: [\n            ?televisionLeading,\n            ...leading,\n            const Spacer(),"),
            ]
            for old, new in substitutions:
                if old not in source:
                    return None
                source = source.replace(old, new, 1)
            file.write_text(source, encoding="utf-8")
            return "source-title selector, upstream leading widgets and TV switcher"
        if path == "native/core/app_storage.go":
            text = file.read_text(encoding="utf-8")
            changes = [
                ('target = filepath.Join(filepath.Clean(resolved), "zhenguojian-downloads")',
                 'target = filepath.Clean(resolved)'),
                ('''if _, err := os.Stat(target); err == nil {
		manager.mu.Unlock()
		return "", "", errors.New("目标中已有应用下载目录，请选择空目录，避免覆盖文件")
	} else if !os.IsNotExist(err) {
		manager.mu.Unlock()
		return "", "", errors.New("无法检查目标目录：" + err.Error())
	}''',
                 '''targetExists := false
	if entries, err := os.ReadDir(target); err == nil {
		targetExists = true
		if len(entries) != 0 {
			manager.mu.Unlock()
			return "", "", errors.New("所選資料夾不是空的，請選擇空資料夾避免覆蓋")
		}
	} else if !os.IsNotExist(err) {
		manager.mu.Unlock()
		return "", "", errors.New("無法讀取所選資料夾，請檢查權限")
	}'''),
                ('''if err := os.Mkdir(target, 0700); err != nil {
		return "", "", errors.New("无法创建目标目录，请检查权限或选择其他位置")
	}''',
                 '''if !targetExists {
		if err := os.Mkdir(target, 0700); err != nil {
			return "", "", errors.New("無法建立目標資料夾，請檢查權限")
		}
	}
	cleanupNewTarget := func() {
		if !targetExists {
			_ = os.RemoveAll(target)
		}
	}'''),
            ]
            for old, new in changes:
                if text.count(old) != 1:
                    return None
                text = text.replace(old, new, 1)
            start = text.index("func (engine *nativeEngine) moveDownloads(")
            end = text.index("func nativePathsOverlap", start)
            section = text[start:end]
            if section.count("_ = os.RemoveAll(target)") != 4:
                return None
            first = section.index("_ = os.RemoveAll(target)") + len("_ = os.RemoveAll(target)")
            section = section[:first] + section[first:].replace("_ = os.RemoveAll(target)", "cleanupNewTarget()")
            text = text[:start] + section + text[end:]
            file.write_text(text, encoding="utf-8")
            return "selected directory itself, empty-folder safety, upstream cleanup return"
        if path == "lib/ui/settings/storage_view.dart":
            text = git("show", "origin/custom:" + path).stdout.decode("utf-8")
            old = "      await repository.moveDownloads(parent);"
            if text.count(old) != 1:
                return None
            text = text.replace(old, "      leftover = await repository.moveDownloads(parent);", 1)
            a = "    String? failureMessage;\n    try {"
            if text.count(a) != 1:
                return None
            text = text.replace(a, "    String? failureMessage;\n    var leftover = (path: '', reason: '');\n    try {", 1)
            pos = text.index("  Future<bool> _migrate(")
            end = text.index("  @override\n  Widget build", pos)
            body = text[pos:end]
            marker = "    return true;\n  }\n"
            if body.count(marker) != 1:
                return None
            warning = """
    if (leftover.path.isNotEmpty && mounted) {
      await showDialog<void>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('舊下載目錄仍有檔案'),
          content: SelectableText(
            '新目錄遷移完成，但舊目錄清理失敗。\\n'
            '${leftover.path}\\n${leftover.reason}\\n'
            '請確認後手動清理。',
          ),
          actions: [
            FilledButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('知道了'),
            ),
          ],
        ),
      );
    }
"""
            body = body.replace(marker, warning + marker, 1)
            text = text[:pos] + body + text[end:]
            file.write_text(text, encoding="utf-8")
            return "Android storage authorization and merge-location with cleanup warning"
        if path == "lib/ui/terms_page.dart":
            if file.exists():
                git("rm", "-f", "--", path)
            return "remove terms page"
        if path in {"native/core/app_storage_test.go", "native/core/provider_huangguo.go"} and not file.exists():
            return "retain upstream deletion of old implementation"
        return None

    def stage_if_present(path: str) -> None:
        """Stage an existing or tracked path; an already-absent path needs no staging."""
        file = pathlib.Path(path)
        tracked = subprocess.run(
            ["git", "ls-files", "--error-unmatch", "--", path],
            capture_output=True,
        ).returncode == 0
        if file.exists() or tracked:
            git("add", "-A", "--", path)

    def restore_rule_path(path: str) -> None:
        """Revert a failed partial edit, including untracked generated files."""
        tracked = subprocess.run(
            ["git", "ls-files", "--error-unmatch", "--", path],
            capture_output=True,
        ).returncode == 0
        if tracked:
            git("restore", "--source=HEAD", "--staged", "--worktree", "--", path)
        else:
            candidate = pathlib.Path(path)
            if candidate.is_file() or candidate.is_symlink():
                candidate.unlink()

    # Known customization rules are required. On future upstream changes,
    # do not silently copy an old core/UI file if its reviewed rule fails.
    rule_paths = {
        "android/app/build.gradle.kts",
        "lib/app_build.dart",
        "scripts/app_build.py",
        "lib/models.dart",
        "lib/ui/app.dart",
        "README.md",
        "test/local_store_access_test.dart",
        "lib/ui/settings/storage_view.dart",
        "lib/ui/terms_page.dart",
        "lib/ui/widgets/source_bar.dart",
        "native/core/app_categories.go",
        "native/core/app_runtime.go",
        "native/core/app_storage.go",
        "native/core/app_storage_test.go",
        "native/core/provider_huangguo.go",
        "native/core/provider_media.go",
    }
    # Only the previously reviewed standalone custom files may be overlaid.
    # All newly changed paths must be reviewed before inclusion.
    overlay_paths = {
        ".github/workflows/build-custom.yml",
        "android/app/src/main/AndroidManifest.xml",
        "android/app/src/main/kotlin/com/guoapp/MainActivity.kt",
        "lib/merge_location.dart",
        "lib/merge_queue.dart",
        "lib/ui/settings/sources_view.dart",
        "lib/ui/widgets/page_header.dart",
        "native/core/provider_apk391_categories.go",
        "native/core/provider_apk391_chengguo.go",
        "native/core/provider_apk391_maccms.go",
        "native/core/provider_apk391_registry.go",
        "native/core/provider_liuguo.go",
        "native/core/provider_lujj31.go",
        "native/core/provider_lujj31_player.go",
        "native/core/provider_lujj31_title_test.go",
        "native/core/provider_xiaoguo.go",
    }
    # Preserve the agreed customization order from the earlier guo2 workflow.
    # A phase is considered complete only after its owned paths are processed.
    customization_phases = {
        "01 UI": [
            "lib/ui/app.dart", "lib/ui/widgets/source_bar.dart",
            "lib/ui/widgets/page_header.dart", "lib/models.dart",
        ],
        "02 Downloads": [
            "native/core/app_storage.go", "native/core/app_storage_test.go",
        ],
        "03 Merge": [
            "lib/merge_location.dart", "lib/merge_queue.dart",
        ],
        "04 Providers": [
            "native/core/app_categories.go", "native/core/app_runtime.go",
            "native/core/provider_media.go", "native/core/provider_huangguo.go",
            "native/core/provider_apk391_categories.go",
            "native/core/provider_apk391_chengguo.go",
            "native/core/provider_apk391_maccms.go",
            "native/core/provider_apk391_registry.go",
            "native/core/provider_liuguo.go", "native/core/provider_lujj31.go",
            "native/core/provider_lujj31_player.go",
            "native/core/provider_lujj31_title_test.go",
            "native/core/provider_xiaoguo.go",
            "lib/ui/settings/sources_view.dart",
        ],
        "05 Settings": [
            "lib/ui/settings/storage_view.dart", "lib/ui/terms_page.dart",
        ],
        "06 Coexist": [
            "android/app/build.gradle.kts",
            "android/app/src/main/AndroidManifest.xml",
            "android/app/src/main/kotlin/com/guoapp/MainActivity.kt",
            "lib/app_build.dart", "scripts/app_build.py",
            "test/local_store_access_test.dart",
            ".github/workflows/build-custom.yml", "README.md",
        ],
    }
    phase_for_path: dict[str, str] = {}
    for phase, phase_paths in customization_phases.items():
        for entry in phase_paths:
            if entry in phase_for_path:
                raise SystemExit("A customization path belongs to multiple phases: " + entry)
            phase_for_path[entry] = phase
    if set(phase_for_path) != rule_paths | overlay_paths:
        raise SystemExit("Phase manifest does not match reviewed custom paths")
    phase_results: dict[str, list[str]] = {phase: [] for phase in customization_phases}
    # Only apply historical paths present in the reference custom change set.
    # Unexpected additions are reported, not silently copied into latest.
    ordered_paths = [
        path for phase_paths in customization_phases.values()
        for path in phase_paths if path in paths
    ]
    applied: list[str] = []
    overrides: list[dict[str, str]] = []
    failures: list[dict[str, str]] = []
    unexpected = sorted(set(paths) - rule_paths - overlay_paths)
    for path in unexpected:
        failures.append({"path": path, "reason": "New customization path; user review required."})
    for path in ordered_paths:
        phase = phase_for_path[path]
        if path in rule_paths:
            try:
                rule = deterministic_override(path)
            except (ValueError, IndexError, OSError, subprocess.CalledProcessError) as exc:
                # A rule may have partially edited its file before failing.
                # Restore the upstream file, never commit a partial edit.
                restore_rule_path(path)
                failures.append({"path": path, "reason": f"Rule incompatible with upstream: {exc}"})
                continue
            if rule is None:
                restore_rule_path(path)
                failures.append({"path": path, "reason": "Expected customization rule did not match upstream code."})
                continue
            stage_if_present(path)
            git("commit", "-m", "customize(rebuild): " + path, "--allow-empty")
            overrides.append({"path": path, "rule": rule})
            phase_results[phase].append(path)
            continue
        if path not in overlay_paths:
            continue
        # A verbatim overlay is safe only when upstream did not change that
        # tracked file since the historical reference. Otherwise we must port
        # the custom behavior, never silently replace new Release features.
        upstream_changed = subprocess.run(
            ["git", "diff", "--quiet", base_sha, release_sha, "--", path],
            capture_output=True,
        ).returncode != 0
        if upstream_changed:
            failures.append({
                "path": path,
                "reason": "Both the upstream Release and old custom changed this file; automatic legacy overwrite is unsafe.",
            })
            continue
        file = pathlib.Path(path)
        original = subprocess.run(
            ["git", "show", "origin/custom:" + path],
            capture_output=True,
        )
        if original.returncode == 0:
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_bytes(original.stdout)
        else:
            existed = subprocess.run(
                ["git", "cat-file", "-e", base_sha + ":" + path],
                capture_output=True,
            ).returncode == 0
            if not existed:
                failures.append({"path": path, "reason": "Legacy file unavailable; user review required."})
                continue
            if file.exists():
                git("rm", "-f", "--", path)
        stage_if_present(path)
        git("commit", "-m", "customize(rebuild): " + path, "--allow-empty")
        applied.append(path)
        phase_results[phase].append(path)

    if failures:
        report_lines = [
            "# guo2 Release customization: decisions required",
            "",
            f"- Upstream Release: `{tag}`",
            f"- Failed or unreviewed customization paths: {len(failures)}",
            "- Customization order: UI → Downloads → Merge → Providers → Settings → Coexist.",
            "- No merge conflict; these are customization compatibility questions.",
            "- No automatic fallback to old upstream core/UI files.",
            f"- Dry run (no remote writes): {dry_run}.",
            "",
            "## Please decide how to modify these items",
        ]
        for n, item in enumerate(failures, 1):
            report_lines.extend([
                f"{n}. `{item['path']}` ({phase_for_path.get(item['path'], 'unclassified')})",
                f"   - Reason: {item['reason']}",
                "   - Decision required: adapt the custom modification to the new upstream structure, skip this customization, or provide a new rule.",
            ])
        report_lines.extend(["", "Reply with your choices for the numbered items before continuing."])
        report_text = "\n".join(report_lines) + "\n"
        report_path = pathlib.Path("RELEASE_SYNC_REPORT.md")
        report_path.write_text(report_text, encoding="utf-8")
        git("add", "--", str(report_path))
        git("commit", "-m", "docs(sync): list customization decisions required")
        if not dry_run:
            git("push", "--force-with-lease", "origin", "HEAD:refs/heads/" + target_branch)
        if out_file := os.environ.get("SYNC_REPORT"):
            pathlib.Path(out_file).write_text(report_text, encoding="utf-8")
        if out := os.environ.get("GITHUB_OUTPUT"):
            with open(out, "a", encoding="utf-8") as stream:
                stream.write(f"branch={target_branch}\n")
                stream.write(f"release={tag}\n")
                stream.write(f"failures={len(failures)}\n")
        if summary := os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(summary, "a", encoding="utf-8") as stream:
                stream.write(report_text)
        print(report_text)
        return 2

    # The upstream 1.0.9 core exported these previously private types.
    # Preserve source modules without reverting any upstream implementation.
    compat = pathlib.Path("native/core/custom_compat_aliases.go")
    upstream_categories = pathlib.Path("native/core/app_categories.go").read_text(encoding="utf-8")
    upstream_media = pathlib.Path("native/core/provider_media.go").read_text(encoding="utf-8")
    if "type Category struct" not in upstream_categories or "type ProviderMedia struct" not in upstream_media:
        raise SystemExit("Upstream API changed: review Category / ProviderMedia aliases")
    compat.write_text(
        "package core\n\n"
        "// Compatibility aliases for site adapters carried forward from custom.\n"
        "// Original upstream Category and ProviderMedia implementations are unchanged.\n"
        "type nativeCategory = Category\n"
        "type providerMedia = ProviderMedia\n"
        "type nativeDrama = LibraryDrama\n"
        "const (\n"
        " sourceLujj31 = \"lujj31\"\n"
        " sourceLiuguo = \"liuguo\"\n"
        " sourceXiaoguo = \"xiaoguo\"\n"
        ")\n"
        "var webProviderNumericID = WebProviderNumericID\n"
        "var webProviderInteger = WebProviderInteger\n"
        "var providerHTMLAttr = ProviderHTMLAttr\n"
        "var providerHTMLClass = ProviderHTMLClass\n"
        "var providerHTMLNodes = ProviderHTMLNodes\n"
        "var providerHTMLFirstClass = ProviderHTMLFirstClass\n"
        "var providerHTMLText = ProviderHTMLText\n"
        "var nativeText = NativeText\n"
        "var nativeNormalize = NativeNormalize\n"
        "var mergeNativeDrama = MergeNativeDrama\n"
        "var nativeDramaScore = NativeDramaScore\n"
        "var sortProviderChapters = SortProviderChapters\n"
        "var chapterEpisodeNumber = ChapterEpisodeNumber\n"
        "var rawEpisode = RawEpisode\n"
        "var episodeIndex = EpisodeIndex\n"
        "var extractPageTitle = ExtractPageTitle\n"
        "var extractDescription = ExtractDescription\n"
        "var extractAttr = ExtractAttr\n"
        "var extractTags = ExtractTags\n"
        "var cleanText = CleanText\n"
        "var firstNonEmpty = FirstNonEmpty\n"
        "var mapString = MapString\n"
        "var mapStringSlice = MapStringSlice\n"
        "var resolveProviderURL = ResolveProviderURL\n"
        "var normalizeProviderMediaURL = NormalizeProviderMediaURL\n"
        "var isProviderHTTPMediaURL = IsProviderHTTPMediaURL\n"
        "var providerReleaseDate = ProviderReleaseDate\n"
        "var providerChinaTime = ProviderChinaTime\n"
        "var providerSourceForURL = ProviderSourceForURL\n"
        "var nativeSearchText = NativeSearchText\n"
        "var truncate = Truncate\n"
        "var providerCoverAddress = ProviderCoverAddress\n"
        "var validNativeCoverURL = ValidNativeCoverURL\n"
        "var providerDramaID = ProviderDramaID\n"
        "var providerChapterID = ProviderChapterID\n"
        "var splitProviderDramaID = SplitProviderDramaID\n",
        encoding="utf-8"
    )
    git("add", "--", str(compat))
    git("commit", "-m", "compat(sync): bridge legacy source type names to upstream public API")

    # Register retained custom site modules into upstream's dynamic Provider API.
    # Without this step a successful compile would still omit the custom sources.
    custom_providers = pathlib.Path("native/core/custom_registry_sources.go")
    custom_providers.write_text("""package core

import "context"

// Registered custom sources use the official upstream Provider Registry.
// Their existing catalog/detail/media parsers remain the source of truth.
type customSiteProvider struct {
 d *Downloader
 descriptor SourceDescriptor
 categories func(context.Context) ([]Category, error)
 catalog func(context.Context, int, string, string) ([]Drama, bool, error)
 detail func(context.Context, string) (Drama, []Chapter, error)
 media func(context.Context, Task) (ProviderMedia, error)
}

func (p *customSiteProvider) Descriptor() SourceDescriptor { return p.descriptor }
func (p *customSiteProvider) Categories(ctx context.Context, force bool) ([]Category, error) {
 return p.categories(ctx)
}
func (p *customSiteProvider) CatalogPage(ctx context.Context, page int, category string) ([]Drama, bool, error) {
 return p.catalog(ctx, page, category, "")
}
func (p *customSiteProvider) Search(ctx context.Context, page int, query string) ([]Drama, bool, error) {
 return p.catalog(ctx, page, "", query)
}
func (p *customSiteProvider) Detail(ctx context.Context, id string) (Drama, []Chapter, error) {
 raw, chapters, err := p.detail(ctx, id)
 if err != nil { return raw, chapters, err }
 // Preserve the catalog title if a Tao detail page exposes a generic
 // playback-line label instead of the actual series title.
 if p.descriptor.ID == sourceLujj31 &&
   (!nativeLujj31CatalogTitleValid(raw.DisplayTitle(), id) ||
     raw.DisplayTitle() == "播放線路" || raw.DisplayTitle() == "播放线路") {
  raw.Title = ""
  raw.Name = ""
 }
 return raw, chapters, nil
}
func (p *customSiteProvider) ResolveMedia(ctx context.Context, task Task) (ProviderMedia, error) {
 return p.media(ctx, task)
}

func registerCustomSource(id, name string, host Host,
 categories func(context.Context) ([]Category, error),
 catalog func(context.Context, int, string, string) ([]Drama, bool, error),
 detail func(context.Context, string) (Drama, []Chapter, error),
 media func(context.Context, Task) (ProviderMedia, error)) Provider {
 d, ok := host.(*Downloader)
 if !ok { return nil }
 return &customSiteProvider{
  d: d, descriptor: SourceDescriptor{ID: id, Name: name, DefaultOff: true,
   ValidCategory: func(category string) bool { return category == "" || WebProviderNumericID.MatchString(category) }},
  categories: categories, catalog: catalog, detail: detail, media: media,
 }
}

func init() {
 Register(SourceDescriptor{ID: sourceLujj31, Name: "桃果 +", DefaultOff: true,
  ValidCategory: func(category string) bool { return category == "" || WebProviderNumericID.MatchString(category) }},
  func(host Host) Provider {
   d, ok := host.(*Downloader); if !ok { return nil }
   return registerCustomSource(sourceLujj31, "桃果 +", host, d.fetchLujj31Categories,
    d.fetchLujj31CatalogPage, d.fetchLujj31Detail, d.resolveLujj31Media)
  })
 Register(SourceDescriptor{ID: sourceLiuguo, Name: "榴果 +", DefaultOff: true,
  ValidCategory: func(category string) bool { return category == "" || WebProviderNumericID.MatchString(category) }},
  func(host Host) Provider {
   d, ok := host.(*Downloader); if !ok { return nil }
   return registerCustomSource(sourceLiuguo, "榴果 +", host, d.fetchLiuguoCategories,
    d.fetchLiuguoCatalogPage, d.fetchLiuguoDetail, d.resolveLiuguoMedia)
  })
 Register(SourceDescriptor{ID: sourceXiaoguo, Name: "宵果 +", DefaultOff: true,
  ValidCategory: validXiaoguoCategory},
  func(host Host) Provider {
   d, ok := host.(*Downloader); if !ok { return nil }
   provider := registerCustomSource(sourceXiaoguo, "宵果 +", host, d.fetchXiaoguoCategories,
    d.fetchXiaoguoCatalogPage, d.fetchXiaoguoDetail, d.resolveXiaoguoMedia)
   if p, ok := provider.(*customSiteProvider); ok {
    p.descriptor.ValidCategory = validXiaoguoCategory
   }
   return provider
  })
 // The carried-forward APK391 adapters are registered by their own IDs so
 // upstream's runtime source list can discover them dynamically.
 for _, entry := range apk391Sources {
  spec := entry
  descriptor := SourceDescriptor{
   ID: spec.ID, Name: spec.Name + " +",
   DefaultOff: true, ValidCategory: func(category string) bool {
    return apk391ValidCategory(spec.ID, category)
   },
  }
  Register(descriptor, func(host Host) Provider {
   d, ok := host.(*Downloader)
   if !ok { return nil }
   return &customSiteProvider{
    d: d, descriptor: descriptor,
    categories: func(ctx context.Context) ([]Category, error) {
     return d.fetchApk391Categories(ctx, spec.ID)
    },
    catalog: func(ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
     return d.fetchApk391CatalogPage(ctx, spec.ID, page, category, query)
    },
    detail: func(ctx context.Context, id string) (Drama, []Chapter, error) {
     return d.fetchApk391Detail(ctx, spec.ID, id)
    },
    media: d.resolveApk391Media,
   }
  })
 }
}
""", encoding="utf-8")
    git("add", "--", str(custom_providers))
    git("commit", "-m", "feat(sync): register Tao, Liu and Xiao parsers in upstream Provider Registry")

    # Keep the legacy site HTTP call signatures while forwarding to the new
    # upstream Host request pipeline (including headers, proxies and limits).
    compat_http = pathlib.Path("native/core/custom_http_compat.go")
    compat_http.write_text("""package core

import (
 "context"
 "golang.org/x/net/html"
)

func (d *Downloader) fetchProviderText(ctx context.Context, address, referer string) (string, error) {
 response, err := d.FetchText(ctx, TextRequest{URL: address, Referer: referer})
 if err != nil { return "", err }
 return response.Body, nil
}

func (d *Downloader) fetchProviderPage(ctx context.Context, address, referer, agent string) (*html.Node, string, error) {
 return FetchHostPage(ctx, d, address, referer, agent)
}
""", encoding="utf-8")
    git("add", "--", str(compat_http))
    git("commit", "-m", "compat(sync): forward legacy page requests into upstream Host")

    title_compat = pathlib.Path("native/core/custom_lujj31_title.go")
    title_compat.write_text("""package core

import "strings"

// Reject generic detail headings that used to overwrite the real Tao catalog title.
func nativeLujj31CatalogTitleValid(title, sourceID string) bool {
 title = strings.TrimSpace(title)
 if title == "" || title == "短剧" || title == "短劇" ||
  title == "影片 "+sourceID || title == "影片" || title == "桃果" ||
  title == "播放線路" || title == "播放线路" {
  return false
 }
 return true
}
""", encoding="utf-8")
    git("add", "--", str(title_compat))
    git("commit", "-m", "fix(sync): retain Tao catalog title when detail heading is generic")

    smoke = pathlib.Path("native/core/custom_registry_sources_test.go")
    smoke.write_text("""package core

import "testing"

// A successful compile must not silently drop custom channels from the
// upstream dynamic Registry. This checks registrations without accessing sites.
func TestReleaseReplayCustomChannelsRegistered(t *testing.T) {
 d := &Downloader{cfg: DefaultConfig()}
 ids := []string{sourceLujj31, sourceLiuguo, sourceXiaoguo}
 for _, spec := range apk391Sources { ids = append(ids, spec.ID) }
 seen := map[string]bool{}
 for _, id := range ids {
  if seen[id] { t.Fatalf("duplicate custom source %q", id) }
  seen[id] = true
  descriptor, ok := registeredDescriptor(id)
  if !ok || descriptor.ID != id { t.Fatalf("source %q not registered", id) }
  provider, ok := d.provider(id)
  if !ok || provider == nil || provider.Descriptor().ID != id {
   t.Fatalf("source %q has no functional provider", id)
  }
  if !nativeSourceAvailable(id) { t.Fatalf("source %q unavailable in unrestricted build", id) }
 }
 if len(ids) == 0 { t.Fatal("no guo2 added channels registered") }
}
""", encoding="utf-8")
    git("add", "--", str(smoke))
    git("commit", "-m", "test(sync): enforce custom channel registration and source availability")

    # The upstream Release moved source registration into a separate Go
    # workspace module. Never fall back to its synthetic example provider.
    source_module = pathlib.Path("native/sources_custom")
    source_module.mkdir(parents=True, exist_ok=True)
    (source_module / "go.mod").write_text(
        "module duanjuapp/native/sources_custom\n\ngo 1.24.1\n",
        encoding="utf-8"
    )
    # Derive the added source list from the earlier reviewed guo2 custom
    # implementation, rather than pinning an arbitrary channel count.
    # Never enumerate or change the original upstream Release source list.
    legacy_registry = run("show", "origin/custom:native/core/provider_apk391_registry.go")
    registry_match = re.search(
        r"var apk391Sources\s*=\s*\[\]apk391SourceSpec\s*\{(.*?)\n\}",
        legacy_registry, re.S,
    )
    if not registry_match:
        raise SystemExit("Unable to read the reviewed guo2 added-source registry")
    registry_ids = re.findall(r'\{\s*"([a-z][a-z0-9-]*)"\s*,', registry_match.group(1))
    # These three separately implemented sites were part of that same custom.
    direct_sources = [
        ("lujj31", "native/core/provider_lujj31.go"),
        ("liuguo", "native/core/provider_liuguo.go"),
        ("xiaoguo", "native/core/provider_xiaoguo.go"),
    ]
    for source_id, source_path in direct_sources:
        exists = subprocess.run(
            ["git", "cat-file", "-e", "origin/custom:" + source_path],
            capture_output=True,
        ).returncode == 0
        if not exists:
            raise SystemExit("Reviewed source missing: " + source_path)
    extra_sources = [item[0] for item in direct_sources] + registry_ids
    if not registry_ids or len(set(extra_sources)) != len(extra_sources):
        raise SystemExit("Reviewed guo2 source registry missing or contains duplicate IDs")
    source_go_ids = ",\n  ".join(json.dumps(source) for source in extra_sources)
    (source_module / "flavor_zhenguojian.go").write_text(
        "//go:build zhenguojian\n\n"
        "package custom\n\n"
        'import "duanjuapp/native/core"\n\n'
        'func init() {\n core.RegisterFlavor("zhenguojian", []string{\n  '
        + source_go_ids + ",\n })\n}\n",
        encoding="utf-8",
    )
    # Functional coverage is enforced by custom_registry_sources_test.go:
    # it checks the actual added Provider instances, not mere display names.
    git("add", "--", str(source_module))
    git("commit", "-m", "feat(sync): provide real custom source module for all-sources builds")

    report = [
        "# Upstream Release → guo2 custom replay",
        "",
        f"- Upstream Release: `{tag}` (`{release_sha}`)",
        f"- Original custom: `{custom_sha}`",
        f"- Historical reference base (no merge): `{base_sha}`",
        f"- Custom file overlays: **{len(applied)}** / {len(paths)}",
        f"- Successful customization rules: **{len(overrides)}**",
        "- Git merge conflicts: **not applicable (no merge performed)**",
        "- Source: the full official Release, not the previous migration branch.",
        "- Existing custom, main and upstream branches were not changed.",
        f"- Dry run (no remote writes): {dry_run}.",
        "",
        "## Customization phase results",
        *[f"- {phase}: {len(items)} paths processed" for phase, items in phase_results.items()],
        "",
        "## Reviewed custom-owned file overlays",
        *["- `" + p + "`" for p in applied],
        "",
        "## Deterministic customization rules",
        *[f"- {entry['path']}: {entry['rule']}" for entry in overrides],
        "",
        "## Customization issues",
        "None detected during rule application. CI verification is still required.",
        "",
        "A conflict-free rebuild does **not** guarantee build compatibility.",
        "CI verification and review are mandatory before merging into custom.",
        "",
    ]
    output = "\n".join(report)
    out_file = pathlib.Path(os.environ.get("SYNC_REPORT", "/tmp/guo2-release-sync.md"))
    out_file.parent.mkdir(parents=True, exist_ok=True)
    out_file.write_text(output, encoding="utf-8")

    # Report lives in the generated branch as well as Actions artifacts.
    report_in_branch = pathlib.Path("RELEASE_SYNC_REPORT.md")
    report_in_branch.write_text(output, encoding="utf-8")
    git("add", "RELEASE_SYNC_REPORT.md")
    git("commit", "-m", "docs(sync): record automated custom replay result")
    if not dry_run:
        git("push", "--force-with-lease", "origin", "HEAD:refs/heads/" + target_branch)

    if output_path := os.environ.get("GITHUB_OUTPUT"):
        with open(output_path, "a", encoding="utf-8") as stream:
            stream.write(f"branch={target_branch}\n")
            stream.write(f"release={tag}\n")
            stream.write("failures=0\n")
            stream.write(f"applied={len(applied)}\n")
    if summary := os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(summary, "a", encoding="utf-8") as stream:
            stream.write(output)
    print(output)
    return 0


if __name__ == "__main__":
    sys.exit(main())
