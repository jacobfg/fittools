package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jacobfg/fittools/internal/fit"
	"github.com/jacobfg/fittools/internal/server"
	"github.com/jacobfg/fittools/web"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func address() string {
	host := os.Getenv("FIT_UI_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, "8080")
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fittools serve | healthcheck | pauses INPUT OUTPUT | continuous INPUT OUTPUT | identity INPUT OUTPUT --from-manufacturer N --from-product N --from-serial N --to-manufacturer N --to-product N --to-serial N [--to-firmware VERSION]")
	}
	switch args[0] {
	case "healthcheck":
		client := http.Client{Timeout: 2 * time.Second}
		r, e := client.Get("http://127.0.0.1:8080/healthz")
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return errors.New("unhealthy server")
		}
		return nil
	case "serve":
		n := 1
		if v := os.Getenv("FIT_MAX_CONCURRENT"); v != "" {
			var e error
			n, e = strconv.Atoi(v)
			if e != nil || n < 1 {
				return errors.New("FIT_MAX_CONCURRENT must be positive")
			}
		}
		handler := server.New(web.Assets(), n)
		handler.PresetsFile = os.Getenv("FIT_DEVICE_PRESETS_FILE")
		handler.DefaultPresets = web.DefaultPresets
		s := &http.Server{Addr: address(), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		done := make(chan error, 1)
		go func() { done <- s.ListenAndServe() }()
		fmt.Fprintln(os.Stderr, "Listening on", s.Addr)
		select {
		case e := <-done:
			return e
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return s.Shutdown(shutdown)
		}
	case "pauses", "continuous", "identity":
		return transform(args)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func transform(args []string) error {
	fields := map[string]string{}
	var paths []string
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key, value, ok := strings.Cut(args[i][2:], "=")
			if !ok {
				i++
				if i >= len(args) {
					return errors.New("missing option value")
				}
				value = args[i]
			}
			switch key {
			case "from-manufacturer", "from-product", "from-serial", "to-manufacturer", "to-product", "to-serial", "to-firmware":
			default:
				return fmt.Errorf("unknown option --%s", key)
			}
			if args[0] != "identity" {
				return errors.New("identity flags require identity command")
			}
			fields[key] = value
		} else {
			paths = append(paths, args[i])
		}
	}
	if len(paths) != 2 {
		return errors.New("provide input and output paths")
	}
	src, e := filepath.Abs(paths[0])
	if e != nil {
		return e
	}
	dst, e := filepath.Abs(paths[1])
	if e != nil {
		return e
	}
	si, e := os.Stat(src)
	if e != nil {
		return e
	}
	di, _ := os.Stat(dst)
	if src == dst || (di != nil && os.SameFile(si, di)) {
		return errors.New("input and output must be different files")
	}
	raw, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	f, e := fit.Parse(raw)
	if e != nil {
		return e
	}
	var records [][]byte
	if args[0] == "identity" {
		read := func(prefix string) (fit.Identity, error) {
			var v [3]uint64
			for i, n := range []string{"manufacturer", "product", "serial"} {
				s, ok := fields[prefix+"-"+n]
				if !ok {
					return fit.Identity{}, fmt.Errorf("missing --%s-%s", prefix, n)
				}
				var err error
				if n == "serial" {
					v[i], err = fit.ParseSerial(s)
				} else {
					base := 10
					if strings.HasPrefix(s, "0x") {
						base = 0
					}
					v[i], err = strconv.ParseUint(s, base, 64)
				}
				if err != nil {
					return fit.Identity{}, err
				}
			}
			return fit.Identity{Manufacturer: v[0], Product: v[1], Serial: v[2]}, nil
		}
		source, e := read("from")
		if e != nil {
			return e
		}
		target, e := read("to")
		if e != nil {
			return e
		}
		var fw *uint64
		if s, ok := fields["to-firmware"]; ok {
			n, e := fit.ParseFirmware(s)
			if e != nil {
				return e
			}
			fw = &n
		}
		records, e = f.ChangeIdentity(source, target, fw)
		if e != nil {
			return e
		}
	} else {
		records, e = f.RestTransform(args[0] == "continuous")
		if e != nil {
			return e
		}
	}
	tmp, e := os.CreateTemp(filepath.Dir(dst), ".fit-output-")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if e = f.Write(tmp, records); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	if e = os.Rename(tmp.Name(), dst); e != nil {
		return e
	}
	_, e = io.WriteString(os.Stdout, "Wrote "+paths[1]+"\n")
	return e
}
