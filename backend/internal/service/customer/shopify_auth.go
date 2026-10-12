package customer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

const ShopifyStorefrontChannel = "shopify_storefront"
const ShopifyStorefrontConfigKey = "customer_auth.shopify_storefront"

var (
	ErrCustomerCredentialInvalid = errors.New("customer.credential_invalid")
	ErrCustomerIdentityNotFound  = errors.New("customer.identity_not_found")
)

type ShopifyStorefrontConfig struct {
	ShopDomain            string `json:"shop_domain"`
	StorefrontAccessToken string `json:"storefront_access_token"`
}
type VerifiedShopifyCustomer struct{ Subject, DisplayName, GivenName, FamilyName, Email, Phone string }

type CustomerAuthService struct {
	db          *gorm.DB
	accounts    *AccountService
	memberships *MembershipService
	tokens      *CustomerTokenService
	client      *http.Client
}

func NewCustomerAuthService(db *gorm.DB, tokens *CustomerTokenService) *CustomerAuthService {
	return &CustomerAuthService{db: db, accounts: NewAccountService(db), memberships: NewMembershipService(db), tokens: tokens, client: &http.Client{Timeout: 5 * time.Second}}
}

func (s *CustomerAuthService) AllowAttempt(ctx context.Context, tenantUUID, pluginID, channel, identifier, ip string) error {
	if s == nil || s.db == nil {
		return ErrCustomerUpstreamDependency
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&modelcustomer.LoginEvent{}).Where("tenant_uuid = ? AND plugin_id = ? AND channel = ? AND identifier_hash = ? AND ip = ? AND created_at > ?", tenantUUID, strings.TrimSpace(pluginID), strings.TrimSpace(channel), customerIdentifierHash(identifier), strings.TrimSpace(ip), time.Now().Add(-5*time.Minute)).Count(&count).Error; err != nil {
		return ErrCustomerUpstreamDependency
	}
	if count >= 10 {
		return ErrCustomerForbidden
	}
	return nil
}

func (s *CustomerAuthService) RecordAttempt(ctx context.Context, tenantUUID, pluginID, channel, identifier, ip, eventType, code string, ok bool) {
	if s == nil || s.db == nil {
		return
	}
	_ = s.db.WithContext(ctx).Create(&modelcustomer.LoginEvent{TenantUUID: tenantUUID, PluginID: strings.TrimSpace(pluginID), Channel: strings.TrimSpace(channel), IdentifierHash: customerIdentifierHash(identifier), IdentityProvider: ShopifyStorefrontChannel, EventType: eventType, OK: ok, ErrorCode: strings.TrimSpace(code), IP: strings.TrimSpace(ip)}).Error
}

func customerIdentifierHash(identifier string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(identifier)))
	return hex.EncodeToString(sum[:])
}

func (s *CustomerAuthService) RegisterShopify(ctx context.Context, tenantUUID, credential string) (TokenPair, Membership, error) {
	verified, pluginID, err := s.verifyShopify(ctx, tenantUUID, credential)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	resolved, err := s.accounts.resolveVerifiedExternalIdentity(ctx, ResolveExternalIdentityInput{TenantUUID: tenantUUID, ProviderSubject: verified.Subject, DisplayName: verified.DisplayName, GivenName: verified.GivenName, FamilyName: verified.FamilyName, Email: verified.Email, Phone: verified.Phone})
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	m, err := s.memberships.ResolveCurrent(ctx, tenantUUID, resolved.CustomerUUID)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	if err := s.ensureShopifyPrimaryContact(ctx, m); err != nil {
		return TokenPair{}, Membership{}, err
	}
	if err := s.markVerified(ctx, pluginID, verified.Subject); err != nil {
		return TokenPair{}, Membership{}, err
	}
	pair, err := s.tokens.IssuePair(ctx, m)
	return pair, m, err
}
func (s *CustomerAuthService) LoginShopify(ctx context.Context, tenantUUID, credential string) (TokenPair, Membership, error) {
	verified, pluginID, err := s.verifyShopify(ctx, tenantUUID, credential)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	provider, err := externalProviderKey(pluginID)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	var identity modelcustomer.AuthIdentity
	if err := s.db.WithContext(ctx).Where("provider = ? AND provider_subject = ? AND status = ?", provider, verified.Subject, modelcustomer.StatusActive).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return TokenPair{}, Membership{}, ErrCustomerIdentityNotFound
		}
		return TokenPair{}, Membership{}, ErrCustomerUpstreamDependency
	}
	m, err := s.memberships.ResolveCurrent(ctx, tenantUUID, identity.CustomerUUID)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	// Login never creates a new identity or membership. After both are found,
	// the verified Core path may repair only a missing primary contact binding.
	if _, err := s.accounts.resolveVerifiedExternalIdentity(ctx, ResolveExternalIdentityInput{TenantUUID: tenantUUID, ProviderSubject: verified.Subject, DisplayName: verified.DisplayName, GivenName: verified.GivenName, FamilyName: verified.FamilyName, Email: verified.Email, Phone: verified.Phone}); err != nil {
		return TokenPair{}, Membership{}, err
	}
	m, err = s.memberships.ResolveCurrent(ctx, tenantUUID, identity.CustomerUUID)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	if err := s.ensureShopifyPrimaryContact(ctx, m); err != nil {
		return TokenPair{}, Membership{}, err
	}
	if err := s.markVerified(ctx, pluginID, verified.Subject); err != nil {
		return TokenPair{}, Membership{}, err
	}
	pair, err := s.tokens.IssuePair(ctx, m)
	return pair, m, err
}

func (s *CustomerAuthService) ensureShopifyPrimaryContact(ctx context.Context, m Membership) error {
	if (m.Type != modelcustomer.AccountTypePerson && m.Type != modelcustomer.AccountTypeCompany) || m.PrimaryContactUUID == "" {
		return customerrepo.ErrExternalIdentityContactRequired
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&modelcustomer.Contact{}).Where("tenant_uuid = ? AND customer_uuid = ? AND uuid = ? AND status = ?", m.TenantUUID, m.CustomerUUID, m.PrimaryContactUUID, modelcustomer.ContactStatusActive).Count(&count).Error; err != nil {
		return ErrCustomerUpstreamDependency
	}
	if count != 1 {
		return customerrepo.ErrExternalIdentityContactRequired
	}
	return nil
}
func (s *CustomerAuthService) verifyShopify(ctx context.Context, tenantUUID, credential string) (VerifiedShopifyCustomer, string, error) {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || !strings.EqualFold(claims.Issuer, "powerx-sts") || strings.TrimSpace(claims.PluginID) == "" {
		return VerifiedShopifyCustomer{}, "", ErrCustomerUnauthorized
	}
	var cfgRow settingmodel.PluginInstanceConfig
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, claims.PluginID, ShopifyStorefrontConfigKey, true).First(&cfgRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return VerifiedShopifyCustomer{}, "", ErrCustomerForbidden
		}
		return VerifiedShopifyCustomer{}, "", ErrCustomerUpstreamDependency
	}
	var cfg ShopifyStorefrontConfig
	if json.Unmarshal(cfgRow.ValueJSON, &cfg) != nil || !strings.HasSuffix(strings.ToLower(strings.TrimSpace(cfg.ShopDomain)), ".myshopify.com") || strings.TrimSpace(cfg.StorefrontAccessToken) == "" {
		return VerifiedShopifyCustomer{}, "", ErrCustomerUpstreamDependency
	}
	body, _ := json.Marshal(map[string]any{"query": "query($token: String!) { customer(customerAccessToken: $token) { id displayName firstName lastName email phone } }", "variables": map[string]string{"token": credential}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+strings.TrimSpace(cfg.ShopDomain)+"/api/2024-10/graphql.json", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shopify-Storefront-Access-Token", cfg.StorefrontAccessToken)
	resp, err := s.client.Do(req)
	if err != nil {
		return VerifiedShopifyCustomer{}, "", ErrCustomerUpstreamDependency
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return VerifiedShopifyCustomer{}, "", ErrCustomerCredentialInvalid
	}
	var out struct {
		Data struct {
			Customer *struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
				FirstName   string `json:"firstName"`
				LastName    string `json:"lastName"`
				Email       string `json:"email"`
				Phone       string `json:"phone"`
			} `json:"customer"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Data.Customer == nil || out.Data.Customer.ID == "" {
		return VerifiedShopifyCustomer{}, "", ErrCustomerCredentialInvalid
	}
	return VerifiedShopifyCustomer{Subject: ShopifyExternalIdentitySubject(cfg.ShopDomain, out.Data.Customer.ID), DisplayName: out.Data.Customer.DisplayName, GivenName: out.Data.Customer.FirstName, FamilyName: out.Data.Customer.LastName, Email: out.Data.Customer.Email, Phone: out.Data.Customer.Phone}, claims.PluginID, nil
}
func externalProviderKey(pluginID string) (string, error) {
	return customerrepo.ExternalIdentityProviderKey(pluginID)
}
func (s *CustomerAuthService) markVerified(ctx context.Context, pluginID, subject string) error {
	provider, err := externalProviderKey(pluginID)
	if err != nil {
		return err
	}
	now := time.Now()
	if err = s.db.WithContext(ctx).Model(&modelcustomer.AuthIdentity{}).Where("provider = ? AND provider_subject = ?", provider, subject).Update("verified_at", &now).Error; err != nil {
		return ErrCustomerUpstreamDependency
	}
	return nil
}

// ShopifyExternalIdentitySubject is shared by verified login and management callers.
// Keep the full customer GID and the configured shop domain; do not substitute shop numeric IDs.
func ShopifyExternalIdentitySubject(shopDomain, customerID string) string {
	return "shop:" + strings.TrimSpace(shopDomain) + ":customer:" + strings.TrimSpace(customerID)
}
