import 'dart:io';

import 'package:flutter/services.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// SAF tree URI is persisted by Android with read/write grants. No broad
/// MANAGE_EXTERNAL_STORAGE permission is required.
class MergeLocation {
  static const _key = 'legacy110.mergeDirectoryTreeUri';
  static const _device = MethodChannel('duanju/device');

  static Future<String?> current() async {
    if (!Platform.isAndroid) return null;
    return (await SharedPreferences.getInstance()).getString(_key);
  }

  static Future<String?> choose() async {
    if (!Platform.isAndroid) return null;
    final selected = await _device.invokeMethod<String>('chooseMergeDirectory');
    if (selected == null) return null;
    await (await SharedPreferences.getInstance()).setString(_key, selected);
    return selected;
  }

  static Future<void> clear() async {
    await (await SharedPreferences.getInstance()).remove(_key);
  }

  static Future<String?> createDocument(String name) async {
    final tree = await current();
    if (tree == null) return null;
    return _device.invokeMethod<String>('createMergeDocument', {
      'treeUri': tree,
      'name': name,
    });
  }
}
