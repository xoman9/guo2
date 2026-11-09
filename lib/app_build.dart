const allSourcesEnabled = bool.fromEnvironment('ALL_SOURCES');
const appName = allSourcesEnabled
    ? String.fromEnvironment('ZHENGUOJIAN_DISPLAY_NAME', defaultValue: '小果影視-共存版')
    : '红果鉴';
const appSlug = allSourcesEnabled ? 'zhenguojian' : 'hongguojian';
