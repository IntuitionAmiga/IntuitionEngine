// lh5_decompress.go - Pure Go LH5 (Lempel-Ziv-Huffman) decompressor.
// Used for VTX file format decompression.
//
// LH5 uses a 13-bit sliding window (8KB dictionary) with
// block-based adaptive Huffman coding.

package main

import (
	"fmt"
)

const (
	lh5DicBit     = 13                 // Dictionary size in bits
	lh5DicSize    = 1 << lh5DicBit     // 8192 byte sliding window
	lh5Threshold  = 3                  // Minimum match length
	lh5NC         = 510                // Character codes: 256 literals + 254 length codes
	lh5NP         = 14                 // Position codes (dicBit + 1)
	lh5NT         = 19                 // Temp tree entries (for encoding code lengths)
	lh5TBit       = 5                  // Bits for temp tree count
	lh5CBit       = 9                  // Bits for char tree count
	lh5PBit       = 4                  // Bits for position tree count
	lh5CTableBits = 12                 // Char lookup table width
	lh5PTableBits = 8                  // Position lookup table width
	lh5CTableSize = 1 << lh5CTableBits // 4096
	lh5PTableSize = 1 << lh5PTableBits // 256
	lh5TreeNodes  = 2 * lh5NC          // Max tree nodes for overflow codes
)

type lh5Decoder struct {
	src    []byte
	srcPos int

	// Bit buffer: bits are MSB-aligned in bitBuf.
	// bitsAvail is the number of valid bits starting from the MSB.
	bitBuf    uint32
	bitsAvail int

	cTable [lh5CTableSize]uint16
	cLen   [lh5NC]uint8
	pTable [lh5PTableSize]uint16
	pLen   [lh5NT]uint8 // sized for max(NT=19, NP=14)

	left  [lh5TreeNodes]uint16
	right [lh5TreeNodes]uint16

	blockRemaining int
}

// decompressLH5 decompresses LH5-compressed data.
// origSize is the expected uncompressed size.
func decompressLH5(src []byte, origSize int) ([]byte, error) {
	if origSize <= 0 {
		return nil, fmt.Errorf("lh5: invalid original size %d", origSize)
	}
	if len(src) == 0 {
		return nil, fmt.Errorf("lh5: empty compressed data")
	}

	d := &lh5Decoder{src: src}

	out := make([]byte, origSize)
	var dict [lh5DicSize]byte
	dictPos := 0
	outPos := 0

	for outPos < origSize {
		c, err := d.decodeC()
		if err != nil {
			return nil, err
		}
		if c < 256 {
			dict[dictPos] = byte(c)
			dictPos = (dictPos + 1) & (lh5DicSize - 1)
			out[outPos] = byte(c)
			outPos++
		} else {
			matchLen := c - 256 + lh5Threshold
			p, err := d.decodeP()
			if err != nil {
				return nil, err
			}
			matchPos := (dictPos - p - 1) & (lh5DicSize - 1)
			for i := 0; i < matchLen && outPos < origSize; i++ {
				b := dict[matchPos]
				dict[dictPos] = b
				dictPos = (dictPos + 1) & (lh5DicSize - 1)
				matchPos = (matchPos + 1) & (lh5DicSize - 1)
				out[outPos] = b
				outPos++
			}
		}
	}

	return out, nil
}

// ensureBits fills the bit buffer to have at least n valid bits.
func (d *lh5Decoder) ensureBits(n int) {
	for d.bitsAvail < n && d.srcPos < len(d.src) {
		d.bitBuf |= uint32(d.src[d.srcPos]) << uint(24-d.bitsAvail)
		d.srcPos++
		d.bitsAvail += 8
	}
}

// peekBits returns the top n bits without consuming them.
func (d *lh5Decoder) peekBits(n int) uint16 {
	d.ensureBits(n)
	return uint16(d.bitBuf >> uint(32-n))
}

// dropBits consumes n bits from the buffer.
func (d *lh5Decoder) dropBits(n int) {
	d.bitBuf <<= uint(n)
	d.bitsAvail -= n
}

// getBits reads and consumes n bits.
func (d *lh5Decoder) getBits(n int) uint16 {
	v := d.peekBits(n)
	d.dropBits(n)
	return v
}

func (d *lh5Decoder) decodeC() (int, error) {
	if d.blockRemaining <= 0 {
		d.blockRemaining = int(d.getBits(16))
		if err := d.readPTLen(lh5NT, lh5TBit, 3); err != nil {
			return 0, err
		}
		if err := d.readCLen(); err != nil {
			return 0, err
		}
		if err := d.readPTLen(lh5NP, lh5PBit, -1); err != nil {
			return 0, err
		}
	}
	d.blockRemaining--

	d.ensureBits(lh5CTableBits)
	j := d.cTable[d.bitBuf>>uint(32-lh5CTableBits)]
	if int(j) >= lh5NC {
		mask := uint32(1) << uint(32-lh5CTableBits-1)
		for int(j) >= lh5NC {
			if d.bitBuf&mask != 0 {
				j = d.right[j]
			} else {
				j = d.left[j]
			}
			mask >>= 1
		}
	}
	d.dropBits(int(d.cLen[j]))
	return int(j), nil
}

func (d *lh5Decoder) decodeP() (int, error) {
	d.ensureBits(lh5PTableBits)
	j := d.pTable[d.bitBuf>>uint(32-lh5PTableBits)]
	if int(j) >= lh5NP {
		mask := uint32(1) << uint(32-lh5PTableBits-1)
		for int(j) >= lh5NP {
			if d.bitBuf&mask != 0 {
				j = d.right[j]
			} else {
				j = d.left[j]
			}
			mask >>= 1
		}
	}
	d.dropBits(int(d.pLen[j]))

	if j == 0 {
		return 0, nil
	}
	extra := int(d.getBits(int(j) - 1))
	return (1 << uint(j-1)) + extra, nil
}

// readPTLen reads a Huffman tree for position/temp decoding.
func (d *lh5Decoder) readPTLen(nn, nBit, iSpecial int) error {
	n := int(d.getBits(nBit))
	if n == 0 {
		c := d.getBits(nBit)
		for i := range nn {
			d.pLen[i] = 0
		}
		for i := range lh5PTableSize {
			d.pTable[i] = c
		}
		return nil
	}

	i := 0
	for i < n && i < nn {
		c := int(d.peekBits(3))
		if c == 7 {
			mask := uint32(1) << uint(32-4)
			for mask&d.bitBuf != 0 {
				c++
				mask >>= 1
				if c > 16 {
					return fmt.Errorf("lh5: invalid code length")
				}
			}
		}
		if c < 7 {
			d.dropBits(3)
		} else {
			d.dropBits(c - 3)
		}
		d.pLen[i] = uint8(c)
		i++

		if i == iSpecial {
			gap := int(d.getBits(2))
			for gap > 0 && i < nn {
				d.pLen[i] = 0
				i++
				gap--
			}
		}
	}
	for i < nn {
		d.pLen[i] = 0
		i++
	}

	return d.makeTable(nn, d.pLen[:nn], lh5PTableBits, d.pTable[:], d.left[:], d.right[:])
}

// readCLen reads the character Huffman tree using the temp tree (in pTable/pLen).
func (d *lh5Decoder) readCLen() error {
	n := int(d.getBits(lh5CBit))
	if n == 0 {
		c := d.getBits(lh5CBit)
		for i := range lh5NC {
			d.cLen[i] = 0
		}
		for i := range lh5CTableSize {
			d.cTable[i] = c
		}
		return nil
	}

	i := 0
	for i < n && i < lh5NC {
		d.ensureBits(lh5PTableBits)
		c := int(d.pTable[d.bitBuf>>uint(32-lh5PTableBits)])
		if c >= lh5NT {
			mask := uint32(1) << uint(32-lh5PTableBits-1)
			for c >= lh5NT {
				if d.bitBuf&mask != 0 {
					c = int(d.right[c])
				} else {
					c = int(d.left[c])
				}
				mask >>= 1
			}
		}
		d.dropBits(int(d.pLen[c]))

		if c <= 2 {
			var runLen int
			switch c {
			case 0:
				runLen = 1
			case 1:
				runLen = int(d.getBits(4)) + 3
			case 2:
				runLen = int(d.getBits(lh5CBit)) + 20
			}
			for runLen > 0 && i < lh5NC {
				d.cLen[i] = 0
				i++
				runLen--
			}
		} else {
			d.cLen[i] = uint8(c - 2)
			i++
		}
	}
	for i < lh5NC {
		d.cLen[i] = 0
		i++
	}

	return d.makeTable(lh5NC, d.cLen[:], lh5CTableBits, d.cTable[:], d.left[:], d.right[:])
}

// makeTable builds a Huffman lookup table from code lengths.
func (d *lh5Decoder) makeTable(nchar int, bitLen []uint8, tableBits int, table []uint16, left, right []uint16) error {
	tableSize := 1 << uint(tableBits)

	var count [17]uint16
	for i := range nchar {
		if bitLen[i] > 16 {
			return fmt.Errorf("lh5: code length %d exceeds 16", bitLen[i])
		}
		count[bitLen[i]]++
	}

	// Calculate starting codes using weight-based approach
	var weight [17]uint32
	for i := 1; i <= 16; i++ {
		weight[i] = 1 << uint(16-i)
	}

	total := uint32(0)
	var start [18]uint32
	for i := 1; i <= 16; i++ {
		start[i] = total
		total += uint32(count[i]) * weight[i]
	}

	// Shift start values for table-sized codes
	m := uint(16 - tableBits)
	for i := 1; i <= tableBits; i++ {
		start[i] >>= m
	}

	avail := uint16(nchar)

	for i := range tableSize {
		table[i] = 0
	}

	for ch := range nchar {
		l := int(bitLen[ch])
		if l == 0 {
			continue
		}

		if l <= tableBits {
			fillCount := 1 << uint(tableBits-l)
			idx := int(start[l])
			for k := 0; k < fillCount && idx+k < tableSize; k++ {
				table[idx+k] = uint16(ch)
			}
			start[l] += uint32(fillCount)
		} else {
			// Code longer than table: build overflow tree
			idx := int(start[l] >> uint(l-tableBits))
			if idx >= tableSize {
				continue
			}
			p := &table[idx]
			for i := tableBits + 1; i <= l; i++ {
				if *p == 0 {
					left[avail] = 0
					right[avail] = 0
					*p = avail
					avail++
				}
				bit := (start[l] >> uint(l-i)) & 1
				if bit != 0 {
					p = &right[*p]
				} else {
					p = &left[*p]
				}
			}
			*p = uint16(ch)
			start[l] += weight[l] >> m
		}
	}

	return nil
}
