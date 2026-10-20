// Package fit transforms raw FIT records without re-encoding unknown fields.
package fit

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var sizes = [...]int{1, 1, 1, 2, 2, 4, 4, 1, 4, 8, 1, 2, 4, 1, 8, 8, 8}
var invalid = map[int]uint64{0: 255, 1: 127, 2: 255, 3: 32767, 4: 65535, 5: 2147483647, 6: 4294967295, 10: 0, 11: 0, 12: 0, 13: 255, 14: 9223372036854775807, 15: 18446744073709551615, 16: 0}

type Field struct{ Number, Size, Type, Offset int }
type Definition struct {
	Raw                                []byte
	Local, Global, Standard, Developer int
	Big                                bool
	Fields                             []Field
}
type Record struct {
	Raw                      []byte
	Def                      *Definition
	IsDefinition, Compressed bool
	Timestamp                int64
}
type File struct {
	Header  []byte
	Records []Record
}

func (d *Definition) field(n int) *Field {
	for i := range d.Fields {
		if d.Fields[i].Number == n {
			return &d.Fields[i]
		}
	}
	return nil
}
func number(b []byte, big bool) uint64 {
	var n uint64
	if big {
		for _, v := range b {
			n = n<<8 | uint64(v)
		}
	} else {
		for i := len(b) - 1; i >= 0; i-- {
			n = n<<8 | uint64(b[i])
		}
	}
	return n
}
func put(b []byte, n uint64, big bool) {
	for i := range b {
		j := i
		if big {
			j = len(b) - 1 - i
		}
		b[j] = byte(n)
		n >>= 8
	}
}
func (r Record) Value(n int) (uint64, bool) {
	f := r.Def.field(n)
	if f == nil || f.Size != sizes[f.Type] {
		return 0, false
	}
	if r.Compressed && n == 253 {
		return uint64(r.Timestamp), r.Timestamp >= 0
	}
	off := f.Offset + 1
	if r.Compressed {
		t := r.Def.field(253)
		if f.Offset > t.Offset {
			off -= t.Size
		}
	}
	v := number(r.Raw[off:off+f.Size], r.Def.Big)
	x, ok := invalid[f.Type]
	return v, !(ok && v == x)
}
func (r Record) v(n int) int64 {
	v, ok := r.Value(n)
	if !ok {
		return -1
	}
	return int64(v)
}
func (r Record) patch(n int, v uint64) ([]byte, error) {
	f := r.Def.field(n)
	if f == nil || f.Size != sizes[f.Type] || (r.Compressed && n == 253) {
		return nil, fmt.Errorf("cannot replace FIT field %d", n)
	}
	if f.Size < 8 && v >= uint64(1)<<(8*f.Size) {
		return nil, fmt.Errorf("value for FIT field %d is out of range", n)
	}
	off := 1 + f.Offset
	if r.Compressed && f.Offset > r.Def.field(253).Offset {
		off -= 4
	}
	b := append([]byte(nil), r.Raw...)
	put(b[off:off+f.Size], v, r.Def.Big)
	return b, nil
}
func (r Record) normal() Record {
	if !r.Compressed {
		return r
	}
	t := r.Def.field(253)
	b := make([]byte, len(r.Raw)+4)
	b[0] = byte(r.Def.Local)
	copy(b[1:], r.Raw[1:1+t.Offset])
	put(b[1+t.Offset:5+t.Offset], uint64(r.Timestamp), r.Def.Big)
	copy(b[5+t.Offset:], r.Raw[1+t.Offset:])
	r.Raw = b
	r.Compressed = false
	return r
}
func CRC(b []byte) uint16 { return crcUpdate(0, b) }
func crcUpdate(c uint16, b []byte) uint16 {
	table := [16]uint16{0, 0xcc01, 0xd801, 0x1400, 0xf001, 0x3c00, 0x2800, 0xe401, 0xa001, 0x6c00, 0x7800, 0xb401, 0x5000, 0x9c01, 0x8801, 0x4400}
	for _, v := range b {
		c = (c >> 4) ^ table[c&15] ^ table[v&15]
		c = (c >> 4) ^ table[c&15] ^ table[v>>4]
	}
	return c
}
func compressedTime(prev int64, offset byte) int64 {
	t := (prev & ^int64(31)) | int64(offset)
	if t < prev {
		t += 32
	}
	return t
}
func Parse(raw []byte) (*File, error) {
	fail := func(s string) (*File, error) { return nil, errors.New(s) }
	if len(raw) < 14 {
		return fail("file is too short to be FIT")
	}
	h := int(raw[0])
	if h != 12 && h != 14 {
		return fail("unsupported FIT header size")
	}
	if len(raw) < h+2 || string(raw[8:12]) != ".FIT" {
		return fail("input is not a FIT file")
	}
	if h == 14 && binary.LittleEndian.Uint16(raw[12:14]) != CRC(raw[:12]) {
		return fail("FIT header CRC does not match")
	}
	if uint64(len(raw)) != uint64(h)+uint64(binary.LittleEndian.Uint32(raw[4:8]))+2 {
		return fail("chained or trailing FIT files are not supported")
	}
	if binary.LittleEndian.Uint16(raw[len(raw)-2:]) != CRC(raw[:len(raw)-2]) {
		return fail("FIT file CRC does not match")
	}
	f := &File{Header: raw[:h]}
	data := raw[h : len(raw)-2]
	var defs [16]*Definition
	prev := int64(-1)
	for p := 0; p < len(data); {
		start := p
		head := data[p]
		p++
		local := int(head & 15)
		if head&128 == 0 && head&64 != 0 {
			if p+5 > len(data) {
				return fail("truncated FIT definition")
			}
			arch := data[p+1]
			if arch > 1 {
				return fail("unsupported FIT architecture")
			}
			d := &Definition{Local: local, Big: arch == 1}
			d.Global = int(number(data[p+2:p+4], d.Big))
			count := int(data[p+4])
			p += 5
			for i := 0; i < count; i++ {
				if p+3 > len(data) {
					return fail("truncated FIT field definition")
				}
				typ := int(data[p+2] & 31)
				if typ >= len(sizes) {
					return fail("unsupported FIT base type")
				}
				d.Fields = append(d.Fields, Field{int(data[p]), int(data[p+1]), typ, d.Standard})
				d.Standard += int(data[p+1])
				p += 3
			}
			if head&32 != 0 {
				if p >= len(data) {
					return fail("truncated FIT developer field count")
				}
				count = int(data[p])
				p++
				for i := 0; i < count; i++ {
					if p+3 > len(data) {
						return fail("truncated FIT developer field definition")
					}
					d.Developer += int(data[p+1])
					p += 3
				}
			}
			d.Raw = data[start:p]
			defs[local] = d
			f.Records = append(f.Records, Record{Raw: d.Raw, Def: d, IsDefinition: true, Timestamp: -1})
			continue
		}
		compressed := head&128 != 0
		if compressed {
			local = int((head >> 5) & 3)
		}
		d := defs[local]
		if d == nil {
			return fail("FIT data record references undefined local message")
		}
		size := d.Standard + d.Developer
		t := int64(-1)
		if compressed {
			ts := d.field(253)
			if prev < 0 || ts == nil || ts.Size != 4 || ts.Type != 6 {
				return fail("compressed FIT record has no valid timestamp anchor or field")
			}
			size -= 4
			t = compressedTime(prev, head&31)
		}
		if p+size > len(data) {
			return fail("truncated FIT data record")
		}
		p += size
		r := Record{Raw: data[start:p], Def: d, Compressed: compressed, Timestamp: t}
		if !compressed {
			r.Timestamp = r.v(253)
		}
		if r.Timestamp >= 0 {
			prev = r.Timestamp
		}
		f.Records = append(f.Records, r)
	}
	return f, nil
}

// Write emits header, retained/transformed records and checksum without another full-file copy.
func (f *File) Write(w io.Writer, records [][]byte) error {
	buffered := bufio.NewWriterSize(w, 64*1024)
	w = buffered
	h := append([]byte(nil), f.Header...)
	var size uint64
	for _, r := range records {
		size += uint64(len(r))
	}
	if size > 0xffffffff {
		return errors.New("FIT output is too large")
	}
	binary.LittleEndian.PutUint32(h[4:8], uint32(size))
	if len(h) == 14 {
		binary.LittleEndian.PutUint16(h[12:14], CRC(h[:12]))
	}
	c := CRC(h)
	if _, e := w.Write(h); e != nil {
		return e
	}
	for _, r := range records {
		c = crcUpdate(c, r)
		if _, e := w.Write(r); e != nil {
			return e
		}
	}
	var tail [2]byte
	binary.LittleEndian.PutUint16(tail[:], c)
	if _, e := w.Write(tail[:]); e != nil {
		return e
	}
	return buffered.Flush()
}
