package main

import (
	"Github/simplePBFT/pbft/network"
	"Github/simplePBFT/zapConfig"
	"flag"
)

func main() {

	nodeID := flag.String("id", "Apple", "节点的NodeID")
	logFile := flag.Int("log", 1, "log文件目录")

	flag.Parse()

	zapLogger := zapConfig.NewLogger(*logFile)
	zapConfig.SugarLogger = zapLogger.GetSugarLogger()
	defer zapLogger.Close() //程序退出之前需要调用此方法更新日志条目,不然会造成丢失日志

	server := network.NewServer(*nodeID)

	server.Start()

}
