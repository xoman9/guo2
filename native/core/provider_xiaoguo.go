package core

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	nethtml "golang.org/x/net/html"
)

// Xiaoguo uses the MacCMS HTML parser from oktv, but newer yese.co
// navigation includes /vod/show/class/<name>/id/<id>.html links.
// The category key stores the actual validated path, not an invented ID.
// The provider is live-only and never preloads or persists its catalog.
const xiaoguoBaseURL = "https://yese.co"

var (
	xiaoguoCategoryPath = regexp.MustCompile(`(?i)^/(?:index\.php/)?vod/(?:type|show)/(?:class/[^/?#]+/)?id/[0-9]{1,6}(?:\.html|/?)$`)
	xiaoguoCategoryKey  = regexp.MustCompile(`^p:[A-Za-z0-9_-]{8,320}$`)
)

func xiaoguoCategoryID(href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil || u.User != nil || (u.Scheme != "" && u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	if u.Host != "" && !strings.EqualFold(u.Hostname(), "yese.co") && !strings.EqualFold(u.Hostname(), "www.yese.co") {
		return ""
	}
	path, err := url.PathUnescape(u.EscapedPath())
	if err != nil || !xiaoguoCategoryPath.MatchString(path) || strings.Contains(path, "..") {
		return ""
	}
	// Canonicalize the duplicate /index.php/vod/show and /vod/show links.
	path = strings.TrimPrefix(path, "/index.php")
	if strings.HasPrefix(path, "/vod/show/") && !strings.Contains(path, "/class/") {
		return ""
	}
	key := "p:" + base64.RawURLEncoding.EncodeToString([]byte(path))
	if len(key) > 322 {
		return ""
	}
	return key
}

func validXiaoguoCategory(category string) bool {
	if category == "" || webProviderNumericID.MatchString(category) {
		return true
	}
	if !xiaoguoCategoryKey.MatchString(category) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(category, "p:"))
	return err == nil && len(decoded) <= 240 && xiaoguoCategoryPath.Match(decoded)
}

func xiaoguoCategoryURL(category string, page int) (string, error) {
	if page < 1 || page > 100000 || !validXiaoguoCategory(category) {
		return "", errors.New("宵果分类参数无效")
	}
	path := "/index.php/vod/show/id/1.html"
	if category != "" {
		if webProviderNumericID.MatchString(category) {
			path = "/index.php/vod/show/id/" + category + ".html"
		} else {
			decoded, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(category, "p:"))
			path = string(decoded)
		}
	}
	if page > 1 {
		path = strings.TrimSuffix(path, ".html")
		path = strings.TrimSuffix(path, "/") + "/page/" + strconv.Itoa(page) + ".html"
	}
	return path, nil
}

func (d *Downloader) xiaoguoPage(ctx context.Context, path string) (*nethtml.Node, string, error) {
	base := d.providerBaseURL(sourceXiaoguo)
	link := lujj31SameSite(base, path)
	if link == "" {
		return nil, "", errors.New("宵果网页地址无效")
	}
	doc, actual, err := d.fetchProviderPage(ctx, link, base+"/", lujj31UserAgent)
	if err != nil {
		return nil, "", err
	}
	if len(providerHTMLNodes(doc, func(n *nethtml.Node) bool { return n.Data == "body" && n.FirstChild != nil })) == 0 {
		return nil, "", errors.New("宵果站点网页内容为空")
	}
	return doc, actual, nil
}

func xiaoguoCategoriesFromHTML(doc *nethtml.Node) []nativeCategory {
	seenID, seenName := map[string]bool{}, map[string]bool{}
	categories := []nativeCategory{}
	navs := providerHTMLNodes(doc, func(n *nethtml.Node) bool { return n.Data == "nav" && lujj31HasClass(n, "app-nav") })
	roots := []*nethtml.Node{doc}
	if len(navs) > 0 {
		roots = navs
	}
	for _, root := range roots {
		for _, a := range providerHTMLNodes(root, func(n *nethtml.Node) bool { return n.Data == "a" }) {
			id := xiaoguoCategoryID(providerHTMLAttr(a, "href"))
			if id == "" || seenID[id] {
				continue
			}
			name := strings.Join(strings.Fields(providerHTMLText(a)), " ")
			if name == "" || name == "全部" || name == "更多" || len([]rune(name)) > 28 || seenName[name] {
				continue
			}
			seenID[id], seenName[name] = true, true
			categories = append(categories, nativeCategory{ID: id, Name: name})
			if len(categories) >= 50 {
				break
			}
		}
		if len(categories) >= 50 {
			break
		}
	}
	return categories
}

func (d *Downloader) fetchXiaoguoCategories(ctx context.Context) ([]nativeCategory, error) {
	doc, _, err := d.xiaoguoPage(ctx, "/")
	if err != nil {
		return nil, err
	}
	categories := xiaoguoCategoriesFromHTML(doc)
	if len(categories) == 0 {
		return nil, errors.New("宵果首页未解析到分类导航")
	}
	// Native caller already adds the single default "全部" category.
	return categories, nil
}

func xiaoguoDrama(original Drama) Drama {
	id := original.SourceID
	original.Source = sourceXiaoguo
	original.ID = providerDramaID(sourceXiaoguo, id)
	original.ChannelName = "宵果"
	return original
}

func (d *Downloader) fetchXiaoguoCatalogPage(ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
	if len(query) > 256 {
		return nil, false, errors.New("宵果搜索关键词过长")
	}
	path, err := xiaoguoCategoryURL(category, page)
	if err != nil {
		return nil, false, err
	}
	if query != "" {
		if page > 1 {
			return []Drama{}, false, nil
		}
		path = "/index.php/vod/search/wd/" + url.PathEscape(query) + ".html"
	}
	doc, _, err := d.xiaoguoPage(ctx, path)
	if err != nil {
		return nil, false, err
	}
	items := lujj31Cards(doc, d.providerBaseURL(sourceXiaoguo))
	// Reuse the tested MacCMS cards without sharing Tao-guo's source identity.
	result := make([]Drama, 0, len(items))
	for _, item := range items {
		result = append(result, xiaoguoDrama(item))
	}
	if len(result) == 0 && page == 1 {
		return nil, false, errors.New("宵果当前页面没有解析到影片")
	}
	return result, len(result) > 0 && query == "", nil
}

func xiaoguoDetailFromHTML(doc *nethtml.Node, base, id string) (Drama, []Chapter, error) {
	drama, chapters, err := lujj31DetailFromHTML(doc, base, id)
	if err != nil {
		return Drama{}, nil, err
	}
	drama = xiaoguoDrama(drama)
	for i := range chapters {
		chapters[i].Source = sourceXiaoguo
		chapters[i].ID = providerChapterID(sourceXiaoguo, id, strconv.Itoa(i+1))
		chapters[i].VideoURL = "xiaoguo-play://" + chapters[i].PageURL
	}
	return drama, chapters, nil
}

// A single player page with player_data is sufficient for a one-episode
// drama. Never synthesize a playable item from an unrelated error/home page.
func xiaoguoSinglePlayback(doc *nethtml.Node, base, id, page string) (Drama, []Chapter, bool) {
	link := lujj31SameSite(base, page)
	if link == "" || lujj31SourceID(link) != id {
		return Drama{}, nil, false
	}
	found := false
	for _, script := range providerHTMLNodes(doc, func(n *nethtml.Node) bool { return n.Type == nethtml.ElementNode && n.Data == "script" }) {
		// providerHTMLText intentionally filters <script> text. Examine
		// raw script text nodes instead, without changing shared HTML helpers.
		for child := script.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == nethtml.TextNode && lujj31PlayerData.MatchString(child.Data) {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return Drama{}, nil, false
	}
	title := "影片 " + id
	for _, heading := range providerHTMLNodes(doc, func(n *nethtml.Node) bool { return n.Data == "h1" }) {
		if value := strings.TrimSpace(providerHTMLText(heading)); value != "" {
			title = truncate(value, 512)
			break
		}
	}
	cover := lujj31DetailPoster(doc, link)
	chapter := Chapter{
		ID:     providerChapterID(sourceXiaoguo, id, "1"),
		Source: sourceXiaoguo, Title: "第1集", CurrentEpisode: rawEpisode(1),
		PageURL: link, VideoURL: "xiaoguo-play://" + link, Referer: base + "/",
	}
	drama := Drama{
		ID: providerDramaID(sourceXiaoguo, id), Source: sourceXiaoguo, SourceID: id,
		Title: title, Name: title, Cover: cover, CoverURL: cover,
		EpisodeCount: 1, TotalEpisode: 1, ChannelName: "宵果",
	}
	return drama, []Chapter{chapter}, true
}

func (d *Downloader) fetchXiaoguoDetail(ctx context.Context, id string) (Drama, []Chapter, error) {
	if !webProviderNumericID.MatchString(id) {
		return Drama{}, nil, errors.New("宵果影片编号无效")
	}
	paths := []string{
		fmt.Sprintf("/index.php/vod/detail/id/%s.html", id),
		// The user-supplied yese.co homepage links to this slash route.
		fmt.Sprintf("/vod/play/id/%s/sid/1/nid/1/", id),
		fmt.Sprintf("/index.php/vod/play/id/%s/sid/1/nid/1.html", id),
	}
	var last error
	for _, path := range paths {
		doc, _, err := d.xiaoguoPage(ctx, path)
		if err != nil {
			last = err
			continue
		}
		drama, chapters, err := xiaoguoDetailFromHTML(doc, d.providerBaseURL(sourceXiaoguo), id)
		if err == nil {
			return drama, chapters, nil
		}
		// Single-video MacCMS pages sometimes expose a player but no
		// episode-list anchors; reuse the user's actual play-page URL.
		if strings.Contains(path, "/play/") {
			if single, episodes, ok := xiaoguoSinglePlayback(doc, d.providerBaseURL(sourceXiaoguo), id, path); ok {
				return single, episodes, nil
			}
		}
		last = err
	}
	if last == nil {
		last = errors.New("宵果详情无法解析")
	}
	return Drama{}, nil, last
}

func (d *Downloader) xiaoguoResolvePage(ctx context.Context, page, id string) (string, error) {
	base := d.providerBaseURL(sourceXiaoguo)
	link := lujj31SameSite(base, page)
	if link == "" || lujj31SourceID(link) != id {
		return "", errors.New("宵果播放页与影片不一致")
	}
	u, _ := url.Parse(link)
	if !strings.Contains(u.Path, "/play/") && !strings.Contains(u.Path, "/vodplay/") {
		return "", errors.New("宵果播放路径无效")
	}
	ua := context.WithValue(ctx, providerTextUserAgentKey{}, lujj31UserAgent)
	body, err := d.fetchProviderText(ua, link, base+"/")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		return "", errors.New("宵果播放页内容为空")
	}
	fields := lujj31PlayerFields(body, id)
	address, err := lujj31PlayerURL(body, id)
	if err == nil && lujj31MediaLike(address) {
		return address, nil
	}
	if token := fields["url"]; token != "" && !isProviderHTTPMediaURL(token) {
		if parsed := d.maccmsSiteCloudParse(ctx, base, body, token, link); isProviderHTTPMediaURL(parsed) {
			return parsed, nil
		}
	}
	if err == nil {
		return address, nil
	}
	return "", err
}

func (d *Downloader) resolveXiaoguoMedia(ctx context.Context, task Task) (providerMedia, error) {
	source, id, valid := splitProviderDramaID(task.DramaID)
	if !valid || source != sourceXiaoguo || !webProviderNumericID.MatchString(id) ||
		!strings.HasPrefix(task.Chapter.ID, providerChapterID(sourceXiaoguo, id, "")) {
		return providerMedia{}, errors.New("宵果分集资料无效")
	}
	base := d.providerBaseURL(sourceXiaoguo)
	page := lujj31SameSite(base, task.Chapter.PageURL)
	if page == "" || lujj31SourceID(page) != id {
		return providerMedia{}, errors.New("宵果播放页无效")
	}
	address, err := d.xiaoguoResolvePage(ctx, page, id)
	if err == nil && lujj31MediaLike(address) {
		media, mediaErr := d.prepareWebProviderMedia(ctx, providerMedia{URL: address, Referer: page}, "宵果")
		if mediaErr == nil {
			return media, nil
		}
		err = mediaErr
	}
	otherCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for _, altPage := range lujj31AltRouteURLs(page) {
		if otherCtx.Err() != nil {
			break
		}
		alt, altErr := d.xiaoguoResolvePage(otherCtx, altPage, id)
		if altErr != nil || !lujj31MediaLike(alt) {
			continue
		}
		media, mediaErr := d.prepareWebProviderMedia(otherCtx, providerMedia{URL: alt, Referer: altPage}, "宵果")
		if mediaErr == nil {
			return media, nil
		}
	}
	if isProviderHTTPMediaURL(address) {
		return d.prepareWebProviderMedia(ctx, providerMedia{URL: address, Referer: page}, "宵果")
	}
	if err != nil {
		return providerMedia{}, err
	}
	return providerMedia{}, errors.New("宵果未取得可用播放线路")
}
