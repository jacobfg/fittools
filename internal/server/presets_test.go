package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const examplePresets = `[{"id":"example","name":"Example <device>","manufacturer":1,"product":4315,"serial":100000002,"firmware":"6.30","note":"Dummy serial"}]`

func TestRuntimePresets(t *testing.T) {
	s := testServer(t)
	s.DefaultPresets = []byte(examplePresets)
	get := func(status int, contains string) {
		t.Helper()
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/device-presets", nil))
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), contains) {
			t.Fatalf("status %d, body %s, headers %v", w.Code, w.Body, w.Header())
		}
		if !json.Valid(w.Body.Bytes()) {
			t.Fatal("non-JSON response")
		}
	}
	get(200, `"firmware":"6.30"`)
	s.PresetsFile = filepath.Join(t.TempDir(), "presets.json")
	get(500, "cannot read configured")
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(s.PresetsFile, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(examplePresets)
	get(200, "100000002")
	write(strings.ReplaceAll(examplePresets, "100000002", "100000009"))
	get(200, "100000009")
	write(`[]`)
	get(200, "[]")
	write(`invalid`)
	get(500, "JSON array")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/device-presets", nil))
	if w.Code != 405 || w.Header().Get("Allow") != "GET" {
		t.Fatal(w.Code, w.Header())
	}
}

func TestInvalidPresets(t *testing.T) {
	for _, data := range []string{
		`null`, `{}`, `[{}]`,
		strings.ReplaceAll(examplePresets, `"example"`, `"custom"`),
		strings.ReplaceAll(examplePresets, `"example"`, `" "`),
		strings.ReplaceAll(examplePresets, `100000002`, `0`),
		strings.ReplaceAll(examplePresets, `100000002`, `4294967296`),
		strings.ReplaceAll(examplePresets, `100000002`, `1.5`),
		strings.ReplaceAll(examplePresets, `4315`, `65535`),
		strings.ReplaceAll(examplePresets, `"6.30"`, `6.30`),
		strings.ReplaceAll(examplePresets, `"6.30"`, `"6.301"`),
		examplePresets[:len(examplePresets)-1] + "," + examplePresets[1:],
	} {
		if _, err := parsePresets([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
