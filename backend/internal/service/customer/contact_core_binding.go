package customer

import (
	"context"
	"errors"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
)

const (
	CustomerContactsServiceReadCapabilityID   = "com.corex.customer.contacts.service_read"
	CustomerContactsServiceManageCapabilityID = "com.corex.customer.contacts.service_manage"
	CustomerContactsCoreEndpoint              = "core://customer/contacts"
)

// ContactCoreBinding is the service-actor Contact boundary. Its callers use
// typed methods and cannot choose an arbitrary Core endpoint or method.
type ContactCoreBinding struct{ service *ContactService }

func NewContactCoreBinding(service *ContactService) *ContactCoreBinding {
	return &ContactCoreBinding{service: service}
}

type ContactCoreListInput struct {
	CustomerUUID, Query, Status string
	Page, PageSize              int
}
type ContactCoreGetInput struct{ CustomerUUID, ContactUUID string }
type ContactCoreResolveIdentityInput struct{ CustomerUUID, Channel, ExternalSubject string }
type ContactCoreCreateInput struct {
	CustomerUUID, DisplayName, GivenName, FamilyName, Status, CreationIntent string
	Roles, Tags                                                              []string
}
type ContactCoreUpdateInput struct {
	CustomerUUID, ContactUUID                  string
	DisplayName, GivenName, FamilyName, Status *string
	Roles, Tags                                *[]string
}
type ContactCoreBindIdentityInput struct{ CustomerUUID, ContactUUID, Channel, ExternalSubject string }

func (b *ContactCoreBinding) ListByCustomer(ctx context.Context, tenantUUID string, in ContactCoreListInput) (ContactPage, error) {
	if b == nil || b.service == nil {
		return ContactPage{}, errors.New("contact core binding unavailable")
	}
	return b.service.ListByCustomer(ctx, ListContactsInput{TenantUUID: tenantUUID, CustomerUUID: in.CustomerUUID, Query: in.Query, Status: in.Status, Page: in.Page, PageSize: in.PageSize})
}
func (b *ContactCoreBinding) Get(ctx context.Context, tenantUUID string, in ContactCoreGetInput) (*modelcustomer.Contact, error) {
	if b == nil || b.service == nil {
		return nil, errors.New("contact core binding unavailable")
	}
	return b.service.Get(ctx, tenantUUID, in.CustomerUUID, in.ContactUUID)
}
func (b *ContactCoreBinding) ResolveIdentity(ctx context.Context, tenantUUID string, in ContactCoreResolveIdentityInput) (*ContactIdentityResolution, error) {
	if b == nil || b.service == nil {
		return nil, errors.New("contact core binding unavailable")
	}
	return b.service.ResolveIdentity(ctx, ResolveContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: in.CustomerUUID, Channel: in.Channel, ExternalSubject: in.ExternalSubject})
}
func (b *ContactCoreBinding) Create(ctx context.Context, tenantUUID string, in ContactCoreCreateInput) (*modelcustomer.Contact, error) {
	if b == nil || b.service == nil {
		return nil, errors.New("contact core binding unavailable")
	}
	return b.service.Create(ctx, CreateContactInput{TenantUUID: tenantUUID, CustomerUUID: in.CustomerUUID, DisplayName: in.DisplayName, GivenName: in.GivenName, FamilyName: in.FamilyName, Status: in.Status, Roles: in.Roles, Tags: in.Tags, CreationIntent: in.CreationIntent})
}
func (b *ContactCoreBinding) Update(ctx context.Context, tenantUUID string, in ContactCoreUpdateInput) (*modelcustomer.Contact, error) {
	if b == nil || b.service == nil {
		return nil, errors.New("contact core binding unavailable")
	}
	return b.service.Update(ctx, UpdateContactInput{TenantUUID: tenantUUID, CustomerUUID: in.CustomerUUID, ContactUUID: in.ContactUUID, DisplayName: in.DisplayName, GivenName: in.GivenName, FamilyName: in.FamilyName, Status: in.Status, Roles: in.Roles, Tags: in.Tags})
}
func (b *ContactCoreBinding) BindIdentity(ctx context.Context, tenantUUID string, in ContactCoreBindIdentityInput) (*modelcustomer.ContactIdentity, error) {
	if b == nil || b.service == nil {
		return nil, errors.New("contact core binding unavailable")
	}
	return b.service.BindIdentity(ctx, BindContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: in.CustomerUUID, ContactUUID: in.ContactUUID, Channel: in.Channel, ExternalSubject: in.ExternalSubject})
}
