package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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

// buildBaseEnv 构建所有钩子脚本共用的基础环境变量
func (r *Runner) buildBaseEnv(ctx context.Context, hookID, hookName, triggerName string, imageIDs, imagePaths []string, noteAbsPath string, customEnv map[string]string) ([]string, error) {
	imageIDsJSON := mustJSON(imageIDs)
	imagePathsJSON := mustJSON(imagePaths)

	var notePathsJSON string
	if noteAbsPath != "" {
		notePathsJSON = mustJSON([]string{noteAbsPath})
	} else {
		notePathsJSON = "[]"
	}

	// 从第一个可用路径推导目录信息
	var dirRel string
	if noteAbsPath != "" {
		dirRel = r.dirRelFromAbsPath(noteAbsPath)
	} else if len(imagePaths) > 0 {
		dirRel = r.dirRelFromAbsPath(imagePaths[0])
	}

	var dirID string
	if dirRel != "" {
		dir, err := r.dirRepo.Get(ctx, dirRel)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve directory ID for %q: %w", dirRel, err)
		}
		dirID = dir.ID().String()
	}

	env := append(os.Environ(),
		"IMAGE_FUNNEL_HOOK_ID="+hookID,
		"IMAGE_FUNNEL_HOOK_NAME="+hookName,
		"IMAGE_FUNNEL_TRIGGER="+triggerName,
		"IMAGE_FUNNEL_ROOT_DIR="+r.rootDir,
		"IMAGE_FUNNEL_DIRECTORY_ID="+dirID,
		"IMAGE_FUNNEL_DIRECTORY_REL_PATH="+dirRel,
		"IMAGE_FUNNEL_GRAPHQL_URL="+r.graphqlURL,
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
