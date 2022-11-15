package network

import (
	"encoding/json"
	"fmt"
	"net/http"

	"Github/simplePBFT/pbft/consensus"
	"Github/simplePBFT/zapConfig"
)

// 共识节点类
type ConsensusNode struct {
	url  string
	node *Node
}

// 创建一个共识节点(包含Node对象和Server对象)
func NewServer(nodeID string) *ConsensusNode {
	node := NewNode(nodeID)                                       //创建Node对象
	consensusNode := &ConsensusNode{node.NodeTable[nodeID], node} //创建Server对象

	consensusNode.setRoute() //注册路由服务(不同类型消息调用不同的处理方法)

	return consensusNode
}

// 启动共识节点的Server对象,监听server.url,获取其他共识节点的消息并提供服务
func (cn *ConsensusNode) Start() {
	zapConfig.SugarLogger.Debugf("ConsensusNode will be started at %s...\n", cn.url)
	if err := http.ListenAndServe(cn.url, nil); err != nil {
		fmt.Println(err)
		return
	}
}

// 为不同类型共识消息设置路由方式
func (cn *ConsensusNode) setRoute() {
	http.HandleFunc("/req", cn.getReq)
	http.HandleFunc("/preprepare", cn.getPrePrepare)
	http.HandleFunc("/prepare", cn.getPrepare)
	http.HandleFunc("/commit", cn.getCommit)
	http.HandleFunc("/reply", cn.getReply)
}

// 主节点接收client的request消息,进行json解码,解码后的消息输入到node.MsgEntrance管道
func (cn *ConsensusNode) getReq(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.RequestMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		zapConfig.SugarLogger.Errorf("Request Msg json解码失败,err:%v", err)
		return
	}
	//zapConfig.SugarLogger.Debugf("当前节点%s获取的Request Msg:  ClientID: %s, Timestamp: %d, Operation: %s\n", cn.node.NodeID, msg.ClientID, msg.Timestamp, msg.Operation)
	cn.node.MsgEntrance <- &msg
}

// 副本节点接收主节点的Pre-Prepare消息,进行json解码,解码后的消息输入到node.MsgEntrance管道
func (cn *ConsensusNode) getPrePrepare(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.PrePrepareMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		zapConfig.SugarLogger.Errorf("Pre-Prepare Msg json解码失败,err:%v", err)
		return
	}
	//zapConfig.SugarLogger.Debugf("当前节点%s获取的Pre-prepare Msg:  ClientID: %s, Operation: %s, SequenceID: %d\n", cn.node.NodeID, msg.RequestMsg.ClientID, msg.RequestMsg.Operation, msg.SequenceID)

	cn.node.MsgEntrance <- &msg
}

// 接收其他节点的Prepare消息,进行json解码,解码后的消息输入到node.MsgEntrance管道
func (cn *ConsensusNode) getPrepare(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.VoteMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		zapConfig.SugarLogger.Errorf("Prepare Msg json解码失败,err:%v", err)
		return
	}
	//zapConfig.SugarLogger.Debugf("当前节点%s获取的Prepare Msg:  NodeID: %s\n", cn.node.NodeID, msg.NodeID)

	cn.node.MsgEntrance <- &msg
}

// 接收其他节点的commit消息,进行json解码,解码后的消息输入到node.MsgEntrance管道
func (cn *ConsensusNode) getCommit(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.VoteMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		zapConfig.SugarLogger.Errorf("Commit Msg json解码失败,err:%v", err)
		return
	}
	//zapConfig.SugarLogger.Debugf("当前节点%s获取的Commit Msg:  NodeID: %s\n", cn.node.NodeID, msg.NodeID)
	cn.node.MsgEntrance <- &msg
}

// 主节点接收其他共识节点的reply消息,进行json解码,解码后的消息直接调用node.GetReply()进行处理(这里就是打印)
func (cn *ConsensusNode) getReply(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.ReplyMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		zapConfig.SugarLogger.Errorf("Reply Msg json解码失败,err:%v", err)
		return
	}
	//zapConfig.SugarLogger.Debugf("当前节点%s获取的Reply Msg: %s by %s\n", cn.node.NodeID, msg.Result, msg.NodeID)
	cn.node.MsgEntrance <- &msg
	//cn.node.GetReply(&msg)
}
