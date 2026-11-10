package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	hongguoSuggestionLimit        = 10
	hongguoSuggestionCacheLimit   = 128
	hongguoSuggestionPendingLimit = 32
	hongguoSuggestionBodyLimit    = 256 * 1024
	hongguoSuggestionTimeout      = 5 * time.Second
	hongguoSuggestionTTL          = 2 * time.Minute
)

type hongguoSearchSuggestion struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type hongguoSuggestionRecord struct {
	Name      string         `json:"name"`
	WordType  string         `json:"word_type"`
	Keyword   any            `json:"keyword"`
	VideoData map[string]any `json:"video_data"`
}

type hongguoSuggestionEntry struct {
	Items     []hongguoSearchSuggestion
	ExpiresAt time.Time
}

type hongguoSuggestionCall struct {
	done  chan struct{}
	items []hongguoSearchSuggestion
	err   error
}

func (d *Downloader) hongguoSearchSuggestions(ctx context.Context, query string) ([]hongguoSearchSuggestion, error) {
	query, err := hongguoSearchKeyword(query)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, hongguoSuggestionTimeout)
	defer cancel()
	client := d.hongguoClient()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		client.mu.Lock()
		if cached, found := client.suggestions[query]; found && time.Now().Before(cached.ExpiresAt) {
			items := append([]hongguoSearchSuggestion{}, cached.Items...)
			client.mu.Unlock()
			return items, nil
		}
		if pending := client.suggestPending[query]; pending != nil {
			client.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-pending.done:

				if errors.Is(pending.err, context.Canceled) || errors.Is(pending.err, context.DeadlineExceeded) {
					continue
				}
				return append([]hongguoSearchSuggestion{}, pending.items...), pending.err
			}
		}
		if len(client.suggestPending) >= hongguoSuggestionPendingLimit {
			client.mu.Unlock()
			return nil, errors.New("红果搜索联想请求较多，请稍后重试")
		}
		pending := &hongguoSuggestionCall{done: make(chan struct{})}
		client.suggestPending[query] = pending
		client.mu.Unlock()

		items, err := d.fetchHongguoSearchSuggestions(ctx, query)
		client.mu.Lock()
		if err == nil {
			now := time.Now()
			oldestKey, oldestTime := "", now.Add(hongguoSuggestionTTL)
			for key, entry := range client.suggestions {
				if !now.Before(entry.ExpiresAt) {
					delete(client.suggestions, key)
				} else if entry.ExpiresAt.Before(oldestTime) {
					oldestKey, oldestTime = key, entry.ExpiresAt
				}
			}
			if len(client.suggestions) >= hongguoSuggestionCacheLimit {
				delete(client.suggestions, oldestKey)
			}
			client.suggestions[query] = hongguoSuggestionEntry{Items: items, ExpiresAt: now.Add(hongguoSuggestionTTL)}
		}
		pending.items, pending.err = items, err
		delete(client.suggestPending, query)
		close(pending.done)
		client.mu.Unlock()
		return append([]hongguoSearchSuggestion{}, items...), err
	}
}

// 0.2.536 contains a separate search/cue/v1 path. Its response layout may
// vary by server build, so accept only bounded, nonempty text suggestions.
const hongguoApkSearchCuePath = "/reading/bookapi/search/cue/v1/"

func hongguoApkCueItems(node any, depth int, out *[]hongguoSearchSuggestion, seen map[string]bool) {
	if depth > 7 || len(*out) >= hongguoSuggestionLimit { return }
	switch value := node.(type) {
	case []any:
		for _, item := range value { hongguoApkCueItems(item, depth+1, out, seen) }
	case map[string]any:
		name := mapString(value, "word", "name", "keyword", "text", "search_word")
		if name != "" {
			if normalized, err := hongguoSearchKeyword(name); err == nil {
				key := strings.ToLower(normalized)
				if !seen[key] {
					seen[key] = true
					*out = append(*out, hongguoSearchSuggestion{Name: normalized, Type: mapString(value, "word_type", "type")})
				}
			}
		}
		for _, key := range []string{"data", "result", "results", "list", "items", "cues", "cue_list", "search_cues", "suggest_list", "words", "suggestions"} {
			if next, ok := value[key]; ok { hongguoApkCueItems(next, depth+1, out, seen) }
		}
	}
}

func (d *Downloader) fetchHongguoApkCueSuggestions(ctx context.Context, query string) ([]hongguoSearchSuggestion, error) {
	params := url.Values{"query": {query}, "count": {strconv.Itoa(hongguoSuggestionLimit)}}
	result, err := d.hongguoAppRequest(ctx, http.MethodGet, hongguoApkSearchCuePath, params, nil)
	if err != nil { return nil, err }
	items := make([]hongguoSearchSuggestion, 0, hongguoSuggestionLimit)
	hongguoApkCueItems(result, 0, &items, make(map[string]bool))
	if len(items) == 0 { return nil, errors.New("新版紅果搜尋提示沒有可識別內容") }
	return items, nil
}

func (d *Downloader) fetchHongguoSearchSuggestions(ctx context.Context, query string) ([]hongguoSearchSuggestion, error) {
	// First try the APK cue endpoint; retain the old suggestion service
	// if a server version does not expose a compatible cue response.
	cueCtx, cueCancel := context.WithTimeout(ctx, 2*time.Second)
	cues, cueErr := d.fetchHongguoApkCueSuggestions(cueCtx, query)
	cueCancel()
	if cueErr == nil && len(cues) > 0 { return cues, nil }
	if ctx.Err() != nil { return nil, ctx.Err() }
	records, err := d.fetchHongguoSuggestionRecords(ctx, query, hongguoSuggestionLimit)
	if err != nil {
		return nil, err
	}
	items := make([]hongguoSearchSuggestion, 0, hongguoSuggestionLimit)
	seen := make(map[string]bool)
	for _, item := range records {
		name, err := hongguoSearchKeyword(item.Name)
		key := strings.ToLower(name)
		if err != nil || seen[key] {
			continue
		}
		seen[key] = true
		kind := item.WordType
		switch kind {
		case "short_play_name", "short_play_category", "common_query", "actor_name", "short_play_actor":
		default:
			kind = ""
		}
		items = append(items, hongguoSearchSuggestion{Name: name, Type: kind})
		if len(items) == hongguoSuggestionLimit {
			break
		}
	}
	return items, nil
}

func (d *Downloader) fetchHongguoSuggestionRecords(ctx context.Context, query string, count int) ([]hongguoSuggestionRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, hongguoSuggestionTimeout)
	defer cancel()
	params := url.Values{"app_id": {"8662"}, "query": {query}, "count": {strconv.Itoa(count)}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, hongguoBaseURL+"/incent_resource/suggestion?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Referer", hongguoBaseURL+"/")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	response, err := d.doCatalogRequestWithTimeout(request, hongguoSuggestionTimeout)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, hongguoSuggestionBodyLimit+1))
	if err != nil {
		return nil, err
	}
	if len(body) > hongguoSuggestionBodyLimit {
		return nil, errors.New("红果搜索联想响应过大")
	}
	if response.StatusCode != http.StatusOK || catalogResponseBlockReason(response, body) != "" {
		return nil, d.catalogResponseError(request, response, body)
	}
	var result struct {
		Items []hongguoSuggestionRecord `json:"suggest_list"`
		Data  struct {
			Items []hongguoSuggestionRecord `json:"suggest_list"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, errors.New("红果搜索联想未返回有效数据")
	}
	if result.Items == nil {
		result.Items = result.Data.Items
	}
	if result.Items == nil {
		return nil, errors.New("红果搜索联想未返回有效数据")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("红果搜索联想未返回有效数据")
	}
	return result.Items, nil
}
