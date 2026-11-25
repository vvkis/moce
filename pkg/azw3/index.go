package azw3

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const indxHeaderLen = 192

var base32Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUV"

// toBase32 formats a non-negative integer in uppercase base32 with zero padding.
func toBase32(v int, minDigits int) string {
	if v < 0 {
		v = 0
	}
	var buf [16]byte
	pos := len(buf)
	for {
		pos--
		buf[pos] = base32Alphabet[v&31]
		v >>= 5
		if v == 0 {
			break
		}
	}
	for len(buf)-pos < minDigits {
		pos--
		buf[pos] = '0'
	}
	return string(buf[pos:])
}

// align4 pads b with zero bytes to a 4-byte boundary.
func align4(b []byte) []byte {
	rem := len(b) % 4
	if rem == 0 {
		return b
	}
	pad := 4 - rem
	out := make([]byte, len(b)+pad)
	copy(out, b)
	return out
}

// encint encodes a non-negative integer as a forward-encoded MOBI variable-width integer.
func encint(v int) []byte {
	if v < 0 {
		v = 0
	}
	var tmp [10]byte
	n := 0
	for {
		tmp[n] = byte(v & 0x7F)
		v >>= 7
		n++
		if v == 0 {
			break
		}
	}
	// Forward-encoded: last emitted byte (LSB, stored at tmp[0]) has MSB set.
	tmp[0] |= 0x80
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = tmp[n-1-i]
	}
	return out
}

// cncxTable builds CNCX string records and maps each unique string to its CNCX offset.
type cncxTable struct {
	records [][]byte
	offsets map[string]int
}

func buildCNCX(strings []string) cncxTable {
	const limit = 0x10000 - 1024
	t := cncxTable{offsets: make(map[string]int, len(strings))}
	var buf bytes.Buffer
	offset := 0
	for _, s := range strings {
		if _, exists := t.offsets[s]; exists {
			continue
		}
		rawStr := []byte(s)
		if len(rawStr) > 500 {
			rawStr = truncateUTF8(rawStr, 500)
		}
		entry := append(encint(len(rawStr)), rawStr...)
		if buf.Len()+len(entry) > limit {
			t.records = append(t.records, align4(buf.Bytes()))
			buf.Reset()
			offset = len(t.records) * 0x10000
		}
		t.offsets[s] = offset
		buf.Write(entry)
		offset += len(entry)
	}
	if buf.Len() > 0 {
		t.records = append(t.records, align4(buf.Bytes()))
	}
	return t
}

func truncateUTF8(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	cut := max
	for cut > 0 && (b[cut]&0xC0) == 0x80 {
		cut--
	}
	return b[:cut]
}

type tagDef struct {
	name    string
	number  byte
	vpe     byte // values per entry
	bitmask byte
	endFlag byte
}

var endTagDef = tagDef{name: "eof", number: 0, vpe: 0, bitmask: 0, endFlag: 1}

func maskShift(mask byte) uint {
	switch mask {
	case 1, 3:
		return 0
	case 2:
		return 1
	case 4, 12:
		return 2
	case 8:
		return 3
	case 16, 48:
		return 4
	case 32:
		return 5
	case 64, 192:
		return 6
	case 128:
		return 7
	default:
		var s uint
		for mask > 0 && (mask&1) == 0 {
			mask >>= 1
			s++
		}
		return s
	}
}

type indexEntry struct {
	key  string
	tags map[string][]int
}

func buildIndexRecords(tagDefs []tagDef, controlByteCount int, entries []indexEntry, cncx cncxTable) [][]byte {
	const recordLimit = 0x10000 - indxHeaderLen - 1048

	type block struct {
		data    bytes.Buffer
		idxt    bytes.Buffer
		count   int
		lastKey []byte
	}
	blocks := []*block{{}}

	var entryBuf bytes.Buffer
	for _, e := range entries {
		entryBuf.Reset()
		keyBytes := []byte(e.key)
		if len(keyBytes) > 255 {
			keyBytes = keyBytes[:255]
		}
		entryBuf.WriteByte(byte(len(keyBytes)))
		entryBuf.Write(keyBytes)

		// Control bytes
		var cb byte
		for _, td := range tagDefs {
			if td.endFlag == 1 {
				entryBuf.WriteByte(cb)
				cb = 0
				continue
			}
			vals := e.tags[td.name]
			if len(vals) > 0 {
				nEntries := len(vals) / int(td.vpe)
				cb |= td.bitmask & byte(nEntries<<maskShift(td.bitmask))
			}
		}

		// Tag values
		for _, td := range tagDefs {
			if td.endFlag == 1 {
				continue
			}
			for _, v := range e.tags[td.name] {
				entryBuf.Write(encint(v))
			}
		}

		raw := entryBuf.Bytes()
		cur := blocks[len(blocks)-1]
		if cur.count > 0 && cur.data.Len()+cur.idxt.Len()+len(raw)+2 > recordLimit {
			cur = &block{}
			blocks = append(blocks, cur)
		}
		var off [2]byte
		binary.BigEndian.PutUint16(off[:], uint16(indxHeaderLen+cur.data.Len()))
		cur.idxt.Write(off[:])
		cur.data.Write(raw)
		cur.count++
		cur.lastKey = append([]byte(nil), keyBytes...)
	}

	// Build secondary INDX data records.
	indexRecords := make([][]byte, 0, len(blocks))
	totalEntries := 0
	for _, blk := range blocks {
		totalEntries += blk.count
		dataPart := align4(blk.data.Bytes())
		idxtPart := align4(append([]byte("IDXT"), blk.idxt.Bytes()...))

		hdr := make([]byte, indxHeaderLen)
		copy(hdr[0:4], "INDX")
		binary.BigEndian.PutUint32(hdr[4:8], indxHeaderLen)
		// 8..12: 0
		binary.BigEndian.PutUint32(hdr[12:16], 1) // secondary index record
		// 16..20: 0
		binary.BigEndian.PutUint32(hdr[20:24], uint32(indxHeaderLen+len(dataPart)))
		binary.BigEndian.PutUint32(hdr[24:28], uint32(blk.count))
		binary.BigEndian.PutUint32(hdr[28:32], 0xFFFFFFFF)
		binary.BigEndian.PutUint32(hdr[32:36], 0xFFFFFFFF)

		rec := make([]byte, 0, len(hdr)+len(dataPart)+len(idxtPart))
		rec = append(rec, hdr...)
		rec = append(rec, dataPart...)
		rec = append(rec, idxtPart...)
		indexRecords = append(indexRecords, rec)
	}

	// Build TAGX block.
	var tagx bytes.Buffer
	tagx.WriteString("TAGX")
	var tagBytes bytes.Buffer
	for _, td := range tagDefs {
		tagBytes.Write([]byte{td.number, td.vpe, td.bitmask, td.endFlag})
	}
	var tmp8 [8]byte
	binary.BigEndian.PutUint32(tmp8[0:4], uint32(12+tagBytes.Len()))
	binary.BigEndian.PutUint32(tmp8[4:8], uint32(controlByteCount))
	tagx.Write(tmp8[:])
	tagx.Write(tagBytes.Bytes())
	tagxAligned := align4(tagx.Bytes())

	// Build geometry & IDXT of header record.
	var geom bytes.Buffer
	var hdrIdxt bytes.Buffer
	hdrIdxt.WriteString("IDXT")
	pos := indxHeaderLen + len(tagxAligned)
	for _, blk := range blocks {
		var u16 [2]byte
		binary.BigEndian.PutUint16(u16[:], uint16(pos))
		hdrIdxt.Write(u16[:])
		start := geom.Len()
		geom.WriteByte(byte(len(blk.lastKey)))
		geom.Write(blk.lastKey)
		binary.BigEndian.PutUint16(u16[:], uint16(blk.count))
		geom.Write(u16[:])
		pos += geom.Len() - start
	}
	geomAligned := align4(geom.Bytes())
	hdrIdxtAligned := align4(hdrIdxt.Bytes())

	// Build primary INDX header record.
	hdr := make([]byte, indxHeaderLen)
	copy(hdr[0:4], "INDX")
	binary.BigEndian.PutUint32(hdr[4:8], indxHeaderLen)
	// 8..16: 0
	binary.BigEndian.PutUint32(hdr[16:20], 2) // inflection / KF8 index type
	binary.BigEndian.PutUint32(hdr[20:24], uint32(indxHeaderLen+len(tagxAligned)+len(geomAligned)))
	binary.BigEndian.PutUint32(hdr[24:28], uint32(len(indexRecords)))
	binary.BigEndian.PutUint32(hdr[28:32], 65001) // UTF-8
	binary.BigEndian.PutUint32(hdr[32:36], 0xFFFFFFFF)
	binary.BigEndian.PutUint32(hdr[36:40], uint32(totalEntries))
	// 40..52: ordt/ligt = 0
	binary.BigEndian.PutUint32(hdr[52:56], uint32(len(cncx.records)))
	// 56..180: 0
	binary.BigEndian.PutUint32(hdr[180:184], indxHeaderLen) // tagx offset
	// 184..192: 0

	headerRec := make([]byte, 0, len(hdr)+len(tagxAligned)+len(geomAligned)+len(hdrIdxtAligned))
	headerRec = append(headerRec, hdr...)
	headerRec = append(headerRec, tagxAligned...)
	headerRec = append(headerRec, geomAligned...)
	headerRec = append(headerRec, hdrIdxtAligned...)
	headerRec = align4(headerRec)

	out := make([][]byte, 0, 1+len(indexRecords)+len(cncx.records))
	out = append(out, headerRec)
	out = append(out, indexRecords...)
	out = append(out, cncx.records...)
	return out
}

type skelEntry struct {
	fileNumber int
	name       string
	chunkCount int
	startPos   int
	length     int
}

type chunkEntry struct {
	insertPos      int
	selector       string
	fileNumber     int
	sequenceNumber int
	startPos       int
	length         int
}

type guideEntry struct {
	typ    string
	title  string
	posFID [2]int
}

type ncxEntry struct {
	index      int
	offset     int
	length     int
	label      string
	depth      int
	hasParent  bool
	parent     int
	hasChild   bool
	firstChild int
	lastChild  int
	posFID     [2]int
}

func buildSkelIndex(skels []skelEntry) [][]byte {
	defs := []tagDef{
		{name: "chunk_count", number: 1, vpe: 1, bitmask: 3, endFlag: 0},
		{name: "geometry", number: 6, vpe: 2, bitmask: 12, endFlag: 0},
		endTagDef,
	}
	entries := make([]indexEntry, 0, len(skels))
	for _, s := range skels {
		entries = append(entries, indexEntry{
			key: s.name,
			tags: map[string][]int{
				"chunk_count": {s.chunkCount, s.chunkCount},
				"geometry":    {s.startPos, s.length, s.startPos, s.length},
			},
		})
	}
	return buildIndexRecords(defs, 1, entries, cncxTable{})
}

func buildChunkIndex(chunks []chunkEntry) [][]byte {
	defs := []tagDef{
		{name: "cncx_offset", number: 2, vpe: 1, bitmask: 1, endFlag: 0},
		{name: "file_number", number: 3, vpe: 1, bitmask: 2, endFlag: 0},
		{name: "sequence_number", number: 4, vpe: 1, bitmask: 4, endFlag: 0},
		{name: "geometry", number: 6, vpe: 2, bitmask: 8, endFlag: 0},
		endTagDef,
	}
	selectors := make([]string, len(chunks))
	for i, c := range chunks {
		selectors[i] = c.selector
	}
	cncx := buildCNCX(selectors)
	entries := make([]indexEntry, 0, len(chunks))
	for _, c := range chunks {
		entries = append(entries, indexEntry{
			key: fmt.Sprintf("%010d", c.insertPos),
			tags: map[string][]int{
				"cncx_offset":     {cncx.offsets[c.selector]},
				"file_number":     {c.fileNumber},
				"sequence_number": {c.sequenceNumber},
				"geometry":        {c.startPos, c.length},
			},
		})
	}
	return buildIndexRecords(defs, 1, entries, cncx)
}

func buildGuideIndex(guides []guideEntry) [][]byte {
	if len(guides) == 0 {
		return nil
	}
	defs := []tagDef{
		{name: "title", number: 1, vpe: 1, bitmask: 1, endFlag: 0},
		{name: "pos_fid", number: 6, vpe: 2, bitmask: 2, endFlag: 0},
		endTagDef,
	}
	titles := make([]string, len(guides))
	for i, g := range guides {
		titles[i] = g.title
	}
	cncx := buildCNCX(titles)
	entries := make([]indexEntry, 0, len(guides))
	for _, g := range guides {
		entries = append(entries, indexEntry{
			key: g.typ,
			tags: map[string][]int{
				"title":   {cncx.offsets[g.title]},
				"pos_fid": {g.posFID[0], g.posFID[1]},
			},
		})
	}
	return buildIndexRecords(defs, 1, entries, cncx)
}

func buildNCXIndex(items []ncxEntry) [][]byte {
	if len(items) == 0 {
		return nil
	}
	defs := []tagDef{
		{name: "offset", number: 1, vpe: 1, bitmask: 1, endFlag: 0},
		{name: "length", number: 2, vpe: 1, bitmask: 2, endFlag: 0},
		{name: "label", number: 3, vpe: 1, bitmask: 4, endFlag: 0},
		{name: "depth", number: 4, vpe: 1, bitmask: 8, endFlag: 0},
		{name: "parent", number: 21, vpe: 1, bitmask: 16, endFlag: 0},
		{name: "first_child", number: 22, vpe: 1, bitmask: 32, endFlag: 0},
		{name: "last_child", number: 23, vpe: 1, bitmask: 64, endFlag: 0},
		{name: "pos_fid", number: 6, vpe: 2, bitmask: 128, endFlag: 0},
		endTagDef,
	}
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.label
	}
	cncx := buildCNCX(labels)

	maxIdx := 0
	for _, it := range items {
		if it.index > maxIdx {
			maxIdx = it.index
		}
	}
	hexWidth := len(fmt.Sprintf("%X", maxIdx))
	if hexWidth < 2 {
		hexWidth = 2
	}
	keyFmt := fmt.Sprintf("%%0%dX", hexWidth)

	entries := make([]indexEntry, 0, len(items))
	for _, it := range items {
		tags := map[string][]int{
			"offset":  {it.offset},
			"length":  {it.length},
			"label":   {cncx.offsets[it.label]},
			"depth":   {it.depth},
			"pos_fid": {it.posFID[0], it.posFID[1]},
		}
		if it.hasParent {
			tags["parent"] = []int{it.parent}
		}
		if it.hasChild {
			tags["first_child"] = []int{it.firstChild}
			tags["last_child"] = []int{it.lastChild}
		}
		entries = append(entries, indexEntry{
			key:  fmt.Sprintf(keyFmt, it.index),
			tags: tags,
		})
	}
	return buildIndexRecords(defs, 1, entries, cncx)
}
