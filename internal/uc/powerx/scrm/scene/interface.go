package scene

import "PowerX/internal/model/scene"

type IsceneInterface interface {

	//
	//  @Description: qrcode
	//
	iQrcodeInterface
}

// iQrcodeInterface
// @Description:
type iQrcodeInterface interface {
	//
	// FindOneSceneQRCodeDetail
	//  @Description: 场景码详情
	//  @param qid
	//  @return *qrcode.QrcodeActive
	//
	FindOneSceneQRCodeDetail(qid string) *scene.SceneQRCode
	//
	// IncreaseSceneCpaNumber
	//  @Description: CPA+1
	//  @param qid
	//
	IncreaseSceneCpaNumber(qid string)
}
