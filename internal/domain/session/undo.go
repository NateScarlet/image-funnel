package session

import (
	"context"
	"main/internal/scalar"
)

// #region Session Methods

// CanUndo 判断会话是否可以执行撤销操作
//
// 撤销条件：
// 1. 撤销栈不为空
func (s *Session) CanUndo() bool {
	return len(s.undoStack) > 0
}

// Undo 撤销上一次图片标记操作，恢复到之前的状态
func (s *Session) Undo() error {
	if len(s.undoStack) == 0 {
		return ErrNothingToUndo
	}

	// 执行撤销函数
	lastFunc := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]
	lastFunc()

	return nil
}

// #endregion

// Undo 撤销操作并保存
func (s *Service) Undo(ctx context.Context, sessionID scalar.ID) error {
	sess, release, err := s.sessionRepo.Acquire(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()

	if err := sess.Undo(); err != nil {
		return err
	}

	s.sessionSaved.Publish(ctx, sess)
	return nil
}
