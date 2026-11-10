package epub

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"strings"
	"time"
)

// ZipFile is one archive entry. Store writes it uncompressed.
type ZipFile struct {
	Name  string
	Data  []byte
	Store bool
}

// WriteZip builds a ZIP archive. The first file stays first, which EPUB
// requires for the uncompressed mimetype entry.
func WriteZip(files []ZipFile, modified time.Time) ([]byte, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("空压缩包")
	}
	if modified.IsZero() {
		modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	dosT, dosD := dosTime(modified)
	var body bytes.Buffer
	type entry struct {
		name     string
		method   uint16
		crc      uint32
		compSize uint32
		rawSize  uint32
		offset   uint32
	}
	entries := make([]entry, 0, len(files))
	for _, f := range files {
		if f.Name == "" || strings.Contains(f.Name, "\\") || strings.HasPrefix(f.Name, "/") {
			return nil, fmt.Errorf("非法文件名：%s", f.Name)
		}
		if len(f.Data) > 0xFFFFFFFF || body.Len() > 0xFFFFFFFF {
			return nil, fmt.Errorf("文件过大：%s", f.Name)
		}
		method := uint16(8)
		payload := f.Data
		comp := payload
		if f.Store {
			method = 0
		} else {
			var buf bytes.Buffer
			w, err := flate.NewWriter(&buf, flate.BestCompression)
			if err != nil {
				return nil, err
			}
			if _, err := w.Write(payload); err != nil {
				return nil, err
			}
			if err := w.Close(); err != nil {
				return nil, err
			}
			comp = buf.Bytes()
		}
		e := entry{
			name:     f.Name,
			method:   method,
			crc:      crc32.ChecksumIEEE(payload),
			compSize: uint32(len(comp)),
			rawSize:  uint32(len(payload)),
			offset:   uint32(body.Len()),
		}
		name := []byte(f.Name)
		put32(&body, 0x04034b50)
		put16(&body, 20)
		put16(&body, 0)
		put16(&body, method)
		put16(&body, dosT)
		put16(&body, dosD)
		put32(&body, e.crc)
		put32(&body, e.compSize)
		put32(&body, e.rawSize)
		put16(&body, uint16(len(name)))
		put16(&body, 0)
		body.Write(name)
		body.Write(comp)
		entries = append(entries, e)
	}
	cdStart := uint32(body.Len())
	var dir bytes.Buffer
	for _, e := range entries {
		name := []byte(e.name)
		put32(&dir, 0x02014b50)
		put16(&dir, 20)
		put16(&dir, 20)
		put16(&dir, 0)
		put16(&dir, e.method)
		put16(&dir, dosT)
		put16(&dir, dosD)
		put32(&dir, e.crc)
		put32(&dir, e.compSize)
		put32(&dir, e.rawSize)
		put16(&dir, uint16(len(name)))
		put16(&dir, 0)
		put16(&dir, 0)
		put16(&dir, 0)
		put16(&dir, 0)
		put32(&dir, 0)
		put32(&dir, e.offset)
		dir.Write(name)
	}
	cdSize := uint32(dir.Len())
	body.Write(dir.Bytes())
	put32(&body, 0x06054b50)
	put16(&body, 0)
	put16(&body, 0)
	put16(&body, uint16(len(entries)))
	put16(&body, uint16(len(entries)))
	put32(&body, cdSize)
	put32(&body, cdStart)
	put16(&body, 0)
	return body.Bytes(), nil
}

func put16(b *bytes.Buffer, v uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	b.Write(buf[:])
}

func put32(b *bytes.Buffer, v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	b.Write(buf[:])
}

func dosTime(t time.Time) (uint16, uint16) {
	t = t.UTC()
	if t.Year() < 1980 {
		t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	d := uint16(t.Day() | int(t.Month())<<5 | (t.Year()-1980)<<9)
	tm := uint16(t.Second()/2 | t.Minute()<<5 | t.Hour()<<11)
	return tm, d
}
