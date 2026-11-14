package custom

import (
 "context"
 "errors"
 "net/url"
 "strings"
 "duanjuapp/native/core"
 "golang.org/x/net/html"
)
func tvNodes(root *html.Node, tag, class string) []*html.Node {
 return core.ProviderHTMLNodes(root,func(n *html.Node)bool{
  return n.Type==html.ElementNode&&(tag==""||n.Data==tag)&&(class==""||core.ProviderHTMLClass(n,class))
 })
}
func tvAttr(root *html.Node,tag,class,attribute string)string{
 for _,n:=range tvNodes(root,tag,class){if v:=strings.TrimSpace(core.ProviderHTMLAttr(n,attribute));v!=""{return v}}
 return ""
}
func tvText(root *html.Node,tag,class string)string{
 for _,n:=range tvNodes(root,tag,class){if v:=core.ProviderHTMLText(n);v!=""{return v}}
 return ""
}
func tvSafeURL(base,raw string)string{
 raw=strings.TrimSpace(raw);if raw==""{return ""}
 parsed,e:=url.Parse(raw);if e!=nil||parsed.User!=nil{return ""}
 if parsed.Scheme!=""&&parsed.Scheme!="http"&&parsed.Scheme!="https"{return ""}
 b,e:=url.Parse(base);if e!=nil{return ""}
 out:=b.ResolveReference(parsed)
 if out.Scheme!="https"&&out.Scheme!="http"||out.User!=nil{return ""}
 return out.String()
}
func tvPage(host core.Host,ctx context.Context,address,referer string)(*html.Node,error){
 doc,_,err:=core.FetchHostPage(ctx,host,address,referer,core.UserAgent)
 if err!=nil{return nil,err}
 if len(tvNodes(doc,"title",""))>0 {
  title:=strings.ToLower(tvText(doc,"title",""))
  if strings.Contains(title,"just a moment")||strings.Contains(title,"attention required")||strings.Contains(title,"verify you are human"){return nil,errors.New("HTTP 403：站源需要浏览器验证，当前会话未获授权")}
 }
 return doc,nil
}
func tvMeta(root *html.Node,property string)string{
 for _,n:=range tvNodes(root,"meta",""){
  if core.ProviderHTMLAttr(n,"property")==property{return core.ProviderHTMLAttr(n,"content")}
 }
 return ""
}
