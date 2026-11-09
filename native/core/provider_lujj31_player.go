package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The source-specific equivalents of oktv's maccmsPlayerURL,
// resolveMaccmsCloudParse and resolveMaccmsAlternateRoutes. Do not modify
// playback of Hongguo or any other provider.
var (
	lujj31PlayerRawFrom = regexp.MustCompile(`(?i)"from"\s*:\s*"([a-zA-Z0-9_]+)"`)
	lujj31PlayerJSPath  = regexp.MustCompile(`(?i)maccms\.path\s*\+\s*['"](/[^'"]*player[^'"]*)['"]`)
	lujj31MaccmsPath    = regexp.MustCompile(`(?i)"path"\s*:\s*"([^"]+)"`)
	lujj31IframeSrc     = regexp.MustCompile(`(?i)<iframe[^>]*\bsrc=["'](https?://[^"'\s]+)["']`)
	lujj31VideoURLVar   = regexp.MustCompile(`(?i)\b(?:var\s+)?video_url\s*=\s*['"](https?://[^'"]+)['"]`)
	lujj31DollarURL     = regexp.MustCompile(`(?i)\$\.url\s*=\s*["'](https?://[^"']+)["']`)
	lujj31IframeB64     = regexp.MustCompile(`(?is)<iframe[^>]*[?&]url=([A-Za-z0-9+/]{20,}={0,2})`)
	lujj31SID           = regexp.MustCompile(`(?i)(/sid/)([0-9]+)(/nid/)`)
)

func lujj31DecodeJS(value string) string {
	if !strings.Contains(value, "%") {
		return value
	}
	// MacCMS's player.js uses JavaScript escape(), which may contain %uXXXX.
	unicode := regexp.MustCompile(`(?i)%u([0-9a-f]{4})`)
	value = unicode.ReplaceAllStringFunc(value, func(seq string) string {
		n, err := strconv.ParseUint(seq[2:], 16, 32)
		if err != nil {
			return seq
		}
		return string(rune(n))
	})
	if out, err := url.QueryUnescape(value); err == nil {
		return out
	}
	return value
}

func lujj31Decrypt(raw, encrypt string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\/`, "/"))
	switch encrypt {
	case "1":
		return lujj31DecodeJS(raw)
	case "2":
		for _, candidate := range []string{raw, lujj31DecodeJS(raw)} {
			decoded, err := base64.StdEncoding.DecodeString(candidate)
			if err != nil {
				decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(candidate, "="))
			}
			if err == nil {
				return lujj31DecodeJS(string(decoded))
			}
		}
	}
	return raw
}

func lujj31PlayerFields(body, id string) map[string]string {
	fields := map[string]string{}
	for _, offset := range lujj31PlayerData.FindAllStringIndex(body, 10) {
		decoder := json.NewDecoder(strings.NewReader(body[offset[1]:]))
		var record map[string]any
		if err := decoder.Decode(&record); err != nil || record == nil {
			continue
		}
		// Drama identity is checked against the same-origin page URL already.
		// Player "id" is not always the drama ID on MacCMS templates.
		raw := mapString(record, "url")
		if raw == "" {
			continue
		}
		for _, key := range []string{"url", "encrypt", "from"} {
			fields[key] = mapString(record, key)
		}
		fields["url"] = lujj31Decrypt(fields["url"], fields["encrypt"])
		return fields
	}
	return fields
}

func lujj31DirectPlayerURL(body string) string {
	if match := lujj31DollarURL.FindStringSubmatch(body); len(match) > 1 {
		return strings.ReplaceAll(match[1], `\/`, "/")
	}
	if address := lujj31DirectMedia.FindString(body); address != "" {
		return strings.ReplaceAll(address, `\/`, "/")
	}
	if match := lujj31IframeB64.FindStringSubmatch(body); len(match) > 1 {
		decoded, err := base64.StdEncoding.DecodeString(match[1])
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(match[1], "="))
		}
		if err == nil {
			for _, entry := range strings.Split(string(decoded), ",") {
				entry = strings.TrimSpace(entry)
				if isProviderHTTPMediaURL(entry) {
					return entry
				}
			}
		}
	}
	return ""
}

func lujj31MediaLike(address string) bool {
	u, err := url.Parse(address)
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	return strings.HasSuffix(p, ".mp4") || strings.HasSuffix(p, ".m3u8") || strings.HasSuffix(p, ".flv")
}

// An HTTP parser/player endpoint is accepted as in oktv. A later media
// attempt determines whether that endpoint is playable; prefer direct media.
func lujj31PlayerURL(body, id string) (string, error) {
	fields := lujj31PlayerFields(body, id)
	if address := fields["url"]; isProviderHTTPMediaURL(address) {
		return address, nil
	}
	if address := lujj31DirectPlayerURL(body); isProviderHTTPMediaURL(address) {
		return address, nil
	}
	return "", errors.New("桃果播放页未解析到 HTTP 播放地址")
}

func lujj31SafeParseEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
		return false
	}
	return true
}

// Ports oktv's MacCMS cloud parser: player/<from>.js declares an iframe
// parser endpoint; the resulting page may declare var video_url or an HLS link.
func (d *Downloader) lujj31CloudParse(ctx context.Context, body, encrypted, page string) string {
	return d.maccmsSiteCloudParse(ctx, d.providerBaseURL(sourceLujj31), body, encrypted, page)
}

// Shared, source-scoped implementation for Tao-guo and Liuguo's oktv cloud player.
func (d *Downloader) maccmsSiteCloudParse(ctx context.Context, base, body, encrypted, page string) string {
	if encrypted == "" || isProviderHTTPMediaURL(encrypted) {
		return ""
	}
	from := ""
	if m := lujj31PlayerRawFrom.FindStringSubmatch(body); len(m) > 1 {
		from = m[1]
	}
	dir := "/static/player/"
	if m := lujj31PlayerJSPath.FindStringSubmatch(body); len(m) > 1 {
		dir = m[1]
	} else if m := lujj31MaccmsPath.FindStringSubmatch(body); len(m) > 1 {
		dir = strings.TrimRight(m[1], "/") + "/static/player/"
	}
	// oktv probes the active player script first, then the two common
	// MacCMS player scripts. Do not rely on a single /<from>.js endpoint.
	paths := []string{}
	if from != "" {
		paths = append(paths, dir+from+".js")
	}
	paths = append(paths, "/static/player/m3u8.js", "/static/player/mac.js")
	ua := context.WithValue(ctx, providerTextUserAgentKey{}, lujj31UserAgent)
	parseBase := ""
	for _, path := range paths {
		script := lujj31SameSite(base, base+path)
		if script == "" {
			continue
		}
		js, err := d.fetchProviderText(ua, script, page)
		if err != nil || len(js) > 512*1024 {
			continue
		}
		if iframe := lujj31IframeSrc.FindStringSubmatch(js); len(iframe) > 1 {
			parseBase = strings.ReplaceAll(iframe[1], `\/`, "/")
			break
		}
	}
	if parseBase == "" {
		return ""
	}
	if !strings.Contains(parseBase, "?") {
		parseBase += "?url="
	}
	endpoint := parseBase + encrypted
	if !lujj31SafeParseEndpoint(endpoint) {
		return ""
	}
	response, err := d.fetchProviderText(ua, endpoint, page)
	if err != nil || len(response) > 512*1024 {
		return ""
	}
	if m := lujj31VideoURLVar.FindStringSubmatch(response); len(m) > 1 && isProviderHTTPMediaURL(m[1]) {
		return m[1]
	}
	if direct := lujj31DirectPlayerURL(response); isProviderHTTPMediaURL(direct) {
		return direct
	}
	return ""
}

func (d *Downloader) lujj31ResolvePage(ctx context.Context, page, id string) (string, error) {
	site := d.providerBaseURL(sourceLujj31)
	if lujj31SameSite(site, page) == "" || lujj31SourceID(page) != id {
		return "", errors.New("桃果播放页网址不属于该影片")
	}
	ua := context.WithValue(ctx, providerTextUserAgentKey{}, lujj31UserAgent)
	body, err := d.fetchProviderText(ua, page, site+"/")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		return "", errors.New("桃果播放页未返回内容")
	}
	fields := lujj31PlayerFields(body, id)
	address, err := lujj31PlayerURL(body, id)
	if err == nil && lujj31MediaLike(address) {
		return address, nil
	}
	if encrypted := fields["url"]; encrypted != "" && !isProviderHTTPMediaURL(encrypted) {
		if parsed := d.lujj31CloudParse(ctx, body, encrypted, page); isProviderHTTPMediaURL(parsed) {
			return parsed, nil
		}
	}
	if err == nil {
		return address, nil
	}
	return "", err
}

func lujj31AltRouteURLs(page string) []string {
	u, err := url.Parse(page)
	if err != nil || u.Host == "" {
		return nil
	}
	match := lujj31SID.FindStringSubmatchIndex(u.Path)
	if len(match) < 8 {
		return nil
	}
	origSID := u.Path[match[4]:match[5]]
	output := []string{}
	for sid := 1; sid <= 12; sid++ {
		if strconv.Itoa(sid) == origSID {
			continue
		}
		other := *u
		other.Path = u.Path[:match[4]] + strconv.Itoa(sid) + u.Path[match[5]:]
		output = append(output, other.String())
	}
	return output
}

func (d *Downloader) lujj31ResolveAlternative(ctx context.Context, page, id string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var failure error
	for _, candidate := range lujj31AltRouteURLs(page) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		address, err := d.lujj31ResolvePage(ctx, candidate, id)
		if err != nil {
			failure = err
			continue
		}
		if lujj31MediaLike(address) {
			return address, nil
		}
		// Fallback to non-file HTTP only after trying direct media candidates.
		if failure == nil {
			failure = fmt.Errorf("桃果其他线路只返回网页地址")
		}
	}
	if failure == nil {
		failure = errors.New("桃果未找到可用的其它线路")
	}
	return "", failure
}

func (d *Downloader) lujj31ResolvePreparedMedia(ctx context.Context, page, id string) (providerMedia, error) {
	primary, err := d.lujj31ResolvePage(ctx, page, id)
	if err == nil && lujj31MediaLike(primary) {
		result, prepareErr := d.prepareWebProviderMedia(ctx, providerMedia{URL: primary, Referer: page}, "桃果")
		if prepareErr == nil {
			return result, nil
		}
		err = prepareErr
	}
	if alternative, altErr := d.lujj31ResolveAlternative(ctx, page, id); altErr == nil {
		return d.prepareWebProviderMedia(ctx, providerMedia{URL: alternative, Referer: page}, "桃果")
	}
	if isProviderHTTPMediaURL(primary) {
		// Matches oktv's last-resort direct HTTP player route behavior.
		return d.prepareWebProviderMedia(ctx, providerMedia{URL: primary, Referer: page}, "桃果")
	}
	if err != nil {
		return providerMedia{}, err
	}
	return providerMedia{}, errors.New("桃果未取得可播放线路")
}
