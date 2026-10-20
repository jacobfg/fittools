package fit

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

type Identity struct {
	Manufacturer uint64 `json:"manufacturer"`
	Product      uint64 `json:"product"`
	Serial       uint64 `json:"serial"`
}
type Device struct {
	Identity
	Firmware *string `json:"firmware"`
}

func (i Identity) Validate() error {
	if i.Manufacturer > 65534 || i.Product > 65534 {
		return errors.New("manufacturer and product must be between 0 and 65534")
	}
	if i.Serial == 0 || i.Serial > 4294967295 {
		return errors.New("serial must be a non-zero FIT uint32z")
	}
	return nil
}
func ParseSerial(s string) (uint64, error) {
	base := 10
	value := s
	l := strings.ToLower(s)
	if strings.HasPrefix(l, "base36:") {
		base = 36
		value = s[7:]
	} else if strings.HasPrefix(l, "0x") {
		base = 16
		value = s[2:]
	} else if strings.ContainsAny(l, "abcdefghijklmnopqrstuvwxyz") {
		base = 36
	}
	return strconv.ParseUint(value, base, 64)
}

var decimalVersion = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func ParseFirmware(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if !decimalVersion.MatchString(s) {
		return 0, errors.New("firmware must be a decimal version such as 6.38")
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok || strings.Contains(s, "/") || v.Sign() < 0 {
		return 0, errors.New("firmware must be a non-negative decimal version")
	}
	v.Mul(v, big.NewRat(100, 1))
	if !v.IsInt() {
		return 0, errors.New("firmware supports at most two decimal places")
	}
	if !v.Num().IsUint64() || v.Num().Uint64() > 65534 {
		return 0, errors.New("firmware must not exceed 655.34")
	}
	return v.Num().Uint64(), nil
}
func fileIdentity(r Record) (Identity, error) {
	m, a := r.Value(1)
	p, b := r.Value(2)
	s, c := r.Value(3)
	if !a || !b || !c {
		return Identity{}, errors.New("File ID is missing manufacturer, product, or serial_number")
	}
	i := Identity{m, p, s}
	return i, i.Validate()
}
func (f *File) RecordingDevice() (Device, error) {
	var d Device
	count := 0
	for _, r := range f.Records {
		if !r.IsDefinition && r.Def.Global == 0 {
			count++
			i, e := fileIdentity(r)
			if e != nil {
				return d, e
			}
			d.Identity = i
		}
	}
	if count != 1 {
		return d, errors.New("FIT must contain exactly one File ID message")
	}
	for _, r := range f.Records {
		if !r.IsDefinition && r.Def.Global == 23 && r.v(0) == 0 && r.v(2) == int64(d.Manufacturer) && r.v(4) == int64(d.Product) && r.v(3) == int64(d.Serial) {
			if v, ok := r.Value(5); ok {
				s := fmt.Sprintf("%d.%02d", v/100, v%100)
				d.Firmware = &s
			}
			break
		}
	}
	return d, nil
}
func (f *File) ChangeIdentity(source, target Identity, firmware *uint64) ([][]byte, error) {
	if e := source.Validate(); e != nil {
		return nil, e
	}
	if e := target.Validate(); e != nil {
		return nil, e
	}
	if firmware != nil && *firmware > 65534 {
		return nil, errors.New("invalid firmware")
	}
	if source == target && firmware == nil {
		return nil, errors.New("source and target device identities must differ unless firmware is being changed")
	}
	out := make([][]byte, 0, len(f.Records))
	found, primary := false, false
	for _, r := range f.Records {
		if r.IsDefinition {
			out = append(out, r.Raw)
			continue
		}
		fields := map[int]uint64{}
		if r.Def.Global == 0 {
			if found {
				return nil, errors.New("FIT contains more than one File ID message")
			}
			i, e := fileIdentity(r)
			if e != nil {
				return nil, e
			}
			if i != source {
				return nil, errors.New("File ID does not match the supplied source device identity")
			}
			found = true
			fields = map[int]uint64{1: target.Manufacturer, 2: target.Product, 3: target.Serial}
		} else if r.Def.Global == 23 && r.v(25) == 5 && r.v(2) == int64(source.Manufacturer) && r.v(4) == int64(source.Product) && (r.v(3) < 0 || r.v(3) == int64(source.Serial)) {
			fields = map[int]uint64{2: target.Manufacturer, 4: target.Product}
			if r.v(3) == int64(source.Serial) {
				fields[3] = target.Serial
				if r.v(0) == 0 {
					primary = true
					// Components have independent firmware; only override the recorder.
					if firmware != nil && r.Def.field(5) != nil {
						fields[5] = *firmware
					}
				}
			}
		}
		for n, v := range fields {
			var e error
			r.Raw, e = r.patch(n, v)
			if e != nil {
				return nil, e
			}
		}
		out = append(out, r.Raw)
	}
	if !found || !primary {
		return nil, errors.New("FIT contains no matching local Device Info record for the recording device")
	}
	return out, nil
}
