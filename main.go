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
	"github.com/r3labs/sse"
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

func AllJobs() ([]Record, error) {
	rows, err := db.Query("select p.name as pipeline_name, j.name as job_name, b.id as build_id, b.name as build_name, b.status, b.start_time, b.end_time, p.paused as pipeline_paused, j.paused as job_paused, p.ordering FROM pipelines p JOIN jobs j ON(j.pipeline_id=p.id AND j.active) JOIN builds b ON(b.job_id=j.id AND b.id in (select max(id) as id from builds group by job_id)) order by p.ordering")
	if err != nil {
		return nil, err
	}

	var records []Record
	for rows.Next() {
		var r = Record{}
		err = rows.Scan(&r.Pipeline, &r.Job, &r.BuildID, &r.BuildNum, &r.Status, &r.StartTime, &r.EndTime, &r.PipelinePaused, &r.JobPaused, &r.Ordering)
		if err != nil {
			fmt.Print(err)
			return nil, err
		} else {
			records = append(records, r)
		}
	}

	return records, nil
}

func main() {
	server := sse.New()

	server.CreateStream("jobs")
	http.HandleFunc("/events", server.HTTPHandler)

	go func() {
		for {
			if records, err := AllJobs(); err == nil {
				if txt, err := json.Marshal(records); err == nil {
					txt = []byte(string(txt) + "\n\n")
					server.Publish("jobs", &sse.Event{Data: txt})
				}
			}
			time.Sleep(time.Second)
		}
	}()

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
