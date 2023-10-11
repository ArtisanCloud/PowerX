package {{.PackageName}}

import (
	{{if .HasTypesImport}}"{{.TypesImportPath}}"{{end}}
	"context"
	{{if .HasFmtImport}}"fmt"{{end}}
	{{if .HasNetHttpImport}}"net/http"{{end}}
)

type {{.GroupName}} struct {
	*PowerX
}

{{range .Routes}}
func (c *{{$.GroupName}}) {{.Handler}}(ctx context.Context, {{if ne .RequestTypeName ""}}req *powerxtypes.{{.RequestTypeName}}{{end}}) (*powerxtypes.{{.ResponseTypeName}}, error) {
	res := &powerxtypes.{{.ResponseTypeName}}{}
	err := c.H.Df().Method(http.Method{{CapFirst .Method}}).
		WithContext(ctx).
		Uri({{FormatPath .Path .Handler $.PathParamsMap}}).
		{{if and (ne (ToUpper .Method) "GET") (ne .RequestTypeName "")}}Json(req).{{else if and (eq (ToUpper .Method) "GET") (ne .RequestTypeName "")}}BindQuery(req).{{end}}
		Result(res)
	if err != nil {
		return nil, err
	}
	return res, nil
}
{{end}}