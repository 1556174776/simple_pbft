package pool

import (
	"Github/simplePBFT/pbft/consensus"
	"sync"
)

type PrePrepareMsgPool struct {
	PPMsgPool map[string]consensus.PrePrepareMsg //包含的request Msg的摘要作为key

	poolMutex sync.Mutex
}

func NewPPMsgPool() *PrePrepareMsgPool {
	return &PrePrepareMsgPool{
		PPMsgPool: make(map[string]consensus.PrePrepareMsg),
	}
}

func (ppmp *PrePrepareMsgPool) AddPPMsg(ppMsg consensus.PrePrepareMsg) {
	ppmp.poolMutex.Lock()
	defer ppmp.poolMutex.Unlock()

	ppmp.PPMsgPool[ppMsg.Digest] = ppMsg
}

func (ppmp *PrePrepareMsgPool) DelPPMsg(digest string) {
	ppmp.poolMutex.Lock()
	defer ppmp.poolMutex.Unlock()

	if _, ok := ppmp.PPMsgPool[digest]; ok {
		delete(ppmp.PPMsgPool, digest)
	}
}

func (ppmp *PrePrepareMsgPool) MsgNum() int {
	ppmp.poolMutex.Lock()
	defer ppmp.poolMutex.Unlock()

	return len(ppmp.PPMsgPool)
}

func (ppmp *PrePrepareMsgPool) GetPPMsgByDigest(digest string) consensus.PrePrepareMsg {
	ppmp.poolMutex.Lock()
	defer ppmp.poolMutex.Unlock()

	return ppmp.PPMsgPool[digest]
}

func (ppmp *PrePrepareMsgPool) GetAllPPMsg() []consensus.PrePrepareMsg {
	ppmp.poolMutex.Lock()
	defer ppmp.poolMutex.Unlock()

	result := make([]consensus.PrePrepareMsg, 0)
	for _, msg := range ppmp.PPMsgPool {
		result = append(result, msg)
	}
	return result
}
