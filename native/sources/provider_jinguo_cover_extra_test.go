package sources

import (
 "encoding/json"
 "testing"
)
func TestJinguoAdditionalCoverFields(t *testing.T){
 var row jinguoRow
 err:=json.Unmarshal([]byte(`{"id":"A123","title":"Demo","coverImage":"https://cdn.example/poster.jpg","tagNames":["test"]}`),&row)
 if err!=nil{t.Fatal(err)}
 drama,ok:=jinguoDrama(row,"")
 if !ok||drama.Cover!="https://cdn.example/poster.jpg"{
  t.Fatalf("cover not extracted: %#v, %v",drama.Cover,ok)
 }
 var nested jinguoRow
 err=json.Unmarshal([]byte(`{"id":"A124","title":"Demo","playletInfo":{"cover":"https://cdn.example/nested.jpg"}}`),&nested)
 if err!=nil{t.Fatal(err)}
 drama,ok=jinguoDrama(nested,"")
 if !ok||drama.Cover!="https://cdn.example/nested.jpg"{
  t.Fatalf("nested cover not extracted: %#v, %v",drama.Cover,ok)
 }
}
