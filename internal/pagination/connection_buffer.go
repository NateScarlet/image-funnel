package pagination

import "slices"

type ConnectionBuffer[T any, TEdge any, TConnection any] interface {
	Writer[T]
	Value() (TConnection, error)
	Length() int
	Reverse()
}

type connectionBufferImpl[T any, TEdge any, TConnection any] struct {
	newEdge       func(node T, cursor string) (TEdge, error)
	newConnection func(edges []TEdge, pageInfo PageInfo) (TConnection, error)

	edges    []TEdge
	pageInfo PageInfo
}

// Reverse implements db.PaginationWriter
func (w *connectionBufferImpl[T, TEdge, TConnection]) Reverse() {
	slices.Reverse(w.edges)
	w.pageInfo.Reverse()
}

// WriteHasNextPage implements db.PaginationWriter
func (w *connectionBufferImpl[T, TEdge, TConnection]) WriteHasNextPage(v bool) {
	w.pageInfo.HasNextPage = v
}

// WriteHasPreviousPage implements db.PaginationWriter
func (w *connectionBufferImpl[T, TEdge, TConnection]) WriteHasPreviousPage(v bool) {
	w.pageInfo.HasPreviousPage = v
}

// WriteNode implements db.PaginationWriter
func (w *connectionBufferImpl[T, TEdge, TConnection]) Write(item T, cursor string) (err error) {
	edge, err := w.newEdge(item, cursor)
	if err != nil {
		return
	}
	w.edges = append(w.edges, edge)
	w.pageInfo.UpdateCursor(cursor)
	return
}

func (w *connectionBufferImpl[T, TEdge, TConnection]) Value() (_ TConnection, err error) {
	return w.newConnection(w.edges, w.pageInfo)
}

func (w *connectionBufferImpl[T, TEdge, TConnection]) Length() int {
	return len(w.edges)
}

func (connectionBufferImpl[T, TEdge, TConnection]) Close() error {
	return nil
}

func NewConnectionBufferBuilder[T any, TEdge any, TConnection any]() func(
	newEdge func(item T, cursor string) (TEdge, error),
	newConnection func(edges []TEdge, pageInfo PageInfo) (TConnection, error),
) ConnectionBuffer[T, TEdge, TConnection] {
	return func(newEdge func(item T, cursor string) (_ TEdge, err error), newConnection func(edges []TEdge, pageInfo PageInfo) (_ TConnection, err error)) ConnectionBuffer[T, TEdge, TConnection] {
		return &connectionBufferImpl[T, TEdge, TConnection]{
			newEdge:       newEdge,
			newConnection: newConnection,
		}
	}
}

// Deprecated: use [NewConnectionBufferBuilder] for better ide auto-completion
func NewConnectionBuffer[T any, TEdge any, TConnection any](
	newEdge func(item T, cursor string) (_ *TEdge, err error),
	newConnection func(edges []TEdge, pageInfo PageInfo) (_ *TConnection, err error),
) ConnectionBuffer[T, TEdge, *TConnection] {
	return &connectionBufferImpl[T, TEdge, *TConnection]{
		newEdge: func(node T, cursor string) (TEdge, error) {
			v, err := newEdge(node, cursor)
			if err != nil || v == nil {
				var zero TEdge
				return zero, err
			}
			return *v, nil
		},
		newConnection: newConnection,
	}
}
