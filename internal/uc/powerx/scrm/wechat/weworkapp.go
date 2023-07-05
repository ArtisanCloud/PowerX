package wechat

import (
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/agent/response"
)

//
// PullDetailWeWorkAppRequest
//  @Description:
//  @receiver this
//  @param agentID
//  @return reply
//  @return err
//
func (this wechatUseCase) PullDetailWeWorkAppRequest(agentID int) (reply *response.ResponseAgentGet, err error) {

    reply, err = this.wework.Agent.Get(this.ctx, agentID)
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.pull.wework.app.detail.error`, reply.ResponseWork)
    }
    return reply, err

}

//
// PullListWeWorkAppRequest
//  @Description:
//  @receiver this
//  @return reply
//  @return err
//
func (this wechatUseCase) PullListWeWorkAppRequest() (reply *response.ResponseAgentList, err error) {

    reply, err = this.wework.Agent.List(this.ctx)
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.pull.wework.app.list.error`, reply.ResponseWork)
    }
    return reply, err

}
