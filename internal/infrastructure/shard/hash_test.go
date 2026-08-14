package shard

import "testing"

func TestIndexStable(t *testing.T) {
	if Index("u_1", 1) != 0 {
		t.Fatal("single shard")
	}
	a := Index("holder-a", 4)
	b := Index("holder-a", 4)
	if a != b {
		t.Fatal("hash should be stable")
	}
	if Index("x", 8) < 0 || Index("x", 8) >= 8 {
		t.Fatal("out of range")
	}
}
