package daemon

import (
	"time"
)

type ArchiveEntry struct {
	ID        int       "json:\"id\""
	Timestamp time.Time "json:\"timestamp\""
	Place     string    "json:\"place\""
	User      string    "json:\"user\""
}
