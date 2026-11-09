import 'dart:io';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:path_provider/path_provider.dart';

import '../../app_device.dart';
import '../../core_bridge.dart';
import '../../local_store.dart';
import '../../merge_location.dart';
import '../app_scope.dart';
import '../widgets/focus_ring.dart';
import '../widgets/group_card.dart';
import '../widgets/motion/motion_widgets.dart';
import '../widgets/status_view.dart';
import 'settings_rows.dart';

String storageSize(int bytes) {
  if (bytes < 0) return '暂不可用';
  if (bytes < 1 << 20) return '${(bytes / 1024).toStringAsFixed(1)} KB';
  if (bytes < 1 << 30) return '${(bytes / (1 << 20)).toStringAsFixed(1)} MB';
  return '${(bytes / (1 << 30)).toStringAsFixed(2)} GB';
}

class StorageView extends StatefulWidget {
  const StorageView({super.key, required this.store});
  final LocalStore store;

  @override
  State<StorageView> createState() => _StorageViewState();
}

class _StorageViewState extends State<StorageView> with WidgetsBindingObserver {
  Map<String, dynamic>? _info;
  String? _error;
  bool _loaded = false;
  bool? _fileAccessGranted;
  bool _requestingFileAccess = false;
  String? _pendingMigrationParent;
  String? _mergeTreeUri;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _checkFileAccess();
    _loadMergeLocation();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      _checkFileAccess();
      _refresh();
    }
  }

  Future<void> _checkFileAccess() async {
    if (!Platform.isAndroid) return;
    try {
      const channel = MethodChannel('duanju/device');
      final granted =
          await channel.invokeMethod<bool>('sharedStorageAccess') ?? false;
      if (mounted) setState(() => _fileAccessGranted = granted);
    } catch (error) {
      if (mounted) setState(() => _error = '無法檢查存取授權：$error');
    }
  }

  Future<void> _manageFileAccess() async {
    if (!Platform.isAndroid || _requestingFileAccess) return;
    setState(() => _requestingFileAccess = true);
    try {
      const channel = MethodChannel('duanju/device');
      final granted =
          await channel.invokeMethod<bool>('sharedStorageAccess') ?? false;
      if (mounted) setState(() => _fileAccessGranted = granted);
      if (!mounted) return;
      if (granted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('已取得檔案存取授權')),
        );
        return;
      }
      // Android 11+: take the user to the app's All Files Access settings.
      // On returning, didChangeAppLifecycleState refreshes authorization.
      await channel.invokeMethod<bool>('requestSharedStorageAccess');
    } catch (error) {
      if (mounted) setState(() => _error = '無法開啟存取授權：$error');
    } finally {
      if (mounted) setState(() => _requestingFileAccess = false);
    }
  }

  Future<void> _loadMergeLocation() async {
    try {
      final uri = await MergeLocation.current();
      if (mounted) setState(() => _mergeTreeUri = uri);
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    }
  }

  Future<void> _chooseMergeLocation() async {
    try {
      final uri = await MergeLocation.choose();
      if (uri != null && mounted) setState(() => _mergeTreeUri = uri);
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    }
  }

  Future<void> _clearMergeLocation() async {
    await MergeLocation.clear();
    if (mounted) setState(() => _mergeTreeUri = null);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_loaded) return;
    _loaded = true;
    _refresh();
  }

  Future<void> _refresh() async {
    try {
      final info = await AppScope.of(context).repository.storage();
      if (mounted) {
        setState(() {
          _info = info;
          _error = null;
        });
      }
    } catch (failure) {
      if (mounted) setState(() => _error = '$failure');
    }
  }

  Future<String?> _pickParent() async {
    if (!Platform.isAndroid && !Platform.isIOS) {
      return FilePicker.getDirectoryPath(dialogTitle: '选择下载保存位置');
    }
    final support = await getApplicationSupportDirectory();
    final directories = <String, String>{support.path: '应用内部存储'};
    if (Platform.isAndroid) {
      final external = await getExternalStorageDirectories() ?? [];
      for (final (index, directory) in external.indexed) {
        directories[directory.path] = index == 0
            ? '设备共享存储（应用目录）'
            : 'SD 卡 ${index + 1}（应用目录）';
      }
    } else {
      final documents = await getApplicationDocumentsDirectory();
      directories[documents.path] = '文件 App 可见目录';
    }
    if (!mounted) return null;
    return showOptionDialog(
      context,
      title: '选择下载位置',
      options: directories,
      selected: null,
    );
  }

  Future<void> _move() async {
    // The native Go download manager requires an absolute filesystem path.
    // Android's system picker resolves supported primary-storage directories.
    String? parent;
    try {
      if (Platform.isAndroid) {
        const channel = MethodChannel('duanju/device');
        // Match 1.0.10+110: authorize shared storage before opening picker.
        final hasAccess =
            await channel.invokeMethod<bool>('sharedStorageAccess') ?? false;
        if (!hasAccess) {
          await channel.invokeMethod<bool>('requestSharedStorageAccess');
          if (mounted) {
            setState(() => _error =
                '請先在系統設定允許「所有檔案存取」，返回後再次選擇下載目錄。');
          }
          return;
        }
        parent = _pendingMigrationParent ??
            await channel.invokeMethod<String>('chooseDownloadDirectory');
        if (parent == null || !mounted) return;
        _pendingMigrationParent = parent;
      } else {
        parent = await _pickParent();
      }
      if (parent == null || !mounted) return;
      final completed = await _migrate(parent, parent);
      if (completed) _pendingMigrationParent = null;
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    }
  }

  Future<void> _restore() async {
    await _migrate('', '應用預設目錄');
  }

  Future<void> _setAutoExport(bool value) => saveSetting(context, () async {
    if (value && !await AppDevice.requestVideoExport()) {
      throw AppFailure('未获得存储权限，无法自动导出');
    }
    await widget.store.setAutoExport(value);
  });

  Future<bool> _migrate(String parent, String destination) async {
    if (!mounted) return false;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('迁移已下载内容？'),
        content: Text(
          '将下载目录里的全部内容迁移到：\n$destination\n\n下载会先暂停，复制成功后清理旧目录。请保证目标有足够空间，并在完成后继续下载。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            autofocus: true,
            onPressed: () => Navigator.pop(context, true),
            child: const Text('迁移'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return false;
    final repository = AppScope.of(context).repository;
    final navigator = Navigator.of(context, rootNavigator: true);
    showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (context) => const PopScope(
        canPop: false,
        child: AlertDialog(
          content: Row(
            spacing: 20,
            children: [
              CircularProgressIndicator(),
              Expanded(child: Text('正在迁移，请保持应用运行…')),
            ],
          ),
        ),
      ),
    );
    String? failureMessage;
    try {
      await repository.moveDownloads(parent);
      // Refresh the actual persisted path from the native download manager.
      await _refresh();
      if (mounted) setState(() => _error = null);
    } catch (failure) {
      failureMessage = '$failure';
      if (mounted) setState(() => _error = failureMessage);
    } finally {
      navigator.pop();
    }
    if (!mounted) return failureMessage == null;
    if (failureMessage != null) {
      await showDialog<void>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('下載目錄遷移失敗'),
          content: Text('$failureMessage\\n\\n原下載目錄與檔案已保留，請確認目標資料夾為空及具有存取權限。'),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('確定'),
            ),
          ],
        ),
      );
      return false;
    }
    return true;
  }

  @override
  Widget build(BuildContext context) => FadeSwitcher(
    alignment: Alignment.topCenter,
    child: KeyedSubtree(
      key: ValueKey(_info != null ? 'content' : _error ?? 'loading'),
      child: _body(context),
    ),
  );

  Widget _body(BuildContext context) {
    final info = _info;
    if (info == null) {
      return _error == null
          ? const StatusView.loading()
          : StatusView(
              title: '无法读取下载目录',
              message: _error,
              icon: Icons.folder_off_outlined,
              action: '重试',
              onAction: _refresh,
            );
    }
    final text = Theme.of(context).textTheme;
    final colors = Theme.of(context).colorScheme;
    final directory = info['directory'] as String? ?? '';
    final admin = widget.store.profile.admin;
    final isDefault = info['default'] == true;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
      children: [
        GroupCard(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 16, 16, 14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                spacing: 4,
                children: [
                  Text(
                    '已使用 ${storageSize((info['bytes'] as num?)?.toInt() ?? 0)}',
                    style: text.headlineSmall,
                  ),
                  Text(
                    '剩余 ${storageSize((info['free'] as num?)?.toInt() ?? -1)} · ${info['files'] ?? 0} 个文件',
                    style: text.bodyMedium?.copyWith(
                      color: colors.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
            const Divider(indent: 16, endIndent: 16),
            SettingsTile(
              title: '当前下载目录',
              subtitle: directory,
              trailing: FocusRing(
                radius: 999,
                child: IconButton(
                  tooltip: '复制路径',
                  onPressed: () =>
                      Clipboard.setData(ClipboardData(text: directory)),
                  icon: const Icon(Icons.copy_rounded),
                ),
              ),
            ),
            const Divider(indent: 16, endIndent: 16),
            SettingsTile(
              title: '更改並遷移目錄',
              subtitle: admin ? (Platform.isAndroid ? '從系統檔案管理員選擇資料夾，並搬移現有下載內容' : '選擇資料夾並搬移現有下載內容') : '需要管理员',
              enabled: admin,
              onTap: _move,
            ),
            const Divider(indent: 16, endIndent: 16),
            SettingsTile(
              title: '恢复默认下载目录',
              subtitle: !admin
                  ? '需要管理员'
                  : isDefault
                  ? '正在使用默认目录'
                  : '迁移回应用默认目录',
              enabled: admin && !isDefault,
              onTap: _restore,
            ),
            const Divider(indent: 16, endIndent: 16),
            SettingsTile(
              title: '存取授權',
              subtitle: !Platform.isAndroid
                  ? '此平台不需要 Android 檔案存取授權'
                  : _fileAccessGranted == true
                  ? '已授權存取共享儲存空間'
                  : _fileAccessGranted == false
                  ? '尚未授權，點擊前往系統設定'
                  : '正在檢查授權狀態',
              enabled: Platform.isAndroid && !_requestingFileAccess,
              onTap: _manageFileAccess,
            ),
          ],
        ),
        if (Platform.isAndroid) ...[
          const SizedBox(height: 16),
          GroupCard(
            children: [
              SettingsTile(
                title: '合併輸出位置',
                subtitle: _mergeTreeUri == null
                    ? '每次合併使用 Movies 資料夾；可設定固定輸出位置'
                    : '已設定固定輸出資料夾（Android SAF 授權）',
                enabled: admin,
                onTap: _chooseMergeLocation,
              ),
              if (_mergeTreeUri != null) ...[
                const Divider(indent: 16, endIndent: 16),
                SettingsTile(
                  title: '清除合併位置',
                  subtitle: '恢復原本的 Movies 輸出方式',
                  enabled: admin,
                  onTap: _clearMergeLocation,
                ),
              ],
            ],
          ),
          const SizedBox(height: 16),
          ListenableBuilder(
            listenable: widget.store,
            builder: (context, _) => GroupCard(
              children: [
                SwitchTile(
                  title: '下载完成后自动导出',
                  subtitle: '导出到 Movies/剧名/，相册和文件管理器可见，卸载后保留',
                  value: widget.store.autoExport,
                  onChanged: _setAutoExport,
                ),
                const Divider(indent: 16, endIndent: 16),
                SwitchTile(
                  title: '自动导出后删除 App 内副本',
                  subtitle: '删除后 App 内不能再离线播放',
                  value: widget.store.deleteAfterAutoExport,
                  onChanged: (value) => saveSetting(
                    context,
                    () => widget.store.setDeleteAfterAutoExport(value),
                  ),
                ),
                const Divider(indent: 16, endIndent: 16),
                SwitchTile(
                  title: '手动导出后删除 App 内副本',
                  subtitle: '在已下载剧页「编辑」里点「导出」后删除',
                  value: widget.store.deleteAfterManualExport,
                  onChanged: (value) => saveSetting(
                    context,
                    () => widget.store.setDeleteAfterManualExport(value),
                  ),
                ),
              ],
            ),
          ),
        ],
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
          child: Text(
            _error ?? '在下载页删除不需要的分集，在本地媒体页删除合并成品，可释放空间。',
            style: text.bodyMedium?.copyWith(
              color: _error == null ? colors.onSurfaceVariant : colors.error,
            ),
          ),
        ),
      ],
    );
  }
}
