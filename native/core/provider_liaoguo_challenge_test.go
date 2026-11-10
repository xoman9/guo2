package core

import (
 "strings"
 "testing"
 nethtml "golang.org/x/net/html"
)

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

func TestLiaoguoRefreshFollowsOnlySafeSameSite(t *testing.T) {
 cases:=[]struct{html, expected string}{
  {`<html><head><meta http-equiv="refresh" content="0;url=/vodtype/19.html"></head></html>`,
   "https://xslrxx.mom/vodtype/19.html"},
  {`<html><head><meta http-equiv="refresh" content="0;url=https://other.example/"></head></html>`, ""},
  {`<html><head><meta http-equiv="refresh" content="0;url=javascript:alert(1)"></head></html>`, ""},
  {`<html><head><meta http-equiv="refresh" content="10;url=/vodtype/19.html"></head></html>`, ""},
 }
 for _,tc:=range cases {
  doc,err:=nethtml.Parse(strings.NewReader(tc.html))
  if err!=nil{t.Fatal(err)}
  if got:=liaoguoMetaRefreshTarget(doc,"https://xslrxx.mom/");got!=tc.expected {
   t.Fatalf("refresh target %q, expected %q",got,tc.expected)
  }
 }
}
