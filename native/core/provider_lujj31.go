package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Ported from oktv's Tao-guo HTML routes (commit 1ba9d1e69de77f62588af3d9763d1858d62aa98c).
// Unlike the old implementation, this provider does not call /api.php/provide/vod/.
const lujj31BaseURL = "https://lujj31.lol"
const lujj31UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36"

var (
	lujj31CategoryPath = regexp.MustCompile(`(?i)^/(?:index\.php/)?vod/type/id/([0-9]{1,6})(?:\.html|/)$`)
	lujj31VideoPath    = regexp.MustCompile(`(?i)/(?:(?:index\.php/)?vod/)?(?:detail|play)/id/([0-9]+)(?:/|\.html|$)|/(?:voddetail|vodplay)/([0-9]+)(?:[-./]|$)`)
	lujj31PlayRoute    = regexp.MustCompile(`(?i)/sid/([0-9]+)/nid/([0-9]+)`)
	lujj31PlayerData   = regexp.MustCompile(`(?i)player_(?:aaaa|data)\s*=\s*`)
	lujj31DirectMedia  = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.(?:m3u8|mp4)(?:\?[^\s"'<>\\]*)?`)
)

func lujj31SameSite(base, href string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil || ref.User != nil {
		return ""
	}
	u := b.ResolveReference(ref)
	if (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Hostname(), b.Hostname()) || u.User != nil {
		return ""
	}
	return u.String()
}

func lujj31SourceID(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	m := lujj31VideoPath.FindStringSubmatch(u.Path)
	if len(m) < 3 {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

func lujj31HasClass(node *html.Node, class string) bool {
	for _, item := range strings.Fields(providerHTMLAttr(node, "class")) {
		if item == class || strings.HasSuffix(item, "-"+class) {
			return true
		}
	}
	return false
}

func (d *Downloader) lujj31Page(ctx context.Context, path string) (*html.Node, string, error) {
	site := d.providerBaseURL(sourceLujj31)
	address := lujj31SameSite(site, path)
	if address == "" {
		return nil, "", errors.New("桃果网页地址无效")
	}
	document, actual, err := d.fetchProviderPage(ctx, address, site+"/", lujj31UserAgent)
	if err != nil {
		return nil, "", err
	}
	nodes := providerHTMLNodes(document, func(n *html.Node) bool { return n.Data == "body" && n.FirstChild != nil })
	if len(nodes) == 0 {
		return nil, "", errors.New("桃果网站返回空白网页，未取得内容")
	}
	return document, actual, nil
}

func (d *Downloader) fetchLujj31Categories(ctx context.Context) ([]nativeCategory, error) {
	doc, _, err := d.lujj31Page(ctx, "/")
	if err != nil {
		return nil, err
	}
	categories := []nativeCategory{{Name: "全部"}}
	seen := map[string]bool{}
	for _, a := range providerHTMLNodes(doc, func(n *html.Node) bool { return n.Data == "a" }) {
		p, err := url.Parse(providerHTMLAttr(a, "href"))
		if err != nil {
			continue
		}
		m := lujj31CategoryPath.FindStringSubmatch(p.Path)
		if len(m) != 2 || seen[m[1]] {
			continue
		}
		title := strings.TrimSpace(providerHTMLText(a))
		if title == "" || len([]rune(title)) > 32 {
			continue
		}
		seen[m[1]] = true
		categories = append(categories, nativeCategory{ID: m[1], Name: title})
		if len(categories) >= 40 {
			break
		}
	}
	if len(categories) < 2 {
		return nil, errors.New("桃果首页未识别到分类导航")
	}
	return categories[1:], nil
}

// oktv's MacCMS cover extractor handles lazy-loaded <img> and CSS
// background-image posters on both images and anchors (myui/macplus).
var lujj31BackgroundURL = regexp.MustCompile(`(?i)background(?:-image)?\s*:\s*url\(\s*['"]?([^'")]+)`)

func lujj31PosterAddress(raw, page string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "blob:") {
		return ""
	}
	address := providerCoverAddress(raw, page)
	if address == "" {
		return ""
	}
	parsed, err := url.Parse(address)
	if err != nil || !validNativeCoverURL(parsed) {
		return ""
	}
	path := strings.ToLower(parsed.Path)
	for _, placeholder := range []string{"/loading.gif", "/load.gif", "/lazyload.gif", "/noimage", "/nopic", "/placeholder"} {
		if strings.Contains(path, placeholder) {
			return ""
		}
	}
	return address
}

func lujj31StylePoster(raw, page string) string {
	if match := lujj31BackgroundURL.FindStringSubmatch(raw); len(match) > 1 {
		return lujj31PosterAddress(match[1], page)
	}
	return ""
}

func lujj31CardPoster(card *html.Node, page string) string {
	nodes := providerHTMLNodes(card, func(n *html.Node) bool {
		return n.Data == "img" || n.Data == "a" || n.Data == "div"
	})
	// Prefer actual lazy-load attributes and CSS over generic placeholders.
	for _, node := range nodes {
		for _, name := range []string{"data-original", "data-src", "data-background", "data-lazy"} {
			if cover := lujj31PosterAddress(providerHTMLAttr(node, name), page); cover != "" {
				return cover
			}
		}
		if cover := lujj31StylePoster(providerHTMLAttr(node, "style"), page); cover != "" {
			return cover
		}
	}
	for _, node := range nodes {
		if node.Data == "img" {
			if cover := lujj31PosterAddress(providerHTMLAttr(node, "src"), page); cover != "" {
				return cover
			}
		}
	}
	return ""
}

func lujj31DetailPoster(doc *html.Node, page string) string {
	for _, node := range providerHTMLNodes(doc, func(n *html.Node) bool { return n.Data == "meta" }) {
		name := strings.ToLower(strings.TrimSpace(providerHTMLAttr(node, "property")))
		if name == "" {
			name = strings.ToLower(strings.TrimSpace(providerHTMLAttr(node, "name")))
		}
		if name == "og:image" || name == "og:image:url" || name == "twitter:image" {
			if cover := lujj31PosterAddress(providerHTMLAttr(node, "content"), page); cover != "" {
				return cover
			}
		}
	}
	for _, name := range []string{"module-item-pic", "detail-pic", "video-info-pic", "pic"} {
		if node := providerHTMLFirstClass(doc, name); node != nil {
			if cover := lujj31CardPoster(node, page); cover != "" {
				return cover
			}
		}
	}
	return lujj31CardPoster(doc, page)
}

func lujj31CardTitle(node *html.Node) string {
	for _, attr := range []string{"title", "alt"} {
		for _, item := range providerHTMLNodes(node, func(n *html.Node) bool { return n.Data == "img" || n.Data == "a" }) {
			if title := strings.TrimSpace(providerHTMLAttr(item, attr)); title != "" {
				return truncate(title, 512)
			}
		}
	}
	for _, a := range providerHTMLNodes(node, func(n *html.Node) bool { return n.Data == "a" }) {
		if title := providerHTMLText(a); title != "" {
			return truncate(title, 512)
		}
	}
	return ""
}

func lujj31Cards(doc *html.Node, base string) []Drama {
	classes := []string{"module-item", "module-poster-item", "myui-vodlist__box", "public-list-box", "video-item", "videoBox", "vodlist__item", "col-lg-2", "col-xl-2", "media-card", "detail-list-item", "thumbnail"}
	var cards []*html.Node
	visited := map[*html.Node]bool{}
	for _, class := range classes {
		for _, node := range providerHTMLNodes(doc, func(n *html.Node) bool { return lujj31HasClass(n, class) }) {
			if !visited[node] {
				visited[node] = true
				cards = append(cards, node)
			}
		}
	}
	if len(cards) == 0 {
		for _, a := range providerHTMLNodes(doc, func(n *html.Node) bool { return n.Data == "a" && lujj31SourceID(providerHTMLAttr(n, "href")) != "" }) {
			for p := a.Parent; p != nil; p = p.Parent {
				if p.Data == "div" || p.Data == "li" {
					if !visited[p] {
						visited[p] = true
						cards = append(cards, p)
					}
					break
				}
			}
		}
	}
	seen := map[string]bool{}
	items := []Drama{}
	for _, card := range cards {
		selected := ""
		for _, a := range providerHTMLNodes(card, func(n *html.Node) bool { return n.Data == "a" }) {
			u := lujj31SameSite(base, providerHTMLAttr(a, "href"))
			if u != "" && lujj31SourceID(u) != "" {
				selected = u
				break
			}
		}
		id := lujj31SourceID(selected)
		if id == "" || seen[id] {
			continue
		}
		title := lujj31CardTitle(card)
		if title == "" {
			continue
		}
		cover := lujj31CardPoster(card, selected)
		seen[id] = true
		items = append(items, Drama{ID: providerDramaID(sourceLujj31, id), Source: sourceLujj31, SourceID: id, Title: title, Name: title, Cover: cover, CoverURL: cover, ChannelName: "桃果"})
		if len(items) >= 200 {
			break
		}
	}
	return items
}

func (d *Downloader) fetchLujj31CatalogPage(ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
	if page < 1 || page > 100000 || len(query) > 256 || (category != "" && !webProviderNumericID.MatchString(category)) {
		return nil, false, errors.New("桃果目录参数无效")
	}
	class := category
	if class == "" {
		class = "1"
	} // oktv's taoguo default category
	path := fmt.Sprintf("/index.php/vod/type/id/%s.html", url.PathEscape(class))
	if page > 1 {
		path = fmt.Sprintf("/index.php/vod/type/id/%s/page/%d.html", url.PathEscape(class), page)
	}
	if query != "" {
		if page > 1 {
			return []Drama{}, false, nil
		}
		path = "/index.php/vod/search/wd/" + url.PathEscape(query) + ".html"
	}
	doc, _, err := d.lujj31Page(ctx, path)
	if err != nil {
		return nil, false, err
	}
	items := lujj31Cards(doc, d.providerBaseURL(sourceLujj31))
	if page == 1 && len(items) == 0 {
		return nil, false, errors.New("桃果网页未解析到影片列表")
	}
	return items, len(items) > 0 && query == "", nil
}

type lujj31Episode struct {
	url, title string
	sid, nid   int
}

func lujj31Episodes(doc *html.Node, base, id string) []lujj31Episode {
	classes := []string{"module-play-list-content", "playlist", "playlink", "content__playlist", "content__list", "tab-pane", "playList"}
	var anchors []*html.Node
	for _, class := range classes {
		for _, block := range providerHTMLNodes(doc, func(n *html.Node) bool { return lujj31HasClass(n, class) }) {
			anchors = append(anchors, providerHTMLNodes(block, func(n *html.Node) bool { return n.Data == "a" })...)
		}
	}
	if len(anchors) == 0 {
		anchors = providerHTMLNodes(doc, func(n *html.Node) bool { return n.Data == "a" })
	}
	lines := map[int][]lujj31Episode{}
	seen := map[string]bool{}
	for _, a := range anchors {
		link := lujj31SameSite(base, providerHTMLAttr(a, "href"))
		if link == "" || lujj31SourceID(link) != id || seen[link] {
			continue
		}
		p, _ := url.Parse(link)
		if !strings.Contains(p.Path, "/play/") && !strings.Contains(p.Path, "/vodplay/") {
			continue
		}
		seen[link] = true
		sid, nid := 1, len(lines[1])+1
		if m := lujj31PlayRoute.FindStringSubmatch(p.Path); len(m) == 3 {
			sid, _ = strconv.Atoi(m[1])
			nid, _ = strconv.Atoi(m[2])
		}
		if sid < 1 || sid > 100 || nid < 1 || nid > 10000 {
			continue
		}
		title := providerHTMLText(a)
		if title == "" {
			title = fmt.Sprintf("第%d集", nid)
		}
		lines[sid] = append(lines[sid], lujj31Episode{url: link, title: truncate(title, 128), sid: sid, nid: nid})
	}
	bestSID := 0
	var best []lujj31Episode
	for sid, line := range lines {
		if len(line) > len(best) || (len(line) == len(best) && (bestSID == 0 || sid < bestSID)) {
			bestSID = sid
			best = line
		}
	}
	sort.SliceStable(best, func(i, j int) bool { return best[i].nid < best[j].nid })
	out := []lujj31Episode{}
	numbers := map[int]bool{}
	for _, e := range best {
		if !numbers[e.nid] {
			numbers[e.nid] = true
			out = append(out, e)
		}
	}
	return out
}

func lujj31DetailFromHTML(doc *html.Node, base, id string) (Drama, []Chapter, error) {
	episodes := lujj31Episodes(doc, base, id)
	if len(episodes) == 0 {
		return Drama{}, nil, errors.New("桃果详情未找到属于所选影片的分集")
	}
	title := ""
	for _, class := range []string{"module-info-heading", "detail-title", "video-info-title", "page-title"} {
		if h := providerHTMLFirstClass(doc, class); h != nil {
			title = providerHTMLText(h)
			if title != "" {
				break
			}
		}
	}
	if title == "" {
		for _, h := range providerHTMLNodes(doc, func(n *html.Node) bool { return n.Data == "h1" }) {
			title = providerHTMLText(h)
			if title != "" {
				break
			}
		}
	}
	if title == "" {
		title = "影片 " + id
	}
	chapters := []Chapter{}
	for _, e := range episodes {
		chapters = append(chapters, Chapter{ID: providerChapterID(sourceLujj31, id, fmt.Sprintf("%d-%d", e.sid, e.nid)), Source: sourceLujj31, Title: e.title, CurrentEpisode: rawEpisode(e.nid), PageURL: e.url, VideoURL: "lujj31-play://" + e.url, Referer: base + "/"})
	}
	cover := lujj31DetailPoster(doc, base)
	return Drama{ID: providerDramaID(sourceLujj31, id), Source: sourceLujj31, SourceID: id, Title: title, Name: title, Cover: cover, CoverURL: cover, TotalEpisode: len(chapters), EpisodeCount: len(chapters), ChannelName: "桃果"}, chapters, nil
}

func (d *Downloader) fetchLujj31Detail(ctx context.Context, id string) (Drama, []Chapter, error) {
	if !webProviderNumericID.MatchString(id) {
		return Drama{}, nil, errors.New("桃果影片 ID 无效")
	}
	paths := []string{fmt.Sprintf("/index.php/vod/detail/id/%s.html", id), fmt.Sprintf("/index.php/vod/play/id/%s.html", id), fmt.Sprintf("/index.php/vod/play/id/%s/sid/1/nid/1.html", id)}
	var lastErr error
	for _, path := range paths {
		doc, _, err := d.lujj31Page(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		drama, chapters, err := lujj31DetailFromHTML(doc, d.providerBaseURL(sourceLujj31), id)
		if err == nil {
			return drama, chapters, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("桃果详情页不可用")
	}
	return Drama{}, nil, lastErr
}

func (d *Downloader) resolveLujj31Media(ctx context.Context, task Task) (providerMedia, error) {
	source, id, ok := splitProviderDramaID(task.DramaID)
	prefix := providerChapterID(sourceLujj31, id, "")
	if !ok || source != sourceLujj31 || !webProviderNumericID.MatchString(id) || !strings.HasPrefix(task.Chapter.ID, prefix) {
		return providerMedia{}, errors.New("桃果分集资料无效")
	}
	site := d.providerBaseURL(sourceLujj31)
	page := lujj31SameSite(site, task.Chapter.PageURL)
	if page == "" || lujj31SourceID(page) != id {
		return providerMedia{}, errors.New("桃果播放页与所选影片不一致")
	}
	u, _ := url.Parse(page)
	if !strings.Contains(u.Path, "/play/") && !strings.Contains(u.Path, "/vodplay/") {
		return providerMedia{}, errors.New("桃果播放路径无效")
	}
	return d.lujj31ResolvePreparedMedia(ctx, page, id)
}
