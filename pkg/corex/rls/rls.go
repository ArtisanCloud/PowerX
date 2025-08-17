package rls

type Expr string

func And(a, b Expr) Expr {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return Expr("(" + string(a) + ") AND (" + string(b) + ")")
}
func TenantFilter(tenantID string) Expr {
	if tenantID == "" {
		return ""
	}
	return Expr("tenant_id = '" + tenantID + "'")
}
