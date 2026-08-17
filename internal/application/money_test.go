package application

import "testing"

func TestFormatAmount(t *testing.T) {
	if got := FormatAmount(1049900, 2); got != "10499.00" {
		t.Fatalf("got %s", got)
	}
	if got := FormatAmount(7150, 0); got != "7150" {
		t.Fatalf("got %s", got)
	}
	if got := FormatAmount(-10, 2); got != "-0.10" {
		t.Fatalf("got %s", got)
	}
}
