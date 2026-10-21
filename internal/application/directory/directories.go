package directory

import (
	"context"
	"fmt"
	"main/internal/domain/relpath"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
)

// directoryPageItem 分页项：目录 DTO 与它在服务端顺序中的位置游标
type directoryPageItem struct {
	dto    *shared.DirectoryDTO
	cursor string
}

// directoryCursor 返回目录在服务端顺序中的位置游标。
//
// 服务端顺序是文件系统遍历的目录名顺序，因此以目录名作为比较键：
// 目录增删不会让其他目录的位置平移，跨页的游标也可直接比较。
func directoryCursor(relPath relpath.Path) (string, error) {
	name := relPath.Base()
	if name == "" {
		return "", fmt.Errorf("directory %q has no name to build cursor from", relPath.String())
	}
	return pagination.TextToCursor(name), nil
}

// Directories 获取子目录列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Directories(
	ctx context.Context,
	parentID scalar.ID,
	filterBy shared.DirectoryFilters,
	first *int,
	after *string,
	last *int,
	before *string,
) (connection *shared.DirectoryConnectionDTO, err error) {
	parentDir, err := h.dirSvc.GetDirectory(ctx, parentID)
	if err != nil {
		return nil, err
	}

	builder := pagination.NewConnectionBufferBuilder[directoryPageItem, *shared.DirectoryEdgeDTO, *shared.DirectoryConnectionDTO]()
	buf := builder(
		func(item directoryPageItem, cursor string) (*shared.DirectoryEdgeDTO, error) {
			return &shared.DirectoryEdgeDTO{
				Node:   item.dto,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.DirectoryEdgeDTO, pageInfo pagination.PageInfo) (*shared.DirectoryConnectionDTO, error) {
			var nodes = make([]*shared.DirectoryDTO, len(edges))
			for i, edge := range edges {
				nodes[i] = edge.Node
			}
			return &shared.DirectoryConnectionDTO{
				Edges:    edges,
				Nodes:    nodes,
				PageInfo: &pageInfo,
			}, nil
		},
	)

	options := pagination.OptionFromInput(after, before, first, last)

	filteredSeq := func(yield func(directoryPageItem, error) bool) {
		dirFilter := h.filterBuilder.Build(filterBy)
		for dir, scanErr := range h.repo.Find(ctx, parentDir.RelPath()) {
			if scanErr != nil {
				if !yield(directoryPageItem{}, scanErr) {
					return
				}
				continue
			}
			if !dirFilter(dir) {
				continue
			}
			cursor, cursorErr := directoryCursor(dir.RelPath())
			if cursorErr != nil {
				if !yield(directoryPageItem{}, cursorErr) {
					return
				}
				continue
			}
			if !yield(directoryPageItem{dto: h.dtoFactory.New(dir), cursor: cursor}, nil) {
				return
			}
		}
	}

	// 目录必须按目录名分页：下标会随目录增删整体平移，导致跨页导航漂移
	err = pagination.ByTextV2(filteredSeq, func(item directoryPageItem) (string, error) {
		return item.cursor, nil
	}, buf, options...)
	if err != nil {
		return nil, err
	}

	return buf.Value()
}
