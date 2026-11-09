package core

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/url"
 "strconv"
 "regexp"
 "strings"
 nethtml "golang.org/x/net/html"
)

// 橙果是短剧网站，使用 /drama/dj-xxxx 与 /play/dj-xxxx/n，
// 不采用 MacCMS 的数字 vod ID。
func chengguoSlug(link string) string {
 u,err:=url.Parse(link);if err!=nil{return ""}
 parts:=strings.Split(strings.Trim(u.Path,"/"),"/")
 if len(parts)!=2||parts[0]!="drama"{return ""}
 slug:=parts[1]
 if !chengguoIDValid(slug){return ""}
 for _,r:=range slug[3:]{
  if !(r>='a'&&r<='z'||r>='0'&&r<='9'||r=='-'){return ""}
 }
 return slug
}
// The /browse SSR output uses <article data-xpch="card-drama">. Its image
// often stays a transparent lazy-loading GIF, while title and /drama link
// are separate siblings. Never discard a verified film for missing cover.
func chengguoCards(doc *nethtml.Node) []Drama {
 seen:=map[string]bool{}
 records:=chengguoNuxtRecords(doc)
 results:=[]Drama{}
 for _,article:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{
  return n.Data=="article"&&providerHTMLAttr(n,"data-xpch")=="card-drama"
 }){
  slug:=""
  for _,a:=range providerHTMLNodes(article,func(n *nethtml.Node)bool{return n.Data=="a"}){
   link:=apk391SameSite("chengguodj",providerHTMLAttr(a,"href"))
   if id:=chengguoSlug(link);id!=""{slug=id;break}
  }
  if slug==""||seen[slug]{continue}
  title:=""
  for _,h:=range providerHTMLNodes(article,func(n *nethtml.Node)bool{return n.Data=="h3"}){
   if text:=strings.TrimSpace(providerHTMLText(h));text!=""{title=text;break}
  }
  if title==""||apk391BadTitle("chengguodj",title){continue}
  cover:=""
  for _,img:=range providerHTMLNodes(article,func(n *nethtml.Node)bool{return n.Data=="img"}){
   // Keep the page's direct image only as a fallback: it may be a stale
   // static poster. Nuxt's original cover URL is AES-CBC encrypted, but the
   // native cover cache already decodes that response to a displayable JPEG.
   // The site's fallback_url currently points to a missing /static/ resource.
   // The browser may use a lazy-loading attribute or the first srcset image;
   // src alone is frequently a transparent placeholder.
   for _,raw:=range []string{providerHTMLAttr(img,"data-src"),providerHTMLAttr(img,"data-original"),providerHTMLAttr(img,"data-lazy"),providerHTMLAttr(img,"data-srcset"),providerHTMLAttr(img,"srcset"),providerHTMLAttr(img,"src")}{
    raw=strings.TrimSpace(raw)
    if strings.Contains(raw,","){raw=strings.TrimSpace(strings.Split(raw,",")[0])}
    if fields:=strings.Fields(raw);len(fields)>0{raw=fields[0]}
    if raw==""||strings.HasPrefix(raw,"data:")||strings.HasPrefix(raw,"blob:"){continue}
    if resolved:=lujj31PosterAddress(raw,apk391BaseURL("chengguodj")+"/");isProviderHTTPMediaURL(resolved)&&!strings.Contains(strings.ToLower(resolved),"placeholder"){
     cover=resolved;break
    }
   }
   if cover!=""{break}
  }
  if record,ok:=records[slug];ok{
   // The canonical original is encrypted in transit, and nativeDecodeCover
   // handles its AES-128-CBC PKCS#7 payload. Prefer it over missing static
   // fallback posters or transparent lazy-loading placeholders.
   if isProviderHTTPMediaURL(record.Cover){cover=record.Cover}
  }
  seen[slug]=true
  title=truncate(title,512)
  episodes:=records[slug].Episodes
  results=append(results,Drama{ID:providerDramaID("chengguodj",slug),Source:"chengguodj",SourceID:slug,Title:title,Name:title,Cover:cover,CoverURL:cover,EpisodeCount:episodes,TotalEpisode:episodes,ChannelName:"橙果"})
  if len(results)>=100{break}
 }
 return results
}
func chengguoHasNext(doc *nethtml.Node,page int)bool {
 want:=fmt.Sprintf("/browse/page-%d",page+1)
 for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"}){
  if u:=apk391SameSite("chengguodj",providerHTMLAttr(a,"href"));u!=""{
   parsed,err:=url.Parse(u);if err==nil&&strings.TrimRight(parsed.Path,"/")==want{return true}
  }
 }
 return false
}
func (d *Downloader) fetchChengguoCatalog(ctx context.Context,page int,category,query string)([]Drama,bool,error){
 if page<1||page>100000||len(query)>256||!apk391ValidCategory("chengguodj",category){return nil,false,errors.New("橙果分页或分类参数无效")}
 path:="/browse"
 if category!=""{path="/"+category}
 if query!=""{return nil,false,errors.New("橙果搜索尚未接入")}
 if page>1 && category==""{path=fmt.Sprintf("/browse/page-%d",page)}
 if page>1 && category!=""{return []Drama{},false,nil} // Category paging not yet verified.
 doc,_,err:=d.apk391Page(ctx,"chengguodj",path)
 if err!=nil{return nil,false,err}
 cards:=chengguoCards(doc)
 if len(cards)==0{return nil,false,fmt.Errorf("橙果页面未取得有效短剧卡片: %s",path)}
 return cards,category==""&&chengguoHasNext(doc,page),nil
}

var chengguoNuxtPattern=regexp.MustCompile(`(?s)<script[^>]+id="__NUXT_DATA__"[^>]*>(.*?)</script>`)
func chengguoNuxtRecords(doc *nethtml.Node) map[string]struct{Cover string; Episodes int} {
 result:=map[string]struct{Cover string; Episodes int}{}
 for _,script:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="script"&&providerHTMLAttr(n,"id")=="__NUXT_DATA__"}) {
  var source string
  for child:=script.FirstChild;child!=nil;child=child.NextSibling{if child.Type==nethtml.TextNode{source+=child.Data}}
  var nodes []json.RawMessage
  if json.Unmarshal([]byte(source),&nodes)!=nil{continue}
  refString:=func(v json.RawMessage)string{
   var index int
   if json.Unmarshal(v,&index)!=nil||index<0||index>=len(nodes){return ""}
   var value string
   _=json.Unmarshal(nodes[index],&value)
   return value
  }
  refInt:=func(v json.RawMessage)int{
   var index int
   if json.Unmarshal(v,&index)!=nil||index<0||index>=len(nodes){return 0}
   var value int
   _=json.Unmarshal(nodes[index],&value)
   return value
  }
  for _,entry:=range nodes{
   var obj map[string]json.RawMessage
   if json.Unmarshal(entry,&obj)!=nil||obj["slug"]==nil||obj["cover"]==nil{continue}
   slug:=refString(obj["slug"])
   if !strings.HasPrefix(slug,"dj-")&&!strings.HasPrefix(slug,"app-"){continue}
   var coverID int
   if json.Unmarshal(obj["cover"],&coverID)!=nil||coverID<0||coverID>=len(nodes){continue}
   var coverFields map[string]json.RawMessage
   if json.Unmarshal(nodes[coverID],&coverFields)!=nil{continue}
   cover:=refString(coverFields["url"])
   fallback:=refString(coverFields["fallback_url"])
   if strings.HasPrefix(fallback,"/"){fallback=apk391BaseURL("chengguodj")+fallback}
   // Preserve the canonical CDN original; nativeCoverCache decrypts it
   // before returning the image file to Flutter. Static fallback paths can
   // return HTTP 404 and must not replace a valid canonical URL.
   encrypted:=false
   if indexRaw,ok:=coverFields["encrypted"];ok{
    var index int
    if json.Unmarshal(indexRaw,&index)==nil && index>=0&&index<len(nodes){_ = json.Unmarshal(nodes[index],&encrypted)}
   }
   // If the original is absent, use an explicit fallback as a last resort.
   // Do not reject an encrypted original: the native image cache decrypts it.
   _ = encrypted
   if !isProviderHTTPMediaURL(cover) && isProviderHTTPMediaURL(fallback){cover=fallback}
   result[slug]=struct{Cover string; Episodes int}{Cover:cover,Episodes:refInt(obj["published_episode_count"])}
  }
 }
 return result
}

func chengguoIDValid(id string)bool{
 if !(strings.HasPrefix(id,"dj-")||strings.HasPrefix(id,"app-"))||len(id)>48{return false}
 for _,r:=range id{if !(r>='a'&&r<='z'||r>='0'&&r<='9'||r=='-'){return false}}
 return true
}
func (d *Downloader) fetchChengguoDetail(ctx context.Context,id string)(Drama,[]Chapter,error){
 if !chengguoIDValid(id){return Drama{},nil,errors.New("橙果影片编号无效")}
 path:="/drama/"+id
 doc,_,err:=d.apk391Page(ctx,"chengguodj",path)
 if err!=nil{return Drama{},nil,err}
 title:=""
 for _,h:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="h1"}) {
  if v:=strings.TrimSpace(providerHTMLText(h));v!=""{title=v;break}
 }
 if title=="" {
  for _,m:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="meta"&&providerHTMLAttr(n,"property")=="og:title"}) {
   title=strings.TrimSpace(providerHTMLAttr(m,"content"));break
  }
 }
 if title==""{title=id}
 cover:=""
 for _,m:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="meta"&&providerHTMLAttr(n,"property")=="og:image"}){
  candidate:=strings.TrimSpace(providerHTMLAttr(m,"content"))
  if isProviderHTTPMediaURL(candidate)&&!strings.Contains(candidate,"social-default"){cover=candidate;break}
 }
 records:=chengguoNuxtRecords(doc)
 if rec,ok:=records[id];ok&&isProviderHTTPMediaURL(rec.Cover){cover=rec.Cover}
 episodes:=map[int]bool{}
 for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"}){
  link:=providerHTMLAttr(a,"href")
  u,err:=url.Parse(link);if err!=nil{continue}
  prefix:="/play/"+id+"/"
  if !strings.HasPrefix(u.Path,prefix){continue}
  n,err:=strconv.Atoi(strings.TrimPrefix(u.Path,prefix))
  if err==nil&&n>=1&&n<=1000{episodes[n]=true}
 }
 count:=records[id].Episodes
 for n:=range episodes{if n>count{count=n}}
 if count==0{count=1}
 if count>500{count=500}
 chapters:=make([]Chapter,0,count)
 for n:=1;n<=count;n++{
  link:=apk391BaseURL("chengguodj")+fmt.Sprintf("/play/%s/%d",id,n)
  chapters=append(chapters,Chapter{ID:providerChapterID("chengguodj",id,strconv.Itoa(n)),Source:"chengguodj",Title:fmt.Sprintf("第%d集",n),CurrentEpisode:rawEpisode(n),PageURL:link,VideoURL:"chengguodj-play://"+id+"/"+strconv.Itoa(n),Referer:apk391BaseURL("chengguodj")+"/"})
 }
 return Drama{ID:providerDramaID("chengguodj",id),Source:"chengguodj",SourceID:id,Title:title,Name:title,Cover:cover,CoverURL:cover,EpisodeCount:count,TotalEpisode:count,ChannelName:"橙果"},chapters,nil
}

var chengguoMediaPattern=regexp.MustCompile(`https?://[^"'<>\s]+?\.(?:m3u8|mp4)(?:\?[^"'<>\s]*)?`)
func (d *Downloader) resolveChengguoMedia(ctx context.Context,task Task)(providerMedia,error){
 source,id,ok:=splitProviderDramaID(task.DramaID)
 if !ok||source!="chengguodj"||!chengguoIDValid(id)||!strings.HasPrefix(task.Chapter.ID,providerChapterID(source,id,"")){
  return providerMedia{},errors.New("橙果播放编号无效")
 }
 page:=apk391SameSite(source,task.Chapter.PageURL)
 if page==""||!strings.Contains(page,"/play/"+id+"/"){return providerMedia{},errors.New("橙果分集来源不一致")}
 body,err:=d.fetchProviderText(context.WithValue(ctx,providerTextUserAgentKey{},lujj31UserAgent),page,apk391BaseURL(source)+"/")
 if err!=nil{return providerMedia{},err}
 // Only forward a confirmed media resource, never the HTML page or an image URL.
 normalized:=strings.ReplaceAll(body,`\u002F`,"/")
 normalized=strings.ReplaceAll(normalized,`\/`,"/")
 matches:=chengguoMediaPattern.FindAllString(normalized,50)
 for _,candidate:=range matches{
  candidate=strings.TrimSuffix(candidate,"\\")
  if isProviderHTTPMediaURL(candidate){
   return d.prepareWebProviderMedia(ctx,providerMedia{URL:candidate,Referer:page},"橙果")
  }
 }
 return providerMedia{},errors.New("橙果播放页面未提供可验证的媒体 URL；需进一步确认站方播放器 API")
}
