package scene

import (
	"PowerX/internal/model/scene"
)

// FindOneSceneQRCodeDetail
//
//	@Description:
//	@receiver this
//	@param qid
//	@return *qrcode.QrcodeActive
func (this *sceneUseCase) FindOneSceneQRCodeDetail(qid string) *scene.SceneQRCode {

	qrcode := this.modelSceneQRCode.qrcode.FindEnableSceneQRCodeByQid(this.db, qid)

	return qrcode

}

// IncreaseSceneCpaNumber
//
//	@Description:
//	@receiver this
//	@param qid
func (this *sceneUseCase) IncreaseSceneCpaNumber(qid string) {

	this.modelSceneQRCode.qrcode.IncreaseCpa(this.db, qid)
}
