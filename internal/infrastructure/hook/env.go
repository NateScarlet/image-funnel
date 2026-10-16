package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"main/internal/domain/device"
	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/scalar"
)

// mustJSON 序列化值为 JSON，确定不可能出错时应 panic
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// envRequest 构造钩子进程基础环境变量的输入
type envRequest struct {
	HookID      string
	HookName    string
	TriggerName string
	ImageIDs    []string
	ImagePaths  []string
	NoteAbsPath string
	CustomEnv   map[string]string
	FallbackDir *directory.Directory
}

// envBuilder 钩子进程环境变量的构造者：注入统一的上下文变量、配置自定义变量与临时令牌。
// 目录信息优先从路径推导，推导不出时才回退到调用者给出的实体。
type envBuilder struct {
	rootDir     string
	dataDir     string
	graphqlURL  string
	tokenSource device.TokenSource
	dirRepo     directory.Repository
}

func (e *envBuilder) Build(ctx context.Context, req envRequest) ([]string, error) {
	notePathsJSON := "[]"
	if req.NoteAbsPath != "" {
		notePathsJSON = mustJSON([]string{req.NoteAbsPath})
	}

	dirID, dirRel := "", ""
	if req.FallbackDir != nil {
		dirID = req.FallbackDir.ID().String()
		dirRel = req.FallbackDir.RelPath().String()
	}

	// 从第一个可用路径推导目录信息，无法推导时使用调用者传入的回退值
	dirRelPath, hasDirRelPath := relpath.Path{}, false
	switch {
	case req.NoteAbsPath != "":
		rel, err := relFromAbsPath(e.rootDir, req.NoteAbsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory for %q: %w", req.NoteAbsPath, err)
		}
		dirRelPath, hasDirRelPath = rel.Dir(), true
	case len(req.ImagePaths) > 0:
		rel, err := relFromAbsPath(e.rootDir, req.ImagePaths[0])
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory for %q: %w", req.ImagePaths[0], err)
		}
		dirRelPath, hasDirRelPath = rel.Dir(), true
	case dirRel != "":
		rel, err := relpath.New(e.rootDir, dirRel)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory for %q: %w", dirRel, err)
		}
		dirRelPath, hasDirRelPath = rel, true
	}

	if hasDirRelPath {
		resolvedDir, err := e.dirRepo.Get(ctx, dirRelPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory ID for %q: %w", dirRelPath, err)
		}
		dirID = resolvedDir.ID().String()
		dirRel = dirRelPath.String()
	}

	env := append(os.Environ(),
		"IMAGE_FUNNEL_HOOK_ID="+req.HookID,
		"IMAGE_FUNNEL_HOOK_NAME="+req.HookName,
		"IMAGE_FUNNEL_TRIGGER="+req.TriggerName,
		"IMAGE_FUNNEL_ROOT_DIR="+e.rootDir,
		"IMAGE_FUNNEL_DIRECTORY_ID="+dirID,
		// 根目录的相对路径为空串（根相对路径的零值），与 GraphQL 中
		// Directory.relPath 的约定一致；脚本拼路径时应直接用 {ROOT_DIR}/{REL_PATH}
		"IMAGE_FUNNEL_DIRECTORY_REL_PATH="+dirRel,
		"IMAGE_FUNNEL_GRAPHQL_URL="+e.graphqlURL,
		"IMAGE_FUNNEL_DATA_DIR="+e.dataDir,
		"IMAGE_FUNNEL_IMAGE_IDS="+mustJSON(req.ImageIDs),
		"IMAGE_FUNNEL_IMAGE_PATHS="+mustJSON(req.ImagePaths),
		"IMAGE_FUNNEL_NOTE_PATHS="+notePathsJSON,
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
	)

	for k, v := range req.CustomEnv {
		env = append(env, k+"="+v)
	}

	// 签发临时的 JWT 令牌供外部脚本高频访问 GraphQL API 鉴权使用
	tok, err := e.tokenSource.NewAccessToken(ctx, scalar.ToID("hook-runner"))
	if err != nil {
		return nil, fmt.Errorf("failed to generate temporary token: %w", err)
	}
	env = append(env, "IMAGE_FUNNEL_TOKEN="+tok.String())

	return env, nil
}
