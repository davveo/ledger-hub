package application

import "time"

func BackoffDuration(attempt int, base, cap time.Duration) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if cap <= 0 {
		cap = 30 * time.Second
	}
	if attempt < 0 {
		attempt = 0
	}
	d := base
	for i := 0; i < attempt; i++ {
		if d > cap/2 {
			return cap
		}
		d = d * 2
	}
	if d > cap {
		return cap
	}
	return d
}
