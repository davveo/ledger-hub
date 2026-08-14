package application

import "testing"

func TestExpectedToAmount(t *testing.T) {
	got, err := ExpectedToAmount(10000, 2, 2, "0.14000000")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1400 {
		t.Fatalf("got %d want 1400", got)
	}
	if !WithinTolerance(1400, 1400, 0) || WithinTolerance(1400, 1402, 1) || !WithinTolerance(1400, 1401, 1) {
		t.Fatal("tolerance")
	}
}
