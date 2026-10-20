package fit

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

//go:embed profile.json
var profileJSON []byte
var profile = func() map[string]map[string]string {
	var p map[string]map[string]string
	if e := json.Unmarshal(profileJSON, &p); e != nil {
		panic(e)
	}
	return p
}()

func profileName(kind string, n int64) any {
	s := profile[kind][strconv.FormatInt(n, 10)]
	if s == "" {
		return nil
	}
	return s
}
func (f *File) Details() (map[string]any, error) {
	d, e := f.RecordingDevice()
	if e != nil {
		return nil, e
	}
	var sessions []Record
	var timer any
	for _, r := range f.Records {
		if r.IsDefinition {
			continue
		}
		if r.Def.Global == 18 {
			sessions = append(sessions, r)
		}
		if r.Def.Global == 34 {
			if v, ok := r.Value(0); ok {
				timer = float64(v) / 1000
			}
		}
	}
	if len(sessions) == 0 {
		return nil, errors.New("FIT contains no Session summary for activity details")
	}
	sum := func(n int, scale float64) any {
		var total float64
		for _, r := range sessions {
			v, ok := r.Value(n)
			if !ok {
				return nil
			}
			total += float64(v)
		}
		return total / scale
	}
	if timer == nil {
		timer = sum(8, 1000)
	}
	manufacturer := profileName("manufacturer", int64(d.Manufacturer))
	var product any
	if d.Manufacturer == 1 || d.Manufacturer == 13 || d.Manufacturer == 15 || d.Manufacturer == 89 {
		product = profileName("garmin_product", int64(d.Product))
	}
	var parts []string
	for _, v := range []any{manufacturer, product} {
		if s, ok := v.(string); ok {
			parts = append(parts, s)
		}
	}
	var name any
	if len(parts) > 0 {
		name = strings.Join(parts, " ")
	}
	return map[string]any{"device": map[string]any{"name": name, "serial": d.Serial, "manufacturer": map[string]any{"number": d.Manufacturer, "name": manufacturer}, "product": map[string]any{"number": d.Product, "name": product}, "firmware": d.Firmware}, "activity": map[string]any{"sport": profileName("sport", sessions[0].v(5)), "sub_sport": profileName("sub_sport", sessions[0].v(6)), "distance_metres": sum(9, 100), "elapsed_seconds": sum(7, 1000), "timer_seconds": timer}}, nil
}
