# guo2 maintenance

- `main`: maintenance workflows (not the APK source branch).
- `upstream`: fast-forward-only mirror of `Xanerix/guoapp:main`.
- `custom`: application code and all personal changes; make each customization an individual commit.

GitHub Actions **Sync upstream into guo2** runs only when manually started via Actions > Run workflow on `main` (no scheduled checks). It fetches upstream, advances `upstream` only if fast-forward, and merges into `custom`. A merge conflict stops without pushing a broken `custom`. When `custom` changes through this workflow, it invokes the Android builder directly; GitHub Actions pushes using `GITHUB_TOKEN` do not initiate separate push-triggered workflows.

**Build guo2 custom Android APK** starts automatically whenever `custom` receives a push (including new customization commits). It can also be launched manually on `main`; it always checks out `custom`, validates Dart, Flutter and Go, builds the all-sources edition in ARM64 and armeabi-v7a, and keeps verified APKs as 14-day Actions artifacts. It never creates a Release automatically.

Before building, configure the existing upstream-compatible GitHub repository Secrets: `ZHENGUOJIAN_KEYSTORE_BASE64`, `ZHENGUOJIAN_KEYSTORE_PASSWORD`, `ZHENGUOJIAN_KEY_ALIAS`, `ZHENGUOJIAN_KEY_PASSWORD` and repository variable `ZHENGUOJIAN_CERT_SHA256`. When none of these five settings is present, the workflow produces a **test-only APK signed using the runner's temporary debug certificate**, with an artifact name containing `test`. Such builds are not stable in signing identity and generally cannot be installed as updates over earlier APKs. A partially configured signing set stops the workflow rather than silently downgrading. With all five values configured, it verifies the release certificate. The workflow uses optional display/package name variables as the upstream project does.

To avoid overwriting custom changes, never copy upstream directly onto `custom` or force-push. Review Actions logs after each sync. If CI compilation or tests fail, keep the previous verified APK and repair on `custom` with a separate commit.

## guo2 coexist signing (November 2026)

Custom Android APK has fixed application ID `com.xoman9.guo2`, separate from upstream `com.guoapp.all`. Automatic build on custom pushes requires four repository Secrets `GUO2_KEYSTORE_BASE64`, `GUO2_KEYSTORE_PASSWORD`, `GUO2_KEY_ALIAS`, `GUO2_KEY_PASSWORD` and the repository variable `GUO2_CERT_SHA256`. Without all five settings the build intentionally stops (no random test-key fallback). Keep the keystore and its password backed up privately; never commit them to this public repository. Do the private one-time signing setup, then future custom commits generate signed APKs automatically. Old app data stay with `com.guoapp.all` and do not transfer automatically.
