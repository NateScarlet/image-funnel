package pagination

import (
	"slices"
	"testing"
)

// textItem 用游标本身代表顺序，便于断言分页边界
type textItem struct {
	name string
}

func textSeq(items []textItem) func(func(textItem, error) bool) {
	return func(yield func(textItem, error) bool) {
		for _, i := range items {
			if !yield(i, nil) {
				return
			}
		}
	}
}

func textCursor(i textItem) (string, error) { return i.name, nil }

type collectingWriter struct {
	cursors       []string
	hasNext       bool
	hasPrevious   bool
	writtenValues []string
}

func (w *collectingWriter) WriteHasPreviousPage(v bool) { w.hasPrevious = v }
func (w *collectingWriter) WriteHasNextPage(v bool)    { w.hasNext = v }
func (w *collectingWriter) Write(item textItem, cursor string) error {
	w.cursors = append(w.cursors, cursor)
	w.writtenValues = append(w.writtenValues, item.name)
	return nil
}
func (w *collectingWriter) Close() error { return nil }

func textItems(names ...string) []textItem {
	items := make([]textItem, len(names))
	for i, name := range names {
		items[i] = textItem{name}
	}
	return items
}

func TestByTextV2_FirstPage(t *testing.T) {
	w := &collectingWriter{}
	err := ByTextV2(textSeq(textItems("a", "b", "c")), textCursor, w, WithFirst(2))
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(w.writtenValues, []string{"a", "b"}) {
		t.Fatalf("unexpected page: %v", w.writtenValues)
	}
	if !w.hasNext {
		t.Fatal("expected next page")
	}
	if w.hasPrevious {
		t.Fatal("first page should not have previous page")
	}
}

// 游标之后的分页必须返回严格位于游标之后的项，跨页游标可直接字符串比较
func TestByTextV2_AfterStartsStrictlyAfterCursor(t *testing.T) {
	w := &collectingWriter{}
	err := ByTextV2(textSeq(textItems("a", "b", "c", "d")), textCursor, w, WithFirst(2), WithAfter("b"))
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(w.writtenValues, []string{"c", "d"}) {
		t.Fatalf("unexpected page: %v", w.writtenValues)
	}
	if !w.hasPrevious {
		t.Fatal("expected previous page after cursor")
	}
	if w.hasNext {
		t.Fatal("unexpected next page")
	}
}

func TestByTextV2_BeforeStopsBeforeCursor(t *testing.T) {
	w := &collectingWriter{}
	err := ByTextV2(textSeq(textItems("a", "b", "c", "d")), textCursor, w, WithFirst(10), WithBefore("c"))
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(w.writtenValues, []string{"a", "b"}) {
		t.Fatalf("unexpected page: %v", w.writtenValues)
	}
	if !w.hasNext {
		t.Fatal("expected next page before cursor")
	}
}

func TestByTextV2_LastKeepsTail(t *testing.T) {
	w := &collectingWriter{}
	err := ByTextV2(textSeq(textItems("a", "b", "c", "d")), textCursor, w, WithLast(2))
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(w.writtenValues, []string{"c", "d"}) {
		t.Fatalf("unexpected page: %v", w.writtenValues)
	}
	if !w.hasPrevious {
		t.Fatal("expected previous page")
	}
}

func TestByTextV2_AfterCanPageThroughAllItems(t *testing.T) {
	items := textItems("a", "b", "c", "d", "e")

	var got []string
	after := ""
	for {
		w := &collectingWriter{}
		if err := ByTextV2(textSeq(items), textCursor, w, WithFirst(2), WithAfter(after)); err != nil {
			t.Fatal(err)
		}
		got = append(got, w.writtenValues...)
		if !w.hasNext {
			break
		}
		if len(w.cursors) == 0 {
			t.Fatal("page without items cannot advance")
		}
		after = w.cursors[len(w.cursors)-1]
	}

	if !slices.Equal(got, []string{"a", "b", "c", "d", "e"}) {
		t.Fatalf("unexpected traversal: %v", got)
	}
}
