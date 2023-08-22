package main

import (
	"Github/simplePBFT/pbft/consensus"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func main() {

	timer := time.Now()

	request := consensus.RequestMsg{
		Timestamp: timer.Unix(),
		ClientID:  "client3",
		Operation: "printf",
	}

	encodeByte, err := json.Marshal(request)
	if err != nil {
		fmt.Println("RequestMsg json encode is failed")
	}

	send("localhost:1111/req", encodeByte)

}

func send(url string, msg []byte) {
	buff := bytes.NewBuffer(msg)
	http.Post("http://"+url, "application/json", buff)
}
