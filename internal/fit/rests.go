package fit

import (
	"errors"
	"math"
	"sort"
)

type interval struct{ start, end int64 }
type transition struct {
	time    int64
	stopped bool
}
type event struct {
	time int64
	kind byte
}

func merge(in []interval) []interval {
	sort.SliceStable(in, func(i, j int) bool { return in[i].start < in[j].start })
	out := []interval{}
	for _, v := range in {
		if v.end <= v.start {
			continue
		}
		if len(out) == 0 || v.start > out[len(out)-1].end {
			out = append(out, v)
		} else if v.end > out[len(out)-1].end {
			out[len(out)-1].end = v.end
		}
	}
	return out
}
func contains(in []interval, t int64) bool {
	i := sort.Search(len(in), func(i int) bool { return in[i].end > t })
	return i < len(in) && in[i].start <= t
}
func isRest(r Record) bool { return r.Def.Global == 312 && r.v(0) == 13 }
func (f *File) rests() ([]interval, error) {
	var raw []interval
	for _, r := range f.Records {
		if !r.IsDefinition && isRest(r) {
			s, e := r.v(9), r.v(1)
			if s < 0 || e < 0 {
				return nil, errors.New("run_rest split is missing start_time or total_elapsed_time")
			}
			raw = append(raw, interval{s, e})
		}
	}
	if len(raw) == 0 {
		return nil, errors.New("No Garmin Ultra Run 'run_rest' splits were found")
	}
	sort.Slice(raw, func(i, j int) bool {
		if raw[i].start == raw[j].start {
			return raw[i].end < raw[j].end
		}
		return raw[i].start < raw[j].start
	})
	var total, allocated int64
	for i, v := range raw {
		total += v.end
		n := int64(math.RoundToEven(float64(total) / 1000))
		raw[i].end = v.start + n - allocated
		allocated = n
	}
	return merge(raw), nil
}
func (f *File) RestTransform(continuous bool) ([][]byte, error) {
	rests, err := f.rests()
	if err != nil {
		return nil, err
	}
	var timerRecords []Record
	end := int64(0)
	for _, r := range f.Records {
		if r.IsDefinition {
			continue
		}
		if r.Timestamp > end {
			end = r.Timestamp
		}
		if r.Def.Global == 18 && r.v(2) >= 0 && r.v(7) >= 0 {
			e := r.v(2) + r.v(7)/1000
			if e > end {
				end = e
			}
		}
		if r.Def.Global == 21 && r.v(0) == 0 && r.Timestamp >= 0 {
			timerRecords = append(timerRecords, r)
		}
	}
	sort.SliceStable(timerRecords, func(i, j int) bool { return timerRecords[i].Timestamp < timerRecords[j].Timestamp })
	var changes []transition
	stopped := false
	for _, r := range timerRecords {
		v := r.v(1)
		if (v == 1 || v == 4) && !stopped {
			stopped = true
			changes = append(changes, transition{r.Timestamp, true})
		} else if v == 0 && stopped {
			stopped = false
			changes = append(changes, transition{r.Timestamp, false})
		}
	}
	pauses := append([]interval(nil), rests...)
	start := int64(-1)
	for _, c := range changes {
		if c.stopped {
			start = c.time
		} else if start >= 0 {
			pauses = append(pauses, interval{start, c.time})
			start = -1
		}
	}
	if start >= 0 && end > start {
		pauses = append(pauses, interval{start, end})
	}
	pauses = merge(pauses)
	stoppedBefore := func(t int64) bool {
		i := sort.Search(len(changes), func(i int) bool { return changes[i].time >= t })
		return i > 0 && changes[i-1].stopped
	}
	var events []event
	boundaries := map[int64]bool{}
	for _, r := range rests {
		boundaries[r.start] = true
		boundaries[r.end] = true
		if !continuous {
			if !stoppedBefore(r.start) {
				events = append(events, event{r.start, 4})
			}
			if !stoppedBefore(r.end) {
				events = append(events, event{r.end, 0})
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].time < events[j].time })
	updates := map[int]uint64{}
	var total uint64
	hasSession := false
	for i, r := range f.Records {
		if r.IsDefinition || isRest(r) {
			continue
		}
		g := r.Def.Global
		if g != 18 && g != 19 && g != 312 {
			continue
		}
		sf, ef, tf := 2, 7, 8
		if g == 312 {
			sf, ef, tf = 9, 1, 2
		}
		elapsed := r.v(ef)
		s := r.v(sf)
		if elapsed < 0 || r.Def.field(tf) == nil || (!continuous && s < 0) {
			continue
		}
		value := elapsed
		if !continuous {
			for _, p := range pauses {
				lo := max(s*1000, p.start*1000)
				hi := min(s*1000+elapsed, p.end*1000)
				value -= max(int64(0), hi-lo)
			}
		}
		value = max(int64(0), value)
		updates[i] = uint64(value)
		if g == 18 {
			total += uint64(value)
			hasSession = true
		}
	}
	if hasSession {
		for i, r := range f.Records {
			if !r.IsDefinition && r.Def.Global == 34 && r.Def.field(0) != nil {
				updates[i] = total
			}
		}
	}
	out := make([][]byte, 0, len(f.Records))
	var active [16]*Definition
	previous := int64(-1)
	next := 0
	emit := func(t int64) {
		for next < len(events) && events[next].time <= t {
			time := events[next].time
			out = append(out, []byte{79, 0, 0, 21, 0, 5, 253, 4, 134, 3, 4, 134, 0, 1, 0, 1, 1, 0, 4, 1, 2})
			for next < len(events) && events[next].time == time {
				b := make([]byte, 12)
				b[0] = 15
				put(b[1:5], uint64(time), false)
				b[10] = events[next].kind
				out = append(out, b)
				next++
			}
			if active[15] != nil {
				out = append(out, active[15].Raw)
			}
			previous = time
		}
	}
	for i, r := range f.Records {
		if r.IsDefinition {
			out = append(out, r.Raw)
			active[r.Def.Local] = r.Def
			continue
		}
		if r.Timestamp >= 0 {
			emit(r.Timestamp)
		}
		g := r.Def.Global
		drop := isRest(r) || (g == 313 && r.v(0) == 13)
		if continuous {
			drop = drop || (g == 21 && r.Timestamp >= 0 && (r.v(0) == 0 || (r.v(1) == 3 && boundaries[r.Timestamp])))
		} else {
			drop = drop || (g != 18 && g != 19 && g != 34 && r.Timestamp >= 0 && contains(rests, r.Timestamp))
		}
		if drop {
			continue
		}
		if r.Compressed && (previous < 0 || compressedTime(previous, r.Raw[0]&31) != r.Timestamp) {
			r = r.normal()
		}
		if value, ok := updates[i]; ok {
			n := 8
			if g == 34 {
				n = 0
			} else if g == 312 {
				n = 2
			}
			r.Raw, err = r.patch(n, value)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, r.Raw)
		if r.Timestamp >= 0 {
			previous = r.Timestamp
		}
	}
	emit(math.MaxInt64)
	return out, nil
}
