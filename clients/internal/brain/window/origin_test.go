package window

import "testing"

func TestOnlyTheProgramsOwnAddressStaysInTheWindow(t *testing.T) {
	own := "http://127.0.0.1:8815/"

	for _, same := range []string{"http://127.0.0.1:8815/#chat", "http://127.0.0.1:8815/settings?x=1", "HTTP://127.0.0.1:8815"} {
		if !sameOrigin(own, same) {
			t.Errorf("%s was sent out of the window", same)
		}
	}

	for _, other := range []string{
		"http://127.0.0.1:8816/", "https://127.0.0.1:8815/", "http://localhost:8815/",
		"https://example.com/", "file:///C:/Users/ana/x.html", "about:blank", "javascript:alert(1)", "",
	} {
		if sameOrigin(own, other) {
			t.Errorf("%s would stay in the window", other)
		}
	}
}
