package loader

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/services/mcp/errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// pkg/corex/loader/spec_loader.go

// SpecLoader 规范加载器接口
type SpecLoader interface {
	LoadToolSpec(path string) (*schemas.ToolSpec, error)
	LoadFlowBlueprint(path string) (*schemas.Flow, error)
	LoadToolSpecsFromDir(dir string) (map[string]*schemas.ToolSpec, error)
	LoadBlueprintsFromDir(dir string) (map[string]*schemas.Flow, error)
}

// YAMLSpecLoader YAML规范加载器
type YAMLSpecLoader struct {
	basePath string
}

// NewYAMLSpecLoader 创建YAML规范加载器
func NewYAMLSpecLoader(basePath string) *YAMLSpecLoader {
	return &YAMLSpecLoader{
		basePath: basePath,
	}
}

// LoadToolSpec 加载工具规范
func (l *YAMLSpecLoader) LoadToolSpec(path string) (*schemas.ToolSpec, error) {
	fullPath := l.resolvePath(path)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, errors.NotFoundf("Failed to read tool spec file '%s': %v", fullPath, err)
	}

	var spec schemas.ToolSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errors.InvalidArgumentf("Failed to parse tool spec YAML '%s': %v", fullPath, err)
	}

	// 验证规范
	if err := l.validateToolSpec(&spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

// LoadFlowBlueprint 加载流程蓝图
func (l *YAMLSpecLoader) LoadFlowBlueprint(path string) (*schemas.Flow, error) {
	fullPath := l.resolvePath(path)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, errors.NotFoundf("Failed to read blueprint file '%s': %v", fullPath, err)
	}

	var bp schemas.Flow
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, errors.InvalidArgumentf("Failed to parse blueprint YAML '%s': %v", fullPath, err)
	}
	bp.FlowPath = fullPath

	// 仅支持顶层 ref；如果有人把 ref 写进 metadata.ref，直接报错（避免双规范）
	if bp.Metadata != nil {
		// 如果你之前把 FlowMetadata 里删掉了 Ref 字段，这里天然不会命中；留这句仅防将来误加
		type metaProbe struct {
			Ref any `yaml:"ref"`
		}
		var probe metaProbe
		if err := yaml.Unmarshal(data, &probe); err == nil && probe.Ref != nil {
			return nil, errors.InvalidArgumentf("metadata.ref is not supported in '%s'; use top-level 'ref' instead", fullPath)
		}
	}

	// ref-only：启用且有 target 时，允许没有 nodes/name
	if bp.Ref != nil && bp.Ref.Enable && strings.TrimSpace(bp.Ref.Target) != "" {
		if bp.FlowID == "" {
			return nil, errors.InvalidArgumentf("ref-only blueprint '%s' missing flow_id", fullPath)
		}
		return &bp, nil
	}

	// 非 ref-only：走常规校验
	if err := l.validateFlowBlueprint(&bp); err != nil {
		return nil, err
	}
	return &bp, nil
}

// LoadToolSpecsFromDir 从目录加载所有工具规范
func (l *YAMLSpecLoader) LoadToolSpecsFromDir(dir string) (map[string]*schemas.ToolSpec, error) {
	fullDir := l.resolvePath(dir)
	result := make(map[string]*schemas.ToolSpec)

	err := filepath.WalkDir(fullDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 只处理YAML文件
		if d.IsDir() || !utils.IsYAML(path) {
			return nil
		}

		// 加载工具规范
		spec, loadErr := l.LoadToolSpec(path)
		if loadErr != nil {
			// 记录错误但继续处理其他文件
			fmt.Printf("Warning: Failed to load tool spec from '%s': %v\n", path, loadErr)
			return nil
		}

		result[spec.ID] = spec
		return nil
	})

	if err != nil {
		return nil, errors.Internalf("Failed to scan tool specs directory '%s': %v", fullDir, err)
	}

	return result, nil
}

// LoadBlueprintsFromDir 从目录加载所有流程蓝图
func (l *YAMLSpecLoader) LoadBlueprintsFromDir(dir string) (map[string]*schemas.Flow, error) {
	fullDir := l.resolvePath(dir)
	result := make(map[string]*schemas.Flow)

	err := filepath.WalkDir(fullDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 只处理YAML文件
		if d.IsDir() || !utils.IsYAML(path) {
			return nil
		}

		// 加载蓝图
		blueprint, loadErr := l.LoadFlowBlueprint(path)
		if loadErr != nil {
			// 记录错误但继续处理其他文件
			fmt.Printf("Warning: Failed to load blueprint from '%s': %v\n", path, loadErr)
			return nil
		}
		//ctx := context.Background()
		//logger.InfoF(ctx, "agent load blueprint files : %s", path)

		result[blueprint.FlowID] = blueprint
		return nil
	})

	if err != nil {
		return nil, errors.Internalf("Failed to scan blueprints directory '%s': %v", fullDir, err)
	}

	return result, nil
}

// resolvePath 解析路径
func (l *YAMLSpecLoader) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(l.basePath, path)
}

// validateToolSpec 验证工具规范
func (l *YAMLSpecLoader) validateToolSpec(spec *schemas.ToolSpec) error {
	validation := errors.NewValidationError()

	if spec.ID == "" {
		validation.AddRequired("id")
	}
	if spec.Name == "" {
		validation.AddRequired("name")
	}
	if spec.Version == "" {
		validation.AddRequired("version")
	}
	if spec.InputSchema == nil {
		validation.AddRequired("input_schema")
	} else {
		if err := l.validateJSONSchema(spec.InputSchema, "input_schema"); err != nil {
			validation.AddInvalid("input_schema", err.Error())
		}
	}

	if spec.OutputSchema != nil {
		if err := l.validateJSONSchema(spec.OutputSchema, "output_schema"); err != nil {
			validation.AddInvalid("output_schema", err.Error())
		}
	}

	return validation.ToError()
}

// validateFlowBlueprint 验证流程蓝图
func (l *YAMLSpecLoader) validateFlowBlueprint(bp *schemas.Flow) error {
	validation := errors.NewValidationError()

	if bp.FlowID == "" {
		validation.AddRequired("flow_id")
	}
	// name 可选：有些纯内部流不需要
	if len(bp.Nodes) == 0 {
		validation.AddRequired("nodes")
	}
	for i, n := range bp.Nodes {
		id := strings.TrimSpace(n.ID)
		kind := strings.TrimSpace(n.Kind)
		use := strings.TrimSpace(n.Use)
		if id == "" && kind == "" && use == "" {
			validation.AddInvalid(fmt.Sprintf("nodes[%d]", i), "one of id/kind/use must be present")
		}
	}
	return validation.ToError()
}

// validateJSONSchema 验证JSON Schema
func (l *YAMLSpecLoader) validateJSONSchema(schema *schemas.JSONSchema, fieldPath string) error {
	if schema.Type == "" {
		return fmt.Errorf("%s.type is required", fieldPath)
	}

	// 验证类型值
	validTypes := []string{"object", "array", "string", "number", "integer", "boolean", "null"}
	validType := false
	for _, validT := range validTypes {
		if schema.Type == validT {
			validType = true
			break
		}
	}
	if !validType {
		return fmt.Errorf("%s.type must be one of: %s", fieldPath, strings.Join(validTypes, ", "))
	}

	// 递归验证属性
	if schema.Type == "object" && schema.Properties != nil {
		for propName, propSchema := range schema.Properties {
			if err := l.validateJSONSchema(propSchema, fmt.Sprintf("%s.properties.%s", fieldPath, propName)); err != nil {
				return err
			}
		}
	}

	// 验证数组项
	if schema.Type == "array" && schema.Items != nil {
		if err := l.validateJSONSchema(schema.Items, fmt.Sprintf("%s.items", fieldPath)); err != nil {
			return err
		}
	}

	return nil
}

// CachedSpecLoader 带缓存的规范加载器
type CachedSpecLoader struct {
	loader    SpecLoader
	toolCache map[string]*schemas.ToolSpec
	bpCache   map[string]*schemas.Flow
}

// NewCachedSpecLoader 创建带缓存的规范加载器
func NewCachedSpecLoader(loader SpecLoader) *CachedSpecLoader {
	return &CachedSpecLoader{
		loader:    loader,
		toolCache: make(map[string]*schemas.ToolSpec),
		bpCache:   make(map[string]*schemas.Flow),
	}
}

// LoadToolSpec 加载工具规范（带缓存）
func (c *CachedSpecLoader) LoadToolSpec(path string) (*schemas.ToolSpec, error) {
	if spec, exists := c.toolCache[path]; exists {
		return spec, nil
	}

	spec, err := c.loader.LoadToolSpec(path)
	if err != nil {
		return nil, err
	}

	c.toolCache[path] = spec
	return spec, nil
}

// LoadFlowBlueprint 加载流程蓝图（带缓存）
func (c *CachedSpecLoader) LoadFlowBlueprint(path string) (*schemas.Flow, error) {
	if bp, exists := c.bpCache[path]; exists {
		return bp, nil
	}

	bp, err := c.loader.LoadFlowBlueprint(path)
	if err != nil {
		return nil, err
	}

	c.bpCache[path] = bp
	return bp, nil
}

// LoadToolSpecsFromDir 从目录加载所有工具规范（带缓存）
func (c *CachedSpecLoader) LoadToolSpecsFromDir(dir string) (map[string]*schemas.ToolSpec, error) {
	return c.loader.LoadToolSpecsFromDir(dir)
}

// LoadBlueprintsFromDir 从目录加载所有流程蓝图（带缓存）
func (c *CachedSpecLoader) LoadBlueprintsFromDir(dir string) (map[string]*schemas.Flow, error) {
	return c.loader.LoadBlueprintsFromDir(dir)
}

// ClearCache 清空缓存
func (c *CachedSpecLoader) ClearCache() {
	c.toolCache = make(map[string]*schemas.ToolSpec)
	c.bpCache = make(map[string]*schemas.Flow)
}
