package app

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/dto"
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/model"
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/persistence/repository"
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/runtime"
	"gorm.io/gorm"
)

type FormUseCase struct {
	repoFormSchema     *repository.FormSchemaRepository     `inject:""`
	repoFormSubmission *repository.FormSubmissionRepository `inject:""`
	executor           *runtime.FormExecutor
	db                 *gorm.DB
}

func NewFormUseCase(db *gorm.DB) *FormUseCase {
	return &FormUseCase{
		repoFormSchema:     repository.NewFormSchemaRepository(db),
		repoFormSubmission: repository.NewFormSubmissionRepository(db),
		executor:           runtime.NewFormExecutor(),
		db:                 db,
	}
}

func (uc *FormUseCase) CreateForm(ctx context.Context, req *dto.CreateFormRequest) (*dto.FormCreateResponse, error) {
	form := &model.FormSchema{
		Title:       req.Title,
		Description: req.Description,
		Fields:      req.Fields,
		Variables:   req.Variables,
		Metadata:    req.Metadata,
	}
	if err := uc.repoFormSchema.Create(ctx, form); err != nil {
		return nil, err
	}
	return &dto.FormCreateResponse{
		FormSchema: form,
	}, nil
}

func (uc *FormUseCase) GetForm(ctx context.Context, id string) (*dto.GetFormResponse, error) {
	form, err := uc.repoFormSchema.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dto.GetFormResponse{
		FormSchema: form,
	}, nil
}
