package reflow

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf8"

	"moce/pkg/chapters"
)

func openMOBI(data []byte, format string) (Book, error) {
	recs, err := palmRecords(data)
	if err != nil || len(recs) == 0 {
		return Book{}, fmt.Errorf("无法读取这本书")
	}
	head := recs[0]
	if len(head) < 16 {
		return Book{}, fmt.Errorf("无法读取这本书")
	}
	text, meta, err := readMOBISection(recs, 0)
	if err != nil {
		return Book{}, err
	}
	if meta.kf8 >= 0 && meta.kf8 < len(recs) {
		if alt, altMeta, err := readMOBISection(recs, meta.kf8); err == nil && strings.TrimSpace(alt) != "" {
			text = alt
			if altMeta.title != "" {
				meta.title = altMeta.title
			}
			if altMeta.author != "" {
				meta.author = altMeta.author
			}
		}
	}
	plain := HTMLToText(text)
	if strings.TrimSpace(plain) == "" {
		return Book{}, fmt.Errorf("这本书里没有可排版的正文")
	}
	var cover []byte
	var coverName string
	if meta.image > 0 && meta.image < len(recs) {
		img := recs[meta.image]
		if len(img) > 8 && (isJPEG(img) || isPNG(img)) {
			cover = img
			if isPNG(img) {
				coverName = "cover.png"
			} else {
				coverName = "cover.jpg"
			}
		}
	}
	return Book{
		Text:       plain,
		Title:      meta.title,
		Author:     meta.author,
		Publisher:  meta.publisher,
		Language:   meta.language,
		Published:  meta.published,
		Identifier: meta.isbn,
		Intro:      meta.intro,
		Tags:       meta.tags,
		Cover:      cover,
		CoverName:  coverName,
		Headings:   htmlHeadings(text),
		Format:     format,
	}, nil
}

func isJPEG(b []byte) bool { return len(b) > 2 && b[0] == 0xff && b[1] == 0xd8 }
func isPNG(b []byte) bool {
	return len(b) > 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n"
}

type mobiMeta struct {
	title, author, publisher, language, published, isbn, intro string
	tags                                                       []string
	image, kf8                                                 int
}

func palmRecords(data []byte) ([][]byte, error) {
	if len(data) < 78 {
		return nil, fmt.Errorf("short")
	}
	n := int(binary.BigEndian.Uint16(data[76:78]))
	if n < 1 || 78+n*8 > len(data) {
		return nil, fmt.Errorf("records")
	}
	offs := make([]int, n)
	for i := 0; i < n; i++ {
		offs[i] = int(binary.BigEndian.Uint32(data[78+i*8:]))
		if offs[i] < 0 || offs[i] > len(data) {
			return nil, fmt.Errorf("offset")
		}
	}
	recs := make([][]byte, n)
	for i := 0; i < n; i++ {
		end := len(data)
		if i+1 < n {
			end = offs[i+1]
		}
		if end < offs[i] {
			return nil, fmt.Errorf("offset")
		}
		recs[i] = data[offs[i]:end]
	}
	return recs, nil
}

func readMOBISection(recs [][]byte, rec0 int) (string, mobiMeta, error) {
	head := recs[rec0]
	if len(head) < 16 {
		return "", mobiMeta{}, fmt.Errorf("无法读取这本书")
	}
	comp := binary.BigEndian.Uint16(head[0:2])
	textLen := binary.BigEndian.Uint32(head[4:8])
	count := int(binary.BigEndian.Uint16(head[8:10]))
	meta := mobiMeta{image: -1, kf8: -1}
	var huffRec, huffN, extra uint32
	if len(head) >= 132 && string(head[16:20]) == "MOBI" {
		headerLen := binary.BigEndian.Uint32(head[20:24])
		if int(headerLen)+16 <= len(head) {
			if headerLen > 108 {
				meta.image = int(u32(head, 16+108))
			}
			if headerLen > 116 {
				huffRec = u32(head, 16+112)
				huffN = u32(head, 16+116)
			}
			if headerLen > 240 {
				extra = u32(head, 16+240)
			}
			exthAt := 16 + int(headerLen)
			meta = mergeMeta(meta, parseEXTH(head[exthAt:]))
			if nameOff := u32(head, 16+84); nameOff > 0 && int(nameOff) < len(head) {
				nlen := int(u32(head, 16+88))
				if nlen > 0 && int(nameOff)+nlen <= len(head) && meta.title == "" {
					meta.title = string(head[nameOff : int(nameOff)+nlen])
				}
			}
		}
	}
	var raw []byte
	for i := 1; i <= count; i++ {
		idx := rec0 + i
		if idx >= len(recs) {
			break
		}
		chunk := trimExtra(recs[idx], extra)
		var plain []byte
		switch comp {
		case 1:
			plain = chunk
		case 2:
			plain = palmLZ77(chunk)
		case 17480:
			decoded, err := huffDecode(chunk, recs, int(huffRec), int(huffN))
			if err != nil {
				return "", mobiMeta{}, fmt.Errorf("无法读取这本书")
			}
			plain = decoded
		default:
			return "", mobiMeta{}, fmt.Errorf("无法读取这本书")
		}
		raw = append(raw, plain...)
	}
	if textLen > 0 && int(textLen) < len(raw) {
		raw = raw[:textLen]
	}
	text := string(raw)
	if !utf8.Valid(raw) {
		text = latin1(raw)
	}
	return text, meta, nil
}

func u32(b []byte, off int) uint32 {
	if off+4 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint32(b[off:])
}

func trimExtra(rec []byte, flags uint32) []byte {
	if flags == 0 || len(rec) == 0 {
		return rec
	}
	end := len(rec)
	for bit := 0; bit < 16; bit++ {
		if flags&(1<<bit) == 0 || end == 0 {
			continue
		}
		size, n := backSize(rec[:end])
		if n == 0 || size > end {
			break
		}
		end -= size
	}
	if end < 0 {
		return rec
	}
	return rec[:end]
}

func backSize(b []byte) (int, int) {
	if len(b) == 0 {
		return 0, 0
	}
	last := int(b[len(b)-1])
	if last&0x80 == 0 {
		return last, 1
	}
	if len(b) < 2 {
		return 0, 0
	}
	v := (int(b[len(b)-2])<<7 | last&0x7f)
	return v, 2
}

func palmLZ77(in []byte) []byte {
	out := make([]byte, 0, len(in)*2)
	for i := 0; i < len(in); {
		c := in[i]
		i++
		switch {
		case c == 0 || (c >= 0x09 && c <= 0x7f):
			out = append(out, c)
		case c >= 0x01 && c <= 0x08:
			n := int(c)
			if i+n > len(in) {
				n = len(in) - i
			}
			out = append(out, in[i:i+n]...)
			i += n
		case c >= 0x80 && c <= 0xbf:
			if i >= len(in) {
				return out
			}
			c2 := in[i]
			i++
			combined := int(c)<<8 | int(c2)
			distance := (combined >> 3) & 0x7ff
			length := (combined & 7) + 3
			if distance == 0 || distance > len(out) {
				continue
			}
			for n := 0; n < length; n++ {
				out = append(out, out[len(out)-distance])
			}
		default:
			out = append(out, ' ', c^0x80)
		}
	}
	return out
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func parseEXTH(b []byte) mobiMeta {
	var meta mobiMeta
	meta.image, meta.kf8 = -1, -1
	if len(b) < 12 || string(b[:4]) != "EXTH" {
		return meta
	}
	n := int(binary.BigEndian.Uint32(b[8:12]))
	pos := 12
	for i := 0; i < n && pos+8 <= len(b); i++ {
		typ := binary.BigEndian.Uint32(b[pos:])
		ln := int(binary.BigEndian.Uint32(b[pos+4:]))
		if ln < 8 || pos+ln > len(b) {
			break
		}
		val := b[pos+8 : pos+ln]
		text := strings.TrimSpace(string(val))
		switch typ {
		case 100:
			meta.author = text
		case 101:
			meta.publisher = text
		case 103:
			meta.intro = text
		case 104:
			meta.isbn = text
		case 105:
			if text != "" {
				meta.tags = append(meta.tags, text)
			}
		case 106:
			meta.published = text
		case 121:
			if len(val) >= 4 {
				meta.kf8 = int(binary.BigEndian.Uint32(val[len(val)-4:]))
			}
		case 503:
			meta.title = text
		case 524:
			meta.language = text
		}
		pos += ln
	}
	return meta
}

func mergeMeta(a, b mobiMeta) mobiMeta {
	if b.title != "" {
		a.title = b.title
	}
	if b.author != "" {
		a.author = b.author
	}
	if b.publisher != "" {
		a.publisher = b.publisher
	}
	if b.language != "" {
		a.language = b.language
	}
	if b.published != "" {
		a.published = b.published
	}
	if b.isbn != "" {
		a.isbn = b.isbn
	}
	if b.intro != "" {
		a.intro = b.intro
	}
	if len(b.tags) > 0 {
		a.tags = b.tags
	}
	if b.kf8 >= 0 {
		a.kf8 = b.kf8
	}
	return a
}

func htmlHeadings(s string) []chapters.Heading {
	low := strings.ToLower(s)
	var out []chapters.Heading
	for _, level := range []int{1, 2, 3} {
		tag := fmt.Sprintf("h%d", level)
		from := 0
		for {
			idx := strings.Index(low[from:], "<"+tag)
			if idx < 0 {
				break
			}
			idx += from
			gt := strings.Index(low[idx:], ">")
			if gt < 0 {
				break
			}
			rest := s[idx+gt+1:]
			end := strings.Index(strings.ToLower(rest), "</"+tag)
			if end < 0 {
				break
			}
			title := strings.TrimSpace(HTMLToText(rest[:end]))
			if title != "" {
				out = append(out, chapters.Heading{Title: title, Depth: level})
			}
			from = idx + gt + 1 + end
		}
	}
	if !nested(out) {
		return nil
	}
	return out
}

func nested(h []chapters.Heading) bool {
	for _, item := range h {
		if item.Depth > 1 {
			return true
		}
	}
	return false
}

func huffDecode(data []byte, recs [][]byte, first, count int) ([]byte, error) {
	if first <= 0 || count < 2 || first+count > len(recs) {
		return nil, fmt.Errorf("huff")
	}
	dict, min, max, err := loadHuff(recs[first])
	if err != nil {
		return nil, err
	}
	dicts, _, err := loadCDIC(recs[first+1 : first+count])
	if err != nil {
		return nil, err
	}
	br := newBits(data)
	var out []byte
	for br.left() > 0 {
		if br.left() < 8 {
			break
		}
		peek := br.peek(32)
		codeLen, term, maxcode := dictEntry(dict, byte(peek>>24))
		if codeLen == 0 || codeLen > br.left() {
			break
		}
		code := br.take(codeLen)
		if !term {
			if codeLen <= 8 {
				out = append(out, byte(code))
			}
			continue
		}
		if code > maxcode {
			return nil, fmt.Errorf("huff")
		}
		_ = max
		index := int(code - min[codeLen])
		piece, err := dictSlice(dicts, index)
		if err != nil {
			return nil, err
		}
		out = append(out, piece...)
	}
	return out, nil
}

func dictEntry(dict []uint32, b byte) (length int, term bool, maxcode uint32) {
	v := dict[b]
	return int(v & 0x1f), v&0x80 != 0, v >> 8
}

func loadHuff(rec []byte) ([]uint32, []uint32, []uint32, error) {
	if len(rec) < 24 || string(rec[:4]) != "HUFF" {
		return nil, nil, nil, fmt.Errorf("huff")
	}
	off1 := int(binary.BigEndian.Uint32(rec[8:12]))
	off2 := int(binary.BigEndian.Uint32(rec[12:16]))
	if off1 <= 0 {
		off1 = 24
	}
	if off2 <= off1 {
		off2 = off1 + 256*4
	}
	if off2+64*4 > len(rec) {
		return nil, nil, nil, fmt.Errorf("huff")
	}
	dict := make([]uint32, 256)
	for i := 0; i < 256; i++ {
		dict[i] = binary.BigEndian.Uint32(rec[off1+i*4:])
	}
	min := make([]uint32, 33)
	max := make([]uint32, 33)
	for i := 0; i < 32; i++ {
		v := binary.BigEndian.Uint32(rec[off2+i*4:])
		min[i+1] = v >> 8 & 0xffffff
		// second table often follows; tolerate a shorter record
	}
	if off2+64*4 <= len(rec) {
		for i := 0; i < 32; i++ {
			max[i+1] = binary.BigEndian.Uint32(rec[off2+32*4+i*4:]) & 0xffffff
		}
	}
	return dict, min, max, nil
}

func loadCDIC(recs [][]byte) ([][]byte, int, error) {
	var slices [][]byte
	bits := 0
	for _, rec := range recs {
		if len(rec) < 16 || string(rec[:4]) != "CDIC" {
			return nil, 0, fmt.Errorf("cdic")
		}
		if bits == 0 {
			bits = int(binary.BigEndian.Uint32(rec[12:16]))
		}
		pos := 16
		for pos+2 <= len(rec) {
			n := int(binary.BigEndian.Uint16(rec[pos:]))
			pos += 2
			comp := n&0x8000 != 0
			n &= 0x7fff
			if n == 0 || pos+n > len(rec) {
				break
			}
			piece := append([]byte(nil), rec[pos:pos+n]...)
			if comp {
				piece = append([]byte{1}, piece...)
			} else {
				piece = append([]byte{0}, piece...)
			}
			slices = append(slices, piece)
			pos += n
		}
	}
	if bits == 0 {
		bits = 16
	}
	return slices, bits, nil
}

func dictSlice(dicts [][]byte, index int) ([]byte, error) {
	if index < 0 || index >= len(dicts) {
		return nil, fmt.Errorf("cdic")
	}
	piece := dicts[index]
	if len(piece) == 0 || piece[0] != 0 {
		return nil, fmt.Errorf("cdic")
	}
	return piece[1:], nil
}

type bitReader struct {
	b   []byte
	bit int
}

func newBits(b []byte) *bitReader { return &bitReader{b: b} }

func (r *bitReader) left() int { return len(r.b)*8 - r.bit }

func (r *bitReader) peek(n int) uint32 {
	var v uint32
	for i := 0; i < n && r.bit+i < len(r.b)*8; i++ {
		bytei := (r.bit + i) / 8
		biti := 7 - (r.bit+i)%8
		v = v<<1 | uint32((r.b[bytei]>>biti)&1)
	}
	return v
}

func (r *bitReader) take(n int) uint32 {
	v := r.peek(n)
	r.bit += n
	return v
}
