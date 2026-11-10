package core

import (
  "strings"
  "testing"
  nethtml "golang.org/x/net/html"
)

func TestLiaoguoUsesExtracted0536Host(t *testing.T) {
  spec, ok := apk391Spec("liaoguo")
  if !ok || spec.Kind != "maccms" {
    t.Fatalf("liaoguo source not registered correctly: %#v", spec)
  }
  if got := apk391BaseURL("liaoguo"); got != "https://aeg.xslrxx.mom" {
    t.Fatalf("expected host extracted from 0.2.536 APK, got %q", got)
  }
  if got := apk391SourceForHost("aeg.xslrxx.mom"); got != "liaoguo" {
    t.Fatalf("APK host did not resolve to liaoguo: %q", got)
  }
  if got := apk391SameSite("liaoguo", "https://xslrxx.mom/"); got != "" {
    t.Fatalf("legacy host unexpectedly accepted as current source: %q", got)
  }
  if got := apk391SameSite("liaoguo", "/"); !strings.HasPrefix(got, "https://aeg.xslrxx.mom/") {
    t.Fatalf("current source root was not used: %q", got)
  }
}

func TestLiaoguoCategoryUsesActualNavigationURL(t *testing.T) {
  document := `<html><body>
    <nav>
      <a href="https://xslrxx.mom/vodtype/19.html">旧站分類</a>
      <a href="/vodtype/19.html">本站分類</a>
      <a href="/index.php/vod/type/id/20.html">另一分類</a>
      <a href="https://not-our-site.example/vodtype/22.html">其他網站</a>
    </nav>
  </body></html>`
  doc, err := nethtml.Parse(strings.NewReader(document))
  if err != nil {t.Fatal(err)}
  if got:=liaoguoCategoryLink(doc, "19");got!="https://aeg.xslrxx.mom/vodtype/19.html" {
    t.Fatalf("wrong category URL: %q", got)
  }
  if got:=liaoguoCategoryLink(doc, "20");got!="https://aeg.xslrxx.mom/index.php/vod/type/id/20.html" {
    t.Fatalf("wrong second category URL: %q", got)
  }
  if got:=liaoguoCategoryLink(doc, "22");got!="" {
    t.Fatalf("foreign website category accepted: %q",got)
  }
  if got:=liaoguoCategoryLink(doc, "../19");got!="" {
    t.Fatalf("invalid category accepted: %q",got)
  }
}
