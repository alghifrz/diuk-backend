package reservation

import (
	"strings"
	"unicode"
)

// MatchPreferredTable picks the best available table for free-text guest
// preferences like "Indoor, Meja 4" or "Outdoor Table 2".
func MatchPreferredTable(notes string, tables []TableSummary) *TableSummary {
	n := normalizePreference(notes)
	if n == "" || len(tables) == 0 {
		return nil
	}

	var best *TableSummary
	bestScore := 0
	for i := range tables {
		t := &tables[i]
		score := preferenceScore(n, t.Name, t.AreaName)
		if score > bestScore {
			bestScore = score
			best = t
		}
	}
	if bestScore < 2 {
		return nil
	}
	return best
}

// MatchPreferredTableRecord is the Store/auto-assign variant of MatchPreferredTable.
func MatchPreferredTableRecord(notes string, tables []TableRecord) *TableRecord {
	n := normalizePreference(notes)
	if n == "" || len(tables) == 0 {
		return nil
	}

	var best *TableRecord
	bestScore := 0
	for i := range tables {
		t := &tables[i]
		score := preferenceScore(n, t.Name, t.AreaName)
		if score > bestScore {
			bestScore = score
			best = t
		}
	}
	if bestScore < 2 {
		return nil
	}
	return best
}

func preferenceScore(notes, tableName, areaName string) int {
	name := normalizePreference(tableName)
	area := normalizePreference(areaName)
	score := 0
	if name != "" && strings.Contains(notes, name) {
		score += 2
	}
	if area != "" && strings.Contains(notes, area) {
		score += 1
	}
	// Number-only fallback: notes mention "4" and table is "Meja 4" / "Table 4".
	if score < 2 {
		if num := trailingNumber(name); num != "" && containsWord(notes, num) {
			if area == "" || strings.Contains(notes, area) {
				score = 2
				if area != "" && strings.Contains(notes, area) {
					score = 3
				}
			}
		}
	}
	return score
}

func normalizePreference(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		default:
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func trailingNumber(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	last := fields[len(fields)-1]
	for _, r := range last {
		if !unicode.IsDigit(r) {
			return ""
		}
	}
	return last
}

func containsWord(haystack, word string) bool {
	for _, part := range strings.Fields(haystack) {
		if part == word {
			return true
		}
	}
	return false
}
