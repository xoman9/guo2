package custom

import (
 "context"
 "errors"
 "fmt"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "time"
 "duanjuapp/native/core"
 "golang.org/x/net/html"
)
const jableSource="jable"
const jableBase="https://jable.tv"
var jableID=regexp.MustCompile(`^[A-Za-z0-9_-]{1,120}$`)
var jableHLS=regexp.MustCompile(`(?i)\bhlsUrl\s*[:=]\s*["']([^"']+)["']`)
var jableDesc=core.SourceDescriptor{ID:jableSource,Name:"Jable+",BaseURL:jableBase,DefaultOff:true,NoHostRotation:true,Description:"TVBox csp_Jable HTML解析；网站验证须由用户完成",ValidCategory:func(s string)bool{return s==""||jableID.MatchString(s)}}
type jableProvider struct{host core.Host}
var _ core.Provider=(*jableProvider)(nil)
var _ core.Searcher=(*jableProvider)(nil)
func init(){core.Register(jableDesc,func(h core.Host)core.Provider{return &jableProvider{host:h}})}
func(p *jableProvider)Descriptor()core.SourceDescriptor{return jableDesc}
func(p *jableProvider)BaseURL()string{return jableBase}
func jableCards(doc *html.Node)[]core.Drama{
 var out []core.Drama
 seen:=map[string]bool{}
 for _,box:=range tvNodes(doc,"div","video-img-box"){
  href:=tvAttr(box,"a","","href");u,e:=url.Parse(tvSafeURL(jableBase+"/",href))
  if e!=nil||u.Hostname()!="jable.tv"{continue}
  seg:=strings.Split(strings.Trim(u.Path,"/"),"/")
  if len(seg)!=2||seg[0]!="videos"||!jableID.MatchString(seg[1])||seen[seg[1]]{continue}
  name:=tvText(box,"h6","")
  pic:=tvAttr(box,"img","","data-src");if pic==""{pic=tvAttr(box,"img","","src")}
  if name==""||strings.HasSuffix(strings.ToLower(pic),".gif"){continue}
  pic=tvSafeURL(jableBase+"/",pic)
  if pic==""{continue}
  id:=seg[1];seen[id]=true
  out=append(out,core.Drama{ID:core.ProviderDramaID(jableSource,id),Source:jableSource,SourceID:id,Title:name,Name:name,Cover:pic,CoverURL:pic,ChannelName:"Jable+",TotalEpisode:1})
 }
 return out
}
func(p *jableProvider)Categories(ctx context.Context,_ bool)([]core.Category,error){
 doc,err:=tvPage(p.host,ctx,jableBase+"/categories/",jableBase+"/")
 if err!=nil{return nil,err}
 out:=[]core.Category{{ID:"",Name:"全部"}}
 seen:=map[string]bool{}
 for _,box:=range tvNodes(doc,"div","img-box"){
  href:=tvAttr(box,"a","","href");u,e:=url.Parse(tvSafeURL(jableBase+"/",href))
  if e!=nil||u.Hostname()!="jable.tv"{continue}
  parts:=strings.Split(strings.Trim(u.Path,"/"),"/")
  if len(parts)!=2||parts[0]!="categories"||!jableID.MatchString(parts[1])||seen[parts[1]]{continue}
  name:=tvText(box,"h4","")
  if name==""{continue};seen[parts[1]]=true
  out=append(out,core.Category{ID:parts[1],Name:name})
 }
 if len(out)<2{return nil,errors.New("Jable+ 分类为空，网页结构可能变化或需要浏览器验证")}
 return out,nil
}
func(p *jableProvider)CatalogPage(ctx context.Context,page int,category string)([]core.Drama,bool,error){
 if page<1||page>10000||(category!=""&&!jableID.MatchString(category)){return nil,false,errors.New("Jable+ 分类或页码无效")}
 target:=jableBase+"/"
 if category!="" {
  q:=url.Values{"mode":{"async"},"function":{"get_block"},"block_id":{"list_videos_common_videos_list"},"sort_by":{"post_date"},"from":{fmt.Sprintf("%02d",page)},"_":{strconv.FormatInt(time.Now().UnixMilli(),10)}}
  target=jableBase+"/categories/"+category+"/?"+q.Encode()
 }else if page>1{target=jableBase+"/latest-updates/?page="+strconv.Itoa(page)}
 doc,err:=tvPage(p.host,ctx,target,jableBase+"/")
 if err!=nil{return nil,false,err}
 cards:=jableCards(doc)
 if len(cards)==0&&page==1{return nil,false,errors.New("Jable+ 未取得影片，网页可能要求验证或解析规则已变更")}
 return cards,len(cards)>=20,nil
}
func(p *jableProvider)Search(ctx context.Context,page int,query string)([]core.Drama,bool,error){
 if page<1||page>10000||len(query)>200{return nil,false,errors.New("Jable+ 搜索参数无效")}
 if strings.TrimSpace(query)==""{return nil,false,nil}
 target:=jableBase+"/search/"+url.PathEscape(strings.TrimSpace(query))+"/"
 if page>1{target+="?page="+strconv.Itoa(page)}
 doc,err:=tvPage(p.host,ctx,target,jableBase+"/")
 if err!=nil{return nil,false,err}
 cards:=jableCards(doc);return cards,len(cards)>=20,nil
}
func(p *jableProvider)Detail(ctx context.Context,id string)(core.Drama,[]core.Chapter,error){
 if !jableID.MatchString(id){return core.Drama{},nil,errors.New("Jable+ ID 无效")}
 page:=jableBase+"/videos/"+id+"/"
 doc,err:=tvPage(p.host,ctx,page,jableBase+"/")
 if err!=nil{return core.Drama{},nil,err}
 title,pic:=tvMeta(doc,"og:title"),tvSafeURL(page,tvMeta(doc,"og:image"))
 if title==""{return core.Drama{},nil,errors.New("Jable+ 详情没有影片标题，请检查网站验证状态")}
 drama:=core.Drama{ID:core.ProviderDramaID(jableSource,id),Source:jableSource,SourceID:id,Title:title,Name:title,Cover:pic,CoverURL:pic,ChannelName:"Jable+",TotalEpisode:1}
 chapter:=core.Chapter{ID:core.ProviderChapterID(jableSource,id,"1"),Source:jableSource,Title:"播放",CurrentEpisode:core.RawEpisode(1),PageURL:page,Referer:page}
 return drama,[]core.Chapter{chapter},nil
}
func(p *jableProvider)ResolveMedia(ctx context.Context,task core.Task)(core.ProviderMedia,error){
 source,id,ok:=core.SplitProviderDramaID(task.DramaID)
 if !ok||source!=jableSource||!jableID.MatchString(id)||task.Chapter.ID!=core.ProviderChapterID(jableSource,id,"1"){return core.ProviderMedia{},errors.New("Jable+ 播放项无效")}
 page:=jableBase+"/videos/"+id+"/"
 doc,err:=tvPage(p.host,ctx,page,jableBase+"/")
 if err!=nil{return core.ProviderMedia{},err}
 var script strings.Builder
 for _,node:=range tvNodes(doc,"script",""){for child:=node.FirstChild;child!=nil;child=child.NextSibling{if child.Type==html.TextNode{script.WriteString(child.Data);script.WriteByte('\n')}}}
 match:=jableHLS.FindStringSubmatch(script.String())
 if len(match)<2{return core.ProviderMedia{},errors.New("Jable+ 未取得 hlsUrl，可能需要浏览器验证或播放脚本已变更")}
 address:=tvSafeURL(page,strings.ReplaceAll(match[1],`\/`, "/"))
 if !core.IsProviderHTTPMediaURL(address){return core.ProviderMedia{},errors.New("Jable+ hlsUrl 不合法")}
 return p.host.PrepareWebMedia(ctx,core.ProviderMedia{URL:address,Referer:page},"Jable+")
}
