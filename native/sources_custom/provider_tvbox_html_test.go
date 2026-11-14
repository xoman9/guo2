package custom

import (
 "context"
 "errors"
 "strings"
 "testing"
 "duanjuapp/native/core"
)
type tvboxFixture struct {
 core.Host
 pages map[string]string
}
func(f *tvboxFixture)FetchText(_ context.Context,req core.TextRequest)(core.TextResponse,error){
 if s,ok:=f.pages[req.URL];ok{return core.TextResponse{Body:s,URL:req.URL},nil}
 return core.TextResponse{},errors.New("missing fixture: "+req.URL)
}
func(f *tvboxFixture)PrepareWebMedia(_ context.Context,m core.ProviderMedia,_ string)(core.ProviderMedia,error){return m,nil}
func TestJableTVBoxParsing(t *testing.T){
 p:=&jableProvider{host:&tvboxFixture{pages:map[string]string{
  jableBase+"/categories/":`<div class="img-box"><a href="/categories/test/"><div class="absolute-center"><h4>Test</h4></div></a></div>`,
  jableBase+"/":`<div class="video-img-box"><a href="/videos/demo-12/"><img data-src="/a.jpg"><div class="detail"><h6>Demo</h6></div></a></div>`,
  jableBase+"/videos/demo-12/":`<html><head><meta property="og:title" content="Demo"><meta property="og:image" content="/poster.jpg"></head><body><script>var hlsUrl="https://media.example/demo.m3u8";</script></body></html>`,
 }}}
 cats,err:=p.Categories(context.Background(),false)
 if err!=nil||len(cats)!=2||cats[1].ID!="test"{t.Fatalf("categories: %+v %v",cats,err)}
 cards,_,err:=p.CatalogPage(context.Background(),1,"")
 if err!=nil||len(cards)!=1||cards[0].SourceID!="demo-12"{t.Fatalf("catalog: %+v %v",cards,err)}
 d,chapters,err:=p.Detail(context.Background(),"demo-12")
 if err!=nil||d.Cover!="https://jable.tv/poster.jpg"||len(chapters)!=1{t.Fatalf("detail: %+v %+v %v",d,chapters,err)}
 media,err:=p.ResolveMedia(context.Background(),core.Task{DramaID:d.ID,Chapter:chapters[0]})
 if err!=nil||media.URL!="https://media.example/demo.m3u8"{t.Fatalf("media: %+v %v",media,err)}
}
func TestMissAVTVBoxParsing(t *testing.T){
 host:=&tvboxFixture{pages:map[string]string{
  missBase+"/":`<a class="block bg-nord3" href="/dm/test">Test</a>`,
  missBase+"/?page=1":`<div class="thumbnail"><a class="text-secondary" href="/abc-123">Demo</a><img data-src="//cdn.example/p.jpg"></div>`,
  missBase+"/abc-123":`<meta property="og:title" content="Demo"><meta property="og:image" content="https://cdn.example/p.jpg">`,
 }}
 p:=&missProvider{host:host}
 cats,err:=p.Categories(context.Background(),false)
 if err!=nil||len(cats)!=2{t.Fatalf("categories: %+v %v",cats,err)}
 cards,_,err:=p.CatalogPage(context.Background(),1,"")
 if err!=nil||len(cards)!=1||cards[0].SourceID!="abc-123"{t.Fatalf("catalog: %+v %v",cards,err)}
 d,episodes,err:=p.Detail(context.Background(),"abc-123")
 if err!=nil||d.Title!="Demo"||len(episodes)!=1{t.Fatalf("detail: %+v %+v %v",d,episodes,err)}
 _,err=p.ResolveMedia(context.Background(),core.Task{DramaID:d.ID,Chapter:episodes[0]})
 if err==nil||!strings.Contains(err.Error(),"解析器"){t.Fatalf("should not pretend to resolve web player: %v",err)}
}
func TestTVBoxBlocksVerificationAndEmptyCover(t *testing.T){
 if tvSafeURL(missBase+"/","")!=""{t.Fatal("empty cover mapped to homepage")}
 p:=&missProvider{host:&tvboxFixture{pages:map[string]string{missBase+"/":`<title>Just a moment...</title>`}}}
 _,err:=p.Categories(context.Background(),false)
 if err==nil||!strings.Contains(err.Error(),"403"){t.Fatalf("browser verification should be explicit: %v",err)}
}
