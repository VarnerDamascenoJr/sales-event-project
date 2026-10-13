package analytics

import "time"

type salePriorRecord struct {
	CreatedAt time.Time
	Completed bool
	Demand    int
}
