package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type Record struct {
	Pipeline       string
	Job            string
	Status         string
	BuildID        int
	BuildNum       int
	StartTime      *time.Time
	EndTime        *time.Time
	PipelinePaused bool
	JobPaused      bool
	Ordering       int
}

var db *sql.DB

func init() {
	var err error
	db, err = sql.Open("postgres", fmt.Sprintf("postgres://%s:%s@35.185.13.69/atc?sslmode=disable", os.Getenv("DB_USERNAME"), os.Getenv("DB_PASSWORD")))
	if err != nil {
		log.Fatal(err)
	}
}

func AllJobs() []Record {
	rows, err := db.Query("select p.name as pipeline_name, j.name as job_name, b.id as build_id, b.name as build_name, b.status, b.start_time, b.end_time, p.paused as pipeline_paused, j.paused as job_paused, p.ordering FROM pipelines p JOIN jobs j ON(j.pipeline_id=p.id AND j.active) JOIN builds b ON(b.job_id=j.id AND b.id in (select max(id) as id from builds group by job_id)) order by p.ordering")
	if err != nil {
		log.Panic(err)
	}

	var records []Record
	for rows.Next() {
		var r = Record{}
		err = rows.Scan(&r.Pipeline, &r.Job, &r.BuildID, &r.BuildNum, &r.Status, &r.StartTime, &r.EndTime, &r.PipelinePaused, &r.JobPaused, &r.Ordering)
		if err != nil {
			fmt.Print(err)
		} else {
			records = append(records, r)
		}
	}

	return records
}

func main() {
	http.HandleFunc("/events/jobs", func(w http.ResponseWriter, r *http.Request) {
		// We need to be able to flush for SSE
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Flushing not supported", http.StatusNotImplemented)
			return
		}

		// Returns a channel that blocks until the connection is closed
		cn, ok := w.(http.CloseNotifier)
		if !ok {
			http.Error(w, "Closing not supported", http.StatusNotImplemented)
			return
		}
		close := cn.CloseNotify()

		// Set headers for SSE
		h := w.Header()
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		h.Set("Content-Type", "text/event-stream")

		// Connect new client
		// cl := make(client, s.bufSize)
		// s.connecting <- cl

		sendData := func() {
			records := AllJobs()
			txt, err := json.Marshal(records)
			if err != nil {
				log.Panic(err)
			}
			w.Write([]byte("data: " + string(txt) + "\n\n"))
			fl.Flush()
			// fmt.Println("Data sent")
		}
		sendData()

		timer := time.NewTicker(time.Second * 3)
		for {
			select {
			case <-close:
				// Disconnect the client when the connection is closed
				// s.disconnecting <- cl
				return

			case <-timer.C:
				sendData()
			}
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if txt, err := ioutil.ReadFile("index.html"); err != nil {
			fmt.Fprintf(w, "500: %+v", err)
		} else {
			fmt.Fprintf(w, string(txt))
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}

	fmt.Println("About to listen on port ", port)
	err := http.ListenAndServe(":"+port, nil) // set listen port
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
