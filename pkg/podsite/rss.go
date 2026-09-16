package podsite

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"pod/pkg/format"
)

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	ITunes  string     `xml:"xmlns:itunes,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string          `xml:"title"`
	Link        string          `xml:"link"`
	Description string          `xml:"description"`
	Language    string          `xml:"language,omitempty"`
	Author      string          `xml:"itunes:author,omitempty"`
	Image       *rssFeedImage   `xml:"image,omitempty"`
	ITunesImage *rssItunesImage `xml:"itunes:image,omitempty"`
	Items       []rssItemXML    `xml:"item"`
}

type rssFeedImage struct {
	URL   string `xml:"url"`
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

type rssItunesImage struct {
	Href string `xml:"href,attr"`
}

type rssItemXML struct {
	Title       string          `xml:"title"`
	Description rssCData        `xml:"description"`
	PubDate     string          `xml:"pubDate"`
	GUID        rssGUID         `xml:"guid"`
	Enclosure   rssEnclosure    `xml:"enclosure"`
	Duration    string          `xml:"itunes:duration,omitempty"`
	Image       *rssItunesImage `xml:"itunes:image,omitempty"`
}

type rssCData struct {
	Value string `xml:",cdata"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

// RenderFeed produces an iTunes-compatible RSS 2.0 document for one show.
//
// baseURL is the public root the library is served from; when it is empty the
// feed is rendered with document-relative URLs, which keeps it usable when the
// directory is opened directly rather than served.
func RenderFeed(show Show, episodes []Episode, baseURL string) ([]byte, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	folder := url.PathEscape(show.Folder)

	channelURL := fmt.Sprintf("%s/%s/", baseURL, folder)
	if baseURL == "" {
		channelURL = fmt.Sprintf("/%s/", folder)
	}
	channel := rssChannel{
		Title:       show.Title,
		Link:        channelURL,
		Description: fmt.Sprintf("Ad-free podcast feed for %s", show.Title),
		Language:    "en",
		Author:      "pod",
	}

	coverURL := feedCoverURL(show, baseURL, folder)
	if coverURL != "" {
		channel.Image = &rssFeedImage{URL: coverURL, Title: show.Title, Link: channelURL}
		channel.ITunesImage = &rssItunesImage{Href: coverURL}
	}

	for _, ep := range episodes {
		channel.Items = append(channel.Items, feedItem(ep, baseURL, folder, coverURL))
	}

	doc := rssDocument{
		Version: "2.0",
		ITunes:  "http://www.itunes.com/dtds/podcast-1.0.dtd",
		Channel: channel,
	}
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal feed xml: %w", err)
	}
	header := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	return append(header, append(data, '\n')...), nil
}

func feedCoverURL(show Show, baseURL, escapedFolder string) string {
	if show.CoverFile != "" && baseURL != "" {
		return fmt.Sprintf("%s/%s/%s", baseURL, escapedFolder, url.PathEscape(show.CoverFile))
	}
	return show.ImageURL
}

func feedItem(ep Episode, baseURL, escapedFolder, coverURL string) rssItemXML {
	// An episode that is not held locally is published pointing at its
	// original enclosure, so subscribing to this feed still gives the show's
	// whole run rather than only the downloaded part.
	encURL := ep.RemoteURL
	if ep.Local() {
		encURL = escapeRelPath(ep.Filename)
		if baseURL != "" {
			encURL = fmt.Sprintf("%s/%s/%s", baseURL, escapedFolder, escapeRelPath(ep.Filename))
		}
	}
	item := rssItemXML{
		Title:       ep.Title,
		Description: rssCData{Value: ep.Description},
		PubDate:     ep.PubDate,
		GUID:        rssGUID{IsPermaLink: false, Value: ep.GUID},
		Enclosure: rssEnclosure{
			URL:    encURL,
			Length: ep.SizeBytes,
			Type:   "audio/mpeg",
		},
	}
	if ep.DurationSec > 0 {
		item.Duration = format.FormatClock(ep.DurationSec)
	}
	if coverURL != "" {
		item.Image = &rssItunesImage{Href: coverURL}
	}
	return item
}

func escapeRelPath(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	for i, seg := range parts {
		parts[i] = url.PathEscape(seg)
	}
	return strings.Join(parts, "/")
}
