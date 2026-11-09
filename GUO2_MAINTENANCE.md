# guo2 maintenance

- `main`: maintenance workflows (not the APK source branch).
- `upstream`: fast-forward-only mirror of `Xanerix/guoapp:main`.
- `custom`: application code and all personal changes; make each customization an individual commit.

GitHub Actions **Sync upstream into guo2** runs only when manually started via Actions > Run workflow on `main` (no scheduled checks). It fetches upstream, advances `upstream` only if fast-forward, and merges into `custom`. A merge conflict stops without pushing a broken `custom`. When `custom` changes through this workflow, it invokes the Android builder directly; GitHub Actions pushes using `GITHUB_TOKEN` do not initiate separate push-triggered workflows.

**Build guo2 custom Android APK** starts automatically whenever `custom` receives a push (including new customization commits). It can also be launched manually on `main`; it always checks out `custom`, validates Dart, Flutter and Go, builds the all-sources edition in ARM64 and armeabi-v7a, and keeps verified APKs as 14-day Actions artifacts. It never creates a Release automatically.

Before building, configure the existing upstream-compatible GitHub repository Secrets: `ZHENGUOJIAN_KEYSTORE_BASE64`, `ZHENGUOJIAN_KEYSTORE_PASSWORD`, `ZHENGUOJIAN_KEY_ALIAS`, `ZHENGUOJIAN_KEY_PASSWORD` and repository variable `ZHENGUOJIAN_CERT_SHA256`. When none of these five settings is present, the workflow produces a **test-only APK signed using the runner's temporary debug certificate**, with an artifact name containing `test`. Such builds are not stable in signing identity and generally cannot be installed as updates over earlier APKs. A partially configured signing set stops the workflow rather than silently downgrading. With all five values configured, it verifies the release certificate. The workflow uses optional display/package name variables as the upstream project does.

To avoid overwriting custom changes, never copy upstream directly onto `custom` or force-push. Review Actions logs after each sync. If CI compilation or tests fail, keep the previous verified APK and repair on `custom` with a separate commit.
