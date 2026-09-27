package podcast

import (
	"bytes"
	"encoding/xml"
	"regexp"
	"strings"
	"time"

	"pod/pkg/backend"
)

type rssXML struct {
	XMLName xml.Name   `xml:"rss"`
	Channel channelXML `xml:"channel"`
}

type channelXML struct {
	Title         string                `xml:"title"`
	LastBuildDate string                `xml:"lastBuildDate"`
	PubDate       string                `xml:"pubDate"`
	Image         channelImageXML       `xml:"image"`
	ITunesImage   channelItunesImageXML `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	ITunesImage2  channelItunesImageXML `xml:"http://www.itunes.com/DTDs/Podcast-1.0.dtd image"`
	Items         []itemXML             `xml:"item"`
}

type channelImageXML struct {
	URL  string `xml:"url"`
	Href string `xml:"href,attr"`
}

type channelItunesImageXML struct {
	Href string `xml:"href,attr"`
	URL  string `xml:"url,attr"`
}

type itemXML struct {
	Title       string        `xml:"title"`
	Description string        `xml:"description"`
	PubDate     string        `xml:"pubDate"`
	GUID        guidXML       `xml:"guid"`
	ID          string        `xml:"id"`
	Enclosure   *enclosureXML `xml:"enclosure"`
	Duration    string        `xml:"duration"`
	Season      string        `xml:"season"`
	Episode     string        `xml:"episode"`
}

type guidXML struct {
	Value string `xml:",chardata"`
}

type enclosureXML struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

var feedTZOffsets = [...]struct {
	suffix string
	offset string
}{
	{" EDT", " -0400"},
	{" EST", " -0500"},
	{" CDT", " -0500"},
	{" CST", " -0600"},
	{" MDT", " -0600"},
	{" MST", " -0700"},
	{" PDT", " -0700"},
	{" PST", " -0800"},
	{" AKDT", " -0800"},
	{" AKST", " -0900"},
	{" HST", " -1000"},
	{" BST", " +0100"},
	{" GMT", " +0000"},
	{" UTC", " +0000"},
	{" CET", " +0100"},
	{" CEST", " +0200"},
	{" EET", " +0200"},
	{" EEST", " +0300"},
	{" AEST", " +1000"},
	{" AEDT", " +1100"},
	{" AWST", " +0800"},
	{" NZST", " +1200"},
	{" NZDT", " +1300"},
}

var feedDateFormats = [...]string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	time.RFC3339,
	time.RFC3339Nano,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 -0700",
	"02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05-0700",
	"2006-01-02",
}

func normalizeFeedTimezone(s string) string {
	for _, tz := range feedTZOffsets {
		if len(s) >= len(tz.suffix) && strings.EqualFold(s[len(s)-len(tz.suffix):], tz.suffix) {
			return s[:len(s)-len(tz.suffix)] + tz.offset
		}
	}
	return s
}

func parseFeedDate(pubDate string) (int64, string) {
	pubDate = strings.TrimSpace(pubDate)
	if pubDate == "" {
		return 0, ""
	}

	normalizedPubDate := normalizeFeedTimezone(pubDate)
	for _, layout := range feedDateFormats {
		if t, err := time.Parse(layout, normalizedPubDate); err == nil {
			return t.UnixMilli(), pubDate
		}
	}
	return 0, pubDate
}

var (
	itunesImageRegex = regexp.MustCompile(`(?i)<itunes:image[^>]+href=["']([^"']+)["']`)
	rssImageRegex    = regexp.MustCompile(`(?i)<image>[\s\S]*?<url>([^<]+)</url>`)
)

func extractChannelImageURL(c channelXML, data []byte) string {
	if u := strings.TrimSpace(c.ITunesImage.Href); u != "" {
		return u
	}
	if u := strings.TrimSpace(c.ITunesImage.URL); u != "" {
		return u
	}
	if u := strings.TrimSpace(c.ITunesImage2.Href); u != "" {
		return u
	}
	if u := strings.TrimSpace(c.Image.URL); u != "" {
		return u
	}
	if u := strings.TrimSpace(c.Image.Href); u != "" {
		return u
	}
	return scanRawXMLForImage(data)
}

func scanRawXMLForImage(data []byte) string {
	channelIdx := bytes.Index(data, []byte("<channel"))
	if channelIdx == -1 {
		return ""
	}
	channelHeader := data[channelIdx:]
	if itemIdx := bytes.Index(channelHeader, []byte("<item")); itemIdx != -1 {
		channelHeader = channelHeader[:itemIdx]
	}
	if m := itunesImageRegex.FindSubmatch(channelHeader); len(m) > 1 {
		return strings.TrimSpace(string(m[1]))
	}
	if m := rssImageRegex.FindSubmatch(channelHeader); len(m) > 1 {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}

// FeedDocument is a parsed RSS feed: its episodes plus the channel-level
// freshness markers used to decide whether the feed changed at all.
type FeedDocument struct {
	Title          string
	ImageURL       string
	LastBuildDate  string
	ChannelPubDate string
	Episodes       []backend.FeedEpisode
}

// LatestGUID is the identity of the feed's newest episode. Used as a
// change marker for feeds that serve neither HTTP validators nor a
// lastBuildDate, where the only way to tell whether anything is new is to look
// at the content itself.
func (d *FeedDocument) LatestGUID() string {
	if d == nil {
		return ""
	}
	var newest *backend.FeedEpisode
	for i := range d.Episodes {
		ep := &d.Episodes[i]
		if newest == nil || ep.PublishedAt > newest.PublishedAt {
			newest = ep
		}
	}
	if newest == nil {
		return ""
	}
	return episodeIdentity(*newest)
}

// episodeIdentity returns the most stable identifier available for an episode,
// preferring the GUID and falling back to the enclosure URL and then the title.
func episodeIdentity(ep backend.FeedEpisode) string {
	if g := strings.TrimSpace(ep.GUID); g != "" {
		return g
	}
	if ep.Enclosure != nil && strings.TrimSpace(ep.Enclosure.URL) != "" {
		return strings.TrimSpace(ep.Enclosure.URL)
	}
	if u := strings.TrimSpace(ep.EnclosureURL); u != "" {
		return u
	}
	return strings.ToLower(strings.TrimSpace(ep.Title))
}

func parseRSSFeed(data []byte) (*FeedDocument, error) {
	var rss rssXML
	if err := xml.Unmarshal(data, &rss); err != nil {
		return nil, err
	}

	doc := &FeedDocument{
		Title:          strings.TrimSpace(rss.Channel.Title),
		ImageURL:       extractChannelImageURL(rss.Channel, data),
		LastBuildDate:  strings.TrimSpace(rss.Channel.LastBuildDate),
		ChannelPubDate: strings.TrimSpace(rss.Channel.PubDate),
	}
	episodes := make([]backend.FeedEpisode, 0, len(rss.Channel.Items))
	for _, it := range rss.Channel.Items {
		if it.Enclosure == nil || strings.TrimSpace(it.Enclosure.URL) == "" {
			continue
		}

		guid := strings.TrimSpace(it.GUID.Value)
		if guid == "" {
			guid = strings.TrimSpace(it.ID)
		}
		if guid == "" && it.Enclosure != nil {
			guid = it.Enclosure.URL
		}

		pubMS, pubStr := parseFeedDate(it.PubDate)

		ep := backend.FeedEpisode{
			Title:            strings.TrimSpace(it.Title),
			DescriptionPlain: strings.TrimSpace(it.Description),
			PubDate:          pubStr,
			PublishedAt:      pubMS,
			GUID:             guid,
			Season:           strings.TrimSpace(it.Season),
			Episode:          strings.TrimSpace(it.Episode),
		}
		if it.Enclosure != nil && it.Enclosure.URL != "" {
			encURL := strings.TrimSpace(it.Enclosure.URL)
			if strings.HasPrefix(encURL, "http://") && !strings.HasPrefix(encURL, "http://127.0.0.1") && !strings.HasPrefix(encURL, "http://localhost") {
				encURL = "https://" + strings.TrimPrefix(encURL, "http://")
			}
			ep.EnclosureURL = encURL
			ep.Enclosure = &backend.FeedEnclosure{
				URL:  encURL,
				Type: strings.TrimSpace(it.Enclosure.Type),
			}
		}
		episodes = append(episodes, ep)
	}
	doc.Episodes = episodes
	return doc, nil
}
