package core

import "testing"

func TestLujj31CatalogTitlePreserved(t *testing.T) {
	id := "987654"
	for _, title := range []string{"正確影片名稱", "桃果短劇 第十二集"} {
		if !nativeLujj31CatalogTitleValid(title, id) {
			t.Errorf("expected catalog title %q to be retained", title)
		}
	}
	for _, title := range []string{"", "短剧", "短劇", "影片", "影片 " + id, "桃果"} {
		if nativeLujj31CatalogTitleValid(title, id) {
			t.Errorf("placeholder %q should not override a real detail title", title)
		}
	}
}

func TestLujj31DetailTitleIsNotSwappedByPageHeader(t *testing.T) {
	catalog := nativeDrama{ID: "lujj31:12345", Source: sourceLujj31, SourceID: "12345", Title: "列表正確片名"}
	detail := nativeDrama{ID: catalog.ID, Source: sourceLujj31, SourceID: "12345", Title: "播放線路", Episodes: 12}
	if nativeLujj31CatalogTitleValid(catalog.Title, catalog.SourceID) {
		detail.Title = catalog.Title
	}
	got := mergeNativeDrama(catalog, detail)
	if got.Title != "列表正確片名" || got.Episodes != 12 {
		t.Fatalf("unexpected merged drama title or episodes: %+v", got)
	}
}
