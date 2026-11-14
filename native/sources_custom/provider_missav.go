package custom

import (
 "context"
 "errors"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "duanjuapp/native/core"
 "golang.org/x/net/html"
)
const missSource="missav"
const missBase="https://missav.com"
var missPath=regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/_-]{0,160}$`)
var missDesc=core.SourceDescriptor{ID:missSource,Name:"MissAV+",BaseURL:missBase,DefaultOff:true,NoHostRotation:true,Description:"TVBox csp_Miss 分类及详情解析；播放页需合法的独立解析器",ValidCategory:func(s string)bool{return s==""||missPath.MatchString(s)}}
type missProvider struct{host core.Host}
var _ core.Provider=(*missProvider)(nil)
var _ core.Searcher=(*missProvider)(nil)
func init(){core.Register(missDesc,func(h core.Host)core.Provider{return &missProvider{host:h}})}
func(p *missProvider)Descriptor()core.SourceDescriptor{return missDesc}
func(p *missProvider)BaseURL()string{return missBase}
func missCards(doc *html.Node)[]core.Drama{
 var out []core.Drama
 seen:=map[string]bool{}
 for _,box:=range tvNodes(doc,"div","thumbnail"){
  href:=tvAttr(box,"a","text-secondary","href")
  address:=tvSafeURL(missBase+"/",href);u,e:=url.Parse(address)
  if e!=nil||u.Hostname()!="missav.com"{continue}
  id:=strings.Trim(u.Path,"/")
  if !missPath.MatchString(id)||seen[id]{continue}
  title:=tvText(box,"a","text-secondary")
  if title==""{continue}
  pic:=tvAttr(box,"img","","data-src")
  if pic==""{pic=tvAttr(box,"img","","src")}
  pic=tvSafeURL(missBase+"/",pic)
  seen[id]=true
  out=append(out,core.Drama{ID:core.ProviderDramaID(missSource,id),Source:missSource,SourceID:id,Title:title,Name:title,Cover:pic,CoverURL:pic,ChannelName:"MissAV+",TotalEpisode:1})
 }
 return out
}
func(p *missProvider)Categories(ctx context.Context,_ bool)([]core.Category,error){
 doc,err:=tvPage(p.host,ctx,missBase+"/",missBase+"/")
 if err!=nil{return nil,err}
 out:=[]core.Category{{ID:"",Name:"全部"}}
 seen:=map[string]bool{}
 for _,a:=range tvNodes(doc,"a","bg-nord3"){
  raw:=tvSafeURL(missBase+"/",core.ProviderHTMLAttr(a,"href"));u,e:=url.Parse(raw)
  if e!=nil||u.Hostname()!="missav.com"{continue}
  id:=strings.Trim(u.Path,"/")
  if !(strings.HasPrefix(id,"dm")||strings.Contains(strings.ToUpper(id),"VR"))||!missPath.MatchString(id)||seen[id]{continue}
  name:=core.ProviderHTMLText(a);if name==""{continue}
  seen[id]=true;out=append(out,core.Category{ID:id,Name:name})
 }
 if len(out)<2{return nil,errors.New("MissAV+ 分类为空，可能需要浏览器验证或网站结构已更改")}
 return out,nil
}
func(p *missProvider)catalog(ctx context.Context,page int,category,query string)([]core.Drama,bool,error){
 if page<1||page>10000||(category!=""&&!missPath.MatchString(category))||len(query)>200{return nil,false,errors.New("MissAV+ 列表参数无效")}
 target:=missBase+"/"
 if query!=""{target+= "search/"+url.PathEscape(query)}
 if query==""&&category!=""{target+=category}
 target+="?page="+strconv.Itoa(page)
 doc,err:=tvPage(p.host,ctx,target,missBase+"/")
 if err!=nil{return nil,false,err}
 cards:=missCards(doc)
 if page==1&&len(cards)==0{return nil,false,errors.New("MissAV+ 未取得影片，网页可能要求浏览器验证")}
 return cards,len(cards)>=20,nil
}
func(p *missProvider)CatalogPage(ctx context.Context,page int,category string)([]core.Drama,bool,error){return p.catalog(ctx,page,category,"")}
func(p *missProvider)Search(ctx context.Context,page int,query string)([]core.Drama,bool,error){
 query=strings.TrimSpace(query);if query==""{return nil,false,nil};return p.catalog(ctx,page,"",query)
}
func(p *missProvider)Detail(ctx context.Context,id string)(core.Drama,[]core.Chapter,error){
 if !missPath.MatchString(id){return core.Drama{},nil,errors.New("MissAV+ 影片 ID 无效")}
 page:=missBase+"/"+id
 doc,err:=tvPage(p.host,ctx,page,missBase+"/")
 if err!=nil{return core.Drama{},nil,err}
 title,pic:=tvMeta(doc,"og:title"),tvSafeURL(page,tvMeta(doc,"og:image"))
 if title==""{return core.Drama{},nil,errors.New("MissAV+ 未取得影片详情，可能需要浏览器验证")}
 drama:=core.Drama{ID:core.ProviderDramaID(missSource,id),Source:missSource,SourceID:id,Title:title,Name:title,Cover:pic,CoverURL:pic,ChannelName:"MissAV+",TotalEpisode:1}
 chapter:=core.Chapter{ID:core.ProviderChapterID(missSource,id,"1"),Source:missSource,Title:"播放",CurrentEpisode:core.RawEpisode(1),PageURL:page,Referer:page}
 return drama,[]core.Chapter{chapter},nil
}
func(p *missProvider)ResolveMedia(ctx context.Context,task core.Task)(core.ProviderMedia,error){
 source,id,ok:=core.SplitProviderDramaID(task.DramaID)
 if !ok||source!=missSource||!missPath.MatchString(id)||task.Chapter.ID!=core.ProviderChapterID(missSource,id,"1"){return core.ProviderMedia{},errors.New("MissAV+ 播放项无效")}
 return core.ProviderMedia{},errors.New("MissAV+ 的 TVBox csp_Miss 依赖独立网页解析器，当前 Go 核心无法直接将影片网页作为媒体播放；未提供有效直链")
}
