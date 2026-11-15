package report

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/sassdavid/relgap/internal/classify"
)

// entryIDKey versions the input of entry IDs. Bump it when normalize or text extraction changes.
const entryIDKey = "relgap-entry/v1"

// EntryID returns the ID of an entry: e- and 8 hex digits of a hash over the tool, the version
// and the entry's normalized text, the same key that drops repeated entries.
func EntryID(tool string, e classify.Entry) string {
	sum := sha256.Sum256([]byte(entryIDKey + "\x00" + tool + "\x00" + e.Version + "\x00" + normalize(e.Text)))
	return "e-" + hex.EncodeToString(sum[:4])
}

// assignIDs sets every entry's ID before any filter runs, so an entry keeps its ID across
// pages, filters and runs. A repeated ID gets -2, -3 and so on, in entry order.
func assignIDs(tool string, entries []classify.Entry) {
	seen := map[string]int{}
	for i := range entries {
		id := EntryID(tool, entries[i])
		seen[id]++
		if n := seen[id]; n > 1 {
			id += "-" + strconv.Itoa(n)
		}
		entries[i].ID = id
	}
}
