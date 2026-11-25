package azw3

import (
	"errors"
	"sort"
)

// encintBackward encodes a non-negative integer as a backward-encoded MOBI
// variable-width integer (MSB set on the first byte, used for trailing record sizes).
func encintBackward(v int) []byte {
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
	// Backward-encoded: first emitted byte (MSB group, stored at tmp[n-1]) has bit 7 set.
	tmp[n-1] |= 0x80
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = tmp[n-1-i]
	}
	return out
}

// encodeTrailingData wraps raw trailing bytes as <raw><backward-vwi(len(raw)+len(vwi))>.
func encodeTrailingData(raw []byte) []byte {
	lsize := 1
	var encoded []byte
	for {
		encoded = encintBackward(len(raw) + lsize)
		if len(encoded) == lsize {
			break
		}
		lsize++
	}
	out := make([]byte, 0, len(raw)+len(encoded))
	out = append(out, raw...)
	out = append(out, encoded...)
	return out
}

// encodeFVWI encodes val shifted left by flagSize bits OR'd with the low flagSize bits of flags.
func encodeFVWI(val, flags, flagSize int) []byte {
	ans := val << flagSize
	for i := 0; i < flagSize; i++ {
		ans |= flags & (1 << i)
	}
	return encint(ans)
}

// encodeTBS encodes a single TBS sequence entry (val + extra flag payloads).
func encodeTBS(val int, extra map[int]int, flagSize int) []byte {
	flags := 0
	for flag := range extra {
		flags |= flag
	}
	out := encodeFVWI(val, flags, flagSize)
	if v, ok := extra[0b0010]; ok {
		out = append(out, encint(v)...)
	}
	if v, ok := extra[0b0100]; ok {
		out = append(out, byte(v))
	}
	if v, ok := extra[0b0001]; ok {
		out = append(out, encint(v)...)
	}
	return out
}

type tbsEntry struct {
	index        int
	start        int
	length       int
	depth        int
	hasParent    bool
	parent       int
	action       string // "starts", "spans", "ends", "completes"
	startOffset  int
	lengthOffset int
	recLength    int
}

type tbsLayer struct {
	depth   int
	entries []*tbsEntry
}

type tbsStrand []tbsLayer

type tbsSequence struct {
	val   int
	extra map[int]int
}

var errNegativeStrandIndex = errors.New("negative strand index")

func fillTBSEntry(base tbsEntry, startOffset, recLength int) *tbsEntry {
	lengthOffset := startOffset + base.length
	var action string
	if startOffset < 0 {
		if lengthOffset > recLength {
			action = "spans"
		} else {
			action = "ends"
		}
	} else {
		if lengthOffset > recLength {
			action = "starts"
		} else {
			action = "completes"
		}
	}
	e := base
	e.action = action
	e.startOffset = startOffset
	e.lengthOffset = lengthOffset
	e.recLength = recLength
	return &e
}

func removeTBSEntry(list *[]*tbsEntry, target *tbsEntry) {
	s := *list
	for i, e := range s {
		if e == target {
			copy(s[i:], s[i+1:])
			*list = s[:len(s)-1]
			return
		}
	}
}

func populateTBSStrand(parent *tbsEntry, entries *[]*tbsEntry) []*tbsEntry {
	ans := []*tbsEntry{parent}
	var firstChild *tbsEntry
	for _, c := range *entries {
		if c.hasParent && c.parent == parent.index {
			firstChild = c
			break
		}
	}
	if firstChild != nil {
		removeTBSEntry(entries, firstChild)
		ans = append(ans, populateTBSStrand(firstChild, entries)...)
		return ans
	}

	// Collect contiguous siblings at the same depth under the same parent.
	currentIndex := parent.index
	var siblings []*tbsEntry
	snapshot := append([]*tbsEntry(nil), (*entries)...)
	for _, entry := range snapshot {
		sameParent := entry.hasParent == parent.hasParent && (!entry.hasParent || entry.parent == parent.parent)
		if entry.depth == parent.depth && sameParent && entry.index == currentIndex+1 {
			currentIndex++
			removeTBSEntry(entries, entry)
			hasChild := false
			for _, c := range *entries {
				if c.hasParent && c.parent == entry.index {
					hasChild = true
					break
				}
			}
			if hasChild {
				siblings = append(siblings, populateTBSStrand(entry, entries)...)
				break
			}
			siblings = append(siblings, entry)
		}
	}
	ans = append(ans, siblings...)
	return ans
}

func separateTBSStrands(local []*tbsEntry) []tbsStrand {
	var strands []tbsStrand
	for len(local) > 0 {
		top := local[0]
		local = local[1:]
		chain := populateTBSStrand(top, &local)
		var layers tbsStrand
		for _, entry := range chain {
			found := false
			for i := range layers {
				if layers[i].depth == entry.depth {
					layers[i].entries = append(layers[i].entries, entry)
					found = true
					break
				}
			}
			if !found {
				layers = append(layers, tbsLayer{depth: entry.depth, entries: []*tbsEntry{entry}})
			}
		}
		strands = append(strands, layers)
	}
	return strands
}

func collectTBSIndexingData(entries []tbsEntry, textRecordLengths []int) [][]tbsStrand {
	sorted := append([]tbsEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].start < sorted[j].start
	})

	data := make([][]tbsStrand, 0, len(textRecordLengths))
	recordStart := 0
	for _, recLen := range textRecordLengths {
		nextRecordStart := recordStart + recLen
		var local []*tbsEntry
		for _, entry := range sorted {
			if entry.start >= nextRecordStart {
				break
			}
			if entry.start+entry.length <= recordStart {
				continue
			}
			local = append(local, fillTBSEntry(entry, entry.start-recordStart, recLen))
		}
		data = append(data, separateTBSStrands(local))
		recordStart += recLen
	}
	return data
}

func encodeTBSStrandsAsSequences(strands []tbsStrand, tbsType int) ([]tbsSequence, error) {
	var ans []tbsSequence
	hasLastIndex := false
	lastIndex := 0
	var firstEntry *tbsEntry
	for _, strand := range strands {
		for _, layer := range strand {
			if len(layer.entries) > 0 && firstEntry == nil {
				firstEntry = layer.entries[0]
				break
			}
		}
		if firstEntry != nil {
			break
		}
	}

	for _, strand := range strands {
		strandSeqs := make([]tbsSequence, 0, len(strand))
		for _, layer := range strand {
			entries := layer.entries
			extra := make(map[int]int)
			if entries[len(entries)-1].action == "spans" {
				extra[0b1] = 0
			}
			if entries[0] == firstEntry {
				extra[0b10] = tbsType
			}
			if len(entries) > 1 {
				extra[0b100] = len(entries)
			}

			parentIdx := 0
			if entries[0].hasParent {
				parentIdx = entries[0].parent
			}
			index := entries[0].index - parentIdx
			if len(ans) > 0 && len(strandSeqs) == 0 && hasLastIndex {
				index = lastIndex - entries[0].index
				if index < 0 {
					if tbsType == 5 {
						index = -index
					} else {
						return nil, errNegativeStrandIndex
					}
				} else {
					extra[0b1000] = 1
				}
			}
			lastIndex = entries[len(entries)-1].index
			hasLastIndex = true
			strandSeqs = append(strandSeqs, tbsSequence{val: index, extra: extra})
		}

		// Consecutive "spans" entries keep the 0b1=0 flag only on the deepest/last one.
		for i := 0; i+1 < len(strandSeqs); i++ {
			_, ok1 := strandSeqs[i].extra[0b1]
			_, ok2 := strandSeqs[i+1].extra[0b1]
			if ok1 && ok2 {
				delete(strandSeqs[i].extra, 0b1)
			}
		}
		ans = append(ans, strandSeqs...)
	}
	return ans, nil
}

func tbsSequencesToBytes(seqs []tbsSequence) []byte {
	var out []byte
	flagSize := 3
	for _, seq := range seqs {
		out = append(out, encodeTBS(seq.val, seq.extra, flagSize)...)
		flagSize = 4
	}
	return out
}

func calculateAllTBS(indexingData [][]tbsStrand, tbsType int) ([][]byte, error) {
	out := make([][]byte, len(indexingData))
	for i, strands := range indexingData {
		seqs, err := encodeTBSStrandsAsSequences(strands, tbsType)
		if err != nil {
			return nil, err
		}
		out[i] = tbsSequencesToBytes(seqs)
	}
	return out, nil
}

// applyTrailingByteSequences appends Calibre/KindleGen-compatible TBS indexing
// data (extra_data_flags bit 1) to text records records[1..len(uncompressedLengths)].
func applyTrailingByteSequences(ncx []ncxEntry, records [][]byte, uncompressedLengths []int) bool {
	if len(ncx) == 0 || len(uncompressedLengths) == 0 {
		return false
	}
	entries := make([]tbsEntry, len(ncx))
	for i, e := range ncx {
		entries[i] = tbsEntry{
			index:     e.index,
			start:     e.offset,
			length:    e.length,
			depth:     e.depth,
			hasParent: e.hasParent,
			parent:    e.parent,
		}
	}
	indexingData := collectTBSIndexingData(entries, uncompressedLengths)
	allBytes, err := calculateAllTBS(indexingData, 8)
	if errors.Is(err, errNegativeStrandIndex) {
		allBytes, err = calculateAllTBS(indexingData, 5)
	}
	if err != nil {
		return false
	}
	for i, tbs := range allBytes {
		records[i+1] = append(records[i+1], encodeTrailingData(tbs)...)
	}
	return true
}
