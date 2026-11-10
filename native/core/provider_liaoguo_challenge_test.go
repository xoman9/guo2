package core

import "testing"

func TestApk391BrowserChallengeIsNotParsedAsCatalog(t *testing.T) {
  for _, challenge := range []string{
    "站点要求浏览器验证，当前请求无法通过",
    "站點要求瀏覽器驗證，當前請求無法通過",
    "Checking your browser before accessing this site",
    "Just a moment... Verify you are human",
  } {
    if !apk391BrowserChallengeText(challenge) {
      t.Fatalf("missed verification page: %q", challenge)
    }
  }
  for _, normal := range []string{
    "最新短劇 影片 第1集",
    "分類 精品 共100部",
    "",
  } {
    if apk391BrowserChallengeText(normal) {
      t.Fatalf("normal catalogue misclassified as challenge: %q", normal)
    }
  }
}
