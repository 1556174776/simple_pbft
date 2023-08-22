package consensus

import (
	"Github/simplePBFT/utils"
	"Github/simplePBFT/zapConfig"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type State struct {
	ViewID         int64    //当前视图id
	MsgLogs        *MsgLogs //记录本轮共识产生的log消息
	LastSequenceID int64    //上一轮被共识的request消息的SequenceID
	CurrentStage   Stage    //当前共识阶段
}

type MsgLogs struct {
	ReqMsg      *RequestMsg
	PrepareMsgs map[string]*VoteMsg
	CommitMsgs  map[string]*VoteMsg
}

type Stage int

const (
	Idle        Stage = iota // Node is created successfully, but the consensus process is not started yet.
	PrePrepared              // The ReqMsgs is processed successfully. The node is ready to head to the Prepare stage.
	Prepared                 // Same with `prepared` stage explained in the original paper.
	Committed                // Same with `committed-local` stage explained in the original paper.
)

// f: # of Byzantine faulty node
// f = (n­1) / 3
// n = 4, in this case.
const f = 1

// lastSequenceID will be -1 if there is no last sequence ID.
// 根据 视图id 和 上一轮被共识的requestMsg的SequenceID 为本轮共识创建新的共识状态对象State
func CreateState(viewID int64, lastSequenceID int64) *State {
	return &State{
		ViewID: viewID,
		MsgLogs: &MsgLogs{
			ReqMsg:      nil,
			PrepareMsgs: make(map[string]*VoteMsg),
			CommitMsgs:  make(map[string]*VoteMsg),
		},
		LastSequenceID: lastSequenceID,
		CurrentStage:   Idle,
	}
}

// 根据传入的RequestMsg消息,更新本轮共识的State对象,同时返回pre-prepare阶段需要使用的PrePrepareMsg消息
func (state *State) StartConsensus(request *RequestMsg) (*PrePrepareMsg, error) {
	// `sequenceID` will be the index of this message.
	sequenceID := time.Now().UnixNano() //PrePrepareMsg消息的sequenceID需要使用当前时间戳为准

	// Find the unique and largest number for the sequence ID
	if state.LastSequenceID != -1 { //要保证本轮需要被共识的request消息的sequenceID大于上一轮达成共识的request消息的sequenceID
		for state.LastSequenceID >= sequenceID {
			sequenceID += 1
		}
	}

	// Assign a new sequence ID to the request message object.
	request.SequenceID = sequenceID

	// Save ReqMsgs to its logs.
	state.MsgLogs.ReqMsg = request //副本节点的State对象也需要将本轮将被共识的request消息记录到log消息池

	// Get the digest of the request message
	digest, err := digest(request) //计算获取request消息的hash值
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	// Change the stage to pre-prepared.
	state.CurrentStage = PrePrepared //更新state对象的共识阶段为Pre-Prepared

	return &PrePrepareMsg{
		ViewID:     state.ViewID,
		SequenceID: sequenceID,
		Digest:     digest,
		RequestMsg: request,
	}, nil
}

// 副本节点接收主节点发送的pre-prepare消息,验证消息正确性,更新共识状态,生成prepareMsg
func (state *State) PrePrepare(prePrepareMsg *PrePrepareMsg) (*VoteMsg, error) {
	// Get ReqMsgs and save it to its logs like the primary.
	state.MsgLogs.ReqMsg = prePrepareMsg.RequestMsg //副本节点需要将pre-prepare消息中中的request消息取出,存放到State对象的log消息池

	// Verify if v, n(a.k.a. sequenceID), d are correct.
	if !state.verifyMsg(prePrepareMsg.ViewID, prePrepareMsg.SequenceID, prePrepareMsg.Digest) { //验证此pre-prepare消息的正确性(sequenceID和数字摘要是否正确)
		return nil, errors.New("pre-prepare message is corrupted")
	}

	// Change the stage to pre-prepared.
	state.CurrentStage = PrePrepared //改变共识状态

	return &VoteMsg{
		ViewID:     state.ViewID,
		SequenceID: prePrepareMsg.SequenceID,
		Digest:     prePrepareMsg.Digest,
		MsgType:    PrepareMsg,
	}, nil
}

// 1.验证收到的prepare消息是否正确
// 2.将获取的prepareMsg添加到日志消息池MsgLogs(key为来源NodeID)
// 3.判断收集的prepareMsg是否足够,能否进入下一commit共识阶段
// 4.如果可以，组建commit消息
func (state *State) Prepare(prepareMsg *VoteMsg) (*VoteMsg, error) {
	if !state.verifyMsg(prepareMsg.ViewID, prepareMsg.SequenceID, prepareMsg.Digest) { //验证prepare消息是否正确
		return nil, errors.New("prepare message is corrupted")
	}

	// Append msg to its logs
	state.MsgLogs.PrepareMsgs[prepareMsg.NodeID] = prepareMsg //将获取的prepareMsg添加到日志消息池MsgLogs(key为来源NodeID)

	// Print current voting status
	zapConfig.SugarLogger.Debugf("当前节点已搜集的Prepare消息Vote Num:%d", len(state.MsgLogs.PrepareMsgs))

	if state.prepared() { //需要判断是否可以进入下一commit共识阶段,如果可以返回组建的commit消息
		// Change the stage to prepared.
		state.CurrentStage = Prepared //更新共识阶段为prepared(已经完成prepare阶段共识)

		return &VoteMsg{
			ViewID:     state.ViewID,
			SequenceID: prepareMsg.SequenceID,
			Digest:     prepareMsg.Digest,
			MsgType:    CommitMsg,
		}, nil
	}

	return nil, nil
}

// 1.验证收到的commit消息是否正确
// 2.将获取的commitMsg添加到日志消息池MsgLogs(key为来源NodeID)
// 3.判断收集的commitMsg是否足够,能否进入下一reply共识阶段
// 4.如果可以，组建reply消息,同时还返回reply消息针对的request消息
func (state *State) Commit(commitMsg *VoteMsg) (*ReplyMsg, *RequestMsg, error) {
	if !state.verifyMsg(commitMsg.ViewID, commitMsg.SequenceID, commitMsg.Digest) { //验证commit消息是否正确
		return nil, nil, errors.New("commit message is corrupted")
	}

	// Append msg to its logs
	state.MsgLogs.CommitMsgs[commitMsg.NodeID] = commitMsg //将获取的commitMsg添加到日志消息池MsgLogs(key为来源NodeID)

	// Print current voting status
	zapConfig.SugarLogger.Debugf("当前节点已搜集的Commit消息Vote Num:%d", len(state.MsgLogs.CommitMsgs))

	if state.committed() { //需要判断是否可以进入下一reply共识阶段,如果可以返回组建的reply消息
		// This node executes the requested operation locally and gets the result.
		result := "Executed"

		// Change the stage to prepared.
		state.CurrentStage = Committed              //更新共识阶段为committed(已经完成commit阶段共识)
		state.LastSequenceID = commitMsg.SequenceID //更新State的sequenceID

		return &ReplyMsg{
			ViewID:    state.ViewID,
			Timestamp: state.MsgLogs.ReqMsg.Timestamp, //时间戳是完成共识的request消息的时间戳
			ClientID:  state.MsgLogs.ReqMsg.ClientID,  //clientID是完成共识的request消息的客户端id
			Result:    result,
		}, state.MsgLogs.ReqMsg, nil
	}

	return nil, nil, nil
}

// 传入的参数分别为待检测消息包含的 viewID,sequenceID,数字摘要(检查sequenceID是否正确,request消息的摘要是否正确)
func (state *State) verifyMsg(viewID int64, sequenceID int64, digestGot string) bool {
	// Wrong view. That is, wrong configurations of peers to start the consensus.
	if state.ViewID != viewID {
		return false
	}

	// Check if the Primary sent fault sequence number. => Faulty primary.
	// TODO: adopt upper/lower bound check.
	if state.LastSequenceID != -1 {
		if state.LastSequenceID >= sequenceID { //待验证的消息不能是已经完成共识的消息(每一条request消息都有唯一的序列号sequenceID, state.LastSequenceID是上一条已经被确认的request消息的序列号)
			return false
		}
	}

	digest, err := digest(state.MsgLogs.ReqMsg) //计算request消息的摘要
	if err != nil {
		fmt.Println(err)
		return false
	}

	// Check digest.
	if digestGot != digest { //检查计算出的摘要和pre-prepare消息中包含的摘要是否相等
		return false
	}

	return true
}

// 判断当前节点是否已经完成了prepare共识阶段,可以进入下一commit阶段
// 1.检查State对象是否有作为共识目标的request消息(MsgLogs.ReqMsg是否有)
// 2.检查当前节点是否已经收集到了足够的 prepareMsg (至少需要收集到2f条其他节点的prepare消息)
func (state *State) prepared() bool {
	if state.MsgLogs.ReqMsg == nil {
		return false
	}

	if len(state.MsgLogs.PrepareMsgs) < 2*f {
		return false
	}

	return true
}

// 判断当前节点是否已经完成了commit共识阶段,可以进入下一reply阶段
// 1.判断是否已经完成prepare阶段
// 2.判断收集的commitMsg消息数是否足够(至少需要收集到2f条其他节点的commit消息)
func (state *State) committed() bool {
	if !state.prepared() {
		return false
	}

	if len(state.MsgLogs.CommitMsgs) < 2*f {
		return false
	}

	return true
}

// 传入消息进行json编码并计算hash值返回
func digest(object interface{}) (string, error) {
	msg, err := json.Marshal(object)

	if err != nil {
		return "", err
	}

	return utils.Hash(msg), nil
}
