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

    applied: list[str] = []
    conflicts: list[dict[str, str]] = []
    for path in paths:
        patch = git("diff", "--binary", base_sha, custom_sha, "--", path).stdout
        if not patch:
            continue
        result = subprocess.run(["git", "apply", "--3way", "--index", "-"],
                                input=patch, capture_output=True)
        if result.returncode:
            error = result.stderr.decode("utf-8", "replace").strip()
            conflicts.append({"path": path, "error": error[:2000]})
            # Discard only the failed patch: each successful file is committed.
            git("reset", "--hard", "HEAD")
            continue
        git("commit", "-m", "replay(custom): " + path, "--allow-empty")
        applied.append(path)

    report = [
        "# Upstream Release → guo2 custom replay",
        "",
        f"- Upstream Release: \`{tag}\` (\`{release_sha}\`)",
        f"- Original custom: \`{custom_sha}\`",
        f"- Three-way merge base: \`{base_sha}\`",
        f"- Applied paths: **{len(applied)}** / {len(paths)}",
        f"- Conflicting paths: **{len(conflicts)}**",
        "- Source: the full official Release, not the previous migration branch.",
        "- Existing custom, main and upstream branches were not changed.",
        "",
        "## Applied paths",
        *["- \`" + p + "\`" for p in applied],
        "",
        "## Paths requiring conflict resolution",
        *([f"- \`{c['path']}\`: \`{c['error'].splitlines()[0][:180]}\`" for c in conflicts]
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
