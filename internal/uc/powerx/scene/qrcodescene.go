package scene

import (
    "PowerX/internal/model/scene"
)

//
// FindOneSceneQrcodeDetail
//  @Description:
//  @receiver this
//  @param qid
//  @return *qrcode.QrcodeActive
//
func (this *sceneUseCase) FindOneSceneQrcodeDetail(qid string) *scene.SceneQrcode {

    qrcode := this.modelSceneQrcode.qrcode.FindEnableSceneQrcodeByQid(this.db, qid)

    return qrcode

}

//
// IncreaseSceneCpaNumber
//  @Description:
//  @receiver this
//  @param qid
//
func (this *sceneUseCase) IncreaseSceneCpaNumber(qid string) {

    this.modelSceneQrcode.qrcode.IncreaseCpa(this.db, qid)
}
