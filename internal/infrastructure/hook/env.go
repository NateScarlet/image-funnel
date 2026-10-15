package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"main/internal/domain/relpath"
	"main/internal/scalar"
)

// #region 环境变量辅助函数

// mustJSON 序列化值为 JSON，确定不可能出错时应 panic
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// #endregion

// buildBaseEnv 构建所有钩子脚本共用的基础环境变量
// dirID 和 dirRel 作为回退值：当无法从路径推导目录信息时使用（例如 post_commit_session 触发器）
func (r *Runner) buildBaseEnv(ctx context.Context, hookID, hookName, triggerName string, imageIDs, imagePaths []string, noteAbsPath string, customEnv map[string]string, dirID string, dirRel string) ([]string, error) {
	imageIDsJSON := mustJSON(imageIDs)
	imagePathsJSON := mustJSON(imagePaths)

	var notePathsJSON string
	if noteAbsPath != "" {
		notePathsJSON = mustJSON([]string{noteAbsPath})
	} else {
		notePathsJSON = "[]"
	}

	// 从第一个可用路径推导目录信息，无法推导时使用调用者传入的回退值
	dirRelPath, hasDirRelPath := relpath.Path{}, false
	switch {
	case noteAbsPath != "":
		rel, err := r.relFromAbsPath(noteAbsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory for %q: %w", noteAbsPath, err)
		}
		dirRelPath, hasDirRelPath = rel.Dir(), true
	case len(imagePaths) > 0:
		rel, err := r.relFromAbsPath(imagePaths[0])
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory for %q: %w", imagePaths[0], err)
		}
		dirRelPath, hasDirRelPath = rel.Dir(), true
	case dirRel != "":
		var err error
		dirRelPath, err = relpath.New(r.rootDir, dirRel)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory for %q: %w", dirRel, err)
		}
		hasDirRelPath = true
	}

	if hasDirRelPath {
		resolvedDir, err := r.dirRepo.Get(ctx, dirRelPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory ID for %q: %w", dirRelPath, err)
		}
		dirID = resolvedDir.ID().String()
		dirRel = dirRelPath.String()
	}

	env := append(os.Environ(),
		"IMAGE_FUNNEL_HOOK_ID="+hookID,
		"IMAGE_FUNNEL_HOOK_NAME="+hookName,
		"IMAGE_FUNNEL_TRIGGER="+triggerName,
		"IMAGE_FUNNEL_ROOT_DIR="+r.rootDir,
		"IMAGE_FUNNEL_DIRECTORY_ID="+dirID,
		// 根目录的相对路径为空串（根相对路径的零值），与 GraphQL 中
		// Directory.relPath 的约定一致；脚本拼路径时应直接用 {ROOT_DIR}/{REL_PATH}
		"IMAGE_FUNNEL_DIRECTORY_REL_PATH="+dirRel,
		"IMAGE_FUNNEL_GRAPHQL_URL="+r.graphqlURL,
		"IMAGE_FUNNEL_DATA_DIR="+r.dataDir,
		"IMAGE_FUNNEL_IMAGE_IDS="+imageIDsJSON,
		"IMAGE_FUNNEL_IMAGE_PATHS="+imagePathsJSON,
		"IMAGE_FUNNEL_NOTE_PATHS="+notePathsJSON,
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
	)

	for k, v := range customEnv {
		env = append(env, k+"="+v)
	}

	// 签发临时的 JWT 令牌供外部脚本高频访问 GraphQL API 鉴权使用
	tok, err := r.tokenSource.NewAccessToken(ctx, scalar.ToID("hook-runner"))
	if err != nil {
		return nil, fmt.Errorf("failed to generate temporary token: %w", err)
	}
	env = append(env, "IMAGE_FUNNEL_TOKEN="+tok.String())

	return env, nil
}
