// Package evidence defines the records that acceptance suites write for
// living-documentation examples and that the livingdoc checker reads.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

const (
	StatePassed  = "passed"
	StateFailed  = "failed"
	StateSkipped = "skipped"
)

// Exchange is one HTTP request and its response as observed by the test client.
type Exchange struct {
	Method         string      `json:"method"`
	Target         string      `json:"target"`
	RequestHeader  http.Header `json:"request_header,omitempty"`
	RequestBody    string      `json:"request_body,omitempty"`
	Status         int         `json:"status"`
	ResponseHeader http.Header `json:"response_header,omitempty"`
	ResponseBody   string      `json:"response_body,omitempty"`
}

// Record is the outcome of one acceptance scenario bound to an example.
type Record struct {
	Example   string     `json:"example"`
	Spec      string     `json:"spec"`
	Location  string     `json:"location"`
	State     string     `json:"state"`
	Exchanges []Exchange `json:"exchanges,omitempty"`
}

// Write stores the record in dir under a name derived from its example and scenario.
func Write(dir string, record Record) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create evidence directory: %w", err)
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evidence for %q: %w", record.Example, err)
	}
	sum := sha256.Sum256([]byte(record.Example + "\x00" + record.Spec + "\x00" + record.Location))
	name := hex.EncodeToString(sum[:8]) + ".json"
	if err := os.WriteFile(filepath.Join(dir, name), encoded, 0o600); err != nil {
		return fmt.Errorf("write evidence for %q: %w", record.Example, err)
	}
	return nil
}

// ReadDir loads every record in dir, ordered by example and scenario.
func ReadDir(dir string) ([]Record, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("list evidence: %w", err)
	}
	records := make([]Record, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path) // #nosec G304 -- operator-supplied evidence directory
		if err != nil {
			return nil, fmt.Errorf("read evidence: %w", err)
		}
		var record Record
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, fmt.Errorf("decode evidence %s: %w", path, err)
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Example != records[j].Example {
			return records[i].Example < records[j].Example
		}
		return records[i].Spec < records[j].Spec
	})
	return records, nil
}
