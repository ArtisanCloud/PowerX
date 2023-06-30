package wechat

import (
    "fmt"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/agent/response"

    "github.com/zeromicro/go-zero/core/logx"
)

//
// AppWechatDetailRequest
//  @Description:
//  @receiver this
//  @param agentID
//  @return reply
//  @return err
//
func (this wechatUseCase) AppWechatDetailRequest(agentID int) (reply *response.ResponseAgentGet, err error) {

    reply, err = this.wework.Agent.Get(this.ctx, agentID)
    if err != nil || reply.ErrCode > 0 {
        logx.Error(reply.ErrMSG)
        err = fmt.Errorf(reply.ErrMSG)
    }
    return reply, err

}

//
// AppWechatListRequest
//  @Description:
//  @receiver this
//  @return reply
//  @return err
//
func (this wechatUseCase) AppWechatListRequest() (reply *response.ResponseAgentList, err error) {

    reply, err = this.wework.Agent.List(this.ctx)
    if err != nil || reply.ErrCode > 0 {
        logx.Error(reply.ErrMSG)
        err = fmt.Errorf(reply.ErrMSG)
    }
    return reply, err

}
