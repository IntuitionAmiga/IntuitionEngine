package main

func mappingCount(bus *MachineBus, start, end uint32) int {
	count := 0
	type key struct {
		start uint32
		end   uint32
	}
	seen := map[key]bool{}
	for page := start & PAGE_MASK; page <= end&PAGE_MASK; page += PAGE_SIZE {
		for _, region := range bus.mapping[page] {
			k := key{start: region.start, end: region.end}
			if seen[k] {
				continue
			}
			if region.end >= start && region.start <= end {
				count++
				seen[k] = true
			}
		}
	}
	return count
}
