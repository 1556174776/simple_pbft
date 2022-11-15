package pool

import (
	"Github/simplePBFT/pbft/consensus"
	"sync"
)

type PrepareMsgPool struct {
	PreMsgPool map[string]consensus.VoteMsg //PrepareMsg的来源节点NodeID作为key

	poolMutex sync.Mutex
}

func NewPreMsgPool() *PrepareMsgPool {
	return &PrepareMsgPool{
		PreMsgPool: make(map[string]consensus.VoteMsg),
	}
}

func (pmp *PrepareMsgPool) AddPreMsg(preMsg consensus.VoteMsg) {
	pmp.poolMutex.Lock()
	defer pmp.poolMutex.Unlock()

	pmp.PreMsgPool[preMsg.NodeID] = preMsg
}

func (pmp *PrepareMsgPool) DelPreMsg(nodeID string) {
	pmp.poolMutex.Lock()
	defer pmp.poolMutex.Unlock()

	if _, ok := pmp.PreMsgPool[nodeID]; ok {
		delete(pmp.PreMsgPool, nodeID)
	}
}

func (pmp *PrepareMsgPool) DelAllPreMsg() {
	pmp.poolMutex.Lock()
	defer pmp.poolMutex.Unlock()

	pmp.PreMsgPool = make(map[string]consensus.VoteMsg)
}

func (pmp *PrepareMsgPool) MsgNum() int {
	pmp.poolMutex.Lock()
	defer pmp.poolMutex.Unlock()
	return len(pmp.PreMsgPool)
}

func (pmp *PrepareMsgPool) GetPreMsgByDigest(nodeID string) consensus.VoteMsg {
	pmp.poolMutex.Lock()
	defer pmp.poolMutex.Unlock()

	return pmp.PreMsgPool[nodeID]
}

func (pmp *PrepareMsgPool) GetAllPreMsg() []consensus.VoteMsg {
	pmp.poolMutex.Lock()
	defer pmp.poolMutex.Unlock()

	result := make([]consensus.VoteMsg, 0)

	for _, msg := range pmp.PreMsgPool {
		result = append(result, msg)
		//fmt.Printf("消息池保存的PrepareMsg --- Digest: %s, SequenceID: %d NodeID: %s \n", msg.Digest, msg.SequenceID, msg.NodeID)
	}

	return result
}
