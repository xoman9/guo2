#!/usr/bin/env python3
"""Rebuild guo2 from the latest stable upstream Release and replay custom changes.

The original custom branch is only read, never changed. Replay is path-by-path
using Git's three-way patch application. Unresolved paths remain upstream and
are reported instead of receiving speculative conflict resolutions.
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
        if path == "android/app/build.gradle.kts":
            text = file.read_text(encoding="utf-8")
            if '?: "真果鉴"' not in text or '?: "com.guoapp.all"' not in text:
                return None
            text = text.replace('?: "真果鉴"', '?: "小果影視-共存版"', 1)
            text = text.replace('?: "com.guoapp.all"', '?: "com.duanju.coexist"', 1)
            file.write_text(text, encoding="utf-8")
            return "coexist branding and application ID"
        if path == "lib/app_build.dart":
            file.write_text(
                "const allSourcesEnabled = bool.fromEnvironment('ALL_SOURCES');\n"
                "const configuredAppFlavor = String.fromEnvironment('APP_FLAVOR');\n"
                "const appFlavor = configuredAppFlavor != ''\n"
                "    ? configuredAppFlavor\n"
                "    : (allSourcesEnabled ? 'zhenguojian' : 'hongguojian');\n"
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

    applied: list[str] = []
    overrides: list[dict[str, str]] = []
    conflicts: list[dict[str, str]] = []
    for path in paths:
        patch = git("diff", "--binary", base_sha, custom_sha, "--", path).stdout
        if not patch:
            continue
        result = subprocess.run(["git", "apply", "--3way", "--index", "-"],
                                input=patch, capture_output=True)
        if result.returncode:
            error = result.stderr.decode("utf-8", "replace").strip()
            # Discard only the failed patch: each successful file is committed.
            git("reset", "--hard", "HEAD")
            rule = deterministic_override(path)
            if rule is not None:
                git("add", "-A")  # handles already-absent upstream paths safely
                git("commit", "-m", "replay(custom): override " + path, "--allow-empty")
                overrides.append({"path": path, "rule": rule})
                continue
            conflicts.append({"path": path, "error": error[:2000]})
            continue
        git("commit", "-m", "replay(custom): " + path, "--allow-empty")
        applied.append(path)

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
        "var webProviderNumericID = WebProviderNumericID\n"
        "var webProviderInteger = WebProviderInteger\n"
        "var providerHTMLAttr = ProviderHTMLAttr\n"
        "var providerHTMLClass = ProviderHTMLClass\n"
        "var providerHTMLNodes = ProviderHTMLNodes\n"
        "var providerHTMLFirstClass = ProviderHTMLFirstClass\n"
        "var providerHTMLText = ProviderHTMLText\n"
        "var nativeText = NativeText\n"
        "var nativeNormalize = NativeNormalize\n"
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
        "var providerDramaID = ProviderDramaID\n"
        "var providerChapterID = ProviderChapterID\n"
        "var splitProviderDramaID = SplitProviderDramaID\n",
        encoding="utf-8"
    )
    git("add", "--", str(compat))
    git("commit", "-m", "compat(sync): bridge legacy source type names to upstream public API")

    report = [
        "# Upstream Release → guo2 custom replay",
        "",
        f"- Upstream Release: `{tag}` (`{release_sha}`)",
        f"- Original custom: `{custom_sha}`",
        f"- Three-way merge base: `{base_sha}`",
        f"- Applied paths: **{len(applied)}** / {len(paths)}",
        f"- Deterministic overrides: **{len(overrides)}**",
        f"- Conflicting paths: **{len(conflicts)}**",
        "- Source: the full official Release, not the previous migration branch.",
        "- Existing custom, main and upstream branches were not changed.",
        "",
        "## Applied paths",
        *["- `" + p + "`" for p in applied],
        "",
        "## Deterministic overrides",
        *[f"- {entry['path']}: {entry['rule']}" for entry in overrides],
        "",
        "## Paths requiring conflict resolution",
        *([f"- `{c['path']}`: `{c['error'].splitlines()[0][:180]}`" for c in conflicts]
          if conflicts else ["None."]),
        "",
        "A clean patch replay does **not** guarantee build compatibility.",
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
    git("push", "--force-with-lease", "origin", "HEAD:refs/heads/" + target_branch)

    if output_path := os.environ.get("GITHUB_OUTPUT"):
        with open(output_path, "a", encoding="utf-8") as stream:
            stream.write(f"branch={target_branch}\n")
            stream.write(f"release={tag}\n")
            stream.write(f"conflicts={len(conflicts)}\n")
            stream.write(f"applied={len(applied)}\n")
    if summary := os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(summary, "a", encoding="utf-8") as stream:
            stream.write(output)
    print(output)
    return 0


if __name__ == "__main__":
    sys.exit(main())
