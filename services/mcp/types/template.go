package types

const (
	TplGenerateDomainModel         = "domain_model"
	TplGenerateGormModel           = "gorm_model"
	TplGenerateRepositoryInterface = "repository_interface"
	TplGenerateRepositoryImpl      = "repository_impl"
	TplGenerateMigration           = "migration"
	TplGenerateUseCase             = "usecase"
	TplGenerateDTO                 = "dto"
	TplGenerateAdapter             = "adapter"
	TplGenerateAPI                 = "api"
	TplGenerateHandler             = "handler"
	TplGenerateGRPCService         = "grpc_service"
	TplGenerateTests               = "tests"
	TplGenerateMocks               = "mocks"

	ActionSuffix   = "generate_"
	TemplateSuffix = ".tmpl"
)
