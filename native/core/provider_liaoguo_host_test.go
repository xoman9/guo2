package core

import (
  "strings"
  "testing"
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
