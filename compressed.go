package postings

import "github.com/dgryski/go-groupvarint"

type compressedBlock struct {
	groups []byte  // the compressed data
	docID  Posting // docID is the first ID in the block
	count  uint16  // how many IDs are in this block
}

type citer struct {
	c *compressedBlock // pointer to compressed posting list

	group   [4]uint32 // the current group
	docID   Posting   // current docID (for tracking deltas)
	current uint16    // index into group[]
	count   uint16    // IDs processed, to match against c.count
	offs    uint16    // offset into c.groups
}

const blockLimitBytes = 4096

func newCompressedBlock(docs []Posting) ([]Posting, compressedBlock) {

	cblock := compressedBlock{
		docID: docs[0],
	}

	prev := docs[0]

	buf := make([]byte, 17)
	deltas := make([]uint32, 4)

	for len(docs) >= 4 {
		deltas[0] = uint32(docs[0].Doc() - prev.Doc())
		deltas[1] = uint32(docs[1].Doc() - docs[0].Doc())
		deltas[2] = uint32(docs[2].Doc() - docs[1].Doc())
		deltas[3] = uint32(docs[3].Doc() - docs[2].Doc())

		b := groupvarint.Encode4(buf, deltas)

		if len(cblock.groups)+len(b) >= blockLimitBytes {
			return docs, cblock
		}

		cblock.groups = append(cblock.groups, b...)
		cblock.count += 4
		prev = docs[3]
		docs = docs[4:]
	}

	// the remaining
	for _, d := range docs {
		b := groupvarint.Encode1(buf, uint32(d.Doc()-prev.Doc()))

		if len(cblock.groups)+len(b) >= blockLimitBytes {
			return docs, cblock
		}

		cblock.groups = append(cblock.groups, b...)
		cblock.count++
		prev = d
		docs = docs[1:]
	}

	return docs, cblock
}

func newCompressedIter(cblock compressedBlock) *citer {

	iter := &citer{
		c:     &cblock,
		docID: cblock.docID,
	}

	// load the first group and set docID so at() is correct
	iter.load()
	iter.current = 1

	return iter
}

func (it *citer) load() {
	rem := it.c.count - it.count
	if rem >= 4 {
		groupvarint.Decode4(it.group[:], it.c.groups[it.offs:])
		it.offs += uint16(groupvarint.BytesUsed[it.c.groups[it.offs]])
	} else {
		for i := uint16(0); i < rem; i++ {
			it.offs += uint16(groupvarint.Decode1(&it.group[i], it.c.groups[it.offs:]))
		}
	}
}

func (it *citer) next() bool {

	it.count++

	// end of this group -- read another
	if it.current == 4 {
		it.load()
		it.current = 0
	}

	// consume next delta in group
	it.docID += Posting(it.group[it.current] << 8)
	it.current++

	return true
}

func (it *citer) at() Posting {
	return Posting(it.docID)
}

func (it *citer) end() bool {
	return it.count >= it.c.count
}