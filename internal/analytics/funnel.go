package analytics

import (
	"sort"
	"time"
)

func BuildFunnelSegments(events []Event, specs []WindowSpec, options FunnelOptions) []FunnelSegment {
	normalizedSpecs := normalizeWindowSpecs(specs)
	normalizedOptions := normalizeFunnelOptions(options)
	records := buildFunnelRecords(events)
	if len(records) == 0 {
		return nil
	}

	segments := make(map[segmentKey]*funnelCounts)
	keys := make([]segmentKey, 0)
	for _, record := range records {
		if !record.Accepted {
			continue
		}
		for _, spec := range normalizedSpecs {
			start := record.AcceptedAt.Truncate(spec.Duration)
			key := segmentKey{
				SalesEventID: record.SalesEventID,
				TicketID:     record.TicketID,
				TicketType:   record.TicketType,
				Provider:     providerOrUnknown(record.Provider),
				WindowSize:   spec.Name,
				WindowStart:  start,
			}
			counts := segments[key]
			if counts == nil {
				counts = &funnelCounts{}
				segments[key] = counts
				keys = append(keys, key)
			}
			addRecordToFunnelCounts(counts, record)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		left := keys[i]
		right := keys[j]
		if !left.WindowStart.Equal(right.WindowStart) {
			return left.WindowStart.Before(right.WindowStart)
		}
		if left.WindowSize != right.WindowSize {
			return left.WindowSize < right.WindowSize
		}
		if left.SalesEventID != right.SalesEventID {
			return left.SalesEventID < right.SalesEventID
		}
		if left.TicketType != right.TicketType {
			return left.TicketType < right.TicketType
		}
		return left.Provider < right.Provider
	})

	segmentsOut := make([]FunnelSegment, 0, len(keys))
	for _, key := range keys {
		spec := windowSpecByName(normalizedSpecs, key.WindowSize)
		counts := segments[key]
		segmentsOut = append(segmentsOut, FunnelSegment{
			SalesEventID: key.SalesEventID,
			TicketID:     key.TicketID,
			TicketType:   key.TicketType,
			Provider:     key.Provider,
			WindowSize:   key.WindowSize,
			WindowStart:  key.WindowStart,
			WindowEnd:    key.WindowStart.Add(spec.Duration),
			Steps:        buildFunnelSteps(*counts, normalizedOptions),
		})
	}
	return segmentsOut
}

func normalizeFunnelOptions(options FunnelOptions) FunnelOptions {
	defaults := DefaultFunnelOptions()
	if !isFinite(options.ConfidenceLevel) || options.ConfidenceLevel == 0 {
		options.ConfidenceLevel = defaults.ConfidenceLevel
	}
	if !isFinite(options.WilsonZ) || options.WilsonZ == 0 {
		options.WilsonZ = defaults.WilsonZ
	}
	if !isFinite(options.PriorAlpha) || options.PriorAlpha <= 0 {
		options.PriorAlpha = defaults.PriorAlpha
	}
	if !isFinite(options.PriorBeta) || options.PriorBeta <= 0 {
		options.PriorBeta = defaults.PriorBeta
	}
	return options
}

func addRecordToFunnelCounts(counts *funnelCounts, record funnelRecord) {
	counts.accepted++
	if record.Pending {
		counts.pending++
	}
	if record.Paid {
		counts.paid++
	}
	if record.TicketIssued {
		counts.ticketIssued++
	}
	if record.CheckedIn {
		counts.checkedIn++
	}
}

func windowSpecByName(specs []WindowSpec, name string) WindowSpec {
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	return WindowSpec{Name: name, Duration: time.Minute}
}
