package http

import (
	"time"

	// Embeds the IANA tz database in the binary. The runtime image is alpine-based
	// and carries no tzdata, so without this LoadLocation("America/Lima") fails in
	// production and every caller of peruNow returns a 500.
	_ "time/tzdata"
)

// peruNow returns the current instant in Peru's wall clock.
func peruNow() (time.Time, error) {
	loc, err := time.LoadLocation("America/Lima")
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().In(loc), nil
}
