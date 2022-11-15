package pool

import (
	"Github/simplePBFT/pbft/consensus"
	"sync"
)

type CommitMsgPool struct {
	CmMsgPool map[string]consensus.VoteMsg //CommitMsg的来源节点NodeID作为key

	poolMutex sync.Mutex
}

func NewCommitMsgPool() *CommitMsgPool {
	return &CommitMsgPool{
		CmMsgPool: make(map[string]consensus.VoteMsg),
	}
}

func (cmp *CommitMsgPool) AddCommitMsg(cmMsg consensus.VoteMsg) {
	cmp.poolMutex.Lock()
	defer cmp.poolMutex.Unlock()

	cmp.CmMsgPool[cmMsg.NodeID] = cmMsg
	//fmt.Printf("Commit Msg添加消息池成功: 来源NodeID:%s\n", cmMsg.NodeID)
}

func (cmp *CommitMsgPool) DelCommitMsg(nodeID string) {
	cmp.poolMutex.Lock()
	defer cmp.poolMutex.Unlock()

	if _, ok := cmp.CmMsgPool[nodeID]; ok {
		delete(cmp.CmMsgPool, nodeID)
	}
}

func (cmp *CommitMsgPool) DelAllCommitMsg() {
	cmp.poolMutex.Lock()
	defer cmp.poolMutex.Unlock()

	cmp.CmMsgPool = make(map[string]consensus.VoteMsg)
}

func (cmp *CommitMsgPool) MsgNum() int {
	cmp.poolMutex.Lock()
	defer cmp.poolMutex.Unlock()
	return len(cmp.CmMsgPool)
}

func (cmp *CommitMsgPool) GetCmMsgByDigest(nodeID string) consensus.VoteMsg {
	cmp.poolMutex.Lock()
	defer cmp.poolMutex.Unlock()

	return cmp.CmMsgPool[nodeID]
}

func (cmp *CommitMsgPool) GetAllCmMsg() []consensus.VoteMsg {
	cmp.poolMutex.Lock()
	defer cmp.poolMutex.Unlock()

	result := make([]consensus.VoteMsg, 0)
	for _, msg := range cmp.CmMsgPool {
		result = append(result, msg)
		//fmt.Printf("消息池保存的CommitMsg --- Digest: %s, SequenceID: %d NodeID: %s \n", msg.Digest, msg.SequenceID, msg.NodeID)

	}
	return result
}
