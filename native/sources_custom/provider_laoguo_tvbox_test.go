package custom

import (
 "context"
 "errors"
 "strings"
 "testing"
 "duanjuapp/native/core"
)
type laoguoFixture struct{
 core.Host
 pages map[string]string
}
func (h *laoguoFixture) FetchText(_ context.Context,request core.TextRequest)(core.TextResponse,error){
 if body,ok:=h.pages[request.URL];ok{return core.TextResponse{Body:body,URL:request.URL},nil}
 return core.TextResponse{},errors.New("TVBox fixture missing: "+request.URL)
}
func (h *laoguoFixture) PrepareWebMedia(_ context.Context,m core.ProviderMedia,_ string)(core.ProviderMedia,error){return m,nil}

func TestLaoguoTVBoxCatalogCategoriesSearchDetailAndPlayback(t *testing.T){
 h:=&laoguoFixture{pages:map[string]string{
  lgAPI+"?ac=list":`{"class":[{"type_id":1,"type_name":"劇集"},{"type_id":2,"type_name":"動漫"}],"list":[]}`,
  lgAPI+"?ac=list&pg=1&t=1":`{"page":1,"pagecount":2,"list":[{"vod_id":12,"vod_name":"測試影片","vod_pic":"/cover.jpg"}]}`,
  lgAPI+"?ac=list&pg=1&wd=test":`{"page":1,"pagecount":1,"list":[{"vod_id":12,"vod_name":"測試影片"}]}`,
  lgAPI+"?ac=detail&ids=12":`{"list":[{"vod_id":12,"vod_name":"測試影片","vod_play_from":"lineA$$$lineB","vod_play_url":"第一集$https://cdn.example/a1.m3u8#第二集$https://cdn.example/a2.mp4$$$第一集$https://cdn.example/b1.mp4"}]}`,
 }}
 p:=&laoguoTVBox{host:h}
 if !lgDescriptor.DefaultOff||lgDescriptor.Name!="老果+"{t.Fatal("wrong default-off channel label")}
 cats,err:=p.Categories(context.Background(),false)
 if err!=nil||len(cats)!=3||cats[1].ID!="1"{t.Fatalf("categories: %+v %v",cats,err)}
 rows,more,err:=p.CatalogPage(context.Background(),1,"1")
 if err!=nil||!more||len(rows)!=1||rows[0].SourceID!="12"{t.Fatalf("catalog: %+v %v %v",rows,more,err)}
 found,more,err:=p.Search(context.Background(),1,"test")
 if err!=nil||more||len(found)!=1{t.Fatalf("search: %+v %v %v",found,more,err)}
 drama,chapters,err:=p.Detail(context.Background(),"12")
 if err!=nil||len(chapters)!=3||drama.TotalEpisode!=3{t.Fatalf("detail: %+v %+v %v",drama,chapters,err)}
 if chapters[0].ID==chapters[2].ID{t.Fatal("duplicate episode IDs across player lines")}
 media,err:=p.ResolveMedia(context.Background(),core.Task{DramaID:drama.ID,Chapter:chapters[0]})
 if err!=nil||media.URL!="https://cdn.example/a1.m3u8"{t.Fatalf("media: %+v %v",media,err)}
 wrong:=chapters[0]
 wrong.PageURL="https://cdn.example/wrong.mp4"
 if _,err:=p.ResolveMedia(context.Background(),core.Task{DramaID:drama.ID,Chapter:wrong});err==nil||!strings.Contains(err.Error(),"mismatch"){
  t.Fatalf("foreign URL accepted: %v",err)
 }
}
func TestLaoguoTVBoxRejectsUnsafeOrUnsupportedEpisodes(t *testing.T){
 chapters:=lgChapters("12","第一集$javascript:alert(1)#第二集$https://cdn.example/watch.php#第三集$https://cdn.example/good.mp4","lineA")
 if len(chapters)!=1||chapters[0].PageURL!="https://cdn.example/good.mp4"{
  t.Fatalf("unsupported player links registered as direct media: %+v",chapters)
 }
 if _,err:=lgDrama(map[string]any{"vod_id":"../bad","vod_name":"test"});err==nil{t.Fatal("unsafe drama ID accepted")}
 if lgDirectMedia("https://example.com/tracker.html")||lgDirectMedia("javascript:alert(1)"){
  t.Fatal("unsupported URL accepted")
 }
}
