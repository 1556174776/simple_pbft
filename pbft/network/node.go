package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"Github/simplePBFT/pbft/consensus"
	"Github/simplePBFT/pool"
	"Github/simplePBFT/utils"
	"Github/simplePBFT/zapConfig"
)

type Node struct {
	NodeID        string                  // 本地节点的NodeID
	NodeTable     map[string]string       // key=nodeID, value=url  记录其他节点的信息
	View          *View                   // 当前视图状态
	CurrentState  *consensus.State        // 当前共识状态
	CommittedMsgs []*consensus.RequestMsg // 存放所有已经完成pbft共识的request消息(完成commit,执行reply之前)
	MsgBuffer     *MsgBuffer              // 缓存池
	MsgEntrance   chan interface{}        // 与业务二进行通信的管道
	MsgDelivery   chan interface{}
	Alarm         chan bool
}

// 消息缓存池
type MsgBuffer struct {
	ReqMsgs        *pool.RequestMsgPool
	PrePrepareMsgs *pool.PrePrepareMsgPool
	PrepareMsgs    *pool.PrepareMsgPool
	CommitMsgs     *pool.CommitMsgPool
	ReplyMsgs      *pool.ReplyMsgPool
}

// 当前视图
type View struct {
	ID      int64  //视图ID
	Primary string //主节点nodeID
}

const ResolvingTimeDuration = time.Millisecond * 1000 // 1 second.
const f = 1                                           // pbft共识所需的参数f

// 根据NodeID创建一个新节点:
// 1.创建必要的节点资源NodeID/NodeTable/View/CurrentState/CommittedMsgs/MsgBuffer)
// 2.开启必要的处理协程 :
//
//	2.1 go node.dispatchMsg() 接收协程
//	2.2 go node.alarmToDispatcher() 定时器触发共识开启协程,另一种方式触发node.dispatchMsg()
//	2.3 go node.resolveMsg() 消息处理协程
func NewNode(nodeID string) *Node {
	const viewID = 10000000000 // 临时用视图ID

	node := &Node{
		// Hard-coded for test.
		NodeID: nodeID,
		NodeTable: map[string]string{
			"MainNode":     "localhost:1111",
			"ReplicaNode1": "localhost:1112",
			"ReplicaNode2": "localhost:1113",
			"ReplicaNode3": "localhost:1114",
		},
		View: &View{
			ID:      viewID,
			Primary: "MainNode",
		},

		// Consensus-related struct
		CurrentState:  nil,
		CommittedMsgs: make([]*consensus.RequestMsg, 0),
		MsgBuffer: &MsgBuffer{
			ReqMsgs:        pool.NewReqMsgPool(),
			PrePrepareMsgs: pool.NewPPMsgPool(),
			PrepareMsgs:    pool.NewPreMsgPool(),
			CommitMsgs:     pool.NewCommitMsgPool(),
			ReplyMsgs:      pool.NewRyMsgPool(),
		},

		// Channels
		MsgEntrance: make(chan interface{}),
		MsgDelivery: make(chan interface{}),
		Alarm:       make(chan bool),
	}

	// Start message dispatcher
	go node.dispatchMsg() //负责从node.MsgEntrance管道读取业务二产生消息存放到对应消息池中

	// Start alarm trigger
	go node.alarmToDispatcher() //定时扫描消息池中的消息,传递给协程3进行共识处理

	// Start message resolver
	go node.resolveMsg()

	return node
}

// 向其他共识节点发送共识消息
func send(url string, msg []byte) {
	buff := bytes.NewBuffer(msg)
	http.Post("http://"+url, "application/json", buff)
}

// 将传入的共识消息进行json编码,然后广播(http Post)给其他所有的共识节点
func (node *Node) Broadcast(msg interface{}, path string) map[string]error {
	errorMap := make(map[string]error)

	for nodeID, url := range node.NodeTable { //遍历node.NodeTable,获取其他共识节点的nodeID和url
		if nodeID == node.NodeID { //跳过本地节点
			continue
		}

		jsonMsg, err := json.Marshal(msg) //对需要进行广播的消息进行json编码
		if err != nil {
			errorMap[nodeID] = err
			continue
		}

		send(url+path, jsonMsg) //发送到其他共识节点的相应目录下(不同目录存放不同阶段的共识消息)
	}

	if len(errorMap) == 0 {
		return nil
	} else {
		return errorMap
	}
}

// 向当前view的主节点发送reply消息
func (node *Node) Reply(msg *consensus.ReplyMsg) error {
	// Print all committed messages.
	for _, value := range node.CommittedMsgs {
		zapConfig.SugarLogger.Infof("通过PBFT完成共识, RequestMsg --- clientID:%s 的 Requst Msg:  %d, %s, %d\n", value.ClientID, value.Timestamp, value.Operation, value.SequenceID)
	}

	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// zapConfig.SugarLogger.Debugf("目标url:%s", node.NodeTable[node.View.Primary])
	send(node.NodeTable[node.View.Primary]+"/reply", jsonMsg) //向当前view的主节点发送reply回复,需要主节点将搜集的reply消息回复给对应的client

	return nil
}

// 主节点根据获取的客户端requestMsg,开启新一轮pbft共识(即产生pre-prepare消息并广播给其他共识节点)
func (node *Node) GetReq(reqMsg *consensus.RequestMsg) error {
	zapConfig.SugarLogger.Infof("接收的Requst Msg: ClientID: %s, Timestamp: %d, Operation: %s\n", reqMsg.ClientID, reqMsg.Timestamp, reqMsg.Operation)

	// Create a new state for the new consensus.
	if node.CurrentState == nil {
		err := node.createStateForNewConsensus() //初次运行，主节点需要为当前视图创建一个State对象
		if err != nil {
			return err
		}
	}

	prePrepareMsg, err := node.CurrentState.StartConsensus(reqMsg) //根据获取的客户端request消息,产生新一轮共识需要使用的prePrepareMsg和State对象(将request消息记录到State对象中)
	if err != nil {
		return err
	}
	//zapConfig.SugarLogger.Infof("当前PBFT viewID:%d", node.CurrentState.ViewID)

	if prePrepareMsg != nil {
		node.Broadcast(prePrepareMsg, "/preprepare") //向其他节点广播新产生的pre-prepare
		zapConfig.SugarLogger.Infof("当前节点id:%s 已完成 Pre-prepare阶段", node.NodeID)
		node.MsgBuffer.ReqMsgs.DelReqMsg(reqMsg.ClientID) //主节点删除消息池中的requestMsg(因为此消息已经被State对象记录)
	}

	return nil
}

// GetPrePrepare can be called when the node's CurrentState is nil.
// Consensus start procedure for normal participants.
// 副本节点根据从主节点获取的pre-prepare消息,创建针对此pre-prepareMsg包含的requestMsg的State对象,同时产生prepare消息,最后将prepareMsg广播发送给其他共识节点
func (node *Node) GetPrePrepare(prePrepareMsg *consensus.PrePrepareMsg) error {
	zapConfig.SugarLogger.Infof("接收的Pre-prepare Msg: ClientID: %s, Operation: %s, SequenceID: %d\n", prePrepareMsg.RequestMsg.ClientID, prePrepareMsg.RequestMsg.Operation, prePrepareMsg.SequenceID)

	if node.CurrentState == nil {
		err := node.createStateForNewConsensus() //初次运行，副本节点需要为当前视图创建一个State对象
		if err != nil {
			return err
		}
	}

	prePareMsg, err := node.CurrentState.PrePrepare(prePrepareMsg) //处理本次传入的pre-prepare消息,产生prepare阶段消息,同时将pre-prepare消息中的request消息记录到State对象中
	if err != nil {
		return err
	}

	if prePareMsg != nil {
		// Attach node ID to the message
		prePareMsg.NodeID = node.NodeID //追加自己的NodeID添加到prepareMsg消息中

		zapConfig.SugarLogger.Infof("当前节点id:%s 已完成 Pre-prepare阶段", node.NodeID)
		node.Broadcast(prePareMsg, "/prepare")                       //将prepareMsg广播发送给其他共识节点
		node.MsgBuffer.PrePrepareMsgs.DelPPMsg(prePrepareMsg.Digest) //副本节点删除消息池中的pre-prepare消息(因为此消息已经被State对象记录)
	}

	return nil
}

// 节点收集其他共识节点发送的prepareMsg,一旦收到的prepareMsg 数目大于等于 2*f 就可以组建commit消息,并广播给其他共识节点
func (node *Node) GetPrepare(prepareMsg *consensus.VoteMsg) error {
	zapConfig.SugarLogger.Infof("接收的Prepare Msg: NodeID: %s\n", prepareMsg.NodeID)

	commitMsg, err := node.CurrentState.Prepare(prepareMsg) //处理本次传入的prepare消息
	if err != nil {
		return err
	}

	if commitMsg != nil { //如果prepare共识阶段目标已达成,将组建的commit消息广播发送给其他共识节点
		// Attach node ID to the message
		commitMsg.NodeID = node.NodeID

		zapConfig.SugarLogger.Infof("当前节点id:%s 已完成 Prepare阶段", node.NodeID)
		node.Broadcast(commitMsg, "/commit")
		node.MsgBuffer.PrepareMsgs.DelAllPreMsg() //当前节点删除消息池中的所有prepare消息(因为消息已经被State对象记录)
		return utils.MSGENOUGH
	}

	return nil
}

// 节点收集其他共识节点发送的commitMsg,一旦收到的commitMsg 数目大于等于 2*f 就可以组建reply消息,发送给主节点
func (node *Node) GetCommit(commitMsg *consensus.VoteMsg) error {
	zapConfig.SugarLogger.Infof("接收的Commit Msg: NodeID: %s\n", commitMsg.NodeID)

	replyMsg, committedMsg, err := node.CurrentState.Commit(commitMsg) //处理本次传入的commit消息
	if err != nil {
		return err
	}

	if replyMsg != nil { //如果commit共识阶段目标已达成,将组建的reply消息广播发送主节点
		if committedMsg == nil { //committedMsg就是reply消息针对的request消息
			return errors.New("committed message is nil, even though the reply message is not nil")
		}

		// Attach node ID to the message
		replyMsg.NodeID = node.NodeID

		// Save the last version of committed messages to node.
		node.CommittedMsgs = append(node.CommittedMsgs, committedMsg) //将完成共识的request消息添加到node.CommittedMsgs消息池中

		// 情况State对象本次共识所缓存的log消息(node.CommittedMsgs已经记录了完成共识的request消息)
		node.CurrentState.MsgLogs.ReqMsg = nil
		node.CurrentState.MsgLogs.PrepareMsgs = make(map[string]*consensus.VoteMsg)
		node.CurrentState.MsgLogs.CommitMsgs = make(map[string]*consensus.VoteMsg)

		zapConfig.SugarLogger.Infof("当前节点id:%s 已完成 Commit阶段", node.NodeID)

		node.Reply(replyMsg) //发送reply消息给主节点

		if node.NodeID != node.View.Primary { //副本节点已经完成共识

			node.CurrentState.CurrentStage = consensus.Idle //重新等待下一次共识开始
		}

		node.MsgBuffer.CommitMsgs.DelAllCommitMsg() //当前节点删除消息池中的所有commit消息(因为消息已经被State对象记录)
		return utils.MSGENOUGH
	}

	return nil
}

func (node *Node) GetReply(msg *consensus.ReplyMsg) {

	zapConfig.SugarLogger.Infof("接收的Result Msg: %s by %s\n", msg.Result, msg.NodeID)

	//TODO:发送给client
}

// 为需要进行共识的客户端的request提供 State对象(包含当前viewID/SequenceID)
func (node *Node) createStateForNewConsensus() error {
	// Check if there is an ongoing consensus process.
	if node.CurrentState != nil {
		return errors.New("another consensus is ongoing")
	}

	// Get the last sequence ID
	//
	var lastSequenceID int64
	if len(node.CommittedMsgs) == 0 {
		lastSequenceID = -1
	} else {
		lastSequenceID = node.CommittedMsgs[len(node.CommittedMsgs)-1].SequenceID
	}

	// Create a new state for this new consensus process in the Primary
	node.CurrentState = consensus.CreateState(node.View.ID, lastSequenceID)

	return nil
}

// 消息接收与调度
func (node *Node) dispatchMsg() {
	for {
		select {
		case msg := <-node.MsgEntrance: //将获取的消息存到对应的消息池中
			err := node.routeMsg(msg)
			if err != nil {
				zapConfig.SugarLogger.Errorln(err)
				// TODO: send err to ErrorChannel
			}
		case <-node.Alarm: //定时扫描消息池中的消息
			err := node.routeMsgWhenAlarmed()
			if err != nil {
				zapConfig.SugarLogger.Errorln(err)
				// TODO: send err to ErrorChannel
			}
		}
	}
}

// 将不同类型的消息添加到各自的消息池中
func (node *Node) routeMsg(msg interface{}) []error {
	switch msg.(type) {
	case *consensus.RequestMsg: //收到客户端的request消息

		reqMsg, ok := msg.(*consensus.RequestMsg)

		if ok {
			node.MsgBuffer.ReqMsgs.AddReqMsg(*reqMsg)
		}
		//zapConfig.SugarLogger.Debugf("requestMsgPool --- clientID:%s,req:%v", reqMsg.ClientID, node.MsgBuffer.ReqMsgs.GetReqMsgByClientID(reqMsg.ClientID))
	case *consensus.PrePrepareMsg: //收到主节点的pre-prepare消息

		ppMsg, ok := msg.(*consensus.PrePrepareMsg)
		if ok {
			node.MsgBuffer.PrePrepareMsgs.AddPPMsg(*ppMsg)
		}
		//zapConfig.SugarLogger.Debugf("PrePrepareMsgPool --- digest:%s,req:%v", ppMsg.Digest, node.MsgBuffer.PrePrepareMsgs.GetPPMsgByDigest(ppMsg.Digest))
	case *consensus.VoteMsg: //收到其他节点的PrepareMsg/CommitMsg消息

		if msg.(*consensus.VoteMsg).MsgType == consensus.PrepareMsg { //收到PrepareMsg消息

			preMsg, ok := msg.(*consensus.VoteMsg)
			if ok {
				node.MsgBuffer.PrepareMsgs.AddPreMsg(*preMsg)
			}

		} else if msg.(*consensus.VoteMsg).MsgType == consensus.CommitMsg { //收到CommitMsg消息
			cmMsg, ok := msg.(*consensus.VoteMsg)
			if ok {
				node.MsgBuffer.CommitMsgs.AddCommitMsg(*cmMsg)
			}
		}

	case *consensus.ReplyMsg:

		ryMsg, ok := msg.(*consensus.ReplyMsg)
		if ok {
			node.MsgBuffer.ReplyMsgs.AddRyMsg(*ryMsg)
		}
		//zapConfig.SugarLogger.Debugf("ReplyMsgPool --- clientID:%s,req:%v", ryMsg.ClientID, node.MsgBuffer.ReplyMsgs.GetRyMsgByClientID(ryMsg.NodeID))
	}

	return nil
}

// 定时扫描所有的消息池,根据当前共识状态决定是否处理
func (node *Node) routeMsgWhenAlarmed() []error {

	if node.CurrentState == nil || node.CurrentState.CurrentStage == consensus.Idle { //还未开始共识

		// 主节点负责读取request消息池
		if node.MsgBuffer.ReqMsgs.MsgNum() != 0 {
			msgs := make([]consensus.RequestMsg, node.MsgBuffer.ReqMsgs.MsgNum())
			copy(msgs, node.MsgBuffer.ReqMsgs.GetAllReqMsg())

			for i := 0; i < node.MsgBuffer.ReqMsgs.MsgNum(); i++ {
				node.MsgBuffer.ReqMsgs.DelReqMsg(msgs[i].ClientID)
			}

			node.MsgDelivery <- msgs
		}
		// 副本节点负责读取pre-prepare消息池
		if node.MsgBuffer.PrePrepareMsgs.MsgNum() != 0 {

			msgs := make([]consensus.PrePrepareMsg, node.MsgBuffer.PrePrepareMsgs.MsgNum())
			copy(msgs, node.MsgBuffer.PrePrepareMsgs.GetAllPPMsg())

			for i := 0; i < node.MsgBuffer.PrePrepareMsgs.MsgNum(); i++ {
				node.MsgBuffer.PrePrepareMsgs.DelPPMsg(msgs[i].Digest)
				//zapConfig.SugarLogger.Debugf("消息扫描  PrePrepareMsg ---  Digest: %s, SequenceID: %d\n", msgs[i].Digest, msgs[i].SequenceID)
			}
			node.MsgDelivery <- msgs
		}

	} else { //正处于pbft共识阶段中
		switch node.CurrentState.CurrentStage {
		case consensus.PrePrepared: //刚刚完成pbft的pre-prepare阶段,取出MsgBuffer.PrepareMsgs缓存池中的全部消息，输入到node.MsgDelivery管道

			if node.MsgBuffer.PrepareMsgs.MsgNum() >= 2*f { //必须保证当前节点收集到了至少 2*f 个节点的PrepareMsg

				//zapConfig.SugarLogger.Debugf("消息扫描 len of PrepareMsg:%d", node.MsgBuffer.PrepareMsgs.MsgNum())
				msgs := make([]consensus.VoteMsg, node.MsgBuffer.PrepareMsgs.MsgNum())
				copy(msgs, node.MsgBuffer.PrepareMsgs.GetAllPreMsg())

				for i := 0; i < node.MsgBuffer.PrepareMsgs.MsgNum(); i++ {
					node.MsgBuffer.PrepareMsgs.DelPreMsg(msgs[i].NodeID)
					//zapConfig.SugarLogger.Debugf("消息扫描  PrepareMsg ---  Digest: %s, SequenceID: %d NodeID: %s \n", msgs[i].Digest, msgs[i].SequenceID, msgs[i].NodeID)
				}

				node.MsgDelivery <- msgs
			}

		case consensus.Prepared: //刚刚完成pbft的prepare阶段,取出MsgBuffer.CommitMsgs缓存池中的全部消息，输入到node.MsgDelivery管道

			if node.MsgBuffer.CommitMsgs.MsgNum() >= 2*f { //必须保证当前节点收集到了至少 2*f 个节点的CommitMsg
				msgs := make([]consensus.VoteMsg, node.MsgBuffer.CommitMsgs.MsgNum())
				copy(msgs, node.MsgBuffer.CommitMsgs.GetAllCmMsg())

				for i := 0; i < node.MsgBuffer.CommitMsgs.MsgNum(); i++ {
					node.MsgBuffer.CommitMsgs.DelCommitMsg(msgs[i].NodeID)
					//zapConfig.SugarLogger.Debugf("消息扫描  CommitMsg ---  Digest: %s, SequenceID: %d NodeID: %s \n", msgs[i].Digest, msgs[i].SequenceID, msgs[i].NodeID)

				}

				node.MsgDelivery <- msgs
			}
		case consensus.Committed: //完成了pbft的commit阶段,取出MsgBuffer.ReplyMsgs缓存池中的全部消息，输入到node.MsgDelivery管道
			//zapConfig.SugarLogger.Debugf("当前共识状态为:consensus.Committed,消息数为%d", node.MsgBuffer.ReplyMsgs.MsgNum())

			if node.MsgBuffer.ReplyMsgs.MsgNum() >= f+1 { //必须保证当前主节点收集到了至少 f+1 个节点的ReplyMsg(可以是主节点自己的reply)
				msgs := make([]consensus.ReplyMsg, node.MsgBuffer.ReplyMsgs.MsgNum())
				copy(msgs, node.MsgBuffer.ReplyMsgs.GetAllRyMsg())

				for i := 0; i < node.MsgBuffer.ReplyMsgs.MsgNum(); i++ {
					node.MsgBuffer.ReplyMsgs.DelRyMsg(msgs[i].NodeID)
					//zapConfig.SugarLogger.Debugf("消息扫描  ReplyMsg --- Reply Msg: %s by %s\n", msgs[i].Result, msgs[i].NodeID)

				}

				node.MsgDelivery <- msgs
			}
		}
	}

	return nil
}

// (被定时器触发)循环从node.MsgDelivery管道中取出消息,进行分类处理
func (node *Node) resolveMsg() {
	for {
		// Get buffered messages from the dispatcher.
		msgs := <-node.MsgDelivery

		switch msgs.(type) {
		case []consensus.RequestMsg: //client发送的request消息

			// for _, v := range msgs.([]*consensus.RequestMsg) {
			// 	zapConfig.SugarLogger.Debugf("准备处理RequestMsg ---  ClientID: %s, Operation: %s, SequenceID: %d\n", v.ClientID, v.Operation, v.SequenceID)
			// }
			var tempMsgs []consensus.RequestMsg = msgs.([]consensus.RequestMsg)

			errs := node.resolveRequestMsg(tempMsgs) //根据此request消息产生pre-prepare消息并广播给其他共识节点
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}
		case []consensus.PrePrepareMsg: //pbft一阶段产生的PrePrepareMsg消息
			var tempMsgs []consensus.PrePrepareMsg = msgs.([]consensus.PrePrepareMsg)

			errs := node.resolvePrePrepareMsg(tempMsgs)
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}
		case []consensus.VoteMsg: //pbft二三阶段产生的PrepareMsg消息和CommitMsg消息

			var tempMsgs []consensus.VoteMsg = msgs.([]consensus.VoteMsg)
			if len(tempMsgs) == 0 {
				break
			}

			//判断voteMsg到底是PrepareMsg消息还是CommitMsg消息,分类处理
			if tempMsgs[0].MsgType == consensus.PrepareMsg {
				errs := node.resolvePrepareMsg(tempMsgs)
				if len(errs) != 0 {
					for _, err := range errs {
						fmt.Println(err)
					}
					// TODO: send err to ErrorChannel
				}
			} else if tempMsgs[0].MsgType == consensus.CommitMsg {
				errs := node.resolveCommitMsg(tempMsgs)
				if len(errs) != 0 {
					for _, err := range errs {
						fmt.Println(err)
					}
					// TODO: send err to ErrorChannel
				}
			}
		case []consensus.ReplyMsg: //pbft完成共识的reply消息
			errs := node.resolveReplyMsg(msgs.([]consensus.ReplyMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		}
	}
}

// 每经过ResolvingTimeDuration时长,产生node.Alarm信号,被动开启新一轮共识
func (node *Node) alarmToDispatcher() {
	for {
		time.Sleep(ResolvingTimeDuration)
		node.Alarm <- true
	}
}

// 处理传入的所有RequestMsg,为每一条RequestMsg开启新一轮pbft共识(产生pre-prepare消息并广播给其他共识节点)
func (node *Node) resolveRequestMsg(msgs []consensus.RequestMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, reqMsg := range msgs {
		err := node.GetReq(&reqMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// 处理传入的所有Pre-PrepareMsg,根据Pre-Prepare Msg组建Prepare Msg并广播给其他共识节点
func (node *Node) resolvePrePrepareMsg(msgs []consensus.PrePrepareMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, prePrepareMsg := range msgs {
		err := node.GetPrePrepare(&prePrepareMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// 处理传入的所有PrepareMsg,根据Prepare Msg组建Commit Msg并广播给其他共识节点
func (node *Node) resolvePrepareMsg(msgs []consensus.VoteMsg) []error {
	errs := make([]error, 0)
	// Resolve messages
	for _, prepareMsg := range msgs {
		err := node.GetPrepare(&prepareMsg)
		if err == utils.MSGENOUGH { //需要获取的共识消息数已达到要求
			break
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// 处理传入的所有Commit Msg,根据Commit Msg组建Reply Msg并广播给其他共识节点
func (node *Node) resolveCommitMsg(msgs []consensus.VoteMsg) []error {
	errs := make([]error, 0)
	// Resolve messages
	for _, commitMsg := range msgs {
		err := node.GetCommit(&commitMsg)
		if err == utils.MSGENOUGH { //需要获取的共识消息数已达到要求
			break
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

// 处理传入的所有Reply Msg,将其全部发送给client(这里就直接打印就可以了)
func (node *Node) resolveReplyMsg(msgs []consensus.ReplyMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, replyMsg := range msgs {
		node.GetReply(&replyMsg)
	}

	if len(errs) != 0 {
		return errs
	}
	node.MsgBuffer.ReplyMsgs.DelAllRyMsg()          //将完成pbft所需的f+1条reply消息回复给客户端,可以清空消息池
	node.CurrentState.CurrentStage = consensus.Idle //主节点结束本次pbft,重新回到初始状态
	return nil
}
