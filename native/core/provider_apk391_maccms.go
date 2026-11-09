package core

import (
 "context"
 "errors"
 "fmt"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "sync"
 "time"

 nethtml "golang.org/x/net/html"
)

// Experimental shared adapter only for sites whose APK registry Kind=maccms.
// Specialty APIs retain explicit unsupported responses until their original
// request/response protocol is reconstructed. No source is pre-cached.
var (
 apk391IDPattern = regexp.MustCompile("(?i)/(?:voddetail|vodplay|detail|play)/([0-9]{1,9})(?:[-./?]|$)")
 apk391SlashID = regexp.MustCompile("(?i)/(?:vod/)?(?:detail|play)/id/([0-9]{1,9})(?:[-./?]|$)")
 apk391CatPattern=regexp.MustCompile("(?i)/(?:vodtype|vodshow|type|list)(?:/id)?[/_-]([0-9]{1,8})(?:[-./?]|$)")
 apk391Numeric=regexp.MustCompile("^[0-9]{1,9}$")
)

func apk391SameSite(source, target string) string {
 base:=apk391BaseURL(source)
 if base==""{return ""}
 b,be:=url.Parse(base);u,ue:=url.Parse(strings.TrimSpace(target))
 if be!=nil||ue!=nil||u.User!=nil{return ""}
 a:=b.ResolveReference(u)
 if a.User!=nil || (a.Scheme!="https"&&a.Scheme!="http") ||
  !strings.EqualFold(strings.TrimPrefix(a.Hostname(),"www."),strings.TrimPrefix(b.Hostname(),"www.")){return ""}
 return a.String()
}

func apk391VideoID(target string) string {
 u,err:=url.Parse(target);if err!=nil{return ""}
 path:=strings.ToLower(u.Path)
 // MacCMS taxonomy routes contain /id/<category> too, and pagination
 // links contain /page/<n>. Neither can be treated as a video.
 for _,part:=range []string{"/vod/show/","/vod/type/","/vodshow/","/vodtype/","/category/","/categories/","/page/"} {
  if strings.Contains(path,part){return ""}
 }
 for _,rx:=range []*regexp.Regexp{apk391SlashID,apk391IDPattern}{
  if m:=rx.FindStringSubmatch(u.Path);len(m)>1{return m[1]}
 }
 return ""
}

func apk391ValidCategory(source,category string)bool {
 if category==""{return true}
 // The live Juguo video7 website exposes many more numeric /vodtype/ IDs
 // than its old 0.2.183 static category list.
 if source=="juguo"{return apk391Numeric.MatchString(category)}
 for _,cat:=range apk391StaticCategoryFallback[source]{
  if cat.ID==category{return true}
 }
 if len(apk391StaticCategoryFallback[source])>0{return false}
 return apk391Numeric.MatchString(category)
}

func (d *Downloader) apk391Page(ctx context.Context,source,path string)(*nethtml.Node,string,error){
 link:=apk391SameSite(source,path)
 if link==""{return nil,"",errors.New("非法或跨站页面")}
 base:=apk391BaseURL(source)
 doc,actual,err:=d.fetchProviderPage(ctx,link,base+"/",lujj31UserAgent)
 if err!=nil{return nil,"",err}
 if len(providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="body"&&n.FirstChild!=nil}))==0{
  return nil,"",errors.New("网页未返回有效正文")
 }
 return doc,actual,nil
}

func (d *Downloader) fetchApk391Categories(ctx context.Context,source string)([]nativeCategory,error){
 spec,ok:=apk391Spec(source)
 if !ok{return nil,errors.New("未知站源")}
 // Extracted native category tables are useful even when a custom
 // website/API adapter is not yet available for live catalog playback.
 if choices:=apk391StaticCategoryFallback[source];len(choices)>0{
  return append([]nativeCategory(nil),choices...),nil
 }
 if spec.Kind!="maccms"{return nil,fmt.Errorf("%s 需要独立 %s 分类协议，尚未完成移植",spec.Name,spec.Kind)}
 doc,_,err:=d.apk391Page(ctx,source,"/")
 if err!=nil{return nil,err}
 seen:=map[string]bool{};items:=[]nativeCategory{}
 for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"}){
  link:=apk391SameSite(source,providerHTMLAttr(a,"href"))
  if link==""{continue}
  m:=apk391CatPattern.FindStringSubmatch(link);if len(m)<2||seen[m[1]]{continue}
  name:=strings.TrimSpace(providerHTMLText(a))
  if name==""||name=="全部"||len([]rune(name))>28{continue}
  seen[m[1]]=true
  items=append(items,nativeCategory{ID:m[1],Name:name})
  if len(items)>=48{break}
 }
 if len(items)==0{return nil,errors.New("网页没有取得有效分类")}
 return items,nil
}

// A category/nav item can link to a video-shaped path, so a URL match
// alone is not evidence of a drama card. This prevents category labels from
// appearing as fake films before a detail-page refresh.
func apk391CategoryLabel(source, value string) bool {
 value = strings.Join(strings.Fields(value), "")
 if value == "" { return true }
 switch value {
 case "全部", "首页", "首頁", "分类", "分類", "更多", "最新", "最新上傳", "最新上传", "國產自拍", "国产自拍", "国产传媒", "國產傳媒", "探花系列", "人妻熟女":
  return true
 }
 for _,category := range apk391StaticCategoryFallback[source] {
  if strings.EqualFold(strings.Join(strings.Fields(category.Name),""),value) { return true }
 }
 return false
}

func apk391InsideNavigation(node *nethtml.Node) bool {
 for p:=node.Parent;p!=nil;p=p.Parent {
  switch p.Data {
  case "nav","header","footer": return true
  }
  if p.Data=="body" {break}
  for _,class:=range []string{"module-play-list-content","playlist","play-list","module-play-list","module-play-list-group","breadcrumb","module-class","module-type","category-list","type-list"} {
   if lujj31HasClass(p,class){return true}
  }
 }
 return false
}

// Only a poster directly associated with this video is trustworthy. The
// generic extractor searches a parent subtree, which can accidentally use
// the previous film's poster when multiple links share one container.
// Ningguo's title and film ID are already correct. Keep the poster scoped
// to the SAME film link; never borrow an image from a surrounding grid/card.
func apk391NingguoPoster(anchor *nethtml.Node,link string) string {
 for _,img:=range providerHTMLNodes(anchor,func(n *nethtml.Node)bool{return n.Data=="img"}) {
  raw:=firstNonEmpty(providerHTMLAttr(img,"data-original"),providerHTMLAttr(img,"data-src"),providerHTMLAttr(img,"data-lazy"),providerHTMLAttr(img,"src"))
  if raw==""{continue}
  cover:=lujj31PosterAddress(raw,link)
  if cover!=""{return cover}
 }
 return ""
}

func apk391ScopedPoster(anchor *nethtml.Node, source, link string) string {
 if source=="ningguo"||source=="mangguo"{return apk391NingguoPoster(anchor,link)}
 if poster:=lujj31CardPoster(anchor,link);poster!="" {return poster}
 id:=apk391VideoID(link)
 for parent,depth:=anchor.Parent,0;parent!=nil&&depth<3;parent,depth=parent.Parent,depth+1 {
  if parent.Data=="body"||parent.Data=="section"{break}
  ownLinks:=0
  for _,a:=range providerHTMLNodes(parent,func(n *nethtml.Node)bool{return n.Data=="a"}) {
   if v:=apk391VideoID(apk391SameSite(source,providerHTMLAttr(a,"href")));v!=""{
    if v!=id{return ""}
    ownLinks++
   }
  }
  if ownLinks==1 {
   return lujj31CardPoster(parent,link)
  }
 }
 return ""
}

// Luguo/Liguo/Ningguo can have list cards with titles or covers from an adjacent film.
// Each displayed item must be verified against its OWN video detail URL.
func apk391RequiresDetail(source string) bool { return source=="luguo" || source=="liguo" }

// A video duration or a taxonomy badge is not a title, even when it appears
// in a poster link's title/alt attribute.
var apk391TimeLabel = regexp.MustCompile(`^[0-9]{1,2}:[0-9]{2}(?::[0-9]{2})?$`)

func apk391BadTitle(source,title string)bool{
 title=strings.TrimSpace(title)
 if apk391CategoryLabel(source,title)||apk391TimeLabel.MatchString(title){return true}
 switch strings.ToLower(title){
 case "热门","熱門","hot","推荐","推薦","new","更新","最新","播放","观看","觀看","视频","影片","video","more":
  return true
 }
 return false
}

func apk391CardTitle(anchor *nethtml.Node, source string) string {
 // Image alt/title takes priority over text from adjacent taxonomy badges.
 for _,img:=range providerHTMLNodes(anchor,func(n *nethtml.Node)bool{return n.Data=="img"}) {
  for _,attribute:=range []string{"alt","title"} {
   title:=strings.TrimSpace(providerHTMLAttr(img,attribute))
   if title!="" && !apk391BadTitle(source,title){return truncate(title,512)}
  }
 }
 for _,attribute:=range []string{"title","data-title"}{
  title:=strings.TrimSpace(providerHTMLAttr(anchor,attribute))
  if title!=""&&!apk391BadTitle(source,title){return truncate(title,512)}
 }
 for _,root:=range []*nethtml.Node{anchor,anchor.Parent} {
  if root==nil {continue}
  for _,n:=range providerHTMLNodes(root,func(n *nethtml.Node)bool{
   return n.Data=="h2"||n.Data=="h3"||n.Data=="h4"||
    lujj31HasClass(n,"module-poster-item-title")||lujj31HasClass(n,"video-title")
  }){
   title:=strings.TrimSpace(providerHTMLText(n))
   if title!=""&&!apk391BadTitle(source,title){return truncate(title,512)}
  }
 }
 title:=strings.TrimSpace(providerHTMLText(anchor))
 if apk391BadTitle(source,title){return ""}
 return truncate(title,512)
}

// Parse the actual luyitian.com wntheme15 card structure (user-supplied HTML).
// The poster anchor contains badges/durations, while the REAL title is in
// the sibling .detail h6.title anchor. Never merge metadata across cards.
// Keep the verified catalog title while navigating to a play/detail page.
// Some wntheme player pages omit a movie heading entirely; a synthetic
// "影片 {id}" must never replace the catalog's real name.
var apk391CatalogNames sync.Map // source:id -> catalog title
var apk391CatalogCovers sync.Map // source:id -> verified catalog poster

func apk391RememberTitle(source,id,title string) {
 title=strings.TrimSpace(title)
 if (source=="luguo"||source=="liguo"||source=="juguo"||source=="zaoguo"||source=="mangguo") && apk391Numeric.MatchString(id) && title!="" && !apk391BadTitle(source,title) && title!="影片 "+id {
  apk391CatalogNames.Store(source+":"+id,truncate(title,512))
 }
}

func apk391RememberedTitle(source,id string)string {
 if title,ok:=apk391CatalogNames.Load(source+":"+id);ok {return title.(string)}
 return ""
}

func apk391RememberCover(source,id,cover string) {
 if (source=="zaoguo"||source=="juguo"||source=="ningguo"||source=="mangguo") && apk391Numeric.MatchString(id) && isProviderHTTPMediaURL(cover){
  apk391CatalogCovers.Store(source+":"+id,cover)
 }
}
func apk391RememberedCover(source,id string)string {
 if value,ok:=apk391CatalogCovers.Load(source+":"+id);ok{return value.(string)}
 return ""
}

func apk391LuguoCards(doc *nethtml.Node) []Drama {
 spec,_:=apk391Spec("luguo")
 out:=[]Drama{};seen:=map[string]bool{}
 for _,card:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="div"&&lujj31HasClass(n,"video-img-box")}) {
  var posterAnchor *nethtml.Node
  for _,box:=range providerHTMLNodes(card,func(n *nethtml.Node)bool{return lujj31HasClass(n,"img-box")}) {
   for _,a:=range providerHTMLNodes(box,func(n *nethtml.Node)bool{return n.Data=="a"}) {
    if apk391VideoID(apk391SameSite("luguo",providerHTMLAttr(a,"href")))!="" {posterAnchor=a;break}
   }
   if posterAnchor!=nil{break}
  }
  if posterAnchor==nil{continue}
  link:=apk391SameSite("luguo",providerHTMLAttr(posterAnchor,"href"))
  id:=apk391VideoID(link)
  if id==""||seen[id]{continue}
  var title string
  for _,detail:=range providerHTMLNodes(card,func(n *nethtml.Node)bool{return lujj31HasClass(n,"detail")}) {
   for _,h:=range providerHTMLNodes(detail,func(n *nethtml.Node)bool{return n.Data=="h6"&&lujj31HasClass(n,"title")}) {
    for _,a:=range providerHTMLNodes(h,func(n *nethtml.Node)bool{return n.Data=="a"}) {
     if apk391VideoID(apk391SameSite("luguo",providerHTMLAttr(a,"href")))!=id{continue}
     candidate:=strings.TrimSpace(providerHTMLText(a))
     if !apk391BadTitle("luguo",candidate){title=truncate(candidate,512);break}
    }
    if title!=""{break}
   }
   if title!=""{break}
  }
  if title==""{continue}
  cover:=""
  for _,img:=range providerHTMLNodes(posterAnchor,func(n *nethtml.Node)bool{return n.Data=="img"}) {
   cover=lujj31PosterAddress(firstNonEmpty(providerHTMLAttr(img,"data-src"),providerHTMLAttr(img,"data-original"),providerHTMLAttr(img,"src")),link)
   if cover!=""{break}
  }
  if cover==""{continue}
  seen[id]=true
  apk391RememberTitle("luguo",id,title)
  out=append(out,Drama{ID:providerDramaID("luguo",id),Source:"luguo",SourceID:id,Title:title,Name:title,Cover:cover,CoverURL:cover,ChannelName:spec.Name})
  if len(out)>=150{break}
 }
 return out
}

// The user-supplied ririlu.cc homepage uses wntheme27: each anchor.card
// owns its img[data-src] and .desc, unlike Luguo's sibling title node.
func apk391LiguoCards(doc *nethtml.Node) []Drama {
 spec,_:=apk391Spec("liguo")
 out:=[]Drama{};seen:=map[string]bool{}
 for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"&&lujj31HasClass(n,"card")}) {
  link:=apk391SameSite("liguo",providerHTMLAttr(a,"href"))
  id:=apk391VideoID(link)
  if id==""||seen[id]{continue}
  title:=""
  for _,desc:=range providerHTMLNodes(a,func(n *nethtml.Node)bool{return lujj31HasClass(n,"desc")}) {
   candidate:=strings.TrimSpace(providerHTMLText(desc))
   if candidate!=""&&!apk391BadTitle("liguo",candidate){title=truncate(candidate,512);break}
  }
  if title==""{continue}
  cover:=""
  for _,img:=range providerHTMLNodes(a,func(n *nethtml.Node)bool{return n.Data=="img"}) {
   raw:=firstNonEmpty(providerHTMLAttr(img,"data-src"),providerHTMLAttr(img,"data-original"))
   if raw==""{continue}
   cover=lujj31PosterAddress(raw,link)
   if cover!=""{break}
  }
  if cover==""{continue}
  seen[id]=true
  apk391RememberTitle("liguo",id,title)
  out=append(out,Drama{ID:providerDramaID("liguo",id),Source:"liguo",SourceID:id,Title:title,Name:title,Cover:cover,CoverURL:cover,ChannelName:spec.Name})
  if len(out)>=150{break}
 }
 return out
}

// Zaoguo's site labels the sidebar as 目录: these are video categories.
// The supplied new entry point is /yhsp/; preserve the MacCMS index.php/vod/type/id/<category>.html
// and card links in the /yhsp/index.php/vod/play/id/<id>/sid/1/nid/1.html form.
func apk391ZaoguoCards(doc *nethtml.Node) []Drama {
 spec,_:=apk391Spec("zaoguo")
 seen:=map[string]bool{};out:=[]Drama{}
 for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"&&lujj31HasClass(n,"thumbnail")}) {
  link:=apk391SameSite("zaoguo",providerHTMLAttr(a,"href"))
  id:=apk391VideoID(link)
  if id==""||seen[id]{continue}
  title:=""
  for _,n:=range providerHTMLNodes(a,func(n *nethtml.Node)bool{return lujj31HasClass(n,"video-title")}) {
   if t:=strings.TrimSpace(providerHTMLText(n));t!=""&&!apk391BadTitle("zaoguo",t){title=truncate(t,512);break}
  }
  if title=="" {title=strings.TrimSpace(providerHTMLAttr(a,"title"))}
  if title==""||apk391BadTitle("zaoguo",title){continue}
  cover:=""
  for _,img:=range providerHTMLNodes(a,func(n *nethtml.Node)bool{return n.Data=="img"}) {
   raw:=firstNonEmpty(providerHTMLAttr(img,"data-original"),providerHTMLAttr(img,"data-src"),providerHTMLAttr(img,"src"))
   if raw!="" {cover=lujj31PosterAddress(raw,link);if cover!=""{break}}
  }
  if cover==""{continue}
  seen[id]=true
  apk391RememberTitle("zaoguo",id,title)
  apk391RememberCover("zaoguo",id,cover)
  out=append(out,Drama{ID:providerDramaID("zaoguo",id),Source:"zaoguo",SourceID:id,Title:title,Name:title,Cover:cover,CoverURL:cover,ChannelName:spec.Name})
  if len(out)>=150{break}
 }
 return out
}

// Juguo's actual video7 template keeps the poster and title in sibling
// dt.preview-item and dd elements under a shared dl. The advertisement
// entries use javascript: links and are intentionally skipped.
func apk391JuguoCards(doc *nethtml.Node) []Drama {
 spec,_:=apk391Spec("juguo")
 seen:=map[string]bool{};out:=[]Drama{}
 for _,item:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="dl"}) {
  var link,id,title,cover string
  for _,a:=range providerHTMLNodes(item,func(n *nethtml.Node)bool{return n.Data=="a"}) {
   absolute:=apk391SameSite("juguo",providerHTMLAttr(a,"href"))
   candidateID:=apk391VideoID(absolute)
   if candidateID==""{continue}
   if id!="" && id!=candidateID{continue}
   id=candidateID;link=absolute
   for _,h:=range providerHTMLNodes(a,func(n *nethtml.Node)bool{return n.Data=="h3"}) {
    candidate:=strings.TrimSpace(providerHTMLText(h))
    if candidate!=""&&!apk391BadTitle("juguo",candidate){title=truncate(candidate,512);break}
   }
   for _,img:=range providerHTMLNodes(a,func(n *nethtml.Node)bool{return n.Data=="img"}) {
    raw:=firstNonEmpty(providerHTMLAttr(img,"data-original"),providerHTMLAttr(img,"data-src"))
    if raw!="" {cover=lujj31PosterAddress(raw,link);break}
   }
  }
  if id==""||title==""||cover==""||seen[id]{continue}
  seen[id]=true
  apk391RememberTitle("juguo",id,title)
  apk391RememberCover("juguo",id,cover)
  out=append(out,Drama{ID:providerDramaID("juguo",id),Source:"juguo",SourceID:id,Title:title,Name:title,Cover:cover,CoverURL:cover,ChannelName:spec.Name})
  if len(out)>=150{break}
 }
 return out
}

func apk391Cards(doc *nethtml.Node,source string)[]Drama{
 if source=="juguo"{if cards:=apk391JuguoCards(doc);len(cards)>0{return cards}}
 if source=="zaoguo"{if rows:=apk391ZaoguoCards(doc);len(rows)>0{return rows}}
 if source=="liguo"{if cards:=apk391LiguoCards(doc);len(cards)>0{return cards}}
 if source=="luguo"{if items:=apk391LuguoCards(doc);len(items)>0{return items}}
 spec,ok:=apk391Spec(source);if !ok{return nil}
 seen:=map[string]bool{};items:=[]Drama{}
 for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"}){
  if apk391InsideNavigation(a){continue}
  link:=apk391SameSite(source,providerHTMLAttr(a,"href"))
  id:=apk391VideoID(link)
  if id==""||seen[id]{continue}
  title:=apk391CardTitle(a,source)
  if title==""{continue}
  cover:=apk391ScopedPoster(a,source,link)
  // Recovered source landing pages mix navigation, video links and categories.
  // Cards without real cover evidence frequently are category navigation.
  if cover==""{continue}
  // Luguo's list titles/posters are not reliably tied to each film.
  // Only verified per-ID detail metadata may appear in the Luguo catalog.
  if apk391RequiresDetail(source) {continue}
  seen[id]=true
  if source=="ningguo"||source=="mangguo"{apk391RememberCover(source,id,cover)}
  if source=="mangguo"{apk391RememberTitle(source,id,title)}
  items=append(items,Drama{ID:providerDramaID(source,id),Source:source,SourceID:id,
   Title:title,Name:title,Cover:cover,CoverURL:cover,ChannelName:spec.Name})
  if len(items)>=150{break}
 }
 return items
}

// The 0.2.391 list sometimes uses a taxonomy label as an item's title and
// provides only a placeholder cover. Check ONLY these uncertain items against
// their own detail pages, on demand, with a strict concurrency/time budget.
// This does not preload, save, or walk the full catalog.
type apk391Suspect struct{ id, page string }

func apk391Suspects(doc *nethtml.Node,source string,already []Drama) []apk391Suspect {
 if (source=="luguo" && len(apk391LuguoCards(doc))>0)||(source=="liguo" && len(apk391LiguoCards(doc))>0){return nil} // verified cards; no detail prefetch
 seen:=map[string]bool{}
 for _,d:=range already{seen[d.SourceID]=true}
 out:=make([]apk391Suspect,0,8)
 for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"}) {
  if apk391InsideNavigation(a){continue}
  page:=apk391SameSite(source,providerHTMLAttr(a,"href"))
  id:=apk391VideoID(page)
  if id==""||seen[id]{continue}
  rawTitle:=strings.TrimSpace(providerHTMLAttr(a,"title"))
  if rawTitle==""{rawTitle=strings.TrimSpace(providerHTMLText(a))}
  if rawTitle=="" && !apk391RequiresDetail(source){continue}
  cover:=apk391ScopedPoster(a,source,page)
  if !apk391RequiresDetail(source) && !apk391BadTitle(source,rawTitle) && cover!=""{continue}
  seen[id]=true
  out=append(out,apk391Suspect{id:id,page:page})
  limit:=8
  if apk391RequiresDetail(source) {limit=16}
  if len(out)>=limit {break}
 }
 return out
}

// For Luguo use poster metadata from the actual detail page, never an
// unrelated thumbnail somewhere else in that page's general document tree.
func apk391DetailPoster(doc *nethtml.Node,page,source string)string{
 // Ningguo detail pages can embed images from unrelated recommendations.
 // Prefer its verified list poster; when no list poster exists, accept only
 // explicitly declared detail-cover metadata, not arbitrary page images.
 if source=="ningguo"||source=="mangguo"{
  for _,node:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="meta"}){
   property:=strings.ToLower(firstNonEmpty(providerHTMLAttr(node,"property"),providerHTMLAttr(node,"name")))
   if property=="og:image"||property=="og:image:url"||property=="twitter:image"{
    if poster:=lujj31PosterAddress(providerHTMLAttr(node,"content"),page);poster!=""{return poster}
   }
  }
  return ""
 }
 if !apk391RequiresDetail(source){return lujj31DetailPoster(doc,page)}
 for _,node:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="meta"}) {
  property:=strings.ToLower(firstNonEmpty(providerHTMLAttr(node,"property"),providerHTMLAttr(node,"name")))
  if property=="og:image"||property=="og:image:url"||property=="twitter:image"{
   if poster:=lujj31PosterAddress(providerHTMLAttr(node,"content"),page);poster!=""{return poster}
  }
 }
 for _,class:=range []string{"detail-pic","video-info-pic","module-item-pic"}{
  if node:=providerHTMLFirstClass(doc,class);node!=nil{
   if poster:=lujj31CardPoster(node,page);poster!=""{return poster}
  }
 }
 return ""
}

func (d *Downloader) apk391EnrichOne(ctx context.Context,source string,entry apk391Suspect)(Drama,bool) {
 doc,actual,err:=d.apk391Page(ctx,source,entry.page)
 if err!=nil || apk391SameSite(source,actual)=="" || apk391VideoID(actual)!=entry.id{return Drama{},false}
 title:=""
 for _,tag:=range []string{"h1","h2"} {
  for _,n:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data==tag}) {
   if text:=strings.TrimSpace(providerHTMLText(n));text!=""&&!apk391BadTitle(source,text) {
    title=text
    break
   }
  }
  if title!=""{break}
 }
 if title=="" {
  for _,n:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="meta"}) {
   name:=strings.ToLower(firstNonEmpty(providerHTMLAttr(n,"property"),providerHTMLAttr(n,"name")))
   if name=="og:title"||name=="twitter:title" {
    if t:=strings.TrimSpace(providerHTMLAttr(n,"content"));t!=""&&!apk391BadTitle(source,t){
     title=t
     break
    }
   }
  }
 }
 cover:=apk391DetailPoster(doc,actual,source)
 if title==""||cover=="" {return Drama{},false}
 spec:=apk391ByID[source]
 return Drama{ID:providerDramaID(source,entry.id),Source:source,SourceID:entry.id,
  Title:truncate(title,512),Name:truncate(title,512),Cover:cover,CoverURL:cover,ChannelName:spec.Name},true
}

func (d *Downloader) apk391EnrichSuspects(ctx context.Context,source string,doc *nethtml.Node,real []Drama) []Drama {
 suspects:=apk391Suspects(doc,source,real)
 if len(suspects)==0{return real}
 timeout:=2*time.Second
 if apk391RequiresDetail(source) {timeout=3*time.Second}
 limited,cancel:=context.WithTimeout(ctx,timeout)
 defer cancel()
 results:=make([]Drama,len(suspects))
 valid:=make([]bool,len(suspects))
 parallel:=3
 if apk391RequiresDetail(source){parallel=6}
 slots:=make(chan struct{},parallel)
 var wg sync.WaitGroup
 for i,entry:=range suspects{
  if limited.Err()!=nil{break}
  wg.Add(1)
  go func(index int,item apk391Suspect){
   defer wg.Done()
   select{
   case slots<-struct{}{}:
    defer func(){<-slots}()
   case <-limited.Done():return
   }
   results[index],valid[index]=d.apk391EnrichOne(limited,source,item)
  }(i,entry)
 }
 wg.Wait()
 for i:=range results {
  if valid[i] {real=append(real,results[i])}
 }
 return real
}

// Return true only when the response advertises a real next page. Some
// sites serve the same HTML for every page, otherwise causing endless spins.
func apk391HasNextPage(doc *nethtml.Node,page int) bool {
 target:=strconv.Itoa(page+1)
 for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"}) {
  text:=strings.TrimSpace(providerHTMLText(a))
  label:=strings.ToLower(providerHTMLAttr(a,"aria-label"))
  rel:=strings.ToLower(providerHTMLAttr(a,"rel"))
  href:=strings.ToLower(providerHTMLAttr(a,"href"))
  if href=="" || href=="#" || strings.HasPrefix(href,"javascript:"){continue}
  // A "Next" label is not sufficient: sites often return the same page
  // with an unchanged link, producing an infinite loading loop.
  isTarget:=strings.Contains(href,"/page/"+target+".") ||
   strings.Contains(href,"/page/"+target+"/") ||
   strings.HasSuffix(href,"/page/"+target) ||
   strings.Contains(href,"-"+target+".html") ||
   strings.Contains(href,"page="+target)
  hasPageNumber:=regexp.MustCompile(`(?i)(/page/[0-9]+|-[0-9]+\.html|[?&]page=[0-9]+)`).MatchString(href)
  if hasPageNumber {if isTarget{return true};continue}
  if strings.Contains(rel,"next")||strings.Contains(label,"next") ||
   text=="下一页"||text=="下一頁"||text=="下页"||text=="下頁"||
   strings.EqualFold(text,"next") {return true}
 }
 return false
}

func (d *Downloader) fetchApk391CatalogPage(ctx context.Context,source string,page int,category,query string)([]Drama,bool,error){
 // Bound external requests and optional detail checks, preventing long spins.
 ctx,cancel:=context.WithTimeout(ctx,7*time.Second)
 defer cancel()
 spec,ok:=apk391Spec(source)
 if !ok{return nil,false,errors.New("未注册的来源")}
 if page<1||page>100000||len(query)>256||!apk391ValidCategory(source,category){
  return nil,false,errors.New("非法分类或分页参数")
 }

 if source=="chengguodj"{return d.fetchChengguoCatalog(ctx,page,category,query)}
 if spec.Kind!="maccms"{
  return nil,false,fmt.Errorf("%s 原生 %s 目录 API 尚未恢复，不会用其他来源假冒",spec.Name,spec.Kind)
 }
 routes:=[]string{}
 if (source=="jiaoguo" || source=="zhenguo") && query=="" {
  // Verified 0.2.391 Jiaoguo website: homepage has no video cards;
  // /index.php/vod/type/... returns 404, /vod/ returns 500.
  // Real catalogue cards live at /vodtype/<category>.html.
  cat:=category
  if cat==""{cat="1"}
  if !apk391Numeric.MatchString(cat){return nil,false,errors.New("分类编号无效")}
  path:=fmt.Sprintf("/vodtype/%s.html",cat)
  if page>1 {path=fmt.Sprintf("/vodtype/%s-%d.html",cat,page)}
  doc,_,err:=d.apk391Page(ctx,source,path)
  if err!=nil{return nil,false,err}
  cards:=d.apk391EnrichSuspects(ctx,source,doc,apk391Cards(doc,source))
  if len(cards)==0{return nil,false,fmt.Errorf("%s 分类页没有可验证的视频卡片: %s",spec.Name,path)}
  return cards,apk391HasNextPage(doc,page),nil
 }
 if source=="juguo" && query=="" {
  path:="/vod/"
  if category!="" {
   path=fmt.Sprintf("/vodtype/%s/",category)
   if page>1 {path=fmt.Sprintf("/vodtype/%s/page/%d/",category,page)}
  }else if page>1{return []Drama{},false,nil}
  doc,_,err:=d.apk391Page(ctx,source,path)
  if err!=nil{return nil,false,err}
  cards:=apk391JuguoCards(doc)
  if len(cards)>0 {return cards,apk391HasNextPage(doc,page),nil}
  return nil,false,fmt.Errorf("橘果页面无可核实的视频卡片: %s",path)
 }
 if source=="zaoguo" && query=="" {
  path:="/yhsp/"
  if category!="" {
   path=fmt.Sprintf("/yhsp/index.php/vod/type/id/%s.html",category)
   if page>1 {path=fmt.Sprintf("/yhsp/index.php/vod/type/id/%s/page/%d.html",category,page)}
  }else if page>1{
   // Homepage pagination is not attested in the supplied directory page.
   return []Drama{},false,nil
  }
  doc,_,err:=d.apk391Page(ctx,source,path)
  if err!=nil{return nil,false,err}
  items:=apk391ZaoguoCards(doc)
  if len(items)==0{return nil,false,fmt.Errorf("棗果目录页面无有效影片卡片: %s",path)}
  return items,apk391HasNextPage(doc,page),nil
 }
 if source=="liguo" && query=="" {
  if page>1{return []Drama{},false,nil} // No pagination link in supplied homepage.
  path:="/"
  if category!=""{path=fmt.Sprintf("/vodshow/%s-----------/",category)}
  doc,_,err:=d.apk391Page(ctx,source,path)
  if err!=nil{return nil,false,err}
  cards:=apk391LiguoCards(doc)
  if len(cards)>0{return cards,false,nil}
  // Keep the legacy per-video fallback for older MacCMS layouts and test fixtures.
  // If the website presents wntheme27 cards, use those directly without enrichment.
 }
 if source=="luguo" && query=="" {
  if category=="" && page==1 {routes=[]string{"/"}}
  if category!="" && page==1 {routes=[]string{fmt.Sprintf("/vodtype/%s/",category)}}
  // No page-2 URL was present in the supplied homepage HTML.
  if page>1{return []Drama{},false,nil}
 }
 if len(routes)>0 {
  doc,_,err:=d.apk391Page(ctx,source,routes[0]);if err!=nil{return nil,false,err}
  cards:=apk391LuguoCards(doc)
  if len(cards)>0{return cards,false,nil}
  // Older fixture/site templates may require the existing per-ID path.
  // Continue to the legacy route loop instead of discarding valid links.
 }
 if query!=""{
  if page>1{return []Drama{},false,nil}
  routes=[]string{"/index.php/vod/search/wd/"+url.PathEscape(query)+".html"}
 }else if category==""{
  if page==1{routes=[]string{"/","/index.php/vod/show/id/1.html"}}
  if page>1{routes=[]string{fmt.Sprintf("/index.php/vod/show/id/1/page/%d.html",page),
   fmt.Sprintf("/vodtype/1-%d.html",page)}}
 }else{
  if page==1{
   routes=[]string{fmt.Sprintf("/index.php/vod/type/id/%s.html",category),
    fmt.Sprintf("/vodtype/%s.html",category),
    fmt.Sprintf("/index.php/vod/show/id/%s.html",category)}
  } else {
   routes=[]string{fmt.Sprintf("/index.php/vod/type/id/%s/page/%d.html",category,page),
    fmt.Sprintf("/vodtype/%s-%d.html",category,page)}
  }
 }
 var failure error
 // The third fallback may block for a long time and usually mirrors the
 // other MacCMS paths; avoid repeating it in the same page request.
 if len(routes)>2 {routes=routes[:2]}
 for _,path:=range routes{
  if err:=ctx.Err();err!=nil{return nil,false,fmt.Errorf("%s 分页请求超时，请重试: %w",spec.Name,err)}
  doc,_,err:=d.apk391Page(ctx,source,path)
  if err!=nil{failure=err;continue}
  rows:=d.apk391EnrichSuspects(ctx,source,doc,apk391Cards(doc,source))
  if len(rows)>0{return rows,query=="" && apk391HasNextPage(doc,page),nil}
  failure=fmt.Errorf("%s 网页无影片：%s",spec.Name,path)
 }
 if failure==nil{failure=errors.New("无可用页面地址")}
 return nil,false,failure
}

// The two routes below are extracted from the original 0.2.391 ARM64
// maccmsDetailCandidates branch shared by luguo and liguo (0x2e3ed18).
// Keep other channels' existing fallback behavior untouched.
func apk391DetailRoutes(source,id string) []string {
 if source=="juguo"{return []string{fmt.Sprintf("/voddetail/%s/",id),fmt.Sprintf("/vodplay/%s-1-1/",id)}}
 if source=="zaoguo"{return []string{fmt.Sprintf("/yhsp/index.php/vod/play/id/%s/sid/1/nid/1.html",id)}}
 if source=="luguo"||source=="liguo" {
  return []string{fmt.Sprintf("/vodplay/%s-1-1/",id),
   fmt.Sprintf("/vodplay/%s-1-1.html",id)}
 }
 return []string{fmt.Sprintf("/index.php/vod/detail/id/%s.html",id),
  fmt.Sprintf("/voddetail/%s.html",id),
  fmt.Sprintf("/vodplay/%s-1-1.html",id),
  fmt.Sprintf("/index.php/vod/play/id/%s/sid/1/nid/1.html",id)}
}

func (d *Downloader) fetchApk391Detail(ctx context.Context,source,id string)(Drama,[]Chapter,error){
 if source=="chengguodj"{return d.fetchChengguoDetail(ctx,id)}
 spec,ok:=apk391Spec(source)
 if !ok||!apk391Numeric.MatchString(id){return Drama{},nil,errors.New("非法影片编号")}

 if spec.Kind!="maccms"{return Drama{},nil,fmt.Errorf("%s 需要专用详情接口",spec.Name)}
 paths:=apk391DetailRoutes(source,id)
 var failure error
 for _,path:=range paths{
  doc,actual,err:=d.apk391Page(ctx,source,path)
  if err!=nil{failure=err;continue}
  chapters:=[]Chapter{};seen:=map[string]bool{}
  for _,a:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="a"}){
   link:=apk391SameSite(source,providerHTMLAttr(a,"href"))
   if link==""||seen[link]||apk391VideoID(link)!=id{continue}
   pu,_:=url.Parse(link)
   if !strings.Contains(strings.ToLower(pu.Path),"play"){continue}
   seen[link]=true
   n:=len(chapters)+1
   title:=providerHTMLText(a);if title==""{title=fmt.Sprintf("第%d集",n)}
   chapters=append(chapters,Chapter{ID:providerChapterID(source,id,strconv.Itoa(n)),
    Source:source,Title:title,CurrentEpisode:rawEpisode(n),PageURL:link,
    VideoURL:"apk391-play://"+link,Referer:apk391BaseURL(source)+"/"})
   if len(chapters)>=500{break}
  }
  if len(chapters)==0{
   found:=false
   for _,node:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="script"}){
    for child:=node.FirstChild;child!=nil;child=child.NextSibling{
     if child.Type==nethtml.TextNode&&lujj31PlayerData.MatchString(child.Data){found=true;break}
    }
    if found{break}
   }
   if !found{failure=errors.New("未找到当前影片的分集或播放器");continue}
   chapters=[]Chapter{{ID:providerChapterID(source,id,"1"),Source:source,Title:"第1集",
    CurrentEpisode:rawEpisode(1),PageURL:actual,VideoURL:"apk391-play://"+actual,
    Referer:apk391BaseURL(source)+"/"}}
  }
  title:=apk391RememberedTitle(source,id)
  if title=="" {
   for _,h:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="h1"}){
    if str:=strings.TrimSpace(providerHTMLText(h));str!="" && !apk391BadTitle(source,str) {title=truncate(str,512);break}
   }
  }
  if title=="" {
   for _,meta:=range providerHTMLNodes(doc,func(n *nethtml.Node)bool{return n.Data=="meta"}) {
    name:=strings.ToLower(firstNonEmpty(providerHTMLAttr(meta,"property"),providerHTMLAttr(meta,"name")))
    if name=="og:title"||name=="twitter:title" {
     candidate:=strings.TrimSpace(providerHTMLAttr(meta,"content"))
     if candidate!=""&&!apk391BadTitle(source,candidate){title=truncate(candidate,512);break}
    }
   }
  }
  if title==""{title="影片 "+id}
  // Keep the same per-film poster selection on the detail/return path:
  // a generic image from the page must not replace the verified catalog art.
  cover:=apk391RememberedCover(source,id)
  if cover==""{cover=apk391DetailPoster(doc,actual,source)}
  return Drama{ID:providerDramaID(source,id),Source:source,SourceID:id,
   Title:title,Name:title,Cover:cover,CoverURL:cover,
   EpisodeCount:len(chapters),TotalEpisode:len(chapters),ChannelName:spec.Name},chapters,nil
 }
 if failure==nil{failure=errors.New("影片详情不可用")}
 return Drama{},nil,failure
}

func (d *Downloader) resolveApk391Media(ctx context.Context,task Task)(providerMedia,error){
 source,id,ok:=splitProviderDramaID(task.DramaID)

 if ok&&source=="chengguodj"{return d.resolveChengguoMedia(ctx,task)}
 spec,exists:=apk391Spec(source)
 if !ok||!exists||spec.Kind!="maccms"||!apk391Numeric.MatchString(id)||
  !strings.HasPrefix(task.Chapter.ID,providerChapterID(source,id,"")){
  return providerMedia{},errors.New("无效影片来源或分集编号")
 }
 page:=apk391SameSite(source,task.Chapter.PageURL)
 // Some zaoguo entries have no stored chapter page URL when resuming
 // from a catalog, despite having a valid source-scoped chapter ID.
 // Reconstruct only this known original play route, never accept a URL
 // pointing to another film or an external host.
 if source=="zaoguo" && (page=="" || apk391VideoID(page)!=id) {
  // The saved chapter URL can be the legacy detail URL, a relative URL
  // resolved against the wrong directory, or a non-HTTP play token.
  // Rebuild exclusively from the verified source-scoped drama ID.
  // Never fetch or trust the mismatched page URL.
  page=apk391SameSite(source,fmt.Sprintf("/yhsp/index.php/vod/play/id/%s/sid/1/nid/1.html",id))
 }
 if page==""||apk391VideoID(page)!=id{return providerMedia{},errors.New("分集播放页来源不一致")}
 ua:=context.WithValue(ctx,providerTextUserAgentKey{},lujj31UserAgent)
 body,err:=d.fetchProviderText(ua,page,apk391BaseURL(source)+"/")
 if err!=nil{return providerMedia{},err}
 fields:=lujj31PlayerFields(body,id)
 address,err:=lujj31PlayerURL(body,id)
 if err==nil&&isProviderHTTPMediaURL(address){
  return d.prepareWebProviderMedia(ctx,providerMedia{URL:address,Referer:page},spec.Name)
 }
 if token:=fields["url"];token!=""&&!isProviderHTTPMediaURL(token){
  parsed:=d.maccmsSiteCloudParse(ctx,apk391BaseURL(source),body,token,page)
  if isProviderHTTPMediaURL(parsed){
   return d.prepareWebProviderMedia(ctx,providerMedia{URL:parsed,Referer:page},spec.Name)
  }
 }
 if err!=nil{return providerMedia{},err}
 return providerMedia{},errors.New("站源没有解析到可播放媒体")
}
