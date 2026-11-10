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

func TestHongguoApkCueExtractsUniqueSuggestions(t *testing.T) {
	response := map[string]any{
		"data": map[string]any{
			"cue_list": []any{
				map[string]any{"word": "冬日戀歌", "word_type": "short_play_name"},
				map[string]any{"word": "冬日戀歌"},
				map[string]any{"name": "星河故事"},
				map[string]any{"word": ""},
			},
		},
	}
	var items []hongguoSearchSuggestion
	hongguoApkCueItems(response, 0, &items, make(map[string]bool))
	if len(items) != 2 {
		t.Fatalf("expected two unique cues, got %+v", items)
	}
	if items[0].Name != "冬日戀歌" || items[1].Name != "星河故事" {
		t.Fatalf("unexpected cue names: %+v", items)
	}
}
