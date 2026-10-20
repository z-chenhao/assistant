package run

import "testing"

func TestRootChild(t *testing.T) {
	root := Admit("one", "", 2)
	child := Admit(root.Task, "root", root.Epoch)
	if root.Task != "one" || root.Parent != "" || child.Parent != "root" || child.Task != root.Task || child.Epoch != 2 {
		t.Fatal("broken tree")
	}
}
