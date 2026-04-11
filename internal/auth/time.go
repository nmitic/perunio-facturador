package auth

import "time"

func unixToTime(unix int64) time.Time {
	return time.Unix(unix, 0)
}
