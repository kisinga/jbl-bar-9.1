package preview

import (
	"context"
	"jbl-bar-toolkit/remote/internal/server"
	"strings"
	"testing"
)

func TestFixtureReplayIsReadOnlyAndDetached(t *testing.T) {
	b, err := New(strings.NewReader(`{"available":true,"power":"On","volume":7,"extensions":{"research.feature":{"observed":1}}}`))
	if err != nil {
		t.Fatal(err)
	}
	copy := b.Snapshot()
	copy["volume"] = 100
	copy["extensions"].(map[string]any)["research.feature"].(map[string]any)["observed"] = 999
	if b.Snapshot()["volume"] != float64(7) {
		t.Fatal("snapshot changed live state")
	}
	if _, err = b.Execute(context.Background(), server.Command{Action: "volume_up"}); err == nil {
		t.Fatal("fixture allowed controls")
	}
	if len(b.Features().Actions) != 0 {
		t.Fatal("fixture advertised controls")
	}
	if b.Snapshot()["extensions"].(map[string]any)["research.feature"].(map[string]any)["observed"] != float64(1) {
		t.Fatal("nested snapshot data was shared")
	}
	if _, err = New(strings.NewReader(`{} {}`)); err == nil {
		t.Fatal("trailing fixture data accepted")
	}
	if _, err = New(strings.NewReader(`null`)); err == nil {
		t.Fatal("null fixture accepted")
	}
	if _, err = New(strings.NewReader(`{}` + strings.Repeat(" ", 65536))); err == nil {
		t.Fatal("oversized fixture accepted")
	}
	b, err = New(strings.NewReader(`{"power":"Unknown"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Snapshot()["volume"]; ok {
		t.Fatal("missing fixture state was invented")
	}
}
