package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"
	"github.com/gorilla/websocket"
)
const(
	Role = "C"
)

type allocation struct{
	Uids map[string]int `json:"allocation"`
}

type aggregator struct{
	NumEdges int `json:"numEdges"`
	UIDandPercent map[string]int `json:"file_and_weight"`
}

func main(){
	var edge_connection sync.WaitGroup
	var worker_done sync.WaitGroup
	// var dir string
	// fmt.Print("Provide dir path: ")
	// fmt.Scan(&dir)
	dir := "./test"

	n, files, err :=  get_data(dir)
	if err != nil {
		log.Fatal("Error while reading dir", err)
	}

	var numWorkers int
	MaxWorkers := runtime.NumCPU()
	fmt.Printf("Enter number of workers(recommended less than %d for your device)\n Workers:",MaxWorkers)
	fmt.Scan(&numWorkers)

	room_id := create_uid()
	fmt.Println("Connection_id:",room_id)

	postbody := map[string]interface{}{
		"role": Role,
		"room_id": room_id,
		"num_edges" : numWorkers,
	}

	if err != nil{
		log.Fatal("Error while creating postbody")
	}

	edge_id := make([]string,0)

	var allocate allocation
	
	algo_service , _, err := websocket.DefaultDialer.Dial("ws://localhost:5000/join", nil)
	if err != nil{
		log.Fatal("Error while creating connection to algorithm service", err)
	}

	err = algo_service.WriteJSON(postbody)
	if err != nil{
		log.Fatal("Error while writing to connection", err)
	}

	err = algo_service.ReadJSON(&allocate)
	if err != nil{
		log.Println("Error while reading from connection ", err)
	}
	fmt.Println("Response from algo service ",allocate)	
	
	for id := range allocate.Uids{
		edge_id = append(edge_id, id)
	}

	for _, id := range edge_id{
		fmt.Printf("ID:%s Weights:%d\n",id,allocate.Uids[id])
	}

	resultchan := make(chan Result)
	wp := Workerpool{resultchan: resultchan}	
	
	wp.start_pool(numWorkers, edge_id, allocate.Uids, &edge_connection, &worker_done)

	go func(){
		i := 1
		for result := range resultchan{
			fmt.Printf("worker: %d status: %v uploaded: %d/%d\n", result.worker_id, result.result, i, n )
			i++
		}
	}()

	edge_connection.Wait()
	for _ , file_path := range files{
		
		worker := wp.pickWorker()
		worker.req_chan <- Request{f: file_path}
	}

	for i := 0; i < numWorkers; i++ {
		close(wp.workers[i].req_chan)
	}

	worker_done.Wait()
	var agg_file = aggregator{
		NumEdges: numWorkers,
		UIDandPercent: allocate.Uids,
	}

	agg_req_body, err := json.Marshal(agg_file)
	if err != nil{
		log.Print("Error while converting into json ",err)
	}

	resp, err := http.Post("http://localhost:8000","application/json",bytes.NewBuffer(agg_req_body))
	if err != nil{
		log.Print("Error while posting to aggregator",err)
	}
	defer resp.Body.Close()

	fmt.Printf("Response Body:%s Response Status:%s",resp.Body, resp.Status)
	select{}


}