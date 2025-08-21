package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/Omkardalvi01/IPD/networking"
)

type result_state int
const(
	SUCCESS result_state = 0
	FAILURE result_state = -1 
	END string = "EOF"
)

type Result struct{
	worker_id int
	result result_state
}

type Request struct{
	f *os.File
}

type Worker struct{
	req_chan <-chan Request
	res_chan chan<- Result
	worker_id int
	conn_id string
}

func (w Worker) start(wg *sync.WaitGroup){
	// uid := create_uid()
	// fmt.Printf("uid for worker %d : %s \n",w.worker_id, uid)
	var stop_worker chan struct{}
	peer ,dc , err := networking.Peerconnection(w.conn_id)
	if err != nil{
		log.Printf("Error with peer connection in worker %d", w.worker_id)
		return 
	}
	defer dc.Close()
	defer peer.Close()

	wg.Done()

	dc.OnOpen(func() {
		fmt.Println("Data channel Open")
		for r := range w.req_chan {

			file_name := strings.Split(r.f.Name(), "/")[1]
			dc.SendText(file_name)

			img , err := get_img_data(r.f.Name()) 
			if err != nil{
				log.Fatal("Error while get image data", err)
			}

			err = dc.Send(img)
			if err != nil{
				log.Fatal("Error while sending image data", err)
			}
			
			dc.SendText(END)

			w.res_chan <- Result{worker_id: w.worker_id, result: SUCCESS}
			r.f.Close()
			
		}
		stop_worker <- struct{}{}
		
	})
	<-stop_worker
}

type Workerpool struct{
	num_workers int
	resultchan chan<- Result
}

func (wp *Workerpool) start_pool(n int, id string, wg *sync.WaitGroup) []chan Request{
	wp.num_workers = n
	data_chan := make([]chan Request,0)
	for i := 0 ; i < n ; i++ {
		rq := make(chan Request)
		w := Worker{worker_id: i, req_chan: rq, res_chan: wp.resultchan, conn_id: id}
		wg.Add(1)
		data_chan = append(data_chan, rq)
		go w.start(wg)
	}
	return data_chan
}


