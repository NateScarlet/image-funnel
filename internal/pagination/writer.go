package pagination

import (
	"io"
	"slices"
)

type Writer[T any] interface {
	WriteHasPreviousPage(v bool)
	WriteHasNextPage(v bool)
	Write(item T, cursor string) (err error)
	io.Closer
}

type reverseWriterEdge[T any] struct {
	node   T
	cursor string
}
type ReverseWriter[T any] struct {
	w     Writer[T]
	edges []reverseWriterEdge[T]
}

// Write implements ReverseWriter
func (w *ReverseWriter[T]) Write(item T, cursor string) (err error) {
	w.edges = append(w.edges, reverseWriterEdge[T]{item, cursor})
	return
}

// WriteHasNextPage implements ReverseWriter
func (w *ReverseWriter[T]) WriteHasNextPage(v bool) {
	w.w.WriteHasPreviousPage(v)
}

// WriteHasPreviousPage implements ReverseWriter
func (w *ReverseWriter[T]) WriteHasPreviousPage(v bool) {
	w.w.WriteHasNextPage(v)
}

// Close implements ReverseWriter
func (w *ReverseWriter[T]) Close() (err error) {
	for _, i := range slices.Backward(w.edges) {
		err = w.w.Write(i.node, i.cursor)
		if err != nil {
			return
		}
	}
	return w.w.Close()
}

func NewReverseWriter[T any](w Writer[T]) *ReverseWriter[T] {
	return &ReverseWriter[T]{w, nil}
}
