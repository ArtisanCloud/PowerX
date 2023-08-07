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
	// FindOneSceneQrcodeDetail
	//  @Description: 场景码详情
	//  @param qid
	//  @return *qrcode.QrcodeActive
	//
	FindOneSceneQrcodeDetail(qid string) *scene.SceneQrcode
	//
	// IncreaseSceneCpaNumber
	//  @Description: CPA+1
	//  @param qid
	//
	IncreaseSceneCpaNumber(qid string)
}
