package core

import "testing"

func TestHongguoApkSearchParsesNestedResults(t *testing.T) {
	response := map[string]any{
		"data": map[string]any{
			"list": []any{
				map[string]any{"video_data": map[string]any{"series_id_str": "123456", "title": "測試短劇"}},
				map[string]any{"video_data": map[string]any{"series_id_str": "no-id", "title": "忽略無效"}},
			},
		},
	}
	var dramas []Drama
	hongguoApkSearchVideoRows(response, 0, &dramas)
	if len(dramas) != 1 || dramas[0].ID == "" {
		t.Fatalf("expected one valid Hongguo series, got %+v", dramas)
	}
}

func TestHongguoApkSearchParserRejectsNonSeriesResponse(t *testing.T) {
	response := map[string]any{
		"data": []any{map[string]any{"title": "推薦分類", "id": "1"}},
	}
	var dramas []Drama
	hongguoApkSearchVideoRows(response, 0, &dramas)
	if len(dramas) != 0 {
		t.Fatalf("non-series entries must not be treated as short dramas: %+v", dramas)
	}
}
