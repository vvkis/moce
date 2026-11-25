package azw3

import "unicode/utf8"

const maxRecordSize = 4096

type textRecordChunk struct {
	data    []byte // Always 4096 bytes for all records except possibly the last
	overlap []byte // 0..3 trailing UTF-8 continuation bytes from the next record
}

// splitTextRecords splits raw UTF-8 bytes into strict 4096-byte PalmDOC text
// records (as required by Kindle's O(1) pos/4096 random-access text reader).
// Whenever a 4096-byte boundary splits a multi-byte UTF-8 rune, the remaining
// 1..3 continuation bytes are returned in overlap so they can be stored in the
// record's trailing multibyte-overlap trailer (extra_data_flags bit 0).
func splitTextRecords(raw []byte) []textRecordChunk {
	if len(raw) == 0 {
		return []textRecordChunk{{data: []byte{}}}
	}
	chunks := make([]textRecordChunk, 0, (len(raw)+maxRecordSize-1)/maxRecordSize)
	for pos := 0; pos < len(raw); {
		end := pos + maxRecordSize
		if end >= len(raw) {
			chunks = append(chunks, textRecordChunk{data: raw[pos:]})
			break
		}
		extra := 0
		for end+extra < len(raw) && extra < 3 && !utf8.RuneStart(raw[end+extra]) {
			extra++
		}
		var overlap []byte
		if extra > 0 {
			overlap = raw[end : end+extra]
		}
		chunks = append(chunks, textRecordChunk{
			data:    raw[pos:end],
			overlap: overlap,
		})
		pos = end
	}
	return chunks
}

// compressPalmDOC encodes data using the standard PalmDOC LZ77 algorithm.
// Decoders read:
//   - 0x00, 0x09..0x7F: literal byte
//   - 0x01..0x08: N literal bytes follow (for 0x01..0x08 and >=0x80 bytes)
//   - 0x80..0xBF + next byte: LZ77 back-reference (distance 1..2047, length 3..10)
//   - 0xC0..0xFF: space (' ') followed by (byte ^ 0x80)
func compressPalmDOC(data []byte) []byte {
	n := len(data)
	out := make([]byte, 0, n)
	i := 0
	for i < n {
		// Try LZ77 match in the last 2047 bytes (min length 3, max length 10).
		bestLen := 0
		bestDist := 0
		if i > 0 && i+3 <= n {
			winStart := i - 2047
			if winStart < 0 {
				winStart = 0
			}
			maxLen := 10
			if n-i < maxLen {
				maxLen = n - i
			}
			b0, b1, b2 := data[i], data[i+1], data[i+2]
			for pos := i - 1; pos >= winStart; pos-- {
				if data[pos] == b0 && data[pos+1] == b1 && data[pos+2] == b2 {
					l := 3
					for l < maxLen && data[pos+l] == data[i+l] {
						l++
					}
					if l > bestLen {
						bestLen = l
						bestDist = i - pos
						if bestLen == maxLen {
							break
						}
					}
				}
			}
		}
		if bestLen >= 3 {
			combined := uint16(0x8000) | uint16(bestDist<<3) | uint16(bestLen-3)
			out = append(out, byte(combined>>8), byte(combined))
			i += bestLen
			continue
		}

		c := data[i]
		// Space + printable ASCII pair compression (0xC0..0xFF).
		if c == ' ' && i+1 < n && data[i+1] >= 0x40 && data[i+1] <= 0x7F {
			out = append(out, data[i+1]^0x80)
			i += 2
			continue
		}
		// Self-representing literal byte.
		if c == 0x00 || (c >= 0x09 && c <= 0x7F) {
			out = append(out, c)
			i++
			continue
		}
		// Run of up to 8 literal bytes that require escaping (e.g. UTF-8 multi-byte).
		start := i
		for i < n && i-start < 8 {
			b := data[i]
			if b == 0x00 || (b >= 0x09 && b <= 0x7F) {
				break
			}
			i++
		}
		runLen := i - start
		out = append(out, byte(runLen))
		out = append(out, data[start:i]...)
	}
	return out
}
