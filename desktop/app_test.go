package main

import "testing"

func TestScrubURLs(t *testing.T) {
	cases := []struct{ in, want string }{
		// Go wraps fetch errors in *url.Error, quoting the full URL.
		{`Get "https://sub.host/api/v1/client/subscribe?token=SECRET": dial tcp: lookup failure`,
			`Get "https://sub.host/api/v1/client/subscribe": dial tcp: lookup failure`},
		// Redirect targets embedded in errors get the same treatment.
		{`302 to https://other.host/dl?sig=abc%2Fdef stopped`,
			`302 to https://other.host/dl stopped`},
		// Fragments are dropped too; text without URLs passes through.
		{`сервер ответил 502 Bad Gateway`, `сервер ответил 502 Bad Gateway`},
		{`https://a.b/c#frag и https://d.e/f?x=1`, `https://a.b/c и https://d.e/f`},
	}
	for _, c := range cases {
		if got := scrubURLs(c.in); got != c.want {
			t.Errorf("scrubURLs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
