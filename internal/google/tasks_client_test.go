package google

import "testing"

func TestTaskPagesRejectIncompleteOrInvalidSnapshots(t *testing.T) {
	for _, data := range []string{"", `{}`, `null`, `{"error":{"code":401}}`, `{"kind":"tasks#tasks","nextPageToken":"more","items":[]}`} {
		if _, err := decodeTaskPages([]byte(data)); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	data := []byte("{\"kind\":\"tasks#tasks\",\"nextPageToken\":\"more\",\"items\":[{\"id\":\"a\"}]}\n{\"kind\":\"tasks#tasks\",\"items\":[{\"id\":\"b\"}]}\n")
	tasks, err := decodeTaskPages(data)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("tasks=%v err=%v", tasks, err)
	}
	tasks, err = decodeTaskPages([]byte(`{"kind":"tasks#tasks"}`))
	if err != nil || tasks == nil || len(tasks) != 0 {
		t.Fatalf("empty=%v err=%v", tasks, err)
	}
}
