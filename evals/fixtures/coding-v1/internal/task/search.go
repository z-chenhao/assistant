package task

type Task struct{ Title, Status string }

func Search(status string) []Task {
	all := []Task{{"Build", "active"}, {"Review", "paused"}}
	result := []Task{}
	for _, t := range all {
		if status == "" || t.Status == status {
			result = append(result, t)
		}
	}
	return result
}
