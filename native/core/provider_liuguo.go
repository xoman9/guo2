package core

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	nethtml "golang.org/x/net/html"
)

// Liuguo has different routes from Tao-guo. This ports the dedicated oktv
// provider_liuguo_catalog.go and MacCMS detail entry at commit 1ba9d1e.
// Lists are fetched on demand; there is deliberately no catalog prefetch.
const liuguoBaseURL = "https://www.llsp.me"

var (
	liuguoTypeRoute  = regexp.MustCompile(`(?i)^/vodtype/([0-9]{1,6})(?:\.html|/)$`)
	liuguoNavRoute   = regexp.MustCompile(`(?i)/(?:[a-z]*type|search|vodshow|label|gbook|rss|map|topic|actor|show/by)/|\?class=`)
	liuguoDetailLink = regexp.MustCompile(`(?i)/(?:index\.php/vod/)?(?:voddetail|detail|vod|show|view|movie|drama|duanju|zywview|xzyxvd)/`)
	liuguoLinkID     = regexp.MustCompile(`(?i)/(?:voddetail|vodplay|djplay|detail|show|vod|drama|duanju|movie|tv|id)/([0-9]+)(?:[-./]|$)`)
	liuguoPlayRoute  = regexp.MustCompile(`(?i)^/(?:vodplay|djplay)/([0-9]+)-([0-9]+)-([0-9]+)(?:\.html|/)$`)
	liuguoMarkup     = regexp.MustCompile(`<[^>]*>`)
)

var liuguoCardClasses = []string{
	"col-lg-2", "col-xl-2", "module-item", "module-poster-item", "public-list-box", "videoBox",
	"detail-list-item", "col-md-6", "col-6", "listItem", "FeaturedList_featuredItem", "BrowseList_listItem",
	"SecondList_secondListItem", "vodlist__item", "v_list", "entry-wrapper", "TagBookList_tagItem",
	"video-item", "vodlist_item", "myui-vodlist__box", "thumbnail", "media-card",
}

func liuguoSameSite(base, target string) string {
	site, err := url.Parse(base)
	if err != nil {
		return ""
	}
	relative, err := url.Parse(strings.TrimSpace(target))
	if err != nil || relative.User != nil {
		return ""
	}
	absolute := site.ResolveReference(relative)
	if (absolute.Scheme != "http" && absolute.Scheme != "https") || !strings.EqualFold(absolute.Hostname(), site.Hostname()) || absolute.User != nil {
		return ""
	}
	return absolute.String()
}

func liuguoID(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	path := strings.TrimSuffix(u.Path, ".html") + "/"
	if m := liuguoLinkID.FindStringSubmatch(path); len(m) > 1 {
		return m[1]
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if webProviderNumericID.MatchString(seg) {
			return seg
		}
		parts := strings.Split(seg, "-")
		if len(parts) >= 2 && webProviderNumericID.MatchString(parts[0]) && strings.Contains(path, "play") {
			return parts[0]
		}
	}
	if id := u.Query().Get("id"); webProviderNumericID.MatchString(id) {
		return id
	}
	return ""
}

func (d *Downloader) liuguoPage(ctx context.Context, path string) (*nethtml.Node, string, error) {
	site := d.providerBaseURL(sourceLiuguo)
	address := liuguoSameSite(site, path)
	if address == "" {
		return nil, "", errors.New("榴果网页地址无效")
	}
	doc, actual, err := d.fetchProviderPage(ctx, address, site+"/", lujj31UserAgent)
	if err != nil {
		return nil, "", err
	}
	if len(providerHTMLNodes(doc, func(node *nethtml.Node) bool { return node.Data == "body" && node.FirstChild != nil })) == 0 {
		return nil, "", errors.New("榴果网页内容为空")
	}
	return doc, actual, nil
}

func (d *Downloader) fetchLiuguoCategories(ctx context.Context) ([]nativeCategory, error) {
	doc, _, err := d.liuguoPage(ctx, "/")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	categories := []nativeCategory{}
	for _, a := range providerHTMLNodes(doc, func(n *nethtml.Node) bool { return n.Data == "a" }) {
		u, err := url.Parse(providerHTMLAttr(a, "href"))
		if err != nil {
			continue
		}
		m := liuguoTypeRoute.FindStringSubmatch(u.Path)
		if len(m) != 2 || seen[m[1]] {
			continue
		}
		name := strings.TrimSpace(providerHTMLText(a))
		if name == "" || name == "全部" || len([]rune(name)) > 28 {
			continue
		}
		seen[m[1]] = true
		categories = append(categories, nativeCategory{ID: m[1], Name: name})
		if len(categories) >= 48 {
			break
		}
	}
	// "全部" is provided by guo's nativeCategories, not repeated here.
	if len(categories) == 0 {
		return nil, errors.New("榴果首页未解析到分类链接")
	}
	return categories, nil
}

func liuguoTitle(card *nethtml.Node) string {
	for _, attribute := range []string{"title", "alt"} {
		for _, node := range providerHTMLNodes(card, func(n *nethtml.Node) bool { return n.Data == "a" || n.Data == "img" }) {
			raw := liuguoMarkup.ReplaceAllString(providerHTMLAttr(node, attribute), "")
			title := strings.Join(strings.Fields(html.UnescapeString(raw)), " ")
			if title != "" {
				return truncate(title, 512)
			}
		}
	}
	for _, a := range providerHTMLNodes(card, func(n *nethtml.Node) bool { return n.Data == "a" }) {
		if title := providerHTMLText(a); title != "" {
			return truncate(title, 512)
		}
	}
	return ""
}

func liuguoCards(doc *nethtml.Node, base string) []Drama {
	cards := []*nethtml.Node{}
	nodes := map[*nethtml.Node]bool{}
	for _, class := range liuguoCardClasses {
		for _, n := range providerHTMLNodes(doc, func(node *nethtml.Node) bool { return lujj31HasClass(node, class) }) {
			if !nodes[n] {
				nodes[n] = true
				cards = append(cards, n)
			}
		}
	}
	if len(cards) == 0 {
		for _, a := range providerHTMLNodes(doc, func(n *nethtml.Node) bool {
			return n.Data == "a" && liuguoDetailLink.MatchString(providerHTMLAttr(n, "href"))
		}) {
			for p := a.Parent; p != nil; p = p.Parent {
				if p.Data == "li" || p.Data == "div" {
					if !nodes[p] {
						nodes[p] = true
						cards = append(cards, p)
					}
					break
				}
			}
		}
	}
	ids := map[string]bool{}
	items := []Drama{}
	for _, card := range cards {
		link := ""
		for _, a := range providerHTMLNodes(card, func(n *nethtml.Node) bool { return n.Data == "a" }) {
			raw := providerHTMLAttr(a, "href")
			if raw == "" || liuguoNavRoute.MatchString(raw) {
				continue
			}
			candidate := liuguoSameSite(base, raw)
			if candidate != "" && liuguoID(candidate) != "" {
				link = candidate
				break
			}
		}
		id := liuguoID(link)
		if id == "" || ids[id] {
			continue
		}
		title := liuguoTitle(card)
		if title == "" {
			continue
		}
		ids[id] = true
		cover := lujj31CardPoster(card, link)
		items = append(items, Drama{ID: providerDramaID(sourceLiuguo, id), Source: sourceLiuguo, SourceID: id, Title: title, Name: title, Cover: cover, CoverURL: cover, ChannelName: "榴果"})
		if len(items) >= 200 {
			break
		}
	}
	return items
}

func (d *Downloader) fetchLiuguoCatalogPage(ctx context.Context, page int, category, query string) ([]Drama, bool, error) {
	if page < 1 || page > 100000 || len(query) > 256 || (category != "" && !webProviderNumericID.MatchString(category)) {
		return nil, false, errors.New("榴果分类或分页参数无效")
	}
	class := category
	if class == "" {
		class = "3"
	} // donor's category for default page
	path := fmt.Sprintf("/vodtype/%s.html", class)
	if page > 1 {
		path = fmt.Sprintf("/vodtype/%s-%d.html", class, page)
	}
	if query != "" {
		if page > 1 {
			return []Drama{}, false, nil
		}
		path = "/vodsearch/" + url.PathEscape(query) + "-.html"
	}
	doc, _, err := d.liuguoPage(ctx, path)
	if err != nil {
		return nil, false, err
	}
	items := liuguoCards(doc, d.providerBaseURL(sourceLiuguo))
	if len(items) == 0 && page == 1 {
		return nil, false, errors.New("榴果目录没有解析到影片")
	}
	return items, len(items) > 0 && query == "", nil
}

type liuguoEpisode struct {
	url, title string
	sid, nid   int
}

func liuguoEpisodes(doc *nethtml.Node, base, id string) []liuguoEpisode {
	classes := []string{"content__playlist", "content__list", "playlink", "pcDrama_catalogItem", "catalogItem",
		"playlist", "module-play-list-content", "tab-pane", "playList"}
	anchors := []*nethtml.Node{}
	for _, name := range classes {
		for _, block := range providerHTMLNodes(doc, func(n *nethtml.Node) bool { return lujj31HasClass(n, name) }) {
			anchors = append(anchors, providerHTMLNodes(block, func(n *nethtml.Node) bool { return n.Data == "a" })...)
		}
	}
	if len(anchors) == 0 {
		anchors = providerHTMLNodes(doc, func(n *nethtml.Node) bool { return n.Data == "a" })
	}
	groups := map[int][]liuguoEpisode{}
	seen := map[string]bool{}
	for _, a := range anchors {
		link := liuguoSameSite(base, providerHTMLAttr(a, "href"))
		if link == "" || seen[link] {
			continue
		}
		u, err := url.Parse(link)
		if err != nil {
			continue
		}
		m := liuguoPlayRoute.FindStringSubmatch(u.Path)
		if len(m) != 4 || m[1] != id {
			continue
		}
		sid, _ := strconv.Atoi(m[2])
		nid, _ := strconv.Atoi(m[3])
		if sid < 1 || sid > 100 || nid < 1 || nid > 10000 {
			continue
		}
		title := providerHTMLText(a)
		if title == "" {
			title = fmt.Sprintf("第%d集", nid)
		}
		seen[link] = true
		groups[sid] = append(groups[sid], liuguoEpisode{url: link, title: truncate(title, 128), sid: sid, nid: nid})
	}
	bestSID := 0
	var best []liuguoEpisode
	for sid, entries := range groups {
		if len(entries) > len(best) || (len(entries) == len(best) && (bestSID == 0 || sid < bestSID)) {
			bestSID = sid
			best = entries
		}
	}
	sort.SliceStable(best, func(i, j int) bool { return best[i].nid < best[j].nid })
	filtered := []liuguoEpisode{}
	nums := map[int]bool{}
	for _, entry := range best {
		if !nums[entry.nid] {
			filtered = append(filtered, entry)
			nums[entry.nid] = true
		}
	}
	return filtered
}

func liuguoDetailFromHTML(doc *nethtml.Node, base, id string) (Drama, []Chapter, error) {
	entries := liuguoEpisodes(doc, base, id)
	if len(entries) == 0 {
		return Drama{}, nil, errors.New("榴果详情页未解析到对应影片的播放分集")
	}
	title := ""
	for _, name := range []string{"module-info-heading", "detail-title", "video-info-title", "page-title"} {
		if n := providerHTMLFirstClass(doc, name); n != nil {
			title = providerHTMLText(n)
			if title != "" {
				break
			}
		}
	}
	if title == "" {
		for _, n := range providerHTMLNodes(doc, func(n *nethtml.Node) bool { return n.Data == "h1" }) {
			title = providerHTMLText(n)
			if title != "" {
				break
			}
		}
	}
	if title == "" {
		title = "影片 " + id
	}
	cover := lujj31DetailPoster(doc, base)
	chapters := []Chapter{}
	for _, entry := range entries {
		chapters = append(chapters, Chapter{
			ID:     providerChapterID(sourceLiuguo, id, fmt.Sprintf("%d-%d", entry.sid, entry.nid)),
			Source: sourceLiuguo, Title: entry.title, CurrentEpisode: rawEpisode(entry.nid),
			PageURL: entry.url, VideoURL: "liuguo-play://" + entry.url, Referer: base + "/",
		})
	}
	return Drama{ID: providerDramaID(sourceLiuguo, id), Source: sourceLiuguo, SourceID: id,
		Title: truncate(title, 512), Name: truncate(title, 512), Cover: cover, CoverURL: cover,
		TotalEpisode: len(chapters), EpisodeCount: len(chapters), ChannelName: "榴果"}, chapters, nil
}

func (d *Downloader) fetchLiuguoDetail(ctx context.Context, id string) (Drama, []Chapter, error) {
	if !webProviderNumericID.MatchString(id) {
		return Drama{}, nil, errors.New("榴果影片编号无效")
	}
	doc, _, err := d.liuguoPage(ctx, fmt.Sprintf("/vodplay/%s-1-1.html", id))
	if err != nil {
		return Drama{}, nil, err
	}
	return liuguoDetailFromHTML(doc, d.providerBaseURL(sourceLiuguo), id)
}

func liuguoAlternateRoutes(page string) []string {
	u, err := url.Parse(page)
	if err != nil {
		return nil
	}
	m := liuguoPlayRoute.FindStringSubmatch(u.Path)
	if len(m) != 4 {
		return nil
	}
	result := []string{}
	for sid := 1; sid <= 12; sid++ {
		if strconv.Itoa(sid) == m[2] {
			continue
		}
		alt := *u
		alt.Path = fmt.Sprintf("/vodplay/%s-%d-%s.html", m[1], sid, m[3])
		result = append(result, alt.String())
	}
	return result
}

func (d *Downloader) liuguoResolvePage(ctx context.Context, page, id string) (string, error) {
	base := d.providerBaseURL(sourceLiuguo)
	link := liuguoSameSite(base, page)
	u, err := url.Parse(link)
	if err != nil || link == "" {
		return "", errors.New("榴果播放页无效")
	}
	m := liuguoPlayRoute.FindStringSubmatch(u.Path)
	if len(m) != 4 || m[1] != id {
		return "", errors.New("榴果分集与所选影片不一致")
	}
	ctx = context.WithValue(ctx, providerTextUserAgentKey{}, lujj31UserAgent)
	body, err := d.fetchProviderText(ctx, link, base+"/")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		return "", errors.New("榴果播放页内容为空")
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

func (d *Downloader) resolveLiuguoMedia(ctx context.Context, task Task) (providerMedia, error) {
	source, id, valid := splitProviderDramaID(task.DramaID)
	if !valid || source != sourceLiuguo || !webProviderNumericID.MatchString(id) ||
		!strings.HasPrefix(task.Chapter.ID, providerChapterID(sourceLiuguo, id, "")) {
		return providerMedia{}, errors.New("榴果分集资料无效")
	}
	site := d.providerBaseURL(sourceLiuguo)
	page := liuguoSameSite(site, task.Chapter.PageURL)
	if page == "" {
		return providerMedia{}, errors.New("榴果播放页无效")
	}
	address, err := d.liuguoResolvePage(ctx, page, id)
	if err == nil && lujj31MediaLike(address) {
		media, mediaErr := d.prepareWebProviderMedia(ctx, providerMedia{URL: address, Referer: page}, "榴果")
		if mediaErr == nil {
			return media, nil
		}
		err = mediaErr
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for _, candidate := range liuguoAlternateRoutes(page) {
		if ctx.Err() != nil {
			break
		}
		alt, altErr := d.liuguoResolvePage(ctx, candidate, id)
		if altErr != nil || !lujj31MediaLike(alt) {
			continue
		}
		media, mediaErr := d.prepareWebProviderMedia(ctx, providerMedia{URL: alt, Referer: candidate}, "榴果")
		if mediaErr == nil {
			return media, nil
		}
	}
	if isProviderHTTPMediaURL(address) {
		return d.prepareWebProviderMedia(ctx, providerMedia{URL: address, Referer: page}, "榴果")
	}
	if err != nil {
		return providerMedia{}, err
	}
	return providerMedia{}, errors.New("榴果未取得可播放线路")
}
