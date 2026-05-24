package inmem

import (
	"context"
	"testing"
	"time"

	"main/internal/domain/image"
	"main/internal/domain/session"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSessionRepository(t *testing.T) {
	repo := NewSessionRepository()
	assert.NotNil(t, repo)
	assert.NotNil(t, repo.sessions)
}

func TestSave(t *testing.T) {
	repo := NewSessionRepository()

	img := image.New(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", scalar.ToID("test-dir"), 1000, time.Now(), nil, 1920, 1080)
	sess := session.New(scalar.ToID("test-id"), scalar.ToID("test-dir"), &shared.ImageFilters{Rating: nil}, 5, []*image.Image{img})
	release, err := repo.Create(sess)
	require.NoError(t, err)
	release() // 先释放 Create 的锁

	_, release2, err := repo.Acquire(context.Background(), sess.ID())
	require.NoError(t, err)
	release2()
}

func TestAcquire(t *testing.T) {
	repo := NewSessionRepository()

	img := image.New(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", scalar.ToID("test-dir"), 1000, time.Now(), nil, 1920, 1080)
	sess := session.New(scalar.ToID("test-id"), scalar.ToID("test-dir"), &shared.ImageFilters{Rating: nil}, 5, []*image.Image{img})
	release1, err := repo.Create(sess)
	require.NoError(t, err)
	release1() // 先释放 Create 的锁

	found, release, err := repo.Acquire(context.Background(), sess.ID())
	require.NoError(t, err)
	assert.Equal(t, sess.ID(), found.ID())
	release()
}

func TestAcquire_NotFound(t *testing.T) {
	repo := NewSessionRepository()

	_, _, err := repo.Acquire(context.Background(), scalar.ToID("non-existent-id"))
	require.Error(t, err)
}

func TestCleanup_SyncDirIndex(t *testing.T) {
	// 备份并恢复原始值
	oldMin := minRetainedSessions
	oldMax := maxSessionIdleTime
	defer func() {
		minRetainedSessions = oldMin
		maxSessionIdleTime = oldMax
	}()

	// 设置保留数量为 1，负的最大空闲时间强制过期
	minRetainedSessions = 1
	maxSessionIdleTime = -time.Hour

	repo := NewSessionRepository()
	dirID := scalar.ToID("dir-1")

	// 创建会话辅助函数
	createSession := func(id string) scalar.ID {
		sid := scalar.ToID(id)
		sess := session.New(sid, dirID, &shared.ImageFilters{}, 0, []*image.Image{})
		release, err := repo.Create(sess)
		require.NoError(t, err)
		release()
		return sid
	}

	// 1. 创建第一个会话
	id1 := createSession("session-1")
	assert.Len(t, repo.sessions, 1)

	// 2. 创建第二个会话，会触发对 id1 的清理（因为 minRetainedSessions=1）
	id2 := createSession("session-2")

	// 验证 id1 是否已被清理（id2 正在创建时 id1 应该是空闲的）
	assert.Len(t, repo.sessions, 1, "should have only 1 session after cleanup")
	assert.Contains(t, repo.sessions, id2)
	assert.NotContains(t, repo.sessions, id1)

	// 3. 核心验证：FindByDirectory 也不应该返回 id1
	var foundIDs []scalar.ID
	for id, err := range repo.FindByDirectory(dirID) {
		require.NoError(t, err)
		foundIDs = append(foundIDs, id)
	}
	assert.Equal(t, []scalar.ID{id2}, foundIDs, "dirIndex should be in sync with sessions")

	// 4. 创建第三个会话，清理 id2
	id3 := createSession("session-3")
	assert.Len(t, repo.sessions, 1)
	assert.Contains(t, repo.sessions, id3)

	foundIDs = nil
	for id, err := range repo.FindByDirectory(dirID) {
		require.NoError(t, err)
		foundIDs = append(foundIDs, id)
	}
	assert.Equal(t, []scalar.ID{id3}, foundIDs)
}

func TestLastSession(t *testing.T) {
	repo := NewSessionRepository()
	dirID := scalar.ToID("dir-1")

	// 1. 无会话情况
	sess, release, err := repo.LastSession(context.Background(), dirID)
	require.NoError(t, err)
	assert.Nil(t, sess)
	assert.Nil(t, release)

	// 2. 创建第一个会话
	sess1 := session.New(scalar.ToID("s1"), dirID, &shared.ImageFilters{}, 0, []*image.Image{})
	release1, err := repo.Create(sess1)
	require.NoError(t, err)
	release1()

	// 等待一小会儿确保 UpdatedAt 有差距
	time.Sleep(10 * time.Millisecond)

	// 3. 创建第二个会话
	sess2 := session.New(scalar.ToID("s2"), dirID, &shared.ImageFilters{}, 0, []*image.Image{})
	release2, err := repo.Create(sess2)
	require.NoError(t, err)
	release2()

	// 验证获取最后会话（因为 sess2 是后创建/更新的，它应该是 latest）
	latestSess, releaseLatest, err := repo.LastSession(context.Background(), dirID)
	require.NoError(t, err)
	defer releaseLatest()
	assert.Equal(t, sess2.ID(), latestSess.ID())
}
