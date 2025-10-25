package app

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/dto"
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/persistence/repository"
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/runtime"
)

type FormUseCase struct {
	repoFormSchema     *repository.FormSchemaRepository     `inject:""`
	repoFormSubmission *repository.FormSubmissionRepository `inject:""`
	executor           *runtime.FormExecutor
}

func NewFormUseCase() *FormUseCase {
	return &FormUseCase{
		repoFormSchema:     repository.NewFormSchemaRepository(),
		repoFormSubmission: repository.NewFormSubmissionRepository(),
		executor:           runtime.NewFormExecutor(),
	}
}

func (uc *FormUseCase) CreateForm(ctx context.Context, req *dto.CreateFormRequest) (*dto.FormCreateResponse, error) {
	form := req.ToDomain()
	if _, err := uc.repoFormSchema.Create(ctx, form); err != nil {
		return nil, err
	}
	return dto.FromDomain(form), nil
}

func (uc *FormUseCase) GetForm(ctx context.Context, id string) (*dto.FormGetFormResponse, error) {
	form, err := uc.repoFormSchema.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return dto.FromDomain(form), nil
}
