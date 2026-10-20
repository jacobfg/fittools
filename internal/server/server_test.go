package server

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jacobfg/fittools/internal/fit"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("../../testdata/web.input.bin")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func request(t *testing.T, s *Server, path string, data []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	p, e := mw.CreateFormFile("fit_file", "Ultra Run.fit")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = p.Write(data)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req := httptest.NewRequest("POST", path, &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	entries, e := os.ReadDir(s.TempDir)
	if e != nil || len(entries) != 0 {
		t.Fatalf("temporary files leaked: %v %v", entries, e)
	}
	return w
}
func testServer(t *testing.T) *Server {
	s := New(fstest.MapFS{"index.html": {Data: []byte("Fittools")}}, 1)
	s.TempDir = t.TempDir()
	return s
}
func TestHTTP(t *testing.T) {
	s := testServer(t)
	b := fixture(t)
	for _, path := range []string{"/", "/healthz"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	for _, path := range []string{"/inspect-identity", "/activity-details"} {
		w := request(t, s, path, b, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var d map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &d); e != nil {
			t.Fatal(e)
		}
		if path == "/inspect-identity" {
			if d["firmware"] != "6.38" || d["serial"] != float64(100000001) {
				t.Fatal(d)
			}
		} else {
			device := d["device"].(map[string]any)
			activity := d["activity"].(map[string]any)
			if device["name"] != "Garmin Fenix 9 Pro 47mm" || activity["distance_metres"] != 1234.5 || activity["timer_seconds"] != float64(30) {
				t.Fatal(d)
			}
		}
	}
	for _, action := range []string{"pauses", "continuous", "identity"} {
		fields := map[string]string{"action": action, "manufacturer": "1", "product": "4315", "serial": "100000002"}
		w := request(t, s, "/transform", b, fields)
		if w.Code != 200 {
			t.Fatal(action, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Header().Get("Content-Disposition"), "Ultra-Run-") {
			t.Fatal(w.Header())
		}
		f, e := fit.Parse(w.Body.Bytes())
		if e != nil {
			t.Fatal(e)
		}
		if action == "identity" {
			d, e := f.RecordingDevice()
			if e != nil || d.Product != 4315 || *d.Firmware != "6.38" {
				t.Fatal(d, e)
			}
		} else {
			count := 0
			for _, r := range f.Records {
				if !r.IsDefinition && r.Def.Global == 20 {
					count++
				}
			}
			want := 2
			if action == "continuous" {
				want = 4
			}
			if count != want {
				t.Fatal(count)
			}
		}
	}
	w := request(t, s, "/transform", b, map[string]string{"action": "identity", "manufacturer": "1", "product": "4315", "serial": "100000002", "firmware": "28.05"})
	f, e := fit.Parse(w.Body.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range f.Records {
		if !r.IsDefinition && r.Def.Global == 23 {
			index, _ := r.Value(0)
			if index == 2 {
				version, _ := r.Value(5)
				if version != 1102 {
					t.Fatal("component firmware changed")
				}
			}
		}
	}
	d, e := f.RecordingDevice()
	if e != nil || *d.Firmware != "28.05" {
		t.Fatal(d, e)
	}
	restored := request(t, s, "/transform", w.Body.Bytes(), map[string]string{"action": "identity", "manufacturer": "1", "product": "4953", "serial": "100000001", "firmware": "6.38"})
	if restored.Code != 200 || !bytes.Equal(restored.Body.Bytes(), b) {
		t.Fatalf("HTTP firmware round trip did not restore original bytes: status %d", restored.Code)
	}

}
func TestErrorsAndLimits(t *testing.T) {
	s := testServer(t)
	b := fixture(t)
	for _, c := range []struct {
		body   []byte
		fields map[string]string
	}{{[]byte("invalid"), map[string]string{"action": "pauses"}}, {b, map[string]string{"action": "unknown"}}, {b, map[string]string{"action": "identity", "manufacturer": "1", "product": "4315", "serial": "0"}}} {
		w := request(t, s, "/transform", c.body, c.fields)
		if w.Code != 400 || w.Header().Get("Content-Type") != "application/json" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	s.MaxBytes = 100
	w := request(t, s, "/transform", b, map[string]string{"action": "pauses"})
	if w.Code != 413 {
		t.Fatal(w.Code, w.Body.String())
	}
	s.slots <- struct{}{}
	w = request(t, s, "/transform", b, nil)
	<-s.slots
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	req := httptest.NewRequest("POST", "/transform", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	s.ServeHTTP(httptest.NewRecorder(), req.WithContext(ctx))
	entries, _ := os.ReadDir(s.TempDir)
	if len(entries) != 0 || len(s.slots) != 0 {
		t.Fatal("canceled request leaked resources")
	}
}

type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header       { return w.header }
func (w *failingWriter) WriteHeader(int)           {}
func (w *failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestDownloadFailureCleansWorkspace(t *testing.T) {
	s := testServer(t)
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	p, _ := mw.CreateFormFile("fit_file", "activity.fit")
	_, _ = p.Write(fixture(t))
	_ = mw.WriteField("action", "pauses")
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/transform", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	s.ServeHTTP(&failingWriter{make(http.Header)}, req)
	entries, _ := os.ReadDir(s.TempDir)
	if len(entries) != 0 || len(s.slots) != 0 {
		t.Fatal("download failure leaked resources")
	}
}
func TestUnknownPaths(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/missing", "/../go.mod"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code == 200 {
			t.Fatal(filepath.Clean(path))
		}
	}
}
