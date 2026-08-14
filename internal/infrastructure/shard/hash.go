package shard

import "hash/fnv"

func Index(holderID string, n int) int {
	if n <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(holderID))
	return int(h.Sum32() % uint32(n))
}
