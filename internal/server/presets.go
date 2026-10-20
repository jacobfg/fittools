package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jacobfg/fittools/internal/fit"
	"net/http"
	"os"
	"strings"
)

type preset struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Manufacturer *uint64 `json:"manufacturer"`
	Product      *uint64 `json:"product"`
	Serial       *uint64 `json:"serial"`
	Firmware     *string `json:"firmware"`
	Note         *string `json:"note"`
}

func parsePresets(data []byte) ([]preset, error) {
	var presets []preset
	if err := json.Unmarshal(data, &presets); err != nil || presets == nil {
		return nil, errors.New("device presets must be a JSON array")
	}
	seen := map[string]bool{"custom": true}
	for i, p := range presets {
		if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Name) == "" || seen[p.ID] || p.Manufacturer == nil || p.Product == nil || p.Serial == nil || p.Firmware == nil || p.Note == nil {
			return nil, fmt.Errorf("device preset %d requires all fields and a unique id other than custom", i+1)
		}
		seen[p.ID] = true
		if err := (fit.Identity{Manufacturer: *p.Manufacturer, Product: *p.Product, Serial: *p.Serial}).Validate(); err != nil {
			return nil, fmt.Errorf("device preset %d: %w", i+1, err)
		}
		if *p.Firmware != "" {
			if _, err := fit.ParseFirmware(*p.Firmware); err != nil {
				return nil, fmt.Errorf("device preset %d: %w", i+1, err)
			}
		}
	}
	return presets, nil
}

func (s *Server) servePresets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		fail(w, http.StatusMethodNotAllowed, errors.New("use GET"))
		return
	}
	data := s.DefaultPresets
	if s.PresetsFile != "" {
		var err error
		data, err = os.ReadFile(s.PresetsFile)
		if err != nil {
			fail(w, http.StatusInternalServerError, errors.New("cannot read configured device presets file"))
			return
		}
	}
	presets, err := parsePresets(data)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	jsonResponse(w, http.StatusOK, presets)
}
