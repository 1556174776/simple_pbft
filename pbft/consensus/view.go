package consensus

const InitialviewID = 100 //初始viewID

type View struct {
	ID        int64          //视图ID
	Primary   string         //主节点地址(url)
	NodeTable map[int]string //记录所有pbft节点(key:node编号   value:url)
}

// 初始化一个视图:
// 1.视图ID是预设的初始值
func NewView(NodeTable map[int]string) *View {

	peerNum := len(NodeTable) //共识节点数目
	index := InitialviewID % peerNum

	return &View{
		ID:        InitialviewID,
		NodeTable: NodeTable,
		Primary:   NodeTable[index],
	}

}

func (v *View) ViewChange() {
	peerNum := len(v.NodeTable) //共识节点数目
	v.ID += 1                   //更新视图ID
	index := int(v.ID) % peerNum
	v.Primary = v.NodeTable[index] //更换主节点
}
