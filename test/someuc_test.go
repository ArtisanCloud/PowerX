package test

import (
	"PowerX/internal/model/customerdomain"
	"context"
	"github.com/brianvoe/gofakeit/v6"
	"testing"
)

func TestGroup(t *testing.T) {
	// step 1
	TestCase(t)

	// step 2
	TestCase2(t)
}

// TestCase 可以使用 gofakeit 包去模拟数据, svcCtx被TestMain初始化到包变量, 可以在测试内部直接调用
// https://github.com/brianvoe/gofakeit
func TestCase(t *testing.T) {
	// fake data
	username := gofakeit.Username()

	t.Logf("username: %s", username)

	// run test func
	// svcCtx...

	//if err != nil {
	//	t.Error("test case failed")
	//}
}

func TestCase2(t *testing.T) {
	// fake data
	username := gofakeit.Username()

	t.Logf("username: %s", username)

	// run test func
	// svcCtx...

	//if err != nil {
	//	t.Error("test case failed")
	//}
}

// TestConsumerCreateConsumer 这是一个实际的测试示例, 用于测试带状态的uc, 通过调用uc的CreateCustomer方法, 创建一个新的customer
func TestConsumerCreateConsumer(t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			// 程序错误 Bug 直接测试失败
			t.Error(err)
		}
	}()

	err := svcCtx.PowerX.Customer.CreateCustomer(context.TODO(), &customerdomain.Customer{
		Name:        gofakeit.Name(),
		Mobile:      gofakeit.Phone(),
		Email:       gofakeit.Email(),
		InviterId:   gofakeit.Int64(),
		Source:      int(gofakeit.Int32()),
		Type:        0,
		IsActivated: true,
	})
	if err != nil {
		//if err == "预期错误" {
		// 期望的错误, 测试通过
		//	return
		//}

		// 不期望的错误 直接测试失败
		t.Error(err)
	}
}
