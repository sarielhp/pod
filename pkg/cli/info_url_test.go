package cli

import "testing"

func TestPublishedURL(t *testing.T) {
	t.Parallel()

	got := publishedURL("http://host:8080/podcasts", "/media/podcasts/clean/The_Show", "feed.xml")
	want := "http://host:8080/podcasts/The_Show/feed.xml"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A trailing slash on the configured base must not double up.
	if got := publishedURL("http://host:8080/podcasts/", "/x/The_Show", "feed.xml"); got != want[:] && got != "http://host:8080/podcasts/The_Show/feed.xml" {
		t.Errorf("trailing slash mishandled: %q", got)
	}

	// Nothing is served when no base URL is configured, and claiming a URL
	// that does not resolve would be worse than printing none.
	if got := publishedURL("", "/x/The_Show", "feed.xml"); got != "" {
		t.Errorf("invented a URL with no base: %q", got)
	}
	if got := publishedURL("http://host/p", "", "feed.xml"); got != "" {
		t.Errorf("invented a URL with no directory: %q", got)
	}
}

func TestPublishedURLEscapesTheFolder(t *testing.T) {
	t.Parallel()
	// Folder names carry spaces and non-ASCII; an unescaped URL is not one a
	// client can fetch.
	got := publishedURL("http://host/p", "/x/A Show & More", "feed.xml")
	if got == "http://host/p/A Show & More/feed.xml" {
		t.Errorf("folder not escaped: %q", got)
	}
	if got := publishedURL("http://host/p", "/x/הבעיה", "feed.xml"); got == "" {
		t.Error("a non-ASCII folder produced no URL")
	}
}
