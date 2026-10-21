package run

import "testing"

func TestGrandchildCancel(t *testing.T) {
	tree := []Run{{ID: "root", Task: "one", Epoch: 1}, {ID: "child", Parent: "root", Task: "one", Epoch: 1}, {ID: "grandchild", Parent: "child", Task: "one", Epoch: 1}}
	Cancel(tree, "root")
	for _, r := range tree {
		if !r.Cancelled || Commit(r, 1) {
			t.Fatalf("late result from %s", r.ID)
		}
	}
}
func TestOldEpoch(t *testing.T) {
	if Commit(Run{Epoch: 2}, 1) {
		t.Fatal("stale result")
	}
}
