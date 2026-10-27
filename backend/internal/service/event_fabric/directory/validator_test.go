package directory

import "testing"

func TestValidateCreateInputName(t *testing.T) {
	cases := []struct {
		name    string
		topic   string
		wantErr bool
	}{
		{name: "字母开头", topic: "progress", wantErr: false},
		// {{tenant_uuid}} 渲染后的租户 UUID 可能以数字开头
		{name: "数字开头的 UUID", topic: "6c868127-f83e-47b0-9e12-3a8c89f6689a", wantErr: false},
		{name: "下划线开头", topic: "_progress", wantErr: true},
		{name: "包含点号", topic: "a.b", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreateInput(CreateTopicInput{
				TenantUUID: "6c868127-f83e-47b0-9e12-3a8c89f6689a",
				Namespace:  "_topic.notify.tenant",
				Name:       tc.topic,
			})
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateCreateInput(name=%q) err=%v, wantErr=%v", tc.topic, err, tc.wantErr)
			}
		})
	}
}
