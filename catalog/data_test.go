package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
)

func TestCatalogDataMatchesRecordedReference(t *testing.T) {
	// R-DQ16-AQWE
	models := make([]string, 0, len(entries))
	for model := range entries {
		models = append(models, model)
	}
	sort.Strings(models)

	rows := make([]referenceRow, 0, len(models))
	for _, model := range models {
		entry := entries[model]
		row := referenceRow{Entry: entry}
		if entry.Reasoning != nil {
			switch {
			case entry.Reasoning.Default.IsUnset():
				row.ReasoningDefault = "unset"
			case entry.Reasoning.Default.Disabled():
				row.ReasoningDefault = "disabled"
			default:
				if level, ok := entry.Reasoning.Default.Level(); ok {
					row.ReasoningDefault = "level:" + level
				} else if budget, ok := entry.Reasoning.Default.Budget(); ok {
					row.ReasoningDefault = fmt.Sprintf("budget:%d", budget)
				}
			}
			if !entry.Reasoning.Accepts(entry.Reasoning.Default) {
				t.Errorf("%s reasoning default is not accepted by its own spec", model)
			}
		}
		rows = append(rows, row)
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	const recordedReference = "11dd7eee9eb3e80894d9c0f1f098e77567de2baac239c4fcb687a12dfeb53b81"
	if got := hex.EncodeToString(digest[:]); got != recordedReference {
		t.Fatalf("catalog data differs from recorded reference table: got %s, want %s", got, recordedReference)
	}
}

type referenceRow struct {
	Entry
	ReasoningDefault string
}
