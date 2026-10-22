// Package server serves the static UI and bounded, ephemeral FIT operations.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jacobfg/fittools/internal/fit"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const MaxUpload = 100 * 1024 * 1024

type Server struct {
	slots          chan struct{}
	MaxBytes       int64
	TempDir        string
	Assets         fs.FS
	PresetsFile    string
	DefaultPresets []byte
}

func New(assets fs.FS, concurrency int) *Server {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Server{slots: make(chan struct{}, concurrency), MaxBytes: MaxUpload, Assets: assets}
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, e error) {
	jsonResponse(w, status, map[string]string{"error": e.Error()})
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	if r.URL.Path == "/device-presets" {
		s.servePresets(w, r)
		return
	}
	if r.URL.Path == "/healthz" {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, "ok\n")
		}
		return
	}
	api := r.URL.Path == "/transform" || r.URL.Path == "/inspect-identity" || r.URL.Path == "/activity-details"
	if !api {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		http.FileServer(http.FS(s.Assets)).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		fail(w, 405, errors.New("use POST"))
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		fail(w, 503, errors.New("Another file is being processed. Please try again shortly."))
		return
	}
	if r.Context().Err() != nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxBytes)
	// Stream multipart uploads to a request-owned directory, keeping multipart buffering bounded.
	mr, e := r.MultipartReader()
	if e != nil {
		fail(w, 400, e)
		return
	}
	dir, e := os.MkdirTemp(s.TempDir, "fittools-")
	if e != nil {
		fail(w, 500, errors.New("cannot create temporary workspace"))
		return
	}
	defer os.RemoveAll(dir)
	fields := map[string]string{}
	filename := ""
	input := filepath.Join(dir, "input.fit")
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			e = err
			break
		}
		if part.FormName() == "fit_file" && part.FileName() != "" {
			if filename != "" {
				e = errors.New("choose a single FIT activity")
				break
			}
			filename = part.FileName()
			if !strings.EqualFold(filepath.Ext(filename), ".fit") {
				e = errors.New("The uploaded file must have a .fit extension")
				break
			}
			file, err := os.Create(input)
			if err != nil {
				e = err
				break
			}
			_, e = io.Copy(file, part)
			closeErr := file.Close()
			if e == nil {
				e = closeErr
			}
		} else {
			b, err := io.ReadAll(io.LimitReader(part, 4097))
			if err != nil {
				e = err
			} else if len(b) > 4096 {
				e = errors.New("form field is too large")
			} else {
				fields[part.FormName()] = string(b)
			}
		}
		_ = part.Close()
		if e != nil {
			break
		}
	}
	if e != nil {
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			fail(w, 413, errors.New("The FIT upload exceeds the 100 MiB limit"))
		} else {
			fail(w, 400, e)
		}
		return
	}
	if filename == "" {
		fail(w, 400, errors.New("Choose a FIT activity to upload"))
		return
	}
	if r.Context().Err() != nil {
		return
	}
	raw, e := os.ReadFile(input)
	if e != nil {
		fail(w, 500, errors.New("cannot read uploaded FIT"))
		return
	}
	f, e := fit.Parse(raw)
	if e != nil {
		fail(w, 400, e)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	if r.URL.Path == "/inspect-identity" {
		d, e := f.RecordingDevice()
		if e != nil {
			fail(w, 400, e)
		} else {
			jsonResponse(w, 200, d)
		}
		return
	}
	if r.URL.Path == "/activity-details" {
		d, e := f.Details()
		if e != nil {
			fail(w, 400, e)
		} else {
			jsonResponse(w, 200, d)
		}
		return
	}
	action := fields["action"]
	var records [][]byte
	suffix := ""
	switch action {
	case "pauses", "continuous":
		records, e = f.RestTransform(action == "continuous")
		suffix = "timer-pauses"
		if action == "continuous" {
			suffix = "continuous"
		}
	case "identity":
		suffix = "new-device"
		var vals [3]uint64
		for i, n := range []string{"manufacturer", "product", "serial"} {
			vals[i], e = strconv.ParseUint(fields[n], 10, 64)
			if e != nil {
				e = fmt.Errorf("replacement %s must be a whole number", n)
				break
			}
		}
		if e == nil {
			var fw *uint64
			if v := strings.TrimSpace(fields["firmware"]); v != "" {
				var n uint64
				n, e = fit.ParseFirmware(v)
				fw = &n
			}
			if e == nil {
				var d fit.Device
				d, e = f.RecordingDevice()
				if e == nil {
					records, e = f.ChangeIdentity(d.Identity, fit.Identity{Manufacturer: vals[0], Product: vals[1], Serial: vals[2]}, fw)
				}
			}
		}
	default:
		e = errors.New("Choose a valid transformation")
	}
	if e != nil {
		fail(w, 400, e)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	output, e := os.Create(filepath.Join(dir, "output.fit"))
	if e != nil {
		fail(w, 500, errors.New("cannot create output"))
		return
	}
	defer output.Close()
	if e = f.Write(output, records); e != nil {
		fail(w, 500, e)
		return
	}
	if _, e = output.Seek(0, 0); e != nil {
		fail(w, 500, e)
		return
	}
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	stem = strings.Trim(unsafeName.ReplaceAllString(stem, "-"), ".-")
	if stem == "" {
		stem = "activity"
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": stem + "-" + suffix + ".fit"}))
	if stat, e := output.Stat(); e == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	}
	_, _ = io.Copy(w, output)
}
