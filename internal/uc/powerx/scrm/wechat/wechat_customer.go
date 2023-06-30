package wechat

import (
    "PowerX/internal/model/scrm/customer"
    "encoding/json"
    "github.com/ArtisanCloud/PowerSocialite/v3/src/models"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/response"
    "github.com/zeromicro/go-zero/core/logx"
    "gorm.io/gorm/clause"
)

//
// CustomerListWechatWorkRequest
//  @Description:
//  @receiver this
//  @param userID
//  @return []*response.ResponseExternalContact
//  @return error
//
func (this wechatUseCase) CustomerListWechatWorkRequest(userID ...string) ([]*response.ResponseExternalContact, error) {

    var err error
    info, _ := this.wework.ExternalContact.BatchGet(this.ctx, userID, ``, 1000)
    contacts := []customer.WeWorkExternalContacts{}
    follows := []customer.WeWorkExternalContactFollow{}

    for _, val := range info.ExternalContactList {
        contacts = append(contacts, transferExternalContactToModel(val.ExternalContact))
        follows = append(follows, transferExternalContactFollowToModel(val.FollowInfo, val.ExternalContact.ExternalUserID))
    }
    err = this.db.Clauses(
        clause.OnConflict{Columns: []clause.Column{{Name: `external_user_id`}}, UpdateAll: true}).CreateInBatches(&contacts, 100).Error
    err = this.db.Clauses(
        clause.OnConflict{Columns: []clause.Column{{Name: `external_user_id`}}, UpdateAll: true}).CreateInBatches(&follows, 100).Error
    if err != nil {
        logx.Errorf(`wechat.customer.contract.error. %v`, err)
    }
    if info != nil {
        return info.ExternalContactList, nil
    }

    return nil, err

}

//
// transferExternalContactToModel
//  @Description:
//  @param contact
//  @return *customer.WeWorkExternalContacts
//
func transferExternalContactToModel(contact *models.ExternalContact) customer.WeWorkExternalContacts {
    return customer.WeWorkExternalContacts{
        ExternalUserId:  contact.ExternalUserID,
        AppId:           ``,
        CorpId:          ``,
        OpenId:          ``,
        UnionId:         contact.UnionID,
        Name:            contact.Name,
        Mobile:          ``,
        Position:        contact.Position,
        Avatar:          contact.Avatar,
        CorpName:        contact.CorpName,
        CorpFullName:    contact.CorpFullName,
        ExternalProfile: ``,
        Gender:          contact.Gender,
        WXType:          int8(contact.Type),
        Status:          1,
        Active:          true,
    }
}

//
// transferExternalContactFollowToModel
//  @Description:
//  @param follow
//  @param externalUserID
//  @return customer.WeWorkExternalContactFollow
//
func transferExternalContactFollowToModel(follow *models.FollowUser, externalUserID string) customer.WeWorkExternalContactFollow {

    tags, _ := json.Marshal(follow.Tags)
    remarkMobiles, _ := json.Marshal(follow.RemarkMobiles)
    return customer.WeWorkExternalContactFollow{
        ExternalUserId: externalUserID,
        UserId:         follow.UserID,
        Remark:         follow.Remark,
        Description:    follow.Description,
        Createtime:     follow.CreateTime,
        Tags:           string(tags),
        WechatChannels: string(remarkMobiles),
        RemarkCorpName: follow.RemarkCorpName,
        RemarkMobiles:  ``,
        OpenUserId:     follow.OperUserID,
        AddWay:         follow.AddWay,
        State:          follow.State,
    }
}
