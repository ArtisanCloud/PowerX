// pkg/corex/flow/loader/resolver.go
package loader

import (
	"fmt"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"path/filepath"
)

// Resolver 负责：索引 -> 解析（单目录，无合并）
type Resolver struct {
	Dir    string
	Loader *YAMLSpecLoader   // 你已有
	index  map[string]string // flowID -> 绝对/相对路径
}

func NewResolver(dir string) *Resolver {
	return &Resolver{
		Dir:    dir,
		Loader: NewYAMLSpecLoader("."),
		index:  map[string]string{},
	}
}

// BuildIndex 扫描单一目录 Dir，建立 flowID -> 文件路径 的索引
func (r *Resolver) BuildIndex() error {
	m, err := r.Loader.LoadBlueprintsFromDir(r.Dir)
	if err != nil {
		return err
	}
	for id, bp := range m {
		path := bp.FlowPath
		if path == "" {
			// 兜底：文件名=flow_id.yaml（如不是该规则，请让 Loader 在 FlowPath 回填真实路径）
			path = filepath.Join(r.Dir, id+".yaml")
		}
		r.index[id] = path
	}
	return nil
}

// Resolve 仅按索引/兜底路径加载，不做 base/merge
func (r *Resolver) Resolve(flowID string) (*flowschema.Flow, error) {
	// 先走索引
	if path, ok := r.index[flowID]; ok {
		f, err := r.Loader.LoadFlowBlueprint(path)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	// 再按文件名兜底
	if f, err := r.Loader.LoadFlowBlueprint(filepath.Join(r.Dir, flowID+".yaml")); err == nil {
		return f, nil
	}
	return nil, fmt.Errorf("flow not found: %s", flowID)
}
