package evidence

import (
	"testing"
)

func TestForTest_RecordsOutcomeWhenTheTestEnds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EvidenceDirEnv, dir)
	t.Run("passing", func(t *testing.T) { ForTest(t, "configuration.precedence") })
	t.Run("skipped", func(t *testing.T) {
		ForTest(t, "deployment.chart")
		t.Skip("tool not installed")
	})
	records, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, record := range records {
		got[record.Example] = record.State
		if record.Location == "" || record.Spec == "" {
			t.Errorf("incomplete record: %+v", record)
		}
	}
	if got["configuration.precedence"] != StatePassed || got["deployment.chart"] != StateSkipped {
		t.Errorf("got %v", got)
	}
}

func TestForTest_DoesNothingWithoutEvidenceDirectory(t *testing.T) {
	t.Setenv(EvidenceDirEnv, "")
	ForTest(t, "configuration.precedence")
}
