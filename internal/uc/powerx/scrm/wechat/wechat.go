package wechat

import (
    "PowerX/internal/types"
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work"
    "gorm.io/gorm"
    "sync"
)

type wechatUseCase struct {
    db     *gorm.DB
    wework *work.Work
    ctx    context.Context
    gLock  *sync.WaitGroup
}

//
// NewOrganizationUseCase
//  @Description:
//  @param db
//  @param wework
//  @return iEmployeeInterface
//
func Repo(db *gorm.DB, wework *work.Work) IWechatInterface {

    return &wechatUseCase{db: db, wework: wework, ctx: context.TODO(), gLock: new(sync.WaitGroup)}

}

//
//  FindManyWechatDepartmentsOption
//  @Description:
//
type FindManyWechatDepartmentsOption struct {
    WeWorkDepId []int
    Name        string
}

//
//  FindManyWechatEmployeesOption
//  @Description:
//
type FindManyWechatEmployeesOption struct {
    WeWorkUserId           string `json:"we_work_user_id"` //员工唯一ID
    Ids                    []int64
    Names                  []string
    Alias                  []string
    Emails                 []string
    Mobile                 []string
    OpenUserId             []string
    WeWorkMainDepartmentId []int64
    Status                 []int
    types.PageEmbedOption
}

type (

    // https://developer.work.weixin.qq.com/document/path/90248
    WechatAppRequestBase struct {
        ChatID  string               `json:"chatid"`
        MsgType string               `json:"msgtype"`
        Safe    int                  `json:"safe"`
        News    WechatAppRequestNews `json:"news"`
    }

    WechatAppRequestNews struct {
        Article []*WechatAppRequestNewsArticle `json:"articles"`
    }

    WechatAppRequestNewsArticle struct {
        Title       string `json:"title"`       // "领奖通知",
        Description string `json:"description"` // "<div class=\"gray\">2016年9月26日</div> <div class=\"normal\">恭喜你抽中iPhone 7一台，领奖码：xxxx</div><div class=\"highlight\">请于2016年10月10日前联系行政同事领取</div>",
        URL         string `json:"url"`         // "URL",
        PicURL      string `json:"picurl"`      // 多"
        AppID       string `json:"appid,omitempty"`
        PagePath    string `json:"pagepath,omitempty"`
    }
)

//
//  WechatCustomerGroup
//  @Description: 客户群组
//
type (
    WechatCustomerGroup struct {
        StatusFilter int `json:"status_filter"`
        OwnerFilter  struct {
            UseridList []string `json:"userid_list"`
        } `json:"owner_filter"`
        Cursor string `json:"cursor"`
        Limit  int    `json:"limit"`
    }
)

//
//  WechatCustomers
//  @Description: 客户
//
type (
    WechatCustomers struct {
        ExternalContact WechatCustomersWithExternalContactExternalProfile `json:"external_contact"`
        FollowUser      []WechatCustomersWithFollowUser                   `json:"follow_user"`

        NextCursor string `json:"next_cursor"`
    }

    WechatCustomersWithExternalContactExternalProfile struct {
        ExternalUserid  string                                            `json:"external_userid"`
        Name            string                                            `json:"name"`
        Position        string                                            `json:"position"`
        Avatar          string                                            `json:"avatar"`
        CorpName        string                                            `json:"corp_name"`
        CorpFullName    string                                            `json:"corp_full_name"`
        Type            int                                               `json:"type"`
        Gender          int                                               `json:"gender"`
        Unionid         string                                            `json:"unionid"`
        ExternalProfile ExternalContactExternalProfileWithExternalProfile `json:"external_profile"`
    }

    ExternalContactExternalProfileWithExternalProfile struct {
        ExternalAttr []ExternalContactExternalProfileExternalProfileWithExternalAttr `json:"external_attr"`
    }

    ExternalContactExternalProfileExternalProfileWithExternalAttr struct {
        Type        int                                                                      `json:"type"`
        Name        string                                                                   `json:"name"`
        Text        ExternalContactExternalProfileExternalProfileExternalAttrWithText        `json:"text"`
        Web         ExternalContactExternalProfileExternalProfileExternalAttrWithWeb         `json:"web"`
        Miniprogram ExternalContactExternalProfileExternalProfileExternalAttrWithMiniprogram `json:"miniprogram"`
    }
    ExternalContactExternalProfileExternalProfileExternalAttrWithText struct {
        Value string `json:"value"`
    }

    ExternalContactExternalProfileExternalProfileExternalAttrWithWeb struct {
        Url   string `json:"url"`
        Title string `json:"title"`
    }
    ExternalContactExternalProfileExternalProfileExternalAttrWithMiniprogram struct {
        Appid    string `json:"appid"`
        Pagepath string `json:"pagepath"`
        Title    string `json:"title"`
    }

    WechatCustomersWithFollowUser struct {
        UserId         string                                      `json:"userId"`
        Remark         string                                      `json:"remark"`
        Description    string                                      `json:"description"`
        Createtime     int                                         `json:"createtime"`
        Tags           []WechatCustomersFollowUserWithTags         `json:"tags"`
        WechatChannels WechatCustomersFollowUserWithWechatChannels `json:"wechat_channels"`
        RemarkCorpName string                                      `json:"remark_corp_name,omitempty"`
        RemarkMobiles  []string                                    `json:"remark_mobiles,omitempty"`
        OpenUserId     string                                      `json:"open_user_id"`
        AddWay         int                                         `json:"add_way"`
        State          string                                      `json:"state,omitempty"`
    }
    WechatCustomersFollowUserWithTags struct {
        GroupName string `json:"group_name"`
        TagName   string `json:"tag_name"`
        TagId     string `json:"tag_id,omitempty"`
        Type      int    `json:"type"`
    }
    WechatCustomersFollowUserWithWechatChannels struct {
        Nickname string `json:"nickname"`
        Source   int    `json:"source"`
    }
)
