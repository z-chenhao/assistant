package run

type Run struct {
	ID, Task, Parent string
	Epoch            int
	Cancelled        bool
}

func Cancel(tree []Run, parent string) {
	for i := range tree {
		if tree[i].ID == parent || tree[i].Parent == parent {
			tree[i].Cancelled = true
		}
	}
}

func Commit(r Run, epoch int) bool { return !r.Cancelled && r.Epoch == epoch }
