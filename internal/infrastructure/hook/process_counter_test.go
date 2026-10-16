package hook

import (
	"context"
	"sync"
)

// countingProcessRunner 进程原语装饰器：记录常驻进程的 spawn 与 kill 次数，其余行为透传给内层实现。
// 让「超时杀进程 → 下次请求重新 spawn」「应用退出 → 常驻进程被终止」这类断言
// 无需依赖真实进程的冷启动速度，也无需观察操作系统的进程表。
type countingProcessRunner struct {
	inner processRunner

	mu     sync.Mutex
	spawns int
	kills  int
}

func (c *countingProcessRunner) Run(ctx context.Context, argv []string, env []string, dir string) (processOutput, error) {
	return c.inner.Run(ctx, argv, env, dir)
}

func (c *countingProcessRunner) SpawnResident(ctx context.Context, argv []string, env []string, dir string) (residentProcess, error) {
	proc, err := c.inner.SpawnResident(ctx, argv, env, dir)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.spawns++
	c.mu.Unlock()
	return &countingResidentProcess{residentProcess: proc, owner: c}, nil
}

func (c *countingProcessRunner) residentSpawns() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.spawns
}

func (c *countingProcessRunner) residentKills() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.kills
}

func (c *countingProcessRunner) recordKill() {
	c.mu.Lock()
	c.kills++
	c.mu.Unlock()
}

// countingResidentProcess 常驻进程句柄装饰器：只统计被终止的次数
type countingResidentProcess struct {
	residentProcess
	owner *countingProcessRunner
}

func (p *countingResidentProcess) Kill() {
	p.owner.recordKill()
	p.residentProcess.Kill()
}
