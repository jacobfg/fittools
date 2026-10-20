package fit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type goldenCase struct {
	Name, Action   string
	Source, Target Identity
	Firmware       *uint64
}

func TestGolden(t *testing.T) {
	b, e := os.ReadFile("../../testdata/cases.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases []goldenCase
	if e = json.Unmarshal(b, &cases); e != nil {
		t.Fatal(e)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			src, e := os.ReadFile(filepath.Join("../../testdata", c.Name+".input.bin"))
			if e != nil {
				t.Fatal(e)
			}
			expected, e := os.ReadFile(filepath.Join("../../testdata", c.Name+".output.bin"))
			if e != nil {
				t.Fatal(e)
			}
			f, e := Parse(src)
			if e != nil {
				t.Fatal(e)
			}
			var records [][]byte
			if c.Action == "identity" {
				records, e = f.ChangeIdentity(c.Source, c.Target, c.Firmware)
			} else {
				records, e = f.RestTransform(c.Action == "continuous")
			}
			if e != nil {
				t.Fatal(e)
			}
			var out bytes.Buffer
			if e = f.Write(&out, records); e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(out.Bytes(), expected) {
				t.Fatalf("output differs from Python oracle: got %d, want %d bytes", out.Len(), len(expected))
			}
			result, e := Parse(out.Bytes())
			if e != nil {
				t.Fatal(e)
			}
			assertGoldenSemantics(t, c.Name, f, result)
		})
	}
}

// Explicit expectations preserve the intent of the original regression scenarios.
func assertGoldenSemantics(t *testing.T, name string, input, output *File) {
	t.Helper()
	values := func(f *File, global, field int) []int64 {
		var result []int64
		for _, r := range f.Records {
			if !r.IsDefinition && r.Def.Global == global {
				result = append(result, r.v(field))
			}
		}
		return result
	}
	check := func(got, want []int64) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
	}
	seconds := func(global int) []int64 {
		v := values(output, global, 253)
		for i := range v {
			v[i] %= 60
		}
		return v
	}
	switch name {
	case "00-identity", "01-identity", "02-identity":
		for i, r := range input.Records {
			if !r.IsDefinition && r.Def.Global == 23 && r.v(25) == 3 && !bytes.Equal(r.Raw, output.Records[i].Raw) {
				t.Fatal("paired sensor changed")
			}
		}
	case "03-continuous":
		check(seconds(20), []int64{0, 10, 15, 20})
		check(seconds(21), []int64{5})
		check(values(output, 21, 0), []int64{10}) // unrelated course-point event
		check(values(output, 18, 8), []int64{30000})
		for _, r := range output.Records {
			if !r.IsDefinition && (r.Def.Global == 312 || r.Def.Global == 313) {
				t.Fatal("rest metadata retained")
			}
		}
	case "04-pauses":
		check(values(output, 18, 8), []int64{20000, 30000})
		check(values(output, 34, 0), []int64{50000})
	case "05-pauses":
		check(seconds(20), []int64{0, 51})
		check(values(output, 18, 8), []int64{15000})
	case "06-pauses":
		check(seconds(21), []int64{5, 25})
		check(values(output, 21, 1), []int64{4, 0})
		check(values(output, 18, 8), []int64{10000})
	case "07-pauses":
		check(seconds(20), []int64{0, 20})
		check(seconds(21), []int64{10, 20})
		check(values(output, 21, 1), []int64{4, 0})
		check(values(output, 18, 7), []int64{30000})
		check(values(output, 18, 8), []int64{20000})
		check(values(output, 18, 59), []int64{30000})
		check(values(output, 34, 0), []int64{20000})
		check(values(output, 312, 0), []int64{12})
		for _, r := range input.Records {
			if r.Def.Global == 0xF100 {
				found := false
				for _, out := range output.Records {
					if bytes.Equal(r.Raw, out.Raw) {
						found = true
					}
				}
				if !found {
					t.Fatal("private record changed")
				}
			}
		}
	}
}
func TestInvalidInputs(t *testing.T) {
	b, e := os.ReadFile("../../testdata/00-identity.input.bin")
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < len(b); i++ {
		if _, e := Parse(b[:i]); e == nil {
			t.Fatalf("accepted truncation %d", i)
		}
	}
	b[len(b)-3] ^= 1
	if _, e = Parse(b); e == nil {
		t.Fatal("accepted corrupt CRC")
	}
}
func TestFirmwareAndSerial(t *testing.T) {
	for s, want := range map[string]uint64{"6.38": 638, "28.05": 2805, "0": 0, "655.34": 65534} {
		v, e := ParseFirmware(s)
		if e != nil || v != want {
			t.Fatalf("%s: %d %v", s, v, e)
		}
	}
	for _, s := range []string{"NaN", "-1", "1.001", "655.35", "1/2", "0x1", "0b10"} {
		if _, e := ParseFirmware(s); e == nil {
			t.Fatal(s)
		}
	}
	for s, want := range map[string]uint64{"123": 123, "0xFF": 255, "ABC123": 623698779, "base36:ABC123": 623698779} {
		v, e := ParseSerial(s)
		if e != nil || v != want {
			t.Fatalf("%s: %d %v", s, v, e)
		}
	}
}
func FuzzParse(f *testing.F) {
	b, _ := os.ReadFile("../../testdata/00-identity.input.bin")
	f.Add(b)
	f.Fuzz(func(t *testing.T, b []byte) {
		parsed, e := Parse(b)
		if e == nil {
			_, _ = parsed.RecordingDevice()
			_, _ = parsed.Details()
			_, _ = parsed.RestTransform(false)
			_, _ = parsed.RestTransform(true)
		}
	})
}
func TestIdentityRejections(t *testing.T) {
	raw, e := os.ReadFile("../../testdata/00-identity.input.bin")
	if e != nil {
		t.Fatal(e)
	}
	f, e := Parse(raw)
	if e != nil {
		t.Fatal(e)
	}
	d, e := f.RecordingDevice()
	if e != nil {
		t.Fatal(e)
	}
	target := Identity{1, 4315, 100000002}
	wrong := d.Identity
	wrong.Serial++
	if _, e = f.ChangeIdentity(wrong, target, nil); e == nil {
		t.Fatal("accepted mismatched identity")
	}
	if _, e = f.ChangeIdentity(d.Identity, d.Identity, nil); e == nil {
		t.Fatal("accepted unchanged identity")
	}
	var onlyID []Record
	for _, r := range f.Records {
		if r.IsDefinition || r.Def.Global != 23 {
			onlyID = append(onlyID, r)
		}
	}
	f.Records = onlyID
	d, e = f.RecordingDevice()
	if e != nil || d.Firmware != nil {
		t.Fatal(d, e)
	}
	if _, e = f.ChangeIdentity(d.Identity, target, nil); e == nil {
		t.Fatal("accepted absent Device Info")
	}
}
func TestBigEndianDeveloperAndCompressed(t *testing.T) {
	header := []byte{14, 16, 0, 0, 0, 0, 0, 0, '.', 'F', 'I', 'T', 0, 0}
	file := &File{Header: header}
	definition := []byte{0x60, 0, 1, 0, 20, 2, 253, 4, 134, 3, 1, 2, 1, 7, 3, 0}
	normal := []byte{0, 0, 0, 0, 100, 120, 1, 2, 3}
	compressed := []byte{0x80 | 5, 121, 4, 5, 6}
	var buf bytes.Buffer
	if e := file.Write(&buf, [][]byte{definition, normal, compressed}); e != nil {
		t.Fatal(e)
	}
	f, e := Parse(buf.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	r := f.Records[2]
	if r.Timestamp != 101 || r.v(3) != 121 {
		t.Fatal(r)
	}
	n := r.normal()
	if !bytes.Equal(n.Raw, []byte{0, 0, 0, 0, 101, 121, 4, 5, 6}) {
		t.Fatal(n.Raw)
	}
	records := [][]byte{f.Records[0].Raw, f.Records[1].Raw, r.Raw}
	var copy bytes.Buffer
	if e = f.Write(&copy, records); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(copy.Bytes(), buf.Bytes()) {
		t.Fatal("round trip changed developer data")
	}
}

// The large case repeats the first normal sample 100,000 times, matching the
// historical synthetic workload (not a representative real activity).
func BenchmarkTransform(b *testing.B) {
	base, err := os.ReadFile("../../testdata/web.input.bin")
	if err != nil {
		b.Fatal(err)
	}
	for _, size := range []struct {
		name   string
		copies int
	}{{"small", 0}, {"100k-samples", 100000}} {
		b.Run(size.name, func(b *testing.B) {
			f, err := Parse(base)
			if err != nil {
				b.Fatal(err)
			}
			var records [][]byte
			added := false
			for _, r := range f.Records {
				records = append(records, r.Raw)
				if !added && !r.IsDefinition && r.Def.Global == 20 {
					for i := 0; i < size.copies; i++ {
						records = append(records, r.Raw)
					}
					added = true
				}
			}
			var buf bytes.Buffer
			if err = f.Write(&buf, records); err != nil {
				b.Fatal(err)
			}
			raw := buf.Bytes()
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f, err := Parse(raw)
				if err != nil {
					b.Fatal(err)
				}
				records, err := f.RestTransform(false)
				if err != nil {
					b.Fatal(err)
				}
				var out bytes.Buffer
				if err = f.Write(&out, records); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestFirmwareRoundTripPreservesComponents(t *testing.T) {
	for _, componentVersion := range []uint64{638, 1102, 65535} {
		for _, override := range []bool{false, true} {
			raw, err := os.ReadFile("../../testdata/00-identity.input.bin")
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			var originalRecords [][]byte
			for _, r := range f.Records {
				if !r.IsDefinition && r.Def.Global == 23 && r.v(0) == 1 {
					r.Raw, err = r.patch(5, componentVersion)
					if err != nil {
						t.Fatal(err)
					}
				}
				originalRecords = append(originalRecords, r.Raw)
				// Watches may report each device repeatedly within an activity.
				if !r.IsDefinition && r.Def.Global == 23 {
					originalRecords = append(originalRecords, r.Raw)
				}
			}
			var original bytes.Buffer
			if err = f.Write(&original, originalRecords); err != nil {
				t.Fatal(err)
			}
			f, err = Parse(original.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			source, err := f.RecordingDevice()
			if err != nil {
				t.Fatal(err)
			}
			target := Identity{1, 4315, 100000002}
			var firmware *uint64
			if override {
				v := uint64(2805)
				firmware = &v
			}
			records, err := f.ChangeIdentity(source.Identity, target, firmware)
			if err != nil {
				t.Fatal(err)
			}
			for i, r := range f.Records {
				if !r.IsDefinition && r.Def.Global == 23 && r.v(0) == 1 {
					changed := r
					changed.Raw = records[i]
					if changed.v(5) != r.v(5) {
						t.Fatalf("component firmware changed: %d -> %d", r.v(5), changed.v(5))
					}
				}
			}
			var changed bytes.Buffer
			if err = f.Write(&changed, records); err != nil {
				t.Fatal(err)
			}
			f, err = Parse(changed.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if override {
				v := uint64(638)
				firmware = &v
			}
			records, err = f.ChangeIdentity(target, source.Identity, firmware)
			if err != nil {
				t.Fatal(err)
			}
			var restored bytes.Buffer
			if err = f.Write(&restored, records); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original.Bytes(), restored.Bytes()) {
				t.Fatalf("round trip changed bytes: component firmware %d, override %v", componentVersion, override)
			}
		}
	}
}
