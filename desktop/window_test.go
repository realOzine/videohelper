package desktop

import "testing"

func TestOwnPage(t *testing.T) {
	for _, raw := range []string{"mygo://localhost/search.html", "http://mygo.localhost/video.html?id=1"} {
		if !ownPage(raw, false) {
			t.Fatalf("own page refused: %s", raw)
		}
	}
	for _, raw := range []string{"https://example.com", "mygo://localhost.evil/search.html", "http://127.0.0.1:5173/search.html", "mygo://user@localhost/search.html"} {
		if ownPage(raw, false) {
			t.Fatalf("external page allowed: %s", raw)
		}
	}
	if !ownPage("http://127.0.0.1:5173/search.html", true) {
		t.Fatal("development frontend refused")
	}
}
