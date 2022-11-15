package pool

import (
	"Github/simplePBFT/pbft/consensus"
	"fmt"
	"sync"
)

type RequestMsgPool struct {
	ReqMsgPool map[string]consensus.RequestMsg //clientID做key值

	poolMutex sync.Mutex
}

func NewReqMsgPool() *RequestMsgPool {
	return &RequestMsgPool{
		ReqMsgPool: make(map[string]consensus.RequestMsg),
	}
}

func (rmp *RequestMsgPool) AddReqMsg(reqMsg consensus.RequestMsg) {
	rmp.poolMutex.Lock()
	defer rmp.poolMutex.Unlock()

	rmp.ReqMsgPool[reqMsg.ClientID] = reqMsg

}

func (rmp *RequestMsgPool) DelReqMsg(id string) {
	rmp.poolMutex.Lock()
	defer rmp.poolMutex.Unlock()

	if _, ok := rmp.ReqMsgPool[id]; ok {
		delete(rmp.ReqMsgPool, id)
	}
}

func (rmp *RequestMsgPool) GetReqMsgByClientID(id string) consensus.RequestMsg {
	rmp.poolMutex.Lock()
	defer rmp.poolMutex.Unlock()

	if result, ok := rmp.ReqMsgPool[id]; ok {
		return result
	} else {
		fmt.Println("该Msg不存在。。。。。。。。。。。")
	}
	return consensus.RequestMsg{}

}

func (rmp *RequestMsgPool) GetAllReqMsg() []consensus.RequestMsg {
	rmp.poolMutex.Lock()
	defer rmp.poolMutex.Unlock()

	result := make([]consensus.RequestMsg, 0)
	for _, msg := range rmp.ReqMsgPool {
		result = append(result, msg)
	}
	return result
}

func (rmp *RequestMsgPool) MsgNum() int {
	rmp.poolMutex.Lock()
	defer rmp.poolMutex.Unlock()

	return len(rmp.ReqMsgPool)
}
