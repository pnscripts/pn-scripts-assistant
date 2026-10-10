package tools

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

/*
 * A DevTools endpoint that speaks the real protocol over a real WebSocket,
 * so what is tested is the frames and the handshake this program writes, not
 * a function standing in for them. The live test below drives a real Chrome.
 */
func fakeDevTools(t *testing.T) (port int) {
	t.Helper()

	mux := http.NewServeMux()

	var srv *httptest.Server

	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"Browser":"Fake/1"}`)
	})

	mux.HandleFunc("/json/list", func(w http.ResponseWriter, _ *http.Request) {
		ws := "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/devtools/page/T1"
		json.NewEncoder(w).Encode([]map[string]string{
			{"id": "T1", "type": "page", "title": "Games", "url": "https://example.test/", "webSocketDebuggerUrl": ws},
			{"id": "W1", "type": "service_worker", "title": "sw", "url": "https://example.test/sw.js"},
		})
	})

	mux.HandleFunc("/json/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "use PUT", http.StatusMethodNotAllowed)

			return
		}

		io.WriteString(w, `{"id":"T2","type":"page","url":`+strconv.Quote(r.URL.RawQuery)+`}`)
	})

	mux.HandleFunc("/devtools/page/T1", func(w http.ResponseWriter, r *http.Request) {
		accept := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))

		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()

		rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(accept[:]) + "\r\n\r\n")
		rw.Flush()

		page := &CDPPage{conn: conn, r: rw.Reader}

		for {
			msg, err := page.readMessage()
			if err != nil {
				return
			}

			var req struct {
				ID     int            `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}

			json.Unmarshal(msg, &req)

			// An event first, which the client must skip.
			serverFrame(conn, `{"method":"Page.loadEventFired","params":{}}`)

			var result string

			switch req.Method {
			case "Runtime.evaluate":
				expr, _ := req.Params["expression"].(string)

				switch {
				case strings.Contains(expr, "throw"):
					result = `{"result":{"type":"object"},"exceptionDetails":{"text":"Uncaught","exception":{"description":"Error: boom"}}}`
				case strings.Contains(expr, "innerText"):
					result = `{"result":{"type":"string","value":"Hello from the page"}}`
				default:
					result = `{"result":{"type":"object","value":{"rows":3}}}`
				}
			case "Page.captureScreenshot":
				result = `{"data":"` + base64.StdEncoding.EncodeToString([]byte("\x89PNG fake")) + `"}`
			default:
				result = `{}`
			}

			serverFrame(conn, `{"id":`+strconv.Itoa(req.ID)+`,"result":`+result+`}`)
		}
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	port, _ = strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])

	return port
}

// serverFrame writes an unmasked text frame, as a server does.
func serverFrame(conn net.Conn, text string) {
	frame := []byte{0x81}

	if n := len(text); n < 126 {
		frame = append(frame, byte(n))
	} else {
		frame = append(frame, 126, 0, 0)
		binary.BigEndian.PutUint16(frame[2:], uint16(n))
	}

	conn.Write(append(frame, text...))
}

func drive(t *testing.T, d DriveBrowser, args string) (string, error) {
	t.Helper()

	return d.Execute(context.Background(), json.RawMessage(args))
}

func TestTheBrowserIsDrivenOverDevTools(t *testing.T) {
	d := DriveBrowser{Port: fakeDevTools(t), Chrome: "/nonexistent"}

	out, err := drive(t, d, `{"action":"tabs"}`)
	if err != nil || !strings.Contains(out, `"T1"`) || strings.Contains(out, "W1") {
		t.Fatalf("tabs: %v %s", err, out)
	}

	out, err = drive(t, d, `{"action":"open","url":"https://itch.io/game/new"}`)
	if err != nil || !strings.Contains(out, `"T2"`) {
		t.Fatalf("open: %v %s", err, out)
	}

	out, err = drive(t, d, `{"action":"eval","tab":"T1","expression":"({rows: document.querySelectorAll('tr').length})"}`)
	if err != nil || out != `{"rows":3}` {
		t.Fatalf("eval: %v %s", err, out)
	}

	if _, err = drive(t, d, `{"action":"eval","tab":"T1","expression":"throw new Error('boom')"}`); err == nil ||
		!strings.Contains(err.Error(), "boom") {
		t.Fatalf("a page that threw was not reported: %v", err)
	}

	out, err = drive(t, d, `{"action":"text","tab":"T1"}`)
	if err != nil || out != "Hello from the page" {
		t.Fatalf("text: %v %q", err, out)
	}

	png := filepath.Join(t.TempDir(), "shots", "cover.png")

	if _, err = drive(t, d, `{"action":"screenshot","tab":"T1","path":"`+png+`"}`); err != nil {
		t.Fatal(err)
	}

	if raw, _ := os.ReadFile(png); string(raw) != "\x89PNG fake" {
		t.Errorf("the screenshot was not written: %q", raw)
	}

	if _, err = drive(t, d, `{"action":"eval","tab":"nope","expression":"1"}`); err == nil {
		t.Error("a tab that does not exist was driven")
	}
}

// The screenshot says where it writes, so project work can be held to it.
func TestTheBrowserSaysWhereItWrites(t *testing.T) {
	writes, _ := DriveBrowser{}.Touches(json.RawMessage(`{"action":"screenshot","path":"/p/shot.png"}`))
	if len(writes) != 1 || writes[0] != "/p/shot.png" {
		t.Errorf("screenshot touches %v", writes)
	}

	writes, _ = DriveBrowser{}.Touches(json.RawMessage(`{"action":"eval","tab":"x","expression":"1"}`))
	if len(writes) != 0 {
		t.Errorf("eval touches %v", writes)
	}
}

/*
 * The real thing: a headless Chrome on this machine, started by the tool,
 * a page opened, read, evaluated and photographed. Only when asked for, since
 * it starts a browser: PN_LIVE_BROWSER=1.
 */
func TestTheBrowserDrivesARealChrome(t *testing.T) {
	if os.Getenv("PN_LIVE_BROWSER") == "" {
		t.Skip("set PN_LIVE_BROWSER=1 to drive a real Chrome")
	}

	if browserOnThisMachine() == "" {
		t.Skip("no Chrome here")
	}

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	page := filepath.Join(t.TempDir(), "page.html")
	os.WriteFile(page, []byte(`<html><body><h1>Tetris</h1><button id=b onclick="this.textContent='played'">play</button></body></html>`), 0o644)

	d := DriveBrowser{Port: port, Profile: filepath.Join(t.TempDir(), "profile"), Headless: true}

	t.Cleanup(func() {
		// Stopped by its own port, never by name: other Chromes on this
		// machine are somebody's.
		out, _ := exec.Command("ss", "-ltnpH", "sport = :"+strconv.Itoa(port)).Output()
		if i := strings.Index(string(out), "pid="); i >= 0 {
			pid, _ := strconv.Atoi(strings.FieldsFunc(string(out)[i+4:], func(r rune) bool { return r < '0' || r > '9' })[0])
			if p, err := os.FindProcess(pid); err == nil && pid > 1 {
				p.Kill()
			}
		}
	})

	out, err := drive(t, d, `{"action":"open","url":"file://`+page+`"}`)
	if err != nil {
		t.Fatal(err)
	}

	var opened struct{ Tab string }
	json.Unmarshal([]byte(out), &opened)

	if out, err = drive(t, d, `{"action":"eval","tab":"`+opened.Tab+`","expression":"new Promise(r => setTimeout(() => { document.getElementById('b').click(); r(document.getElementById('b').textContent) }, 300))"}`); err != nil || out != `"played"` {
		t.Fatalf("eval: %v %s", err, out)
	}

	if out, err = drive(t, d, `{"action":"text","tab":"`+opened.Tab+`"}`); err != nil || !strings.Contains(out, "Tetris") {
		t.Fatalf("text: %v %q", err, out)
	}

	shot := filepath.Join(t.TempDir(), "shot.png")

	if _, err = drive(t, d, `{"action":"screenshot","tab":"`+opened.Tab+`","path":"`+shot+`"}`); err != nil {
		t.Fatal(err)
	}

	if raw, _ := os.ReadFile(shot); len(raw) < 100 || string(raw[1:4]) != "PNG" {
		t.Errorf("not a picture: %d bytes", len(raw))
	}

}
