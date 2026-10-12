package customer

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil || deps == nil || deps.DB == nil {
		return
	}
	h := NewHandler(deps)
	g := protectedGroup.Group("/admin/customers")
	g.GET("/overview", h.Overview)
	g.GET("/accounts", h.ListAccounts)
	g.POST("/accounts", h.CreateAccount)
	g.GET("/accounts/:customer_uuid", h.GetAccount)
	g.PATCH("/accounts/:customer_uuid", h.UpdateAccountProfile)
	g.PATCH("/accounts/:customer_uuid/status", h.UpdateStatus)
	g.GET("/accounts/:customer_uuid/auth-identities", h.ListAuthIdentities)
	g.GET("/accounts/:customer_uuid/tenant-memberships", h.ListMemberships)
	g.GET("/accounts/:customer_uuid/login-events", h.ListLoginEvents)
	g.GET("/mini-app-entries", h.ListMiniAppEntries)
	g.POST("/imports/preview", h.PreviewAccountImport)
	g.GET("/imports/:import_uuid", h.GetAccountImport)
	g.POST("/imports/:import_uuid/commit", h.CommitAccountImport)
	g.POST("/contact-repairs/preview", h.PreviewContactRepair)
	g.GET("/contact-repairs/:repair_uuid", h.GetContactRepair)
	g.POST("/contact-repairs/:repair_uuid/commit", h.CommitContactRepair)
	g.GET("/:customer_uuid/contacts", h.ListContacts)
	g.POST("/:customer_uuid/contacts", h.CreateContact)
	g.GET("/:customer_uuid/contacts/:contact_uuid", h.GetContact)
	g.PATCH("/:customer_uuid/contacts/:contact_uuid", h.UpdateContact)
	g.POST("/:customer_uuid/contacts/:contact_uuid/primary", h.SetPrimaryContact)
	g.POST("/:customer_uuid/contacts:resolve-identity", h.ResolveContactIdentity)
	g.POST("/:customer_uuid/contacts/:contact_uuid/identities", h.BindContactIdentity)
	g.GET("/:customer_uuid/contacts/:contact_uuid/identities", h.ListContactIdentities)
}
