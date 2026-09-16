package backend

import "testing"

func TestStripHTMLEntities(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want, what string }{
		{"<p>Hello</p>", "Hello", "tags are dropped"},
		{"a &amp; b", "a & b", "a known entity is decoded"},
		{"&lt;tag&gt;", "<tag>", "decoded entities are not re-parsed as tags"},
		{"5 &frac12; cups", "5 &frac12; cups", "an unknown entity passes through"},
		{"AT&T stock", "AT&T stock", "a bare ampersand with no semicolon survives"},
		{"a &amp b", "a &amp b", "an unterminated entity is left alone"},
		{"&quot;x&quot;", `"x"`, "quotes decode"},
		{"a&nbsp;b", "a b", "non-breaking space becomes a space"},
		{"  spaced  ", "spaced", "surrounding whitespace is trimmed"},
		{"<a href=\"x\">link</a> tail", "link tail", "attributes inside tags are dropped"},
	}
	for _, c := range cases {
		if got := StripHTML(c.in); got != c.want {
			t.Errorf("%s: StripHTML(%q) = %q, want %q", c.what, c.in, got, c.want)
		}
	}
}
