package customer

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestAccountPhoneVerificationRequiresMatchingNumber(t *testing.T) {
	phone := "+8613800001234"
	evidence := map[string]any{PhoneVerifiedAtKey: time.Now().UTC().Format(time.RFC3339), PhoneVerifiedViaKey: "wechat_miniapp:wx-test", PhoneVerifiedHashKey: PhoneVerificationHash(phone)}
	raw, err := json.Marshal(evidence)
	require.NoError(t, err)
	require.True(t, (Account{PrimaryPhone: phone, Metadata: datatypes.JSON(raw)}).HasVerifiedPhone())
	require.False(t, (Account{PrimaryPhone: "+8613900005678", Metadata: datatypes.JSON(raw)}).HasVerifiedPhone())
	require.False(t, (Account{Metadata: datatypes.JSON(raw)}).HasVerifiedPhone())
	delete(evidence, PhoneVerifiedHashKey)
	raw, err = json.Marshal(evidence)
	require.NoError(t, err)
	require.False(t, (Account{PrimaryPhone: phone, Metadata: datatypes.JSON(raw)}).HasVerifiedPhone(), "旧的时间标记不能证明当前号码")
}
