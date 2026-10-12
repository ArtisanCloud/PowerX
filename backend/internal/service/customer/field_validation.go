package customer

import modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"

// validCustomerText 与 PostgreSQL varchar(n) 一致按 Unicode 字符数计长。
// Customer、Contact、资料更新及 CSV 预览共用，拒绝非法编码和控制字符。
func validCustomerText(value string, limit int, required bool) bool {
	return modelcustomer.ValidText(value, limit, required)
}

func validContactNames(display, given, family string) bool {
	return validCustomerText(display, 128, true) && validCustomerText(given, 128, false) && validCustomerText(family, 128, false)
}
