package usage

import "time"

type planJSON struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type periodJSON struct {
	Type     string `json:"type"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

type featureJSON struct {
	Feature   string `json:"feature"`
	Used      int64  `json:"used"`
	Limit     *int   `json:"limit"`
	Remaining *int64 `json:"remaining"`
}

type snapshotJSON struct {
	Plan   planJSON      `json:"plan"`
	Period periodJSON    `json:"period"`
	Usage  []featureJSON `json:"usage"`
}

type historyPeriodJSON struct {
	Type     string        `json:"type"`
	Start    string        `json:"start"`
	End      string        `json:"end"`
	Timezone string        `json:"timezone"`
	Usage    []featureJSON `json:"usage"`
}

type historyJSON struct {
	Plan   planJSON            `json:"plan"`
	Items  []historyPeriodJSON `json:"items"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

func snapshotResponse(s Snapshot) snapshotJSON {
	return snapshotJSON{
		Plan:   planJSON{Code: s.Plan.Code, Name: s.Plan.Name},
		Period: periodResponse(s.Period),
		Usage:  featuresResponse(s.Usage),
	}
}

func historyResponse(h History) historyJSON {
	items := make([]historyPeriodJSON, 0, len(h.Items))
	for _, item := range h.Items {
		items = append(items, historyPeriodJSON{
			Type:     item.Period.PeriodType,
			Start:    formatPeriodTime(item.Period.PeriodStart, item.Period.Timezone),
			End:      formatPeriodTime(item.Period.PeriodEnd, item.Period.Timezone),
			Timezone: item.Period.Timezone,
			Usage:    featuresResponse(item.Usage),
		})
	}
	return historyJSON{
		Plan:   planJSON{Code: h.Plan.Code, Name: h.Plan.Name},
		Items:  items,
		Limit:  h.Limit,
		Offset: h.Offset,
	}
}

func periodResponse(p Period) periodJSON {
	return periodJSON{
		Type:     p.PeriodType,
		Start:    formatPeriodTime(p.PeriodStart, p.Timezone),
		End:      formatPeriodTime(p.PeriodEnd, p.Timezone),
		Timezone: p.Timezone,
	}
}

func featuresResponse(items []FeatureUsage) []featureJSON {
	out := make([]featureJSON, 0, len(items))
	for _, item := range items {
		out = append(out, featureJSON{
			Feature:   item.Feature.String(),
			Used:      item.Used,
			Limit:     item.Limit,
			Remaining: item.Remaining,
		})
	}
	return out
}

func formatPeriodTime(ts time.Time, tzName string) string {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return ts.UTC().Format(time.RFC3339)
	}
	return ts.In(loc).Format(time.RFC3339)
}
