package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tariffCalculator/store"
)

// slowPost sends a multipart POST to path with the body in chunks, pausing
// between them, and returns the status and page. cut closes the connection
// after the first chunk when the server hasn't answered by then.
func slowPost(t *testing.T, srv *httptest.Server, path string, contentType string, chunks []string, pause time.Duration, declared int) (int, string) {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: x\r\nX-Feedback: 1\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n", path, contentType, declared)
	for i, c := range chunks {
		if i > 0 {
			time.Sleep(pause)
		}
		if _, err := io.WriteString(conn, c); err != nil {
			break
		}
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestSlowUploadsGetLongerToSend(t *testing.T) {
	uploadReadTimeout = 800 * time.Millisecond
	t.Cleanup(func() { uploadReadTimeout = 90 * time.Second })
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	mux := http.NewServeMux()
	mux.Handle("/", routesWith(st))
	mux.HandleFunc("/probe", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusRequestTimeout)
		}
	})
	srv := httptest.NewUnstartedServer(mux)
	srv.Config.ReadTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	const multipart = "multipart/form-data; boundary=BB"
	field := "--BB\r\nContent-Disposition: form-data; name=\"text\"\r\n\r\nhello\r\n"
	end := "--BB--\r\n"
	whole := field + end
	pieces := []string{field[:20], field[20:], end}

	// Paused for longer than the general timeout, but within the upload one.
	status, page := slowPost(t, srv, "/feedback", multipart, pieces, 300*time.Millisecond, len(whole))
	if strings.Contains(page, "too slow") || status == http.StatusRequestEntityTooLarge {
		t.Errorf("a slow feedback post was cut: %d %q", status, page)
	}

	// The same on an ordinary route is cut by the general timeout.
	status, _ = slowPost(t, srv, "/probe", "application/x-www-form-urlencoded", []string{"skill=", "x", "&a=b"}, 300*time.Millisecond, 12)
	if status != http.StatusRequestTimeout {
		t.Errorf("a slow body on an ordinary route got through: %d", status)
	}

	// A body that stalls past the upload deadline says the connection was slow.
	status, page = slowPost(t, srv, "/feedback", multipart, []string{field[:20], field[20:]}, 1000*time.Millisecond, len(whole))
	if status != http.StatusRequestTimeout || !strings.Contains(page, "connection was too slow") {
		t.Errorf("a stalled upload: %d %q", status, page)
	}

	// A body that is too big keeps its own message.
	big := strings.Repeat("a", maxFeedbackBytes+10)
	status, page = slowPost(t, srv, "/feedback", multipart, []string{field[:20], field[20:] + big}, 0, len(whole)+len(big))
	if status != http.StatusRequestEntityTooLarge || !strings.Contains(page, "too much") {
		t.Errorf("a too-big body: %d %q", status, page)
	}
}
