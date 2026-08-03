// Package ie64archive reads and writes deterministic Unix archives.
package ie64archive

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

const magic = "!<arch>\n"

type Member struct {
	Name string
	Data []byte
}
type Archive struct {
	Members []Member
	Symbols map[string]int
}

func Replace(old, replacements []Member) []Member {
	out := append([]Member(nil), old...)
	at := map[string]int{}
	for i, m := range out {
		at[m.Name] = i
	}
	for _, m := range replacements {
		m.Data = append([]byte(nil), m.Data...)
		if i, ok := at[m.Name]; ok {
			out[i] = m
		} else {
			at[m.Name] = len(out)
			out = append(out, m)
		}
	}
	return out
}

func memberPayload(m Member) (string, []byte, error) {
	if m.Name == "" || strings.ContainsAny(m.Name, "\n\r") {
		return "", nil, fmt.Errorf("invalid archive member name %q", m.Name)
	}
	if len(m.Name) <= 15 && !strings.Contains(m.Name, " ") {
		return m.Name + "/", m.Data, nil
	}
	name := []byte(m.Name)
	return fmt.Sprintf("#1/%d", len(name)), append(name, m.Data...), nil
}

func header(name string, size int) []byte {
	h := bytes.Repeat([]byte{' '}, 60)
	copy(h, name)
	copy(h[16:], "0           ")
	copy(h[28:], "0     ")
	copy(h[34:], "0     ")
	copy(h[40:], "100644  ")
	copy(h[48:], fmt.Sprintf("%-10d", size))
	copy(h[58:], "`\n")
	return h
}

// Marshal writes a deterministic archive with a GNU-compatible symbol index.
func Marshal(members []Member) ([]byte, error) {
	type encoded struct {
		name string
		data []byte
	}
	enc := make([]encoded, len(members))
	symbols := []string{}
	symbolMember := []int{}
	for i, m := range members {
		n, d, e := memberPayload(m)
		if e != nil {
			return nil, e
		}
		enc[i] = encoded{n, d}
		obj, e := ie64obj.Parse(m.Data)
		if e != nil {
			continue
		}
		for _, s := range obj.Symbols {
			if s.Section != ie64obj.SHNUndef && s.Bind != ie64obj.STBLocal && s.Name != "" {
				symbols = append(symbols, s.Name)
				symbolMember = append(symbolMember, i)
			}
		}
	}
	indexSize := 4 + 4*len(symbols)
	for _, s := range symbols {
		indexSize += len(s) + 1
	}
	firstMember := 8 + 60 + indexSize
	if indexSize&1 != 0 {
		firstMember++
	}
	offsets := make([]uint32, len(members))
	off := firstMember
	for i, e := range enc {
		offsets[i] = uint32(off)
		off += 60 + len(e.data)
		if len(e.data)&1 != 0 {
			off++
		}
	}
	idx := make([]byte, indexSize)
	binary.BigEndian.PutUint32(idx, uint32(len(symbols)))
	p := 4
	for _, mi := range symbolMember {
		binary.BigEndian.PutUint32(idx[p:], offsets[mi])
		p += 4
	}
	for _, s := range symbols {
		copy(idx[p:], s)
		p += len(s) + 1
	}
	var out bytes.Buffer
	out.WriteString(magic)
	out.Write(header("/", len(idx)))
	out.Write(idx)
	if len(idx)&1 != 0 {
		out.WriteByte('\n')
	}
	for i, e := range enc {
		_ = i
		out.Write(header(e.name, len(e.data)))
		out.Write(e.data)
		if len(e.data)&1 != 0 {
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}

func parseDecimal(field []byte) (int, error) {
	v := strings.TrimSpace(string(field))
	if v == "" {
		return 0, nil
	}
	return strconv.Atoi(v)
}

// Parse accepts GNU and BSD extended-name archive members.
func Parse(data []byte) (*Archive, error) {
	if len(data) < 8 || string(data[:8]) != magic {
		return nil, fmt.Errorf("not a Unix archive")
	}
	out := &Archive{Symbols: map[string]int{}}
	headerToMember := map[int]int{}
	gnuIndex := map[string]int{}
	var stringTable []byte
	for off := 8; off < len(data); {
		if off+60 > len(data) {
			return nil, fmt.Errorf("truncated archive header")
		}
		h := data[off : off+60]
		if string(h[58:60]) != "`\n" {
			return nil, fmt.Errorf("invalid archive header at %d", off)
		}
		size, e := parseDecimal(h[48:58])
		if e != nil || size < 0 || off+60+size > len(data) {
			return nil, fmt.Errorf("invalid archive member size at %d", off)
		}
		rawName := strings.TrimSpace(string(h[:16]))
		payload := data[off+60 : off+60+size]
		next := off + 60 + size
		if next&1 != 0 {
			next++
		}
		if rawName == "/" {
			if len(payload) >= 4 {
				count := int(binary.BigEndian.Uint32(payload))
				if 4+count*4 <= len(payload) {
					names := payload[4+count*4:]
					pos := 0
					for i := 0; i < count; i++ {
						end := bytes.IndexByte(names[pos:], 0)
						if end < 0 {
							break
						}
						gnuIndex[string(names[pos:pos+end])] = int(binary.BigEndian.Uint32(payload[4+i*4:]))
						pos += end + 1
					}
				}
			}
			off = next
			continue
		}
		if rawName == "//" {
			stringTable = append([]byte(nil), payload...)
			off = next
			continue
		}
		name := strings.TrimSuffix(rawName, "/")
		body := payload
		if strings.HasPrefix(rawName, "#1/") {
			n, e := strconv.Atoi(strings.TrimPrefix(rawName, "#1/"))
			if e != nil || n < 0 || n > len(payload) {
				return nil, fmt.Errorf("invalid BSD archive name")
			}
			name = string(payload[:n])
			body = payload[n:]
		} else if strings.HasPrefix(rawName, "/") && len(rawName) > 1 {
			n, e := strconv.Atoi(strings.TrimSuffix(rawName[1:], "/"))
			if e != nil || n < 0 || n >= len(stringTable) {
				return nil, fmt.Errorf("invalid GNU archive name")
			}
			end := bytes.IndexByte(stringTable[n:], '/')
			if end < 0 {
				end = bytes.IndexByte(stringTable[n:], 0)
			}
			if end < 0 {
				end = len(stringTable) - n
			}
			name = string(stringTable[n : n+end])
		}
		headerToMember[off] = len(out.Members)
		out.Members = append(out.Members, Member{Name: name, Data: append([]byte(nil), body...)})
		off = next
	}
	for name, headerOff := range gnuIndex {
		if member, ok := headerToMember[headerOff]; ok {
			out.Symbols[name] = member
		}
	}
	/* Host archivers may emit an empty or partial index for an otherwise valid
	 * IE64 archive because they do not recognise the target ELF machine.  Scan
	 * every IE64 member to complete the index while retaining its normal
	 * first-definition ordering. */
	for i, m := range out.Members {
		obj, e := ie64obj.Parse(m.Data)
		if e != nil {
			continue
		}
		for _, s := range obj.Symbols {
			if s.Section != ie64obj.SHNUndef && s.Bind != ie64obj.STBLocal {
				if _, exists := out.Symbols[s.Name]; !exists {
					out.Symbols[s.Name] = i
				}
			}
		}
	}
	return out, nil
}
