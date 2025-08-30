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
	req_chan chan Request
	res_chan chan<- Result
	conn_id string
	worker_id int
	current int
	weight int
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
	resultchan chan<- Result
	workers []*Worker
	num_workers int
}

func (wp *Workerpool) start_pool(n int, id []string, weights map[string]int, wg *sync.WaitGroup) {
	wp.num_workers = n
	for i := 0 ; i < n ; i++ {
		w := Worker{worker_id: i, req_chan: make(chan Request), res_chan: wp.resultchan, conn_id: id[i], weight: weights[id[i]], current: 0}
		wp.workers = append(wp.workers, &w)
		wg.Add(1)
		go w.start(wg)
	}
}

func (wp *Workerpool) pickWorker() *Worker{
	var best *Worker
	total := 0

	for _, w := range wp.workers{
		w.current += w.weight
		total += w.weight
		if best == nil || w.current > best.current{
			best = w
		}
	}

	best.current -= total
	return best

}
