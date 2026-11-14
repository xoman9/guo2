package custom

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "duanjuapp/native/core"
)

const lgSource = "laosebizy.com"
const lgBase = "https://apilsbzy1.com"
const lgAPI = lgBase+"/api.php/provide/vod/"
var lgID = regexp.MustCompile("^[A-Za-z0-9_-]{1,100}$")
var lgCat = regexp.MustCompile("^[0-9]{1,12}$")
var lgDescriptor = core.SourceDescriptor{
 ID:lgSource, Name:"老果+", BaseURL:lgBase, DefaultOff:true, NoHostRotation:true,
 Description:"TVBox MacCMS JSON 站源；未經線上驗證",
 ValidCategory:func(s string)bool{return s==""||lgCat.MatchString(s)},
}
type laoguoTVBox struct{ host core.Host }
var _ core.Provider=(*laoguoTVBox)(nil)
var _ core.Searcher=(*laoguoTVBox)(nil)
func init(){core.Register(lgDescriptor,func(h core.Host)core.Provider{return &laoguoTVBox{host:h}})}
func (p *laoguoTVBox) Descriptor()core.SourceDescriptor{return lgDescriptor}
func (p *laoguoTVBox) BaseURL()string{return lgBase}
func lgStr(v any)string{
 switch x:=v.(type){
 case string:return strings.TrimSpace(x)
 case json.Number:return string(x)
 case float64:return strconv.FormatFloat(x,'f',-1,64)
 }
 return ""
}
func lgMap(v any)map[string]any{m,_:=v.(map[string]any);return m}
func lgRows(v any)[]any{rows,_:=v.([]any);return rows}
func lgInt(v any)(int,bool){
 n,err:=strconv.Atoi(lgStr(v))
 return n,err==nil&&n>=0&&n<=100000000
}
func (p *laoguoTVBox) request(ctx context.Context,args url.Values)(map[string]any,error){
 if args.Get("ac")!="list"&&args.Get("ac")!="detail"{return nil,errors.New("invalid TVBox action")}
 response,err:=p.host.FetchText(ctx,core.TextRequest{URL:lgAPI+"?"+args.Encode(),Referer:lgBase+"/"})
 if err!=nil{return nil,err}
 decoder:=json.NewDecoder(strings.NewReader(response.Body));decoder.UseNumber()
 var obj map[string]any
 if err:=decoder.Decode(&obj);err!=nil{return nil,fmt.Errorf("invalid TVBox JSON: %w",err)}
 if obj==nil{return nil,errors.New("empty TVBox JSON")}
 return obj,nil
}
func (p *laoguoTVBox) Categories(ctx context.Context,_ bool)([]core.Category,error){
 response,err:=p.request(ctx,url.Values{"ac":{"list"}})
 if err!=nil{return nil,err}
 categories:=[]core.Category{{ID:"",Name:"全部"}}
 seen:=map[string]bool{}
 for _,v:=range lgRows(response["class"]){
  item:=lgMap(v);id,name:=lgStr(item["type_id"]),lgStr(item["type_name"])
  if !lgCat.MatchString(id)||name==""||seen[id]{continue}
  seen[id]=true;categories=append(categories,core.Category{ID:id,Name:name})
  if len(categories)>300{return nil,errors.New("too many categories")}
 }
 if len(categories)==1{return nil,errors.New("TVBox categories unavailable")}
 return categories,nil
}
func lgDrama(item map[string]any)(core.Drama,error){
 id,title:=lgStr(item["vod_id"]),lgStr(item["vod_name"])
 if !lgID.MatchString(id)||title==""{return core.Drama{},errors.New("invalid TVBox video")}
 count,_:=lgInt(item["vod_total"])
 return core.Drama{
  ID:core.ProviderDramaID(lgSource,id),Source:lgSource,SourceID:id,
  Title:title,Name:title,ChannelName:"老果+",
  Cover:core.ResolveProviderURL(lgBase+"/",lgStr(item["vod_pic"])),
  TotalEpisode:count,Desc:lgStr(item["vod_content"]),
 },nil
}
func (p *laoguoTVBox) catalog(ctx context.Context,page int,category,keyword string)([]core.Drama,bool,error){
 if page<1||page>100000||(category!=""&&!lgCat.MatchString(category))||len(keyword)>256{
  return nil,false,errors.New("invalid TVBox list request")
 }
 args:=url.Values{"ac":{"list"},"pg":{strconv.Itoa(page)}}
 if category!=""{args.Set("t",category)}
 if keyword!=""{args.Set("wd",keyword)}
 response,err:=p.request(ctx,args)
 if err!=nil{return nil,false,err}
 list,ok:=response["list"].([]any)
 if !ok||len(list)>500{return nil,false,errors.New("invalid TVBox list")}
 items:=make([]core.Drama,0,len(list));seen:=map[string]bool{}
 for _,v:=range list{
  drama,err:=lgDrama(lgMap(v))
  if err!=nil{return nil,false,err}
  if !seen[drama.ID]{seen[drama.ID]=true;items=append(items,drama)}
 }
 more:=false
 if pages,ok:=lgInt(response["pagecount"]);ok&&pages>0{more=page<pages
 }else if total,ok:=lgInt(response["total"]);ok{
  if limit,ok:=lgInt(response["limit"]);ok&&limit>0{more=page*limit<total}
 }
 return items,more,nil
}
func (p *laoguoTVBox) CatalogPage(ctx context.Context,page int,category string)([]core.Drama,bool,error){
 return p.catalog(ctx,page,category,"")
}
func (p *laoguoTVBox) Search(ctx context.Context,page int,keyword string)([]core.Drama,bool,error){
 keyword=strings.TrimSpace(keyword)
 if keyword==""{return nil,false,nil}
 return p.catalog(ctx,page,"",keyword)
}
func lgChapters(id,raw,from string)[]core.Chapter{
 var result []core.Chapter
 lines:=strings.Split(raw,"$$$");names:=strings.Split(from,"$$$")
 for i,line:=range lines{
  label:=fmt.Sprintf("線路%d",i+1)
  if i<len(names)&&strings.TrimSpace(names[i])!=""{label=strings.TrimSpace(names[i])}
  for j,part:=range strings.Split(line,"#"){
   title,address,ok:=strings.Cut(strings.TrimSpace(part),"$")
   address=strings.TrimSpace(address)
   if !ok||!lgDirectMedia(address){continue}
   title=strings.TrimSpace(title)
   if title==""{title=fmt.Sprintf("第%d集",j+1)}
   result=append(result,core.Chapter{
    ID:core.ProviderChapterID(lgSource,id,fmt.Sprintf("%d-%d",i+1,j+1)),
    Source:lgSource,Title:label+" · "+title,
    CurrentEpisode:core.RawEpisode(j+1),PageURL:address,Referer:lgBase+"/",
   })
  }
 }
 return result
}
func lgDirectMedia(address string)bool{
 parsed,err:=url.Parse(address)
 if err!=nil||parsed.User!=nil||!core.IsProviderHTTPMediaURL(address){return false}
 path:=strings.ToLower(parsed.Path)
 return strings.HasSuffix(path,".mp4")||strings.HasSuffix(path,".m3u8")||
  strings.HasSuffix(path,".mpd")||strings.HasSuffix(path,".m4v")
}
func (p *laoguoTVBox) Detail(ctx context.Context,id string)(core.Drama,[]core.Chapter,error){
 if !lgID.MatchString(id){return core.Drama{},nil,errors.New("invalid TVBox video ID")}
 response,err:=p.request(ctx,url.Values{"ac":{"detail"},"ids":{id}})
 if err!=nil{return core.Drama{},nil,err}
 rows:=lgRows(response["list"])
 if len(rows)!=1{return core.Drama{},nil,errors.New("TVBox detail missing or ambiguous")}
 item:=lgMap(rows[0]);drama,err:=lgDrama(item)
 if err!=nil||drama.SourceID!=id{return core.Drama{},nil,errors.New("TVBox detail ID mismatch")}
 chapters:=lgChapters(id,lgStr(item["vod_play_url"]),lgStr(item["vod_play_from"]))
 if len(chapters)==0{return core.Drama{},nil,errors.New("TVBox direct episode links unavailable")}
 drama.TotalEpisode=len(chapters)
 return drama,chapters,nil
}
func (p *laoguoTVBox) ResolveMedia(ctx context.Context,task core.Task)(core.ProviderMedia,error){
 source,id,ok:=core.SplitProviderDramaID(task.DramaID)
 if !ok||source!=lgSource||!lgID.MatchString(id){return core.ProviderMedia{},errors.New("invalid TVBox drama")}
 _,chapters,err:=p.Detail(ctx,id)
 if err!=nil{return core.ProviderMedia{},err}
 for _,ch:=range chapters{
  if ch.ID!=task.Chapter.ID{continue}
  if ch.PageURL!=task.Chapter.PageURL{return core.ProviderMedia{},errors.New("TVBox chapter URL mismatch")}
  if !lgDirectMedia(ch.PageURL){return core.ProviderMedia{},errors.New("unsupported TVBox player route")}
  return p.host.PrepareWebMedia(ctx,core.ProviderMedia{URL:ch.PageURL,Referer:lgBase+"/"},"老果+")
 }
 return core.ProviderMedia{},errors.New("TVBox episode not found in selected drama")
}
