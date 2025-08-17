package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	modelForm "github.com/ArtisanCloud/PowerX/pkg/dynamic_form/persistence/model"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// FormSchemaRepository 表单结构仓储
type FormSchemaRepository struct {
	*repository.BaseRepository[modelForm.FormSchemaRecord]
}

// NewFormSchemaRepository 创建表单结构仓储
func NewFormSchemaRepository(db *gorm.DB) *FormSchemaRepository {
	return &FormSchemaRepository{
		BaseRepository: repository.NewBaseRepository[modelForm.FormSchemaRecord](db),
	}
}

// Create 创建表单结构
func (r *FormSchemaRepository) Create(ctx context.Context, form *domainModel.FormSchema) error {
	// 如果没有ID，生成一个
	if form.ID == "" {
		form.ID = fmt.Sprintf("form_%s", uuid.New().String())
	}

	// 序列化字段定义
	fieldsJSON, err := json.Marshal(form.Fields)
	if err != nil {
		return fmt.Errorf("序列化字段定义失败: %w", err)
	}

	// 序列化变量
	variablesJSON, err := json.Marshal(form.Variables)
	if err != nil {
		return fmt.Errorf("序列化变量失败: %w", err)
	}

	// 序列化元数据
	metadataJSON, err := json.Marshal(form.Metadata)
	if err != nil {
		return fmt.Errorf("序列化元数据失败: %w", err)
	}

	// 创建数据库记录
	record := &model.FormSchemaRecord{
		ID:          form.ID,
		Title:       form.Title,
		Description: form.Description,
		Fields:      datatypes.JSON(fieldsJSON),
		Variables:   datatypes.JSON(variablesJSON),
		Metadata:    datatypes.JSON(metadataJSON),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 保存到数据库
	_, err = r.BaseRepository.Create(ctx, record)
	if err != nil {
		return fmt.Errorf("保存表单结构失败: %w", err)
	}

	return nil
}

// FindByID 根据ID查找表单结构
func (r *FormSchemaRepository) FindByID(ctx context.Context, id string) (*domainModel.FormSchema, error) {
	record, err := r.BaseRepository.GetFirst(ctx, "id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("查询表单失败: %w", err)
	}
	if record == nil {
		return nil, fmt.Errorf("表单不存在: %s", id)
	}

	// 反序列化字段定义
	var fields []domainModel.FormField
	if err := json.Unmarshal(record.Fields, &fields); err != nil {
		return nil, fmt.Errorf("反序列化字段定义失败: %w", err)
	}

	// 反序列化变量
	var variables map[string]string
	if err := json.Unmarshal(record.Variables, &variables); err != nil {
		return nil, fmt.Errorf("反序列化变量失败: %w", err)
	}

	// 反序列化元数据
	var metadata map[string]interface{}
	if err := json.Unmarshal(record.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("反序列化元数据失败: %w", err)
	}

	// 构建领域模型
	form := &domainModel.FormSchema{
		ID:          record.ID,
		Title:       record.Title,
		Description: record.Description,
		Fields:      fields,
		Variables:   variables,
		Metadata:    metadata,
	}

	return form, nil
}

// FormSubmissionRepository 表单提交仓储
type FormSubmissionRepository struct {
	*repository.BaseRepository[model.FormSubmission]
}

// NewFormSubmissionRepository 创建表单提交仓储
func NewFormSubmissionRepository(db *gorm.DB) *FormSubmissionRepository {
	return &FormSubmissionRepository{
		BaseRepository: repository.NewBaseRepository[model.FormSubmission](db),
	}
}

// Save 保存表单提交
func (r *FormSubmissionRepository) Save(ctx context.Context, formID string, input, cleaned map[string]interface{}, errors map[string]string) (string, error) {
	// 生成提交ID
	submissionID := fmt.Sprintf("submission_%s", uuid.New().String())

	// 序列化数据
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("序列化输入数据失败: %w", err)
	}

	cleanedJSON, err := json.Marshal(cleaned)
	if err != nil {
		return "", fmt.Errorf("序列化清理后数据失败: %w", err)
	}

	errorsJSON, err := json.Marshal(errors)
	if err != nil {
		return "", fmt.Errorf("序列化错误信息失败: %w", err)
	}

	// 创建提交记录
	submission := &model.FormSubmission{
		ID:           submissionID,
		FormSchemaID: formID,
		Input:        datatypes.JSON(inputJSON),
		Cleaned:      datatypes.JSON(cleanedJSON),
		Errors:       datatypes.JSON(errorsJSON),
		CreatedAt:    time.Now(),
	}

	// 保存到数据库
	_, err = r.BaseRepository.Create(ctx, submission)
	if err != nil {
		return "", fmt.Errorf("保存表单提交失败: %w", err)
	}

	return submissionID, nil
}
