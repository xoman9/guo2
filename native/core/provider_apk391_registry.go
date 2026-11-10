package core

import "strings"

// Only enabled adapters from the user's 1.0.11 channel shortlist.
// Domains are legacy candidates and may change; no catalog is prefetched.
type apk391SourceSpec struct{ ID, Name, Kind, Host string }

var apk391Sources = []apk391SourceSpec{
  {"youguo","柚果","maccms","youavhub.com"},
  {"meiguo","美果","maccms","shiresm.lol"},
  {"yingguo","櫻果","maccms","missav.app"},
  {"luguo","露果","maccms","luyitian.com"},
  {"liguo","荔果","maccms","ririlu.cc"},
  {"juguo","橘果","maccms","bulunhufait.buzz"},
  {"ningguo","檸果","maccms","toptv15.cyou"},
  {"mangguo","芒果","maccms","zsrenqi.xyz"},
  {"chengguodj","橙果","web","chengguodj.com"},
  {"jiaoguo","角果","maccms","haijiao126.cc"},
  {"liaoguo","撩果","maccms","aeg.xslrxx.mom"},
}

var apk391ByID = func() map[string]apk391SourceSpec {
  out := make(map[string]apk391SourceSpec, len(apk391Sources))
  for _, spec := range apk391Sources { out[spec.ID] = spec }
  return out
}()
func apk391Spec(source string) (apk391SourceSpec,bool) {
  spec, ok := apk391ByID[strings.ToLower(strings.TrimSpace(source))]
  return spec, ok
}
func apk391SourceForHost(host string) string {
  host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)),"www.")
  for _, spec := range apk391Sources {
    if host == strings.TrimPrefix(spec.Host,"www.") { return spec.ID }
  }
  return ""
}
func apk391BaseURL(source string) string {
  if spec, ok := apk391Spec(source); ok { return "https://" + spec.Host }
  return ""
}
